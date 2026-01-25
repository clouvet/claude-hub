package claude

import "encoding/json"

// StreamMessage represents a message in the stream-json protocol
type StreamMessage struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message,omitempty"`
	Delta   json.RawMessage `json:"delta,omitempty"`

	// For system messages
	Subtype   string `json:"subtype,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

// UserMessage represents a user message to send to Claude
type UserMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"` // string or array of content blocks
}

// ContentDelta represents a streaming content delta
type ContentDelta struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// AssistantMessage represents Claude's response message
type AssistantMessage struct {
	Model      string          `json:"model"`
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Role       string          `json:"role"`
	Content    json.RawMessage `json:"content"`
	StopReason *string         `json:"stop_reason"`
	Usage      json.RawMessage `json:"usage,omitempty"`
}
