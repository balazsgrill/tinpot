package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/balazsgrill/tinpot"
	"github.com/google/uuid"
)

//go:embed static
var staticContent embed.FS

// Configuration
var (
	MQTTBroker = getEnv("MQTT_BROKER", "tcp://localhost:1883")
	RootPath   = getEnv("ROOT_PATH", "")
)

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Execution Registry
type ExecutionState struct {
	EventChan chan StreamEvent
	mu        sync.Mutex
	Done      bool
}

var (
	executions = make(map[string]*ExecutionState)
	execMu     sync.RWMutex
)

func registerExecution(id string) *ExecutionState {
	execMu.Lock()
	defer execMu.Unlock()
	state := &ExecutionState{
		EventChan: make(chan StreamEvent, 1000), // Buffered to assume non-blocking for reasonable volume
	}
	executions[id] = state
	return state
}

func getExecution(id string) *ExecutionState {
	execMu.RLock()
	defer execMu.RUnlock()
	return executions[id]
}

func removeExecution(id string) {
	execMu.Lock()
	defer execMu.Unlock()
	delete(executions, id)
}

func main() {
	mgr := NewMqttActionManager(MQTTBroker)

	// Setup Router
	mux := http.NewServeMux()

	// API Routes
	mux.HandleFunc("GET /api/actions", func(w http.ResponseWriter, r *http.Request) {
		listActions(w, r, mgr)
	})
	mux.HandleFunc("POST /api/actions/{name}/execute", func(w http.ResponseWriter, r *http.Request) {
		executeAction(w, r, mgr, false)
	})
	mux.HandleFunc("POST /api/actions/{name}/sync_execute", func(w http.ResponseWriter, r *http.Request) {
		executeAction(w, r, mgr, true)
	})
	mux.HandleFunc("GET /api/executions/{id}/stream", func(w http.ResponseWriter, r *http.Request) {
		streamLogs(w, r)
	})
	mux.HandleFunc("GET /api/executions/{id}/status", getStatus)
	mux.HandleFunc("POST /api/executions/{id}/cancel", cancelAction)

	// Static Files - Serve from embedded FS
	mux.Handle("/static/", http.FileServer(http.FS(staticContent)))

	// Serve Index
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		// Read from embedded FS
		indexFile, err := staticContent.ReadFile("static/index.html")
		if err != nil {
			http.Error(w, "Failed to load index.html", http.StatusInternalServerError)
			return
		}
		html := string(indexFile)
		// Inject Base Path
		script := fmt.Sprintf(`<script>window.BASE_PATH = "%s";</script>`, RootPath)
		html = strings.Replace(html, "<!-- BASE_PATH_INJECTION -->", script, 1)
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	})

	// Serve Execution View with Injection
	mux.HandleFunc("GET /static/execution.html", func(w http.ResponseWriter, r *http.Request) {
		// Read from embedded FS
		fileData, err := staticContent.ReadFile("static/execution.html")
		if err != nil {
			http.Error(w, "Failed to load execution.html", http.StatusInternalServerError)
			return
		}
		html := string(fileData)
		// Inject Base Path
		script := fmt.Sprintf(`<script>window.BASE_PATH = "%s";</script>`, RootPath)
		html = strings.Replace(html, "<!-- BASE_PATH_INJECTION -->", script, 1)
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(html))
	})

	// Health/Ready
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		if mgr.IsConnected() {
			writeJSON(w, 200, map[string]string{"status": "healthy"})
		} else {
			writeJSON(w, 503, map[string]string{"status": "unhealthy", "detail": "MQTT not connected"})
		}
	})

	handler := corsMiddleware(mux)

	port := getEnv("PORT", "8000")
	log.Println("Starting Coordinator on :" + port)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
		log.Fatal(err)
	}
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func listActions(w http.ResponseWriter, r *http.Request, mgr tinpot.ActionManager) {
	writeJSON(w, 200, mgr.ListActions())
}

