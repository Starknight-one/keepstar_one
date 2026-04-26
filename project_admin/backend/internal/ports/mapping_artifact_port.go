package ports

import (
	"context"

	"keepstar-admin/internal/domain"
)

// MappingArtifactPort persists per-tenant mapping artifacts (one LLM call
// produces one; subsequent imports apply via plain code).
type MappingArtifactPort interface {
	// SaveMappingArtifact upserts the tenant's artifact, bumping version
	// and updating discovered_at/validated_at appropriately.
	SaveMappingArtifact(ctx context.Context, schema *domain.TenantCatalogSchema) error

	// GetMappingArtifact returns the current artifact for a tenant, or nil
	// if none exists yet (first-time import path).
	GetMappingArtifact(ctx context.Context, tenantID string) (*domain.TenantCatalogSchema, error)

	// MarkStale flips status to 'stale' for a tenant — called after curator
	// promotes a candidate, since downstream re-imports must regenerate.
	MarkStale(ctx context.Context, tenantID string) error

	// MarkAllStaleForVertical flips all artifacts in a vertical to stale.
	// Called when curator promotes an attribute that affects many tenants.
	MarkAllStaleForVertical(ctx context.Context, vertical string) error
}
