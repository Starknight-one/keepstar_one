package ports

import (
	"context"
	"time"
)

// Invitation is a single-use workspace invite. TokenHash is sha256(plaintext).
// AcceptedAt nil = still pending; non-nil = already accepted (enforcement is
// at the usecase layer so we can return a specific error).
type Invitation struct {
	ID         string
	TenantID   string
	Email      string
	Role       string
	TokenHash  string
	InviterID  string
	ExpiresAt  time.Time
	AcceptedAt *time.Time
	CreatedAt  time.Time
}

// InvitationPreview is the public preview an invitee sees before deciding to
// accept — includes tenant + inviter context but nothing sensitive.
type InvitationPreview struct {
	Email       string    `json:"email"`
	Role        string    `json:"role"`
	TenantID    string    `json:"tenant_id"`
	TenantName  string    `json:"tenant_name"`
	TenantSlug  string    `json:"tenant_slug"`
	InviterName string    `json:"inviter_name"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type InvitationPort interface {
	Create(ctx context.Context, inv *Invitation) error
	FindActive(ctx context.Context, tokenHash string) (*Invitation, error)
	MarkAccepted(ctx context.Context, id string) error
	ListForTenant(ctx context.Context, tenantID string) ([]*Invitation, error)
	// Preview joins tenant + inviter so we can show the invitee who invited
	// them before they decide to accept.
	Preview(ctx context.Context, tokenHash string) (*InvitationPreview, error)
	// CountRecentByInviter is used by the rate-limit check (max N per 24h).
	CountRecentByInviter(ctx context.Context, inviterID string, since time.Time) (int, error)
	// GetByID fetches an invitation row by primary key — used by Resend to
	// pull the original recipient/tenant/role tuple. Returns (nil, nil) when
	// no row matches.
	GetByID(ctx context.Context, id string) (*Invitation, error)
}
