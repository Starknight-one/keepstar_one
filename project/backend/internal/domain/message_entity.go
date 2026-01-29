package domain

import "time"

// MessageRole defines who sent the message
type MessageRole string

const (
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
)

// Message represents a chat message
type Message struct {
	ID        string      `json:"id"`
	Role      MessageRole `json:"role"`
	Content   string      `json:"content"`
	Widgets   []Widget    `json:"widgets,omitempty"`
	Formation *Formation  `json:"formation,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
}
