package postgres

import (
	"context"
	"fmt"
)

// migrationV5ThemeInit creates the V5 design-system theme table. One row per
// tenant holds that tenant's --kw-* token OVERRIDES as JSONB; absence of a row
// means "use the canonical defaults" (domain.DefaultTheme, IsDefault=true).
//
// This is a fresh, standalone v5-owned table — it does NOT touch master_products
// (the 1600-column scar) and is unrelated to the catalog.schema_version gate
// (which only governs ADMIN-owned catalog.* schema). The FK + UNIQUE(tenant_id)
// mirror the v5_presets convention (preset_migrations.go:21): one theme per
// tenant, cascade-deleted with the tenant.
const migrationV5ThemeInit = `
CREATE TABLE IF NOT EXISTS v5_themes (
    tenant_id   UUID PRIMARY KEY REFERENCES catalog.tenants(id) ON DELETE CASCADE,
    tokens      JSONB NOT NULL,
    version     INTEGER NOT NULL DEFAULT 1,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
`

// RunThemeMigrations applies the V5 theme schema. Idempotent
// (CREATE TABLE IF NOT EXISTS) — registered in cmd/server/main.go's boot
// migration slice and fails LOUD (os.Exit(1)) if it errors.
func (c *Client) RunThemeMigrations(ctx context.Context) error {
	if _, err := c.pool.Exec(ctx, migrationV5ThemeInit); err != nil {
		return fmt.Errorf("v5 theme migration: %w", err)
	}
	return nil
}
