package main_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	mqttserver "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEndToEnd(t *testing.T) {
	// 1. Start MQTT Broker
	port := getFreePort()
	broker := mqttserver.New(nil)
	tcp := listeners.NewTCP(listeners.Config{
		ID:      "t1",
		Address: fmt.Sprintf("localhost:%d", port),
	})
	broker.AddListener(tcp)

	// Allow all
	broker.AddHook(new(auth.AllowHook), nil)

	go func() {
		err := broker.Serve()
		if err != nil {
			log.Fatal(err)
		}
	}()
	defer broker.Close()

	// Wait for broker
	time.Sleep(1 * time.Second)
	mqttURL := fmt.Sprintf("tcp://localhost:%d", port)

	// 2. Build Binaries
	rootDir, _ := filepath.Abs("../..")
	binDir := filepath.Join(rootDir, "bin")
	os.MkdirAll(binDir, 0755)

	coordBin := filepath.Join(binDir, "coordinator")
	workerBin := filepath.Join(binDir, "worker")

	// Build Coordinator
	cmd := exec.Command("go", "build", "-o", coordBin, ".")
	cmd.Dir = filepath.Join(rootDir, "cmd/coordinator")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Run(), "Failed to build coordinator")

	// Build Worker
	cmd = exec.Command("go", "build", "-o", workerBin, ".")
	cmd.Dir = filepath.Join(rootDir, "cmd/worker")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Run(), "Failed to build worker")

	// 3. Start Coordinator
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	coordPort := getFreePort()
	coordCmd := exec.CommandContext(ctx, coordBin)
	coordCmd.Env = append(os.Environ(),
		fmt.Sprintf("MQTT_BROKER=%s", mqttURL),
		fmt.Sprintf("PORT=%d", coordPort),
	)
	coordCmd.Stdout = os.Stdout
	coordCmd.Stderr = os.Stderr

	err := coordCmd.Start()
	require.NoError(t, err, "Coordinator failed to start")

	// 4. Start Worker
	workerCmd := exec.CommandContext(ctx, workerBin)
	workerCmd.Env = append(os.Environ(),
		fmt.Sprintf("MQTT_BROKER=%s", mqttURL),
		fmt.Sprintf("ACTIONS_DIR=%s", filepath.Join(rootDir, "actions")),
		fmt.Sprintf("APP_DIR=%s", filepath.Join(rootDir, "app")),
		// Need to set PYTHONPATH if needed, but worker sets it in setupPython
	)
	workerCmd.Stdout = os.Stdout
	workerCmd.Stderr = os.Stderr // Capture worker logs

	err = workerCmd.Start()
	require.NoError(t, err, "Worker failed to start")

	// 5. Wait for Action Discovery
	// Poll GET http://localhost:<port>/api/actions
	apiURL := fmt.Sprintf("http://localhost:%d", coordPort)
	require.Eventually(t, func() bool {
		resp, err := http.Get(apiURL + "/api/actions")
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return false
		}
		var actions map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&actions)

		// Check for 'clean_cache' or similar from example_actions
		// Assuming example_actions.py has 'clean_cache'
		_, ok := actions["clean_cache"]
		return ok
	}, 30*time.Second, 1*time.Second, "Actions not discovered in time")

	// 6. Execute Action (Sync)
	payload := map[string]interface{}{
		"parameters": map[string]interface{}{
			"days": 5,
		},
	}
	payloadBytes, _ := json.Marshal(payload)

	resp, err := http.Post(
		apiURL+"/api/actions/clean_cache/sync_execute",
		"application/json",
		bytes.NewBuffer(payloadBytes),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, 200, resp.StatusCode, "Sync execution failed: "+string(body))

	var execResp map[string]interface{}
	json.Unmarshal(body, &execResp)

	assert.Equal(t, "SUCCESS", execResp["status"])
	// Check result
	// clean_cache returns {"files_deleted": 42} usually (based on README)
	// Or we check example_actions.py content
	fmt.Printf("Execution Result: %v\n", execResp)

	// 7. Execute Action (Async)
	// Use health_check for logs
	payload = map[string]interface{}{
		"parameters": map[string]interface{}{
			"duration": 3,
		},
	}
	payloadBytes, _ = json.Marshal(payload)

	resp, err = http.Post(
		apiURL+"/api/actions/health_check/execute",
		"application/json",
		bytes.NewBuffer(payloadBytes),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	body, _ = io.ReadAll(resp.Body)
	assert.Equal(t, 200, resp.StatusCode, "Async execution submission failed: "+string(body))

	var asyncResp struct {
		ExecutionID string `json:"execution_id"`
		Status      string `json:"status"`
		StreamURL   string `json:"stream_url"`
	}
	json.Unmarshal(body, &asyncResp)
	assert.NotEmpty(t, asyncResp.ExecutionID)
	assert.Equal(t, "submitted", asyncResp.Status)
	assert.NotEmpty(t, asyncResp.StreamURL)

	// 8. Stream Logs
	streamURL := apiURL + asyncResp.StreamURL

	ctxStream, cancelStream := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelStream()

	reqStream, err := http.NewRequestWithContext(ctxStream, "GET", streamURL, nil)
	require.NoError(t, err)

	streamResp, err := http.DefaultClient.Do(reqStream)
	require.NoError(t, err)
	defer streamResp.Body.Close()

	assert.Equal(t, 200, streamResp.StatusCode)

	reader := streamResp.Body
	buf := make([]byte, 1024)
	var streamOutput string

	// Simple SSE reader
	// In a real client we'd use line scanner, but here simple read loop until close
	// Actually, we should probably read line by line to parse events properly?
	// Or just accummulate and check contents?
	// Since the connection stays open until completion, we can read until EOF.

	for {
		n, err := reader.Read(buf)
		if n > 0 {
			streamOutput += string(buf[:n])
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			if err == context.DeadlineExceeded {
				t.Fatal("Stream timed out")
			}
			t.Logf("Stream read error: %v", err)
			break
		}
	}
	fmt.Println("Stream finished.")

	// Verify Logs
	assert.Contains(t, streamOutput, "Starting health check")
	assert.Contains(t, streamOutput, "[1/3] Checking system health")

	// Verify Completion
	// SSE format: data: {"type": "complete", "data": ...}
	assert.Contains(t, streamOutput, `"type":"complete"`)
	assert.Contains(t, streamOutput, `"successful":true`)
}

func getFreePort() int {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0
	}
	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
