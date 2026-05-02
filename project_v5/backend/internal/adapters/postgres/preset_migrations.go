package postgres

import (
	"context"
	"fmt"
)

// migrationV5PresetInit creates the V5 preset schema. Two-table split
// (header + immutable versions) so the future v9-canvas microservice can
// produce drafts without touching the read path. The shape is the FINAL
// shape, not a stopgap — admin's KeepstarCanvas tables are flagged for
// deletion (see docs/v5-known-gaps.md).
//
// Status enum mirrors the V4 KeepstarCanvas convention (draft/published/
// archived). "Latest published" lookup is `ORDER BY version DESC WHERE
// status='published' LIMIT 1`, never the latest_version_id pointer (which
// tracks whatever version is currently being edited, draft or published).
const migrationV5PresetInit = `
CREATE TABLE IF NOT EXISTS v5_presets (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          UUID NOT NULL REFERENCES catalog.tenants(id) ON DELETE CASCADE,
    name               VARCHAR(200) NOT NULL,
    category           VARCHAR(100) NOT NULL DEFAULT 'product',
    entity_type        VARCHAR(100) NOT NULL DEFAULT 'product',
    description        TEXT NOT NULL DEFAULT '',
    default_replicate  BOOLEAN NOT NULL DEFAULT TRUE,
    latest_version_id  UUID,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, name)
);

CREATE INDEX IF NOT EXISTS idx_v5_presets_tenant ON v5_presets(tenant_id);

CREATE TABLE IF NOT EXISTS v5_preset_versions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    preset_id       UUID NOT NULL REFERENCES v5_presets(id) ON DELETE CASCADE,
    version         INTEGER NOT NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'draft'
                    CHECK (status IN ('draft','published','archived')),
    doc_json        JSONB NOT NULL,
    author_user_id  UUID,
    published_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(preset_id, version)
);

CREATE INDEX IF NOT EXISTS idx_v5_preset_versions_preset
    ON v5_preset_versions(preset_id);
CREATE INDEX IF NOT EXISTS idx_v5_preset_versions_status
    ON v5_preset_versions(preset_id, status);
`

// RunPresetMigrations applies the V5 preset schema. Idempotent.
func (c *Client) RunPresetMigrations(ctx context.Context) error {
	if _, err := c.pool.Exec(ctx, migrationV5PresetInit); err != nil {
		return fmt.Errorf("v5 preset migration: %w", err)
	}
	return nil
}
