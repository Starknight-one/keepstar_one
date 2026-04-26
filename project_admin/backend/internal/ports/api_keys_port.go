package ports

import (
	"context"

	"keepstar-admin/internal/domain"
)

// APIKeysPort manages tenant-scoped API keys for the public REST API.
//
// Plain key contract: returned only by Create. Verify takes the plain key
// (header X-API-Key) and resolves it back to a tenant + key id by indexed
// prefix lookup + bcrypt compare against the stored hash.
type APIKeysPort interface {
	// Create generates a new key, hashes it, and persists it. Returns the
	// stored APIKey row plus the plain text — the caller must surface the
	// plain text to the user once and never persist it elsewhere.
	Create(ctx context.Context, tenantID, label string) (key domain.APIKey, plain string, err error)

	// List returns all non-revoked keys for a tenant. KeyHash never leaves
	// the adapter — only metadata.
	List(ctx context.Context, tenantID string) ([]domain.APIKey, error)

	// Revoke marks a key as revoked (soft, preserves history).
	Revoke(ctx context.Context, tenantID, apiKeyID string) error

	// Verify resolves a plain X-API-Key header value to a tenant + key id.
	// Returns ErrAPIKeyInvalid on miss.
	Verify(ctx context.Context, plain string) (tenantID, apiKeyID string, err error)

	// TouchLastUsed bumps last_used_at to NOW(). Best-effort; errors are
	// logged but not propagated to the request path.
	TouchLastUsed(ctx context.Context, apiKeyID string) error
}
