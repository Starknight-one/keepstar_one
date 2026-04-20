package ports

import (
	"context"
	"time"
)

// OAuthLoginState is the short-lived nonce issued at OAuth login start and
// consumed in the callback. Single-use — callback DELETEs after match.
type OAuthLoginState struct {
	State     string
	Kind      string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// OAuthLoginStatePort persists pre-login OAuth states (Google, Telegram). We
// reuse admin.oauth_states by giving these states NULL tenant_id — the user
// has no tenant context yet at login time.
type OAuthLoginStatePort interface {
	Create(ctx context.Context, state *OAuthLoginState) error
	Consume(ctx context.Context, state string) (*OAuthLoginState, error)
}
