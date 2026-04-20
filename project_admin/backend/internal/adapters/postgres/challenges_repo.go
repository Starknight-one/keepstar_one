package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"keepstar-admin/internal/ports"
)

type ChallengesRepo struct {
	client *Client
}

func NewChallengesRepo(client *Client) *ChallengesRepo {
	return &ChallengesRepo{client: client}
}

func (r *ChallengesRepo) Create(ctx context.Context, c *ports.Challenge) error {
	metaBytes, err := json.Marshal(c.Meta)
	if err != nil {
		return fmt.Errorf("marshal challenge meta: %w", err)
	}

	q := `INSERT INTO admin.auth_challenges
			(user_id, email, kind, code_hash, expires_at, meta_json)
		  VALUES ($1, $2, $3, $4, $5, $6)
		  RETURNING id, created_at`

	var userID any
	if c.UserID != "" {
		userID = c.UserID
	}
	var email any
	if c.Email != "" {
		email = c.Email
	}

	return r.client.pool.QueryRow(ctx, q,
		userID, email, c.Kind, c.CodeHash, c.ExpiresAt, metaBytes,
	).Scan(&c.ID, &c.CreatedAt)
}

func (r *ChallengesRepo) FindActive(ctx context.Context, kind, codeHash string) (*ports.Challenge, error) {
	q := `SELECT id, COALESCE(user_id::text, ''), COALESCE(email::text, ''),
				 kind, code_hash, expires_at, consumed_at, meta_json, created_at
		  FROM admin.auth_challenges
		  WHERE kind = $1 AND code_hash = $2
				AND consumed_at IS NULL AND expires_at > NOW()
		  LIMIT 1`

	var c ports.Challenge
	var metaBytes []byte
	err := r.client.pool.QueryRow(ctx, q, kind, codeHash).Scan(
		&c.ID, &c.UserID, &c.Email, &c.Kind, &c.CodeHash, &c.ExpiresAt,
		&c.ConsumedAt, &metaBytes, &c.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find challenge: %w", err)
	}
	if len(metaBytes) > 0 {
		_ = json.Unmarshal(metaBytes, &c.Meta)
	}
	return &c, nil
}

func (r *ChallengesRepo) Consume(ctx context.Context, id string) error {
	q := `UPDATE admin.auth_challenges SET consumed_at = NOW() WHERE id = $1 AND consumed_at IS NULL`
	tag, err := r.client.pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("consume challenge: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("challenge %s already consumed or missing", id)
	}
	return nil
}

func (r *ChallengesRepo) DeleteExpired(ctx context.Context) error {
	q := `DELETE FROM admin.auth_challenges WHERE expires_at < NOW() - INTERVAL '7 days'`
	_, err := r.client.pool.Exec(ctx, q)
	return err
}
