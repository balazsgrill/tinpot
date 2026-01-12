package main

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

// Stream Event
type StreamEvent struct {
	Type string      `json:"type"` // "log" or "complete"
	Data interface{} `json:"data"` // LogEntry or ResultResponse-like map
}
