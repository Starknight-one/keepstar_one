// Package postgres — api_keys adapter for the public REST API (M10).
//
// Plain key shape: "kp_<32 random bytes base64-url>" → ~52 chars total.
// Stored:
//   - key_prefix: first 12 chars ("kp_xxxxxxxx") for indexed lookup.
//   - key_hash:   bcrypt of the full plain key.
//
// Verify path: SELECT WHERE key_prefix = $1 AND revoked_at IS NULL → bcrypt
// compare against each candidate hash. With unique prefixes the inner loop is
// 1 iteration in the common case; collisions are still cheap (bcrypt cost 10).
package postgres

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

const (
	apiKeyPrefixBytes = 12 // "kp_" + 9 chars
	apiKeyTokenBytes  = 32
	apiKeyBcryptCost  = 10
)

type APIKeysAdapter struct {
	client *Client
	log    *logger.Logger
}

func NewAPIKeysAdapter(client *Client, log *logger.Logger) *APIKeysAdapter {
	return &APIKeysAdapter{client: client, log: log}
}

var _ ports.APIKeysPort = (*APIKeysAdapter)(nil)

func (a *APIKeysAdapter) Create(ctx context.Context, tenantID, label string) (domain.APIKey, string, error) {
	if tenantID == "" {
		return domain.APIKey{}, "", errors.New("tenant_id required")
	}
	plain, err := generateAPIKey()
	if err != nil {
		return domain.APIKey{}, "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), apiKeyBcryptCost)
	if err != nil {
		return domain.APIKey{}, "", fmt.Errorf("hash key: %w", err)
	}
	prefix := plain
	if len(prefix) > apiKeyPrefixBytes {
		prefix = prefix[:apiKeyPrefixBytes]
	}

	var key domain.APIKey
	err = a.client.pool.QueryRow(ctx, `
		INSERT INTO catalog.api_keys (tenant_id, key_hash, key_prefix, label)
		VALUES ($1, $2, $3, NULLIF($4, ''))
		RETURNING id, tenant_id, COALESCE(label, ''), key_prefix, last_used_at, revoked_at, created_at`,
		tenantID, string(hash), prefix, label,
	).Scan(&key.ID, &key.TenantID, &key.Label, &key.KeyPrefix, &key.LastUsedAt, &key.RevokedAt, &key.CreatedAt)
	if err != nil {
		return domain.APIKey{}, "", fmt.Errorf("insert api_key: %w", err)
	}
	return key, plain, nil
}

func (a *APIKeysAdapter) List(ctx context.Context, tenantID string) ([]domain.APIKey, error) {
	rows, err := a.client.pool.Query(ctx, `
		SELECT id, tenant_id, COALESCE(label, ''), COALESCE(key_prefix, ''),
			last_used_at, revoked_at, created_at
		FROM catalog.api_keys
		WHERE tenant_id = $1
		ORDER BY revoked_at IS NULL DESC, created_at DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list api_keys: %w", err)
	}
	defer rows.Close()
	var out []domain.APIKey
	for rows.Next() {
		var k domain.APIKey
		if err := rows.Scan(&k.ID, &k.TenantID, &k.Label, &k.KeyPrefix,
			&k.LastUsedAt, &k.RevokedAt, &k.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (a *APIKeysAdapter) Revoke(ctx context.Context, tenantID, apiKeyID string) error {
	tag, err := a.client.pool.Exec(ctx, `
		UPDATE catalog.api_keys SET revoked_at = NOW()
		WHERE id = $1 AND tenant_id = $2 AND revoked_at IS NULL`,
		apiKeyID, tenantID)
	if err != nil {
		return fmt.Errorf("revoke api_key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (a *APIKeysAdapter) Verify(ctx context.Context, plain string) (string, string, error) {
	if len(plain) < apiKeyPrefixBytes {
		return "", "", domain.ErrAPIKeyInvalid
	}
	prefix := plain[:apiKeyPrefixBytes]
	rows, err := a.client.pool.Query(ctx, `
		SELECT id, tenant_id, key_hash
		FROM catalog.api_keys
		WHERE key_prefix = $1 AND revoked_at IS NULL`, prefix)
	if err != nil {
		return "", "", fmt.Errorf("query api_keys: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, tid, hash string
		if err := rows.Scan(&id, &tid, &hash); err != nil {
			return "", "", err
		}
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil {
			return tid, id, nil
		}
	}
	return "", "", domain.ErrAPIKeyInvalid
}

func (a *APIKeysAdapter) TouchLastUsed(ctx context.Context, apiKeyID string) error {
	_, err := a.client.pool.Exec(ctx,
		`UPDATE catalog.api_keys SET last_used_at = NOW() WHERE id = $1`, apiKeyID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("touch api_key: %w", err)
	}
	return nil
}

// generateAPIKey returns a random token of the form "kp_<base64-url>". The
// kp_ prefix lets a leaked key be grep'd in logs and the format-checked at the
// edge before the database lookup.
func generateAPIKey() (string, error) {
	buf := make([]byte, apiKeyTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	return "kp_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
