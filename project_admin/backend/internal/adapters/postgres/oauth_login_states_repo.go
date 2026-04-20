package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"keepstar-admin/internal/ports"
)

// OAuthLoginStatesRepo writes OAuth-login states to the shared
// admin.oauth_states table with NULL tenant_id. The single-use semantics are
// enforced by DELETE RETURNING — a second callback with the same state will
// come back empty.
type OAuthLoginStatesRepo struct {
	client *Client
}

func NewOAuthLoginStatesRepo(c *Client) *OAuthLoginStatesRepo {
	return &OAuthLoginStatesRepo{client: c}
}

func (r *OAuthLoginStatesRepo) Create(ctx context.Context, s *ports.OAuthLoginState) error {
	_, err := r.client.pool.Exec(ctx,
		`INSERT INTO admin.oauth_states (state, tenant_id, kind, expires_at)
		 VALUES ($1, NULL, $2, $3)`,
		s.State, s.Kind, s.ExpiresAt)
	if err != nil {
		return fmt.Errorf("create oauth login state: %w", err)
	}
	return nil
}

func (r *OAuthLoginStatesRepo) Consume(ctx context.Context, state string) (*ports.OAuthLoginState, error) {
	var out ports.OAuthLoginState
	err := r.client.pool.QueryRow(ctx,
		`DELETE FROM admin.oauth_states
		 WHERE state = $1 AND expires_at > NOW()
		 RETURNING state, kind, created_at, expires_at`,
		state,
	).Scan(&out.State, &out.Kind, &out.CreatedAt, &out.ExpiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("oauth state not found or expired")
		}
		return nil, fmt.Errorf("consume oauth login state: %w", err)
	}
	return &out, nil
}
