package ports

import (
	"context"
	"time"
)

// Session represents a refresh-token family row. The plaintext refresh token
// is never stored — only TokenHash (sha256 hex). Rotation replaces one row
// with a new one and marks the old revoked.
//
// BrowserName / OSName / DeviceKind are parsed from UserAgent at create time.
// GeoCountry / GeoCity are derived from IP via MaxMind GeoLite2 when the DB
// is available; otherwise both stay empty.
type Session struct {
	ID             string
	UserID         string
	TenantID       string
	TokenHash      string
	UserAgent      string
	IP             string
	BrowserName    string
	BrowserVersion string
	OSName         string
	DeviceKind     string // "desktop" | "mobile" | "tablet" | "bot" | ""
	GeoCountry     string // ISO 3166-1 alpha-2, e.g. "US"
	GeoCity        string
	CreatedAt      time.Time
	ExpiresAt      time.Time
	RevokedAt      *time.Time
}

type SessionPort interface {
	Create(ctx context.Context, s *Session) error
	FindActive(ctx context.Context, tokenHash string) (*Session, error)
	FindByID(ctx context.Context, id string) (*Session, error)
	Revoke(ctx context.Context, id string) error
	RevokeAllForUser(ctx context.Context, userID string) error
	ListActiveForUser(ctx context.Context, userID string) ([]*Session, error)
}
