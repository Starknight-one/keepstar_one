package domain

import "time"

// ChatUser represents a chat visitor
type ChatUser struct {
	ID          string            `json:"id"`
	ExternalID  string            `json:"externalId,omitempty"`
	TenantID    string            `json:"tenantId"`
	Fingerprint string            `json:"fingerprint,omitempty"`
	IPAddress   string            `json:"ipAddress,omitempty"`
	UserAgent   string            `json:"userAgent,omitempty"`
	Metadata    map[string]any    `json:"metadata,omitempty"`
	FirstSeenAt time.Time         `json:"firstSeenAt"`
	LastSeenAt  time.Time         `json:"lastSeenAt"`
	CreatedAt   time.Time         `json:"createdAt"`
}
