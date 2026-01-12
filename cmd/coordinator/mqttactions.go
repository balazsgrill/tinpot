package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"

	"github.com/balazsgrill/tinpot"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"
)

type mqttActionManager struct {
	client  mqtt.Client
	actions map[string]tinpot.MqttAction
	mu      sync.RWMutex
}

func (m *mqttActionManager) IsConnected() bool {
	return m.client.IsConnected()
}

func NewMqttActionManager(brokerurl string) tinpot.ActionManager {
	// Setup MQTT
	opts := mqtt.NewClientOptions().AddBroker(brokerurl)
	opts.SetClientID("tinpot-coordinator-" + uuid.New().String())
	opts.SetAutoReconnect(true)

	// Create client
	client := mqtt.NewClient(opts)

	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("Failed to connect to MQTT: %v", token.Error())
	}

	m := &mqttActionManager{
		client:  client,
		actions: make(map[string]tinpot.MqttAction),
	}
	// Subscribe to action announcements
	// Note: We use a wrapper closure to match the library's callback signature if needed,
	// but paho.mqtt MessageHandler matches.
	client.Subscribe(tinpot.MQTT_TOPIC_PREFIX+"+", 1, m.onActionAnnounced)
	return m
}

func (m *mqttActionManager) onActionAnnounced(c mqtt.Client, msg mqtt.Message) {
	topic := msg.Topic()
	parts := strings.Split(topic, "/")
	if len(parts) != 3 {
		return
	}
	actionName := parts[2]

	if len(msg.Payload()) == 0 {
		m.mu.Lock()
		delete(m.actions, actionName)
		m.mu.Unlock()
		log.Printf("Action removed: %s", actionName)
		return
	}

	var act tinpot.MqttAction
	if err := json.Unmarshal(msg.Payload(), &act); err != nil {
		log.Printf("Failed to unmarshal action %s: %v", actionName, err)
		return
	}

	m.mu.Lock()
	m.actions[actionName] = act
	m.mu.Unlock()
	log.Printf("Action discovered: %s", actionName)
}

func (m *mqttActionManager) ListActions() map[string]tinpot.ActionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]tinpot.ActionInfo)
	for name, act := range m.actions {
		result[name] = tinpot.ActionInfo{
			Name:        name,
			Description: act.Description,
			Group:       act.Group,
			Parameters:  act.Parameters,
		}
	}
	return result
}

type mqttActionExecution struct {
	action *tinpot.MqttAction
	client mqtt.Client

	execId string
}

type CloserFunc func() error

func (cf CloserFunc) Close() error {
	return cf()
}

func (act *mqttActionExecution) Closer(topics ...string) io.Closer {
	return CloserFunc(func() error {
		t := act.client.Unsubscribe(topics...)
		t.Wait()
		return t.Error()
	})
}

func (act *mqttActionExecution) handleResponse(msg mqtt.Message, response tinpot.ActionResponse) {
	var res tinpot.MqttResultResponse
	if err := json.Unmarshal(msg.Payload(), &res); err != nil {
		return
	}
	if response != nil {
		if res.Status == "SUCCESS" {
			// Need to cast res.Result to map[string]interface{} if possible,
			// but interface says method signature is Result map...
			// The Python worker sends a JSON object usually.
			// If it's a map, great. If primitive, we might have issues matching the callback signature.
			// Reviewing Actions.go: func(error string, result map[string]interface{})
			// If result is not a map, we wrap it?

			var resMap map[string]interface{}
			if m, ok := res.Result.(map[string]interface{}); ok {
				resMap = m
			} else {
				// Wrap it
				resMap = map[string]interface{}{"value": res.Result}
			}
			response("", resMap)
		} else {
			response(res.Error, nil)
		}
	}
}

func (act *mqttActionExecution) trigger(parameters map[string]interface{}, response tinpot.ActionResponse, logs tinpot.ActionLogs) {
	// Extract or generate Execution ID
	var execID string
	if id, ok := parameters["_execution_id"].(string); ok {
		execID = id
	} else {
		execID = uuid.New().String()
	}

	// Filter internal parameters
	actualParams := make(map[string]interface{})
	for k, v := range parameters {
		if !strings.HasPrefix(k, "_") {
			actualParams[k] = v
		}
	}

	resultTopic := fmt.Sprintf("tinpot/exec/%s/result", execID)
	logTopic := fmt.Sprintf("tinpot/exec/%s/log", execID)
	closer := act.Closer(resultTopic, logTopic)

	// 1. Subscribe to Log Topic (if logs callback provided)
	if logs != nil {
		act.client.Subscribe(logTopic, 0, func(c mqtt.Client, msg mqtt.Message) {
			var entry tinpot.MqttLogEntry
			if err := json.Unmarshal(msg.Payload(), &entry); err == nil {
				logs(entry.Level, entry.Message) // Just pass message or structured? Interface asks for level, message
			}
		})
	}

	subToken := act.client.Subscribe(resultTopic, 1, func(c mqtt.Client, msg mqtt.Message) {
		defer closer.Close()
		if response != nil {
			act.handleResponse(msg, response)
		}
	})
	subToken.Wait()
	if subToken.Error() != nil {
		log.Printf("Failed to subscribe to result topic: %v", subToken.Error())
	}

	// 3. Publish Execution Request
	req := ExecutionRequest{
		ExecutionID: execID,
		Parameters:  actualParams,
		ResultTopic: resultTopic,
		LogTopic:    logTopic,
	}
	payloadBytes, _ := json.Marshal(req)
	token := act.client.Publish(act.action.TriggerTopic, 1, false, payloadBytes)
	token.Wait()

	if token.Error() != nil {
		closer.Close()
		if response != nil {
			responseWithErr(response, fmt.Sprintf("failed to publish request: %v", token.Error()))
		}
		return
	}
}

func (m *mqttActionManager) GetAction(name string) tinpot.ActionTrigger {
	m.mu.RLock()
	act, ok := m.actions[name]
	m.mu.RUnlock()

	if !ok {
		return nil
	}

	execution := &mqttActionExecution{
		action: &act,
		client: m.client,
	}

	return execution.trigger
}

func responseWithErr(response tinpot.ActionResponse, err string) {
	response(err, nil)
}
