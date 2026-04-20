package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"keepstar_v4/internal/ports"
)

// CanvasPresetAdapter reads tenant-scoped published presets from the
// `admin.tenant_presets` + `admin.tenant_preset_versions` tables owned by the
// admin backend (KeepstarCanvas Phase 1).
//
// This adapter is READ-ONLY from the V4 chat backend's perspective — all
// writes happen via the admin HTTP API. The two services share one Postgres
// cluster, so a publish in admin becomes visible to the V4 runtime as soon
// as the TenantPresetLoader's TTL cache expires (60s in Phase 2).
type CanvasPresetAdapter struct {
	client *Client
}

// NewCanvasPresetAdapter constructs the adapter.
func NewCanvasPresetAdapter(client *Client) *CanvasPresetAdapter {
	return &CanvasPresetAdapter{client: client}
}

// compile-time interface assertion — guards against drift.
var _ ports.CanvasPresetPort = (*CanvasPresetAdapter)(nil)

// canvasPresetSelect is the column list shared by Get/List so both return the
// same projection. Joined against `catalog.tenants` so the caller may pass
// either a tenant slug or a tenant UUID, matching FieldDefinitionAdapter's
// tenantIDOrSlug contract.
const canvasPresetSelect = `
	SELECT
		p.id,
		p.tenant_id,
		p.name,
		p.category,
		p.entity_type,
		p.description,
		p.default_replicate,
		v.version,
		v.ops_json
	FROM admin.tenant_presets p
	JOIN admin.tenant_preset_versions v ON v.preset_id = p.id
	JOIN catalog.tenants t ON t.id = p.tenant_id
`

// GetPublishedPreset returns the latest published version of a tenant's
// preset by name. Returns (nil, false, nil) if the preset does not exist, has
// no published version, or belongs to another tenant — all three map to the
// same "fall back to global registry" behaviour at the loader layer.
func (a *CanvasPresetAdapter) GetPublishedPreset(ctx context.Context, tenantIDOrSlug, name string) (*ports.CanvasPresetView, bool, error) {
	if tenantIDOrSlug == "" || name == "" {
		return nil, false, nil
	}

	query := canvasPresetSelect + `
		WHERE (p.tenant_id::text = $1 OR t.slug = $1)
		  AND p.name = $2
		  AND v.status = 'published'
		ORDER BY v.version DESC
		LIMIT 1
	`

	view := &ports.CanvasPresetView{}
	var opsRaw []byte
	err := a.client.pool.QueryRow(ctx, query, tenantIDOrSlug, name).Scan(
		&view.PresetID,
		&view.TenantID,
		&view.Name,
		&view.Category,
		&view.EntityType,
		&view.Description,
		&view.DefaultReplicate,
		&view.Version,
		&opsRaw,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("get published preset: %w", err)
	}
	view.OpsJSON = normalizeOpsRaw(opsRaw)
	return view, true, nil
}

// ListPublishedPresets returns every preset in the tenant's library that has
// at least one published version, each paired with its latest published
// version (never draft). Used by Phase 3 to assemble the
// <tenant_design_context> block in Agent2's system prompt.
//
// The DISTINCT ON (p.id) combined with ORDER BY p.id, v.version DESC picks
// exactly one row per preset — the highest published version. Presets with
// only draft versions are silently excluded.
func (a *CanvasPresetAdapter) ListPublishedPresets(ctx context.Context, tenantIDOrSlug string) ([]ports.CanvasPresetView, error) {
	if tenantIDOrSlug == "" {
		return nil, nil
	}

	query := `
		SELECT DISTINCT ON (p.id)
			p.id,
			p.tenant_id,
			p.name,
			p.category,
			p.entity_type,
			p.description,
			p.default_replicate,
			v.version,
			v.ops_json
		FROM admin.tenant_presets p
		JOIN admin.tenant_preset_versions v ON v.preset_id = p.id
		JOIN catalog.tenants t ON t.id = p.tenant_id
		WHERE (p.tenant_id::text = $1 OR t.slug = $1)
		  AND v.status = 'published'
		ORDER BY p.id, v.version DESC
	`

	rows, err := a.client.pool.Query(ctx, query, tenantIDOrSlug)
	if err != nil {
		return nil, fmt.Errorf("list published presets: %w", err)
	}
	defer rows.Close()

	var out []ports.CanvasPresetView
	for rows.Next() {
		var view ports.CanvasPresetView
		var opsRaw []byte
		if err := rows.Scan(
			&view.PresetID,
			&view.TenantID,
			&view.Name,
			&view.Category,
			&view.EntityType,
			&view.Description,
			&view.DefaultReplicate,
			&view.Version,
			&opsRaw,
		); err != nil {
			return nil, fmt.Errorf("scan published preset: %w", err)
		}
		view.OpsJSON = normalizeOpsRaw(opsRaw)
		out = append(out, view)
	}
	return out, rows.Err()
}

// normalizeOpsRaw returns a safe json.RawMessage — an empty DB result becomes
// `[]` so downstream parsers never hit "unexpected end of JSON input".
func normalizeOpsRaw(b []byte) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage("[]")
	}
	return json.RawMessage(b)
}
