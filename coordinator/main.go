package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Configuration
var (
	MQTTBroker = getEnv("MQTT_BROKER", "tcp://localhost:1883")
	StaticDir  = getEnv("STATIC_DIR", "../static") // Adjust default for dev
	RootPath   = getEnv("ROOT_PATH", "")
)

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Action definition (received via MQTT)
type Action struct {
	Description  string                 `json:"description"`
	Group        string                 `json:"group"`
	Parameters   map[string]interface{} `json:"parameters"`
	TriggerTopic string                 `json:"trigger_topic"`
}

// Global Actions Registry
var (
	actions = make(map[string]Action)
	mu      sync.RWMutex
)

// Execution Request Payload
type ExecutionRequest struct {
	ExecutionID string                 `json:"execution_id"`
	Parameters  map[string]interface{} `json:"parameters"`
	ResultTopic string                 `json:"result_topic"`
	LogTopic    string                 `json:"log_topic"`
}

// API Request/Response models
type ExecuteActionRequest struct {
	Parameters map[string]interface{} `json:"parameters"`
}

type ExecutionResponse struct {
	ExecutionID string `json:"execution_id"`
	ActionName  string `json:"action_name"`
	Status      string `json:"status"`
	StreamURL   string `json:"stream_url"`
}

type SyncExecutionResponse struct {
	ExecutionID string      `json:"execution_id"`
	ActionName  string      `json:"action_name"`
	Status      string      `json:"status"`
	Result      interface{} `json:"result"`
}

// Log Entry
type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
}

// Result Entry
type ResultResponse struct {
	Status string      `json:"status"`
	Result interface{} `json:"result"`
	Error  string      `json:"error,omitempty"`
}

func main() {
	// Setup MQTT
	opts := mqtt.NewClientOptions().AddBroker(MQTTBroker)
	opts.SetClientID("tinpot-coordinator-" + uuid.New().String())
	opts.SetAutoReconnect(true)

	// Action Discovery Handler
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		log.Println("Connected to MQTT Broker")
		// Subscribe to action announcements
		token := c.Subscribe("tinpot/actions/+", 1, func(c mqtt.Client, m mqtt.Message) {
			topic := m.Topic()
			// Topic format: tinpot/actions/<name>
			parts := strings.Split(topic, "/")
			if len(parts) != 3 {
				return
			}
			actionName := parts[2]

			// If payload is empty, remove action (retained clear)
			if len(m.Payload()) == 0 {
				mu.Lock()
				delete(actions, actionName)
				mu.Unlock()
				log.Printf("Action removed: %s", actionName)
				return
			}

			var act Action
			if err := json.Unmarshal(m.Payload(), &act); err != nil {
				log.Printf("Failed to unmarshal action %s: %v", actionName, err)
				return
			}

			mu.Lock()
			actions[actionName] = act
			mu.Unlock()
			log.Printf("Action discovered: %s", actionName)
		})
		token.Wait()
	})

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("Failed to connect to MQTT: %v", token.Error())
	}

	// Setup Router
	r := gin.Default()

	// CORS
	r.Use(cors.Default())

	// API Routes
	api := r.Group("/api")
	{
		api.GET("/actions", listActions)
		api.POST("/actions/:name/execute", func(c *gin.Context) { executeAction(c, client, false) })
		api.POST("/actions/:name/sync_execute", func(c *gin.Context) { executeAction(c, client, true) })
		api.GET("/executions/:id/stream", func(c *gin.Context) { streamLogs(c, client) })
		// Status endpoint - we will just assume RUNNING if not stored or check logs?
		// For now, let's implement a dummy status or try to fetch validation
		api.GET("/executions/:id/status", getStatus)
		api.POST("/executions/:id/cancel", cancelAction)
	}

	// Static Files
	// r.Static("/static", StaticDir) // Replaced with custom handler for injection?
	// The original main.py mounted /static to /static.
	r.Static("/static", StaticDir)

	// Serve Index
	r.GET("/", func(c *gin.Context) {
		indexFile, err := os.ReadFile(fmt.Sprintf("%s/index.html", StaticDir))
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to load index.html")
			return
		}
		html := string(indexFile)
		// Inject Base Path
		script := fmt.Sprintf(`<script>window.BASE_PATH = "%s";</script>`, RootPath)
		html = strings.Replace(html, "<!-- BASE_PATH_INJECTION -->", script, 1)
		c.Header("Content-Type", "text/html")
		c.String(http.StatusOK, html)
	})

	// Health/Ready
	r.GET("/health", func(c *gin.Context) {
		if client.IsConnected() {
			c.JSON(200, gin.H{"status": "healthy"})
		} else {
			c.JSON(503, gin.H{"status": "unhealthy", "detail": "MQTT not connected"})
		}
	})

	port := getEnv("PORT", "8000")
	log.Println("Starting Coordinator on :" + port)
	r.Run(":" + port)
}

func listActions(c *gin.Context) {
	mu.RLock()
	defer mu.RUnlock()
	// Return map directly or list? Existing API returns map
	c.JSON(200, actions)
}

