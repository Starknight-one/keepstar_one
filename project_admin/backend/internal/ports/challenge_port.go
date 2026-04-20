package ports

import (
	"context"
	"time"
)

// Challenge kinds — mirrors the CHECK constraint in admin.auth_challenges.
const (
	ChallengeEmailVerify   = "email_verify"
	ChallengePasswordReset = "password_reset"
	ChallengeTOTPSetup     = "totp_setup"
	ChallengeEmail2FA      = "email_2fa"
	ChallengeMagicLink     = "magic_link"
)

// Challenge is a single-use code/token issued for one of the flows above.
// CodeHash is sha256(code) — plaintext is never stored. Meta is a free-form
// bag for flow-specific extras (e.g. intended tenant on an invite accept).
type Challenge struct {
	ID         string
	UserID     string // may be empty for pre-account flows
	Email      string
	Kind       string
	CodeHash   string
	ExpiresAt  time.Time
	ConsumedAt *time.Time
	Meta       map[string]any
	CreatedAt  time.Time
}

type ChallengePort interface {
	Create(ctx context.Context, c *Challenge) error
	FindActive(ctx context.Context, kind, codeHash string) (*Challenge, error)
	Consume(ctx context.Context, id string) error
	DeleteExpired(ctx context.Context) error
}
