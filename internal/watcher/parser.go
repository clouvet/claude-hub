package watcher

import (
	"encoding/json"
	"time"
)

// ClaudeMessage represents a message in a Claude .jsonl file
type ClaudeMessage struct {
	Type        string          `json:"type"`
	UUID        string          `json:"uuid"`
	ParentUUID  string          `json:"parentUuid"`
	SessionID   string          `json:"sessionId"`
	Timestamp   string          `json:"timestamp"`
	Message     json.RawMessage `json:"message,omitempty"`
	IsSidechain bool            `json:"isSidechain"`

	// For agent messages
	AgentID string `json:"agentId,omitempty"`
}

// ParsedMessage represents a parsed message ready for broadcasting
type ParsedMessage struct {
	Type      string
	Role      string // "user" or "assistant"
	Content   string
	Timestamp time.Time
	Raw       json.RawMessage
}

// ParseJSONLLine parses a single JSONL line
func ParseJSONLLine(line string) (*ClaudeMessage, error) {
	var msg ClaudeMessage
	if err := json.Unmarshal([]byte(line), &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// ExtractContent extracts displayable content from a Claude message
func ExtractContent(msg *ClaudeMessage) (*ParsedMessage, error) {
	// Skip non-displayable message types
	if msg.Type == "file-history-snapshot" || msg.Type == "queue-operation" {
		return nil, nil
	}

	// Parse timestamp
	timestamp, _ := time.Parse(time.RFC3339, msg.Timestamp)

	parsed := &ParsedMessage{
		Type:      msg.Type,
		Timestamp: timestamp,
		Raw:       []byte{}, // Will be filled if needed
	}

	// Extract content based on message type
	if msg.Type == "user" {
		parsed.Role = "user"

		// Parse message content
		var content struct {
			Role    string      `json:"role"`
			Content interface{} `json:"content"`
		}
		if err := json.Unmarshal(msg.Message, &content); err == nil {
			// Handle both string and array content
			switch v := content.Content.(type) {
			case string:
				parsed.Content = v
			case []interface{}:
				// For array content (with images, etc), extract text blocks
				for _, block := range v {
					if blockMap, ok := block.(map[string]interface{}); ok {
						if blockMap["type"] == "text" {
							if text, ok := blockMap["text"].(string); ok {
								parsed.Content += text
							}
						}
					}
				}
			}
		}
	} else if msg.Type == "assistant" {
		parsed.Role = "assistant"

		// Parse assistant message
		var assistantMsg struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal(msg.Message, &assistantMsg.Message); err == nil {
			// Content is an array of content blocks
			var contentBlocks []map[string]interface{}
			if err := json.Unmarshal(assistantMsg.Message.Content, &contentBlocks); err == nil {
				for _, block := range contentBlocks {
					if block["type"] == "text" {
						if text, ok := block["text"].(string); ok {
							parsed.Content += text
						}
					}
				}
			}
		}
	}

	// Only return if we have content
	if parsed.Content != "" {
		// Filter out internal Claude Code command messages
		if shouldSkipMessage(parsed.Content) {
			return nil, nil
		}
		return parsed, nil
	}

	return nil, nil
}

// shouldSkipMessage checks if a message contains internal Claude metadata
func shouldSkipMessage(content string) bool {
	// Skip messages with internal Claude Code markers
	markers := []string{
		"<local-command-caveat>",
		"<command-name>",
		"<command-message>",
		"<command-args>",
		"<local-command-stdout>",
		"<local-command-stderr>",
		"<system-reminder>",
	}

	for _, marker := range markers {
		if len(content) >= len(marker) && content[:len(marker)] == marker {
			return true
		}
	}

	return false
}