func executeAction(c *gin.Context, mqttClient mqtt.Client, syncMode bool) {
	actionName := c.Param("name")

	mu.RLock()
	act, ok := actions[actionName]
	mu.RUnlock()

	if !ok {
		c.JSON(404, gin.H{"detail": fmt.Sprintf("Action not found: %s", actionName)})
		return
	}

	var req ExecuteActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"detail": "Invalid request body"})
		return
	}

	execID := uuid.New().String()
	resultTopic := fmt.Sprintf("tinpot/exec/%s/result", execID)
	logTopic := fmt.Sprintf("tinpot/exec/%s/log", execID)

	payload := ExecutionRequest{
		ExecutionID: execID,
		Parameters:  req.Parameters,
		ResultTopic: resultTopic,
		LogTopic:    logTopic,
	}

	payloadBytes, _ := json.Marshal(payload)

	// We subscribe to the result topic BEFORE publishing to avoid a race condition
	// where the result is published before we are ready to receive it.
	resultChan := make(chan ResultResponse, 1)
	subToken := mqttClient.Subscribe(resultTopic, 1, func(client mqtt.Client, msg mqtt.Message) {
		var res ResultResponse
		if err := json.Unmarshal(msg.Payload(), &res); err != nil {
			return
		}
		select {
		case resultChan <- res:
		default:
		}
	})
	subToken.Wait()
	defer mqttClient.Unsubscribe(resultTopic)

	// Publish Execution Request
	token := mqttClient.Publish(act.TriggerTopic, 1, false, payloadBytes)
	token.Wait()
	if token.Error() != nil {
		c.JSON(500, gin.H{"detail": "Failed to publish execution request"})
		return
	}

	if syncMode {
		// Wait for result
		select {
		case res := <-resultChan:
			c.JSON(200, SyncExecutionResponse{
				ExecutionID: execID,
				ActionName:  actionName,
				Status:      res.Status,
				Result:      res.Result,
			})
		case <-time.After(30 * time.Second): // Timeout
			c.JSON(504, gin.H{"detail": "Execution timed out"})
		}
		return
	}

	// Async Response
	c.JSON(200, ExecutionResponse{
		ExecutionID: execID,
		ActionName:  actionName,
		Status:      "submitted",
		StreamURL:   fmt.Sprintf("/api/executions/%s/stream", execID),
	})
}

func streamLogs(c *gin.Context, mqttClient mqtt.Client) {
	execID := c.Param("id")
	logTopic := fmt.Sprintf("tinpot/exec/%s/log", execID)
	// Also listen for completion
	resultTopic := fmt.Sprintf("tinpot/exec/%s/result", execID)

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	msgChan := make(chan string, 10)

	// Callback for logs
	logHandler := func(client mqtt.Client, msg mqtt.Message) {
		var logEntry LogEntry
		json.Unmarshal(msg.Payload(), &logEntry)

		sseData := map[string]interface{}{
			"type": "log",
			"data": logEntry,
		}
		bytes, _ := json.Marshal(sseData)
		select {
		case msgChan <- fmt.Sprintf("data: %s\n\n", bytes):
		default:
		}
	}

	// Callback for result (completion)
	resultHandler := func(client mqtt.Client, msg mqtt.Message) {
		var result ResultResponse
		json.Unmarshal(msg.Payload(), &result)

		sseData := map[string]interface{}{
			"type":       "complete",
			"state":      result.Status,
			"successful": result.Status == "SUCCESS",
		}
		if result.Status == "SUCCESS" {
			sseData["result"] = result.Result
		} else {
			sseData["error"] = result.Error
		}

		bytes, _ := json.Marshal(sseData)
		select {
		case msgChan <- fmt.Sprintf("data: %s\n\n", bytes):
		default:
		}
		close(msgChan)
	}

	// Subscribe
	token1 := mqttClient.Subscribe(logTopic, 0, logHandler)
	token2 := mqttClient.Subscribe(resultTopic, 1, resultHandler)
	token1.Wait()
	token2.Wait()

	defer func() {
		mqttClient.Unsubscribe(logTopic)
		mqttClient.Unsubscribe(resultTopic)
	}()

	// Stream
	clientGone := c.Writer.CloseNotify()

	// Send connected
	encoded, _ := json.Marshal(map[string]string{"type": "connected", "execution_id": execID})
	c.Writer.Write([]byte(fmt.Sprintf("data: %s\n\n", encoded)))
	c.Writer.Flush()

	for {
		select {
		case msg, ok := <-msgChan:
			if !ok {
				return // Closed (complete)
			}
			c.Writer.Write([]byte(msg))
			c.Writer.Flush()
		case <-clientGone:
			return
		}
	}
}

func getStatus(c *gin.Context) {
	// Status check is tricky without persistence.
	// For now, return a dummy running or try to fetch result if retained
	// If result is retained, we can return completed.
	// If not, maybe it's running? Or it doesn't exist.

	// Note: We don't have access to mqttClient here easily without passing it...
	// Just stub it for now as "PENDING"
	c.JSON(200, gin.H{
		"execution_id": c.Param("id"),
		"state":        "PENDING",
		"ready":        false,
	})
}

func cancelAction(c *gin.Context) {
	// Not supported
	c.JSON(501, gin.H{"detail": "Cancellation not supported"})
}
