package tinpot

type ActionResponse func(error string, result map[string]interface{})
type ActionLogs func(level string, message string)

// ActionTrigger triggers the execution of the action. It is expected to be asynchronous
type ActionTrigger func(parameters map[string]interface{}, response ActionResponse, logs ActionLogs)

type ActionInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Group       string   `json:"group"`
	Parameters  []string `json:"parameters"`
}

type ActionManager interface {
	GetAction(name string) ActionTrigger
	ListActions() map[string]ActionInfo
	IsConnected() bool
}

type MqttAction struct {
	Description  string   `json:"description"`
	Group        string   `json:"group"`
	Parameters   []string `json:"parameters"`
	TriggerTopic string   `json:"trigger_topic"`
}

const (
	MQTT_TOPIC_PREFIX = "tinpot/actions/"
)

// Log Entry
type MqttLogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
}

// Result Entry
type MqttResultResponse struct {
	Status string      `json:"status"`
	Result interface{} `json:"result"`
	Error  string      `json:"error,omitempty"`
}
