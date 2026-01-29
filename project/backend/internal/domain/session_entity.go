package domain

import "time"

// Session represents a chat session
type Session struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenantId"`
	Messages  []Message `json:"messages"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
