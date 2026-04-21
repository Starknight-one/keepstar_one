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

	if err := c.runAdminAuthV2Migrations(ctx); err != nil {
		return fmt.Errorf("admin auth v2 migrations: %w", err)
	}

	return nil
}

// runAdminAuthV2Migrations extends the auth schema with OAuth + 2FA columns
// on admin_users and adds the four tables that hold the new auth surface:
// user_tenants (many-to-many membership), sessions (refresh tokens),
// auth_challenges (email/reset/2FA codes), invitations. Safe to re-run —
// every statement is IF NOT EXISTS or ADD COLUMN IF NOT EXISTS.
func (c *Client) runAdminAuthV2Migrations(ctx context.Context) error {
	migrations := []string{
		`CREATE EXTENSION IF NOT EXISTS citext;`,

		// admin_users: OAuth + 2FA columns, relax NOT NULL on password/tenant
		`ALTER TABLE admin.admin_users
			ADD COLUMN IF NOT EXISTS email_verified_at    TIMESTAMPTZ,
			ADD COLUMN IF NOT EXISTS google_sub           TEXT,
			ADD COLUMN IF NOT EXISTS telegram_id          BIGINT,
			ADD COLUMN IF NOT EXISTS telegram_handle      TEXT,
			ADD COLUMN IF NOT EXISTS avatar_url           TEXT,
			ADD COLUMN IF NOT EXISTS last_login_at        TIMESTAMPTZ,
			ADD COLUMN IF NOT EXISTS totp_secret_encrypted TEXT,
			ADD COLUMN IF NOT EXISTS totp_enabled_at      TIMESTAMPTZ;`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_admin_users_google_sub
			ON admin.admin_users(google_sub) WHERE google_sub IS NOT NULL;`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_admin_users_telegram_id
			ON admin.admin_users(telegram_id) WHERE telegram_id IS NOT NULL;`,
		`ALTER TABLE admin.admin_users ALTER COLUMN password_hash DROP NOT NULL;`,
		`ALTER TABLE admin.admin_users ALTER COLUMN tenant_id     DROP NOT NULL;`,

		// many-to-many workspace membership
		`CREATE TABLE IF NOT EXISTS admin.user_tenants (
			user_id     UUID NOT NULL REFERENCES admin.admin_users(id) ON DELETE CASCADE,
			tenant_id   UUID NOT NULL REFERENCES catalog.tenants(id)   ON DELETE CASCADE,
			role        TEXT NOT NULL CHECK (role IN ('owner','admin','member','viewer')),
			invited_by  UUID REFERENCES admin.admin_users(id),
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (user_id, tenant_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_user_tenants_tenant
			ON admin.user_tenants(tenant_id);`,

		// Backfill: every existing admin_users row gets a membership row.
		// Role maps 1:1 from the legacy column; unknown roles collapse to
		// 'member' so the CHECK constraint doesn't reject the insert.
		`INSERT INTO admin.user_tenants (user_id, tenant_id, role)
			SELECT u.id, u.tenant_id,
				CASE WHEN u.role IN ('owner','admin','member','viewer') THEN u.role ELSE 'member' END
			FROM admin.admin_users u
			WHERE u.tenant_id IS NOT NULL
			ON CONFLICT (user_id, tenant_id) DO NOTHING;`,

		// refresh token store — hashes only, never the plaintext token
		`CREATE TABLE IF NOT EXISTS admin.sessions (
			id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id             UUID NOT NULL REFERENCES admin.admin_users(id) ON DELETE CASCADE,
			tenant_id           UUID REFERENCES catalog.tenants(id),
			refresh_token_hash  TEXT NOT NULL UNIQUE,
			user_agent          TEXT,
			ip                  INET,
			created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			expires_at          TIMESTAMPTZ NOT NULL,
			revoked_at          TIMESTAMPTZ
		);`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user_active
			ON admin.sessions(user_id) WHERE revoked_at IS NULL;`,

		// codes / tokens for email_verify, password_reset, totp_setup, email_2fa, magic_link
		`CREATE TABLE IF NOT EXISTS admin.auth_challenges (
			id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id      UUID REFERENCES admin.admin_users(id) ON DELETE CASCADE,
			email        CITEXT,
			kind         TEXT NOT NULL CHECK (kind IN ('email_verify','password_reset','totp_setup','email_2fa','magic_link')),
			code_hash    TEXT NOT NULL,
			expires_at   TIMESTAMPTZ NOT NULL,
			consumed_at  TIMESTAMPTZ,
			meta_json    JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		`CREATE INDEX IF NOT EXISTS idx_auth_challenges_lookup
			ON admin.auth_challenges(kind, code_hash) WHERE consumed_at IS NULL;`,
		`CREATE INDEX IF NOT EXISTS idx_auth_challenges_expires
			ON admin.auth_challenges(expires_at);`,

		// workspace invitations
		`CREATE TABLE IF NOT EXISTS admin.invitations (
			id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id   UUID NOT NULL REFERENCES catalog.tenants(id) ON DELETE CASCADE,
			email       CITEXT NOT NULL,
			role        TEXT NOT NULL CHECK (role IN ('owner','admin','member','viewer')),
			token_hash  TEXT NOT NULL UNIQUE,
			inviter_id  UUID REFERENCES admin.admin_users(id),
			expires_at  TIMESTAMPTZ NOT NULL,
			accepted_at TIMESTAMPTZ,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		`CREATE INDEX IF NOT EXISTS idx_invitations_email
			ON admin.invitations(email) WHERE accepted_at IS NULL;`,
		`CREATE INDEX IF NOT EXISTS idx_invitations_tenant
			ON admin.invitations(tenant_id);`,

		// Google OAuth at login has no tenant context yet; relax the constraint.
		`ALTER TABLE admin.oauth_states ALTER COLUMN tenant_id DROP NOT NULL;`,

		// ---------- Billing ----------
		// One row per tenant. token_multiplier lets us vary how much the
		// displayed token count is marked up over raw API tokens (default
		// 5× — we sell chat at ~5× the underlying Anthropic spend). alert /
		// overdraft columns back the two preference cards on the billing
		// page. cycle_start / cycle_end gate the current-period aggregation.
		`CREATE TABLE IF NOT EXISTS admin.billing_subscriptions (
			tenant_id         UUID PRIMARY KEY REFERENCES catalog.tenants(id) ON DELETE CASCADE,
			plan              TEXT NOT NULL DEFAULT 'growth',
			status            TEXT NOT NULL DEFAULT 'active',
			cycle_start       DATE NOT NULL DEFAULT date_trunc('month', now())::date,
			cycle_end         DATE NOT NULL DEFAULT (date_trunc('month', now()) + interval '1 month')::date,
			token_multiplier  NUMERIC(5,2) NOT NULL DEFAULT 5.00,
			alert_threshold   INT  NOT NULL DEFAULT 80,
			alert_slack       BOOL NOT NULL DEFAULT false,
			overdraft_enabled BOOL NOT NULL DEFAULT false,
			overdraft_notify  BOOL NOT NULL DEFAULT true,
			updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
		);`,

		// tokens_used is the displayed number (already multiplier-adjusted).
		// amount_usd is nullable because an "upcoming" invoice has no final
		// total yet. pdf_url is populated once payment succeeds; it's just
		// a string — the actual PDF generation isn't wired.
		`CREATE TABLE IF NOT EXISTS admin.billing_invoices (
			id            TEXT PRIMARY KEY,
			tenant_id     UUID NOT NULL REFERENCES catalog.tenants(id) ON DELETE CASCADE,
			period_start  DATE NOT NULL,
			period_end    DATE NOT NULL,
			tokens_used   BIGINT NOT NULL DEFAULT 0,
			amount_usd    NUMERIC(10,2),
			status        TEXT NOT NULL DEFAULT 'upcoming',
			pdf_url       TEXT,
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		`CREATE INDEX IF NOT EXISTS idx_billing_invoices_tenant
			ON admin.billing_invoices(tenant_id, period_start DESC);`,
	}

	for i, m := range migrations {
		if _, err := c.pool.Exec(ctx, m); err != nil {
			return fmt.Errorf("auth v2 migration %d failed: %w", i+1, err)
		}
	}
	return nil
}
