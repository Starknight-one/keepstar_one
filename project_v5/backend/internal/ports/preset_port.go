package ports

import (
	"context"

	"keepstar_v5/internal/domain"
)

// PresetPort is the read surface V5 uses to load tenant-edited presets at
// render time. The future v9-canvas microservice (Stream B) will be the
// write side; this interface stays read-only until that lands.
//
// Both methods accept a tenant slug OR UUID — same convention as
// CatalogAdapter.GetTenantBySlug callers, who pass slugs while internal
// pipelines pass UUIDs. Adapter does the right lookup.
type PresetPort interface {
	// GetPublishedPreset returns the latest published version of (tenantSlugOrID, name).
	// Returns domain.ErrPresetNotFound when no such preset exists or no version
	// has been published yet.
	GetPublishedPreset(ctx context.Context, tenantSlugOrID string, name string) (*domain.Preset, error)

	// ListPublishedPresets returns the latest published version of every
	// preset for the tenant. Sorted by name for stable test ordering.
	ListPublishedPresets(ctx context.Context, tenantSlugOrID string) ([]domain.Preset, error)
}
