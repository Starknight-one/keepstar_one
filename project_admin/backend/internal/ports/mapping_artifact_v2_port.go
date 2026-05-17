package ports

import (
	"context"
	"time"

	"keepstar-admin/internal/domain"
)

// MappingArtifactV2Port persists per-tenant mapping artifacts in
// catalog.tenant_catalog_schema.mapping_artifact (JSONB column shared with
// the legacy MappingArtifact (v1) writer — distinct rows; mapping_artifact
// version field disambiguates).
//
// **Type note**: the port name retains the V2 suffix for filesystem stability
// but the in-memory type is now MappingArtifactV3. The adapter accepts a v2
// JSONB on read and lifts it to v3 transparently — see LiftV2ToV3 in
// domain/mapping_artifact_v3.go. Writes always emit v3.
type MappingArtifactV2Port interface {
	// Save upserts the artifact for a tenant. Bumps artifact_version,
	// stamps discovered_at = NOW(), validated_at = NOW(), status = 'active'.
	// Existing artifact body in the row is overwritten.
	Save(ctx context.Context, tenantID string, artifact *domain.MappingArtifactV3) error

	// Get returns the current artifact for a tenant, or (nil, meta, nil)
	// when the row exists but holds a legacy v1 artifact that v3 readers
	// should treat as absent. Returns (nil, nil, nil) when no row at all.
	Get(ctx context.Context, tenantID string) (*domain.MappingArtifactV3, *MappingArtifactMeta, error)

	// MarkStale flips status to 'stale' without touching the artifact body
	// — used by update_orchestrator when a mapping miss is logged, before
	// it triggers a narrow re-discovery.
	MarkStale(ctx context.Context, tenantID string) error
}

// MappingArtifactMeta is the row-level scaffolding that lives alongside the
// artifact body in tenant_catalog_schema. Kept separate from the artifact
// itself so the body stays purely about mapping rules.
type MappingArtifactMeta struct {
	ArtifactVersion int       // monotonic per tenant; tools/UI sort by this
	Status          string    // 'active' | 'stale' | 'needs_human_review'
	DiscoveredAt    time.Time
	ValidatedAt     *time.Time
}