func executeAction(w http.ResponseWriter, r *http.Request, mgr tinpot.ActionManager, syncMode bool) {
	actionName := r.PathValue("name")

	trigger := mgr.GetAction(actionName)
	if trigger == nil {
		writeJSON(w, 404, map[string]string{"detail": fmt.Sprintf("Action not found: %s", actionName)})
		return
	}

	var req ExecuteActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"detail": "Invalid request body"})
		return
	}

	// Request Parameters
	params := req.Parameters
	if params == nil {
		params = make(map[string]interface{})
	}

	// Generate Execution ID and inject it
	execID := uuid.New().String()
	params["_execution_id"] = execID

	if syncMode {
		var finalResult map[string]interface{}
		var finalError string
		var wg sync.WaitGroup
		wg.Add(1)

		trigger(params, func(err string, res map[string]interface{}) {
			finalError = err
			finalResult = res
			wg.Done()
		}, nil) // No logs callback for sync

		wg.Wait()

		status := "SUCCESS"
		if finalError != "" {
			status = "FAILURE"
		}

		writeJSON(w, 200, SyncExecutionResponse{
			ExecutionID: execID,
			ActionName:  actionName,
			Status:      status,
			Result:      finalResult,
		})
		return
	}

	// Async
	state := registerExecution(execID)

	// Log Callback
	logCallback := func(level string, message string) {
		event := StreamEvent{
			Type: "log",
			Data: tinpot.MqttLogEntry{
				Timestamp: time.Now().Format(time.RFC3339),
				Level:     level,
				Message:   message,
			},
		}
		// Non-blocking send to not stall execution
		select {
		case state.EventChan <- event:
		default:
			log.Printf("Dropped log for %s due to full buffer", execID)
		}
	}

	// Response Callback
	responseCallback := func(err string, res map[string]interface{}) {
		success := err == ""
		status := "SUCCESS"
		if !success {
			status = "FAILURE"
		}

		data := map[string]interface{}{
			"state":      status,
			"successful": success,
		}
		if success {
			data["result"] = res
		} else {
			data["error"] = err
		}

		event := StreamEvent{
			Type: "complete",
			Data: data,
		}

		// Send complete and close
		select {
		case state.EventChan <- event:
		default:
		}
		close(state.EventChan)

		// Cleanup after some time?
		// We can't cleanup immediately if client is still reading buffered channel?
		// Channel is safely closed, client will drain it.
		// But map entry persists.
		// We should start a timer to remove execution from map.
		go func() {
			time.Sleep(1 * time.Minute)
			removeExecution(execID)
		}()
	}

	go trigger(params, responseCallback, logCallback)

	// Async Response
	writeJSON(w, 200, ExecutionResponse{
		ExecutionID: execID,
		ActionName:  actionName,
		Status:      "submitted",
		StreamURL:   fmt.Sprintf("/api/executions/%s/stream", execID),
	})
}

func streamLogs(w http.ResponseWriter, r *http.Request) {
	execID := r.PathValue("id")

	state := getExecution(execID)
	if state == nil {
		writeJSON(w, 404, map[string]string{"detail": "Execution not found"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// For http.ResponseWriter, we check if it supports flushing
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	// Send connected
	encoded, _ := json.Marshal(map[string]string{"type": "connected", "execution_id": execID})
	fmt.Fprintf(w, "data: %s\n\n", encoded)
	flusher.Flush()

	// Iterate over channel
	ctx := r.Context()
	for {
		select {
		case event, ok := <-state.EventChan:
			if !ok {
				// Channel closed (completed)
				return
			}
			bytes, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", bytes)
			flusher.Flush()
		case <-ctx.Done():
			return
		}
	}
}

func getStatus(w http.ResponseWriter, r *http.Request) {
	state := getExecution(r.PathValue("id"))
	status := "PENDING"
	if state == nil {
		status = "UNKNOWN"
	} else {
		// We could check if channel is closed?
		// Or add Status field to state.
	}

	writeJSON(w, 200, map[string]interface{}{
		"execution_id": r.PathValue("id"),
		"state":        status,
		"ready":        false,
	})
}

func cancelAction(w http.ResponseWriter, r *http.Request) {
	// Not supported
	writeJSON(w, 501, map[string]string{"detail": "Cancellation not supported"})
}
