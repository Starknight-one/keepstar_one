package postgres

import (
	"context"
	"fmt"

	"keepstar_v4/internal/ports"
)

// CanvasComponentAdapter reads tenant-scoped published components from the
// `admin.tenant_components` + `admin.tenant_component_versions` tables owned
// by the admin backend (KeepstarCanvas Phase 1).
//
// READ-ONLY from the V4 chat backend's perspective — all writes happen via
// the admin HTTP API. Mirrors CanvasPresetAdapter's slug/UUID tolerance.
type CanvasComponentAdapter struct {
	client *Client
}

// NewCanvasComponentAdapter constructs the adapter.
func NewCanvasComponentAdapter(client *Client) *CanvasComponentAdapter {
	return &CanvasComponentAdapter{client: client}
}

// compile-time interface assertion.
var _ ports.CanvasComponentPort = (*CanvasComponentAdapter)(nil)

// ListPublishedComponents returns every component for the tenant that has at
// least one published version, each paired with its latest published version.
// DISTINCT ON (c.id) + ORDER BY c.id, v.version DESC picks exactly one row
// per component — the highest published version. Components with only draft
// versions are silently excluded.
func (a *CanvasComponentAdapter) ListPublishedComponents(ctx context.Context, tenantIDOrSlug string) ([]ports.CanvasComponentView, error) {
	if tenantIDOrSlug == "" {
		return nil, nil
	}

	query := `
		SELECT DISTINCT ON (c.id)
			c.id, c.tenant_id, c.name, c.category, c.description,
			v.version, v.ops_json
		FROM admin.tenant_components c
		JOIN admin.tenant_component_versions v ON v.component_id = c.id
		JOIN catalog.tenants t ON t.id = c.tenant_id
		WHERE (c.tenant_id::text = $1 OR t.slug = $1)
		  AND v.status = 'published'
		ORDER BY c.id, v.version DESC
	`

	rows, err := a.client.pool.Query(ctx, query, tenantIDOrSlug)
	if err != nil {
		return nil, fmt.Errorf("list published components: %w", err)
	}
	defer rows.Close()

	var out []ports.CanvasComponentView
	for rows.Next() {
		var view ports.CanvasComponentView
		var opsRaw []byte
		if err := rows.Scan(
			&view.ComponentID,
			&view.TenantID,
			&view.Name,
			&view.Category,
			&view.Description,
			&view.Version,
			&opsRaw,
		); err != nil {
			return nil, fmt.Errorf("scan published component: %w", err)
		}
		view.OpsJSON = normalizeOpsRaw(opsRaw)
		out = append(out, view)
	}
	return out, rows.Err()
}
