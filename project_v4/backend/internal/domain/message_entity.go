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
	ID         string      `json:"id"`
	SessionID  string      `json:"sessionId,omitempty"`
	Role       MessageRole `json:"role"`
	Content    string      `json:"content"`
	Widgets    []Widget    `json:"widgets,omitempty"`
	Formation  *Formation  `json:"formation,omitempty"`
	TokensUsed int         `json:"tokensUsed,omitempty"`
	ModelUsed  string      `json:"modelUsed,omitempty"`
	LatencyMs  int         `json:"latencyMs,omitempty"`
	SentAt     time.Time   `json:"sentAt"`
	ReceivedAt *time.Time  `json:"receivedAt,omitempty"`
	Timestamp  time.Time   `json:"timestamp"`
}
