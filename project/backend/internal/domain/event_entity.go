package domain

import "time"

// EventType defines types of chat events
type EventType string

const (
	EventChatOpened      EventType = "chat_opened"
	EventMessageSent     EventType = "message_sent"
	EventMessageReceived EventType = "message_received"
	EventChatClosed      EventType = "chat_closed"
	EventWidgetClicked   EventType = "widget_clicked"
	EventSessionTimeout  EventType = "session_timeout"
)

// ChatEvent represents a trackable chat event
type ChatEvent struct {
	ID        string         `json:"id"`
	SessionID string         `json:"sessionId,omitempty"`
	UserID    string         `json:"userId,omitempty"`
	EventType EventType      `json:"eventType"`
	EventData map[string]any `json:"eventData,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
}
