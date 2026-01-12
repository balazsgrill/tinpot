package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/balazsgrill/tinpot"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
)

//go:embed all:lib
var embeddedLib embed.FS

// Configuration
var (
	MQTTBroker = getEnv("MQTT_BROKER", "tcp://localhost:1883")
	ActionsDir = getEnv("ACTIONS_DIR", "../actions")
)

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {

	mgr := NewPyActionManager()
	opts := mqtt.NewClientOptions().AddBroker(MQTTBroker)
	clientID := "tinpot-worker-" + uuid.New().String()
	opts.SetClientID(clientID)
	opts.SetAutoReconnect(true)

	opts.SetOnConnectHandler(func(c mqtt.Client) {
		log.Println("Connected to MQTT Broker")
		announceActions(mgr, c)
		subscribeToActions(mgr, c)
	})

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("Failed to connect to MQTT: %v", token.Error())
	}

	select {}
}

func extractEmbeddedLib() (string, error) {
	tempDir, err := os.MkdirTemp("", "tinpot-worker-lib-*")
	if err != nil {
		return "", err
	}

	// Our embedded FS has "lib" at root.
	// We want to extract contents of "lib" to tempDir.
	// So tempDir/tinpot/...
	// Because "lib" contains "tinpot" folder.

	err = fs.WalkDir(embeddedLib, "lib", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel("lib", path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		destPath := filepath.Join(tempDir, relPath)

		if d.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		data, err := embeddedLib.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(destPath, data, 0644)
	})

	if err != nil {
		os.RemoveAll(tempDir)
		return "", err
	}

	return tempDir, nil
}

func triggerTopicForAction(actionName string) string {
	return fmt.Sprintf("tinpot/actions/%s/trigger", actionName)
}

func announceTopicForAction(actionName string) string {
	return fmt.Sprintf("tinpot/actions/%s", actionName)
}

func toMqttAction(act tinpot.ActionInfo) tinpot.MqttAction {
	return tinpot.MqttAction{
		Description:  act.Description,
		Group:        act.Group,
		Parameters:   act.Parameters,
		TriggerTopic: triggerTopicForAction(act.Name),
	}
}

func announceActions(mgr tinpot.ActionManager, c mqtt.Client) {
	actions := mgr.ListActions()
	for _, act := range actions {
		topic := announceTopicForAction(act.Name)
		payload, _ := json.Marshal(toMqttAction(act))
		c.Publish(topic, 1, true, payload).Wait()
	}
}

func subscribeToActions(mgr tinpot.ActionManager, c mqtt.Client) {
	actions := mgr.ListActions()
	for _, act := range actions {
		topic := triggerTopicForAction(act.Name)
		c.Subscribe(topic, 1, func(cl mqtt.Client, msg mqtt.Message) {
			go executeAction(mgr, cl, act.Name, msg)
		})
	}
}

type ExecutionRequest struct {
	ExecutionID string                 `json:"execution_id"`
	Parameters  map[string]interface{} `json:"parameters"`
	ResultTopic string                 `json:"result_topic"`
	LogTopic    string                 `json:"log_topic"`
}

func sendResult(c mqtt.Client, req ExecutionRequest, status string, result interface{}, error string) {
	resp := tinpot.MqttResultResponse{
		Status: status,
		Result: result,
		Error:  error,
	}
	payload, _ := json.Marshal(resp)
	token := c.Publish(req.ResultTopic, 1, true, payload)
	token.Wait()
	if token.Error() != nil {
		log.Printf("Failed to publish result: %v", token.Error())
	}
}

func executeAction(mgr tinpot.ActionManager, c mqtt.Client, actionName string, msg mqtt.Message) {
	var req ExecutionRequest
	err := json.Unmarshal(msg.Payload(), &req)
	if err != nil {
		log.Printf("Failed to unmarshal action %s: %v", actionName, err)
		return
	}

	var responseCallback tinpot.ActionResponse
	responseCallback = func(error string, result map[string]interface{}) {
		status := "SUCCESS"
		if error != "" {
			status = "FAILURE"
		}
		sendResult(c, req, status, result, error)
	}

	var logsCallback tinpot.ActionLogs
	logsCallback = func(level, message string) {
		entry := tinpot.MqttLogEntry{
			Timestamp: time.Now().Format(time.RFC3339),
			Level:     level,
			Message:   message,
		}
		data, _ := json.Marshal(entry)
		c.Publish(req.LogTopic, 1, true, data)
	}

	mgr.GetAction(actionName)(req.Parameters, responseCallback, logsCallback)
}
