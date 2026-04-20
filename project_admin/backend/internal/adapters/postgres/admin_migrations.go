package postgres

import (
	"context"
	"fmt"
)

// RunAdminMigrations creates admin-specific tables.
func (c *Client) RunAdminMigrations(ctx context.Context) error {
	migrations := []string{
		`CREATE SCHEMA IF NOT EXISTS admin;`,
		`CREATE TABLE IF NOT EXISTS admin.admin_users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			email VARCHAR(255) NOT NULL UNIQUE,
			password_hash VARCHAR(255) NOT NULL,
			tenant_id UUID NOT NULL REFERENCES catalog.tenants(id),
			role VARCHAR(50) NOT NULL DEFAULT 'owner',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		);`,
		`CREATE TABLE IF NOT EXISTS admin.import_jobs (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id UUID NOT NULL REFERENCES catalog.tenants(id),
			file_name VARCHAR(500) NOT NULL,
			status VARCHAR(50) NOT NULL DEFAULT 'pending',
			total_items INTEGER NOT NULL DEFAULT 0,
			processed_items INTEGER NOT NULL DEFAULT 0,
			error_count INTEGER NOT NULL DEFAULT 0,
			errors JSONB DEFAULT '[]',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			completed_at TIMESTAMPTZ
		);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_catalog_products_tenant_master
			ON catalog.products(tenant_id, master_product_id);`,

		// ---------- KeepstarCanvas ----------
		// Tenant-scoped preset library. Each preset is owned by one tenant and
		// always points at its latest version (draft or published). Ops live
		// in the versions table, never on the preset row directly.
		`CREATE TABLE IF NOT EXISTS admin.tenant_presets (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id UUID NOT NULL REFERENCES catalog.tenants(id) ON DELETE CASCADE,
			name VARCHAR(200) NOT NULL,
			category VARCHAR(100) NOT NULL DEFAULT 'product',
			default_replicate BOOLEAN NOT NULL DEFAULT TRUE,
			entity_type VARCHAR(100) NOT NULL DEFAULT 'product',
			description TEXT NOT NULL DEFAULT '',
			latest_version_id UUID,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (tenant_id, name)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_presets_tenant
			ON admin.tenant_presets(tenant_id);`,

		// Immutable preset versions. A new edit on a published version MUST
		// fork a new draft row — published rows are never mutated, so old
		// chat sessions stay reproducible.
		`CREATE TABLE IF NOT EXISTS admin.tenant_preset_versions (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			preset_id UUID NOT NULL REFERENCES admin.tenant_presets(id) ON DELETE CASCADE,
			version INTEGER NOT NULL,
			status VARCHAR(20) NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','published','archived')),
			ops_json JSONB NOT NULL DEFAULT '[]'::jsonb,
			author_user_id UUID,
			published_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (preset_id, version)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_preset_versions_preset
			ON admin.tenant_preset_versions(preset_id);`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_preset_versions_status
			ON admin.tenant_preset_versions(preset_id, status);`,

		// Reusable component library — same shape as presets. Components are
		// referenced by presets via $ref in ops, letting one shared fragment
		// (price_pill, rating_stars, ...) appear across many presets.
		`CREATE TABLE IF NOT EXISTS admin.tenant_components (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id UUID NOT NULL REFERENCES catalog.tenants(id) ON DELETE CASCADE,
			name VARCHAR(200) NOT NULL,
			category VARCHAR(100) NOT NULL DEFAULT 'atom',
			description TEXT NOT NULL DEFAULT '',
			latest_version_id UUID,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (tenant_id, name)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_components_tenant
			ON admin.tenant_components(tenant_id);`,
		`CREATE TABLE IF NOT EXISTS admin.tenant_component_versions (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			component_id UUID NOT NULL REFERENCES admin.tenant_components(id) ON DELETE CASCADE,
			version INTEGER NOT NULL,
			status VARCHAR(20) NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','published','archived')),
			ops_json JSONB NOT NULL DEFAULT '[]'::jsonb,
			author_user_id UUID,
			published_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (component_id, version)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_component_versions_component
			ON admin.tenant_component_versions(component_id);`,

		// Design tokens (colors, spacing, radii, ...) grouped by optional
		// theme axes (e.g. axis="season", value="halloween"). Default theme
		// is rows with theme_axis = '' and theme_value = ''.
		`CREATE TABLE IF NOT EXISTS admin.tenant_design_tokens (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id UUID NOT NULL REFERENCES catalog.tenants(id) ON DELETE CASCADE,
			category VARCHAR(100) NOT NULL,
			name VARCHAR(200) NOT NULL,
			value TEXT NOT NULL,
			theme_axis VARCHAR(100) NOT NULL DEFAULT '',
			theme_value VARCHAR(100) NOT NULL DEFAULT '',
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (tenant_id, category, name, theme_axis, theme_value)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_design_tokens_tenant
			ON admin.tenant_design_tokens(tenant_id);`,

		// Per-tenant version counter for Agent2 prompt cache invalidation.
		// Bumped on every preset/component/token publish — the V4 chat
		// backend keys its fieldsPromptCache on this so a publish in admin
		// busts prompt cache for exactly one tenant.
		`CREATE TABLE IF NOT EXISTS admin.tenant_design_context_version (
			tenant_id UUID PRIMARY KEY REFERENCES catalog.tenants(id) ON DELETE CASCADE,
			version BIGINT NOT NULL DEFAULT 1,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,

		// ---------- Onboarding: integrations ----------
		// Per-tenant source connections (Shopify, CSV upload, Google Sheets).
		// credentials_encrypted holds a v1.{nonce}.{ct} AES-256-GCM envelope —
		// see internal/crypto/secretbox. Never NULL for OAuth sources.
		`CREATE TABLE IF NOT EXISTS admin.tenant_integrations (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id UUID NOT NULL REFERENCES catalog.tenants(id) ON DELETE CASCADE,
			kind TEXT NOT NULL CHECK (kind IN ('shopify','csv','google_sheets')),
			status TEXT NOT NULL CHECK (status IN ('connected','syncing','error','disconnected')),
			display_name TEXT,
			external_id TEXT,
			credentials_encrypted TEXT,
			config JSONB NOT NULL DEFAULT '{}'::jsonb,
			last_sync_at TIMESTAMPTZ,
			last_sync_job_id UUID,
			last_error TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (tenant_id, kind, external_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_integrations_tenant
			ON admin.tenant_integrations(tenant_id);`,
		`CREATE INDEX IF NOT EXISTS idx_tenant_integrations_kind_status
			ON admin.tenant_integrations(kind, status) WHERE status = 'connected';`,

		// Short-lived OAuth nonces. state is a HMAC-signed random value issued
		// on Install start; callback verifies and consumes the row. Expired
		// rows are swept by a background ticker.
		`CREATE TABLE IF NOT EXISTS admin.oauth_states (
			state VARCHAR(128) PRIMARY KEY,
			tenant_id UUID NOT NULL REFERENCES catalog.tenants(id) ON DELETE CASCADE,
			kind TEXT NOT NULL,
			shop_domain TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			expires_at TIMESTAMPTZ NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_oauth_states_expires
			ON admin.oauth_states(expires_at);`,
	}

	for i, m := range migrations {
		if _, err := c.pool.Exec(ctx, m); err != nil {
			return fmt.Errorf("admin migration %d failed: %w", i+1, err)
		}
	}
	return nil
}
