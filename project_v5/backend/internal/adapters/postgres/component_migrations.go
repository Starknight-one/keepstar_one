package postgres

import (
	"context"
	"fmt"
)

// migrationV5ComponentInit creates the V5 component schema. Same
// header/version split as v5_presets — chunk 5 introduces these as the
// FINAL shape for canvas-microservice-authored reusables.
const migrationV5ComponentInit = `
CREATE TABLE IF NOT EXISTS v5_components (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          UUID NOT NULL REFERENCES catalog.tenants(id) ON DELETE CASCADE,
    name               VARCHAR(200) NOT NULL,
    category           VARCHAR(100) NOT NULL DEFAULT 'atom',
    description        TEXT NOT NULL DEFAULT '',
    latest_version_id  UUID,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, name)
);

CREATE INDEX IF NOT EXISTS idx_v5_components_tenant ON v5_components(tenant_id);

CREATE TABLE IF NOT EXISTS v5_component_versions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    component_id    UUID NOT NULL REFERENCES v5_components(id) ON DELETE CASCADE,
    version         INTEGER NOT NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'draft'
                    CHECK (status IN ('draft','published','archived')),
    doc_json        JSONB NOT NULL,
    author_user_id  UUID,
    published_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(component_id, version)
);

CREATE INDEX IF NOT EXISTS idx_v5_component_versions_component
    ON v5_component_versions(component_id);
CREATE INDEX IF NOT EXISTS idx_v5_component_versions_status
    ON v5_component_versions(component_id, status);
`

// RunComponentMigrations applies the V5 component schema. Idempotent.
func (c *Client) RunComponentMigrations(ctx context.Context) error {
	if _, err := c.pool.Exec(ctx, migrationV5ComponentInit); err != nil {
		return fmt.Errorf("v5 component migration: %w", err)
	}
	return nil
}
