package ports

import (
	"context"
	"encoding/json"
)

// CanvasComponentView is the runtime-facing projection of a tenant-owned
// component's latest published version. OpsJSON is carried as raw bytes so
// Phase 7 (component render-time resolution) needs zero port rework.
type CanvasComponentView struct {
	ComponentID string          `json:"componentId"`
	TenantID    string          `json:"tenantId"`
	Name        string          `json:"name"`
	Category    string          `json:"category"`
	Description string          `json:"description"`
	Version     int             `json:"version"`
	OpsJSON     json.RawMessage `json:"opsJson"`
}

// CanvasComponentPort reads tenant-scoped published components from the admin
// storage layer. Callers pass either a tenant UUID or slug — adapters accept
// both, mirroring CanvasPresetPort's tenantIDOrSlug contract.
type CanvasComponentPort interface {
	// ListPublishedComponents returns every component for the tenant that has
	// at least one published version, each paired with its latest published
	// version. Used by Phase 3 to populate the <tenant_design_context> block.
	ListPublishedComponents(ctx context.Context, tenantIDOrSlug string) ([]CanvasComponentView, error)
}
