package domain

import "time"

// SessionStatus defines the status of a chat session
type SessionStatus string

const (
	SessionStatusActive   SessionStatus = "active"
	SessionStatusClosed   SessionStatus = "closed"
	SessionStatusArchived SessionStatus = "archived"
)

// Session represents a chat session
type Session struct {
	ID             string            `json:"id"`
	UserID         string            `json:"userId,omitempty"`
	TenantID       string            `json:"tenantId"`
	Status         SessionStatus     `json:"status"`
	Messages       []Message         `json:"messages"`
	Metadata       map[string]any    `json:"metadata,omitempty"`
	StartedAt      time.Time         `json:"startedAt"`
	EndedAt        *time.Time        `json:"endedAt,omitempty"`
	LastActivityAt time.Time         `json:"lastActivityAt"`
	CreatedAt      time.Time         `json:"createdAt"`
	UpdatedAt      time.Time         `json:"updatedAt"`
}
