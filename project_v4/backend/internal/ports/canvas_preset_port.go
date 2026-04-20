package ports

import (
	"context"
	"encoding/json"
)

// CanvasPresetView is the runtime-facing projection of a tenant-owned
// preset's latest published version. It carries ops as a raw JSON array so
// this port stays engine-agnostic — the engine_v4 layer parses the bytes into
// its own Op slice at cache population time.
//
// Draft versions are never returned by this port — only the latest published
// version is visible to the V4 chat runtime so old chat sessions stay
// reproducible against the exact ops that were published at serve time.
type CanvasPresetView struct {
	PresetID         string          `json:"presetId"`
	TenantID         string          `json:"tenantId"`
	Name             string          `json:"name"`
	Category         string          `json:"category"`
	EntityType       string          `json:"entityType"`
	Description      string          `json:"description"`
	DefaultReplicate bool            `json:"defaultReplicate"`
	Version          int             `json:"version"`
	OpsJSON          json.RawMessage `json:"opsJson"`
}

// CanvasPresetPort reads tenant-scoped published presets from the admin
// storage layer (the same Postgres cluster the admin backend writes to via
// the KeepstarCanvas CRUD endpoints).
//
// Callers pass either a tenant UUID or a tenant slug — adapters are expected
// to accept both, mirroring FieldDefinitionPort's tenantIDOrSlug contract.
//
// All methods are best-effort: on error, missing data, or disabled DB, the
// caller should fall back to the engine_v4 global preset registry so the V4
// chat backend keeps serving even when the tenant preset store is unreachable.
type CanvasPresetPort interface {
	// GetPublishedPreset returns the latest published version of the named
	// preset for the given tenant, or (nil, false, nil) when there is no
	// published row. Draft/archived rows are invisible to this port.
	GetPublishedPreset(ctx context.Context, tenantIDOrSlug, name string) (*CanvasPresetView, bool, error)

	// ListPublishedPresets returns every preset for the tenant that has at
	// least one published version, with each entry pointing at its latest
	// published version. Used by Agent2's per-tenant design-context block
	// (Phase 3) and will back the admin canvas library view in Phase 4.
	ListPublishedPresets(ctx context.Context, tenantIDOrSlug string) ([]CanvasPresetView, error)
}
