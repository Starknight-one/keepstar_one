package ports

import (
	"context"
	"time"

	"keepstar-admin/internal/domain"
)

// MappingArtifactV2Port persists per-tenant MappingArtifactV2 in
// catalog.tenant_catalog_schema. Distinct from the legacy
// MappingArtifactPort which still operates on the old MappingArtifact
// struct + supporting fields (junk rules, brand mapping, etc.).
//
// When discovery_v2 + apply_v2 fully replace the legacy pair we'll delete
// the old port and rename this one.
type MappingArtifactV2Port interface {
	// Save upserts the artifact for a tenant. Bumps artifact_version,
	// stamps discovered_at = NOW(), validated_at = NOW(),
	// status = 'validated'. Old artifact in the row is overwritten.
	Save(ctx context.Context, tenantID string, artifact *domain.MappingArtifactV2) error

	// Get returns the current artifact for a tenant, or nil when none
	// exists yet (cold-start path). Returns (nil, nil) for "no row".
	Get(ctx context.Context, tenantID string) (*domain.MappingArtifactV2, *MappingArtifactMeta, error)

	// MarkStale flips status to 'stale' without touching the artifact body
	// — used by update_orchestrator when a mapping miss is logged, before
	// it triggers a narrow re-discovery.
	MarkStale(ctx context.Context, tenantID string) error
}

// MappingArtifactMeta is the row-level scaffolding that lives alongside
// the artifact body in tenant_catalog_schema. Kept separate from
// MappingArtifactV2 so the body stays purely about mapping rules.
type MappingArtifactMeta struct {
	ArtifactVersion int       // monotonic per tenant; tools/UI sort by this
	Status          string    // 'validated' | 'stale' | 'needs_human_review'
	DiscoveredAt    time.Time
	ValidatedAt     *time.Time
}
