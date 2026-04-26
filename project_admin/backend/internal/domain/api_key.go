package domain

import "time"

// APIKey represents a tenant-scoped API key for the public REST API. The
// plain text token is shown to the user once (on create) and stored only as
// a bcrypt hash + a short prefix used for indexed lookup.
type APIKey struct {
	ID         string     `json:"id"`
	TenantID   string     `json:"tenantId"`
	Label      string     `json:"label,omitempty"`
	KeyPrefix  string     `json:"keyPrefix,omitempty"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
}
