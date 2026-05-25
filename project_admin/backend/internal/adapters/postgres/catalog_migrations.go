package postgres

import (
	"context"
	"fmt"
)

// RunCatalogMigrations ensures catalog schema exists (idempotent, same as main backend).
func (c *Client) RunCatalogMigrations(ctx context.Context) error {
	migrations := []string{
		`CREATE SCHEMA IF NOT EXISTS catalog;`,
		`CREATE TABLE IF NOT EXISTS catalog.tenants (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			slug VARCHAR(100) NOT NULL UNIQUE,
			name VARCHAR(255) NOT NULL,
			type VARCHAR(50) NOT NULL,
			settings JSONB DEFAULT '{}',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		);`,
		`CREATE TABLE IF NOT EXISTS catalog.categories (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name VARCHAR(255) NOT NULL,
			slug VARCHAR(100) NOT NULL,
			parent_id UUID REFERENCES catalog.categories(id),
			created_at TIMESTAMPTZ DEFAULT NOW()
		);`,
		`CREATE TABLE IF NOT EXISTS catalog.master_products (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			sku VARCHAR(100) NOT NULL UNIQUE,
			name VARCHAR(500) NOT NULL,
			description TEXT,
			brand VARCHAR(255),
			category_id UUID REFERENCES catalog.categories(id),
			images JSONB DEFAULT '[]',
			attributes JSONB DEFAULT '{}',
			owner_tenant_id UUID REFERENCES catalog.tenants(id),
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		);`,
		`CREATE TABLE IF NOT EXISTS catalog.products (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id UUID NOT NULL REFERENCES catalog.tenants(id),
			master_product_id UUID REFERENCES catalog.master_products(id),
			name VARCHAR(500),
			description TEXT,
			price INTEGER NOT NULL,
			currency VARCHAR(10) DEFAULT 'RUB',
			stock_quantity INTEGER DEFAULT 0,
			rating NUMERIC(2,1) DEFAULT 0,
			images JSONB DEFAULT '[]',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_tenants_slug ON catalog.tenants(slug);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_products_tenant ON catalog.products(tenant_id);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_products_master ON catalog.products(master_product_id);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_master_products_category ON catalog.master_products(category_id);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_master_products_owner ON catalog.master_products(owner_tenant_id);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_master_products_sku ON catalog.master_products(sku);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_categories_slug ON catalog.categories(slug);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_catalog_categories_slug_unique ON catalog.categories(slug);`,
		`CREATE EXTENSION IF NOT EXISTS vector;`,
		`DO $$ BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = 'catalog' AND table_name = 'master_products' AND column_name = 'embedding'
			) THEN
				ALTER TABLE catalog.master_products ADD COLUMN embedding vector(384);
			END IF;
		END $$;`,
		`CREATE INDEX IF NOT EXISTS idx_master_products_embedding
			ON catalog.master_products USING hnsw (embedding vector_cosine_ops);`,
		`ALTER TABLE catalog.tenants ADD COLUMN IF NOT EXISTS catalog_digest JSONB DEFAULT NULL;`,

		// Stock table
		`CREATE TABLE IF NOT EXISTS catalog.stock (
			tenant_id UUID NOT NULL REFERENCES catalog.tenants(id),
			product_id UUID NOT NULL REFERENCES catalog.products(id) ON DELETE CASCADE,
			quantity INTEGER NOT NULL DEFAULT 0,
			reserved INTEGER NOT NULL DEFAULT 0,
			updated_at TIMESTAMPTZ DEFAULT NOW(),
			PRIMARY KEY (tenant_id, product_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_stock_tenant ON catalog.stock(tenant_id);`,

		// Seed stock from existing products
		`INSERT INTO catalog.stock (tenant_id, product_id, quantity)
		SELECT tenant_id, id, stock_quantity FROM catalog.products
		WHERE stock_quantity > 0
		ON CONFLICT DO NOTHING;`,

		// Tags
		`ALTER TABLE catalog.products ADD COLUMN IF NOT EXISTS tags JSONB DEFAULT '[]';`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_products_tags ON catalog.products USING gin(tags);`,

		// PIM Redesign: structured columns on master_products
		`ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS short_name VARCHAR(200);`,
		`ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS original_name VARCHAR(500);`,
		`ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS product_line VARCHAR(200);`,
		`ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS product_form VARCHAR(30);`,
		`ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS texture VARCHAR(30);`,
		`ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS routine_step VARCHAR(30);`,
		`ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS routine_time VARCHAR(10);`,
		`ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS application_method VARCHAR(30);`,
		`ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS skin_type TEXT[];`,
		`ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS concern TEXT[];`,
		`ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS key_ingredients TEXT[];`,
		`ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS target_area TEXT[];`,
		`ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS free_from TEXT[];`,
		`ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS marketing_claim VARCHAR(300);`,
		`ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS benefits TEXT[];`,
		`ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS how_to_use TEXT;`,
		`ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS volume VARCHAR(50);`,
		`ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS inci_text TEXT;`,
		`ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS enrichment_version SMALLINT DEFAULT 0;`,

		// PIM Redesign: ingredients reference table
		`CREATE TABLE IF NOT EXISTS catalog.ingredients (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			inci_name VARCHAR(500) NOT NULL,
			name_ru VARCHAR(500),
			slug VARCHAR(200) NOT NULL,
			function VARCHAR(100),
			created_at TIMESTAMPTZ DEFAULT NOW()
		);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_catalog_ingredients_inci_name
			ON catalog.ingredients (LOWER(inci_name));`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_catalog_ingredients_slug
			ON catalog.ingredients (slug);`,
		`CREATE TABLE IF NOT EXISTS catalog.product_ingredients (
			master_product_id UUID NOT NULL REFERENCES catalog.master_products(id) ON DELETE CASCADE,
			ingredient_id UUID NOT NULL REFERENCES catalog.ingredients(id) ON DELETE CASCADE,
			position SMALLINT NOT NULL DEFAULT 0,
			is_key BOOLEAN NOT NULL DEFAULT false,
			PRIMARY KEY (master_product_id, ingredient_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_product_ingredients_ingredient
			ON catalog.product_ingredients (ingredient_id);`,

		// PIM Redesign: indexes for typed columns
		`CREATE INDEX IF NOT EXISTS idx_catalog_mp_product_form ON catalog.master_products (product_form);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_mp_routine_step ON catalog.master_products (routine_step);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_mp_brand ON catalog.master_products (brand);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_mp_short_name ON catalog.master_products (short_name);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_mp_cat_form ON catalog.master_products (category_id, product_form);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_mp_skin_type ON catalog.master_products USING gin (skin_type);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_mp_concern ON catalog.master_products USING gin (concern);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_mp_key_ingredients ON catalog.master_products USING gin (key_ingredients);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_mp_target_area ON catalog.master_products USING gin (target_area);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_mp_free_from ON catalog.master_products USING gin (free_from);`,

		// PIM Redesign: structured volume columns
		`ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS volume_ml INT;`,
		`ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS weight_g INT;`,
		`ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS unit_count SMALLINT NOT NULL DEFAULT 1;`,

		// PIM Cleanup: drop legacy columns (data migrated to typed PIM columns)
		`ALTER TABLE catalog.master_products DROP COLUMN IF EXISTS short_name;`,
		`ALTER TABLE catalog.master_products DROP COLUMN IF EXISTS volume;`,
		`ALTER TABLE catalog.master_products DROP COLUMN IF EXISTS attributes;`,
		`ALTER TABLE catalog.master_products DROP COLUMN IF EXISTS inci_text;`,
		`DROP INDEX IF EXISTS idx_catalog_mp_short_name;`,

		// Onboarding: source tracking for idempotent re-imports from external
		// systems (Shopify product.id, CSV filename+row, Sheets row). SKU
		// dedup stays the primary key; this is a secondary idempotency path.
		`ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS source_system VARCHAR(32);`,
		`ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS source_id VARCHAR(255);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_master_products_source
			ON catalog.master_products(owner_tenant_id, source_system, source_id)
			WHERE source_system IS NOT NULL AND source_id IS NOT NULL;`,

		// Onboarding: soft-delete for products. Shopify webhook products/delete
		// sets deleted_at; the chat backend search must filter WHERE deleted_at IS NULL.
		`ALTER TABLE catalog.products ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_products_deleted_at
			ON catalog.products(deleted_at) WHERE deleted_at IS NOT NULL;`,

		// =========================================================================
		// Admin Catalog Spec (2026-04-23) — additive new schema
		// Design: docs/New features/admin_catalog_design_2026-04-23.md
		// All idempotent. Old tables stay; reads/writes evolve in subsequent code.
		// =========================================================================

		// --- Master products: additive metadata (vertical, Tier 3 bag, confidence) ---
		`ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS vertical TEXT NOT NULL DEFAULT 'cosmetics';`,
		`ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS tier3 JSONB NOT NULL DEFAULT '{}'::jsonb;`,
		`ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS confidence TEXT NOT NULL DEFAULT 'unverified'
			CHECK (confidence IN ('unverified', 'reviewed', 'approved'));`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_mp_vertical ON catalog.master_products(vertical);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_mp_confidence ON catalog.master_products(confidence);`,

		// --- master_variants: parent-child variant model (Option C) ---
		// One row per SKU. GTIN array — same product can have multiple barcodes
		// (different regions, reissues). axes JSONB stores original axis values
		// from the source ({size:"236ml", color:"Silver"}).
		// Dual-store fields: typed (parsed) + _raw (audit), parse_status JSONB
		// records per-field parser outcome (ok|ambiguous|failed|dual_mismatch).
		`CREATE TABLE IF NOT EXISTS catalog.master_variants (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			master_product_id UUID NOT NULL REFERENCES catalog.master_products(id) ON DELETE CASCADE,
			sku TEXT,
			gtins TEXT[] NOT NULL DEFAULT '{}',
			image_url TEXT,
			weight_g INTEGER,
			volume_ml INTEGER,
			length_mm INTEGER,
			width_mm INTEGER,
			height_mm INTEGER,
			color TEXT,
			size TEXT,
			material TEXT,
			axes JSONB NOT NULL DEFAULT '{}'::jsonb,
			variant_kind TEXT NOT NULL DEFAULT 'real'
				CHECK (variant_kind IN ('real', 'addon', 'bundle')),
			weight_raw TEXT,
			volume_raw TEXT,
			parse_status JSONB NOT NULL DEFAULT '{}'::jsonb,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_mv_master ON catalog.master_variants(master_product_id);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_mv_gtins ON catalog.master_variants USING gin(gtins);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_mv_sku ON catalog.master_variants(sku) WHERE sku IS NOT NULL;`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_mv_kind ON catalog.master_variants(variant_kind);`,

		// --- master_cosmetics: per-vertical Tier 2 typed columns ---
		// Reshaped 2026-05-14 (catalog flow rebuild): PK is now master_product_id
		// (not master_variant_id; master_variants is being phased out), and the
		// column set covers all 17 cosmetic Tier-2 attributes plus an extra
		// JSONB slot for one-offs. apply_v2 writes here for cosmetics tenants;
		// V5 chat JOINs to read.
		`CREATE TABLE IF NOT EXISTS catalog.master_cosmetics (
			master_product_id   UUID PRIMARY KEY REFERENCES catalog.master_products(id) ON DELETE CASCADE,
			skin_type           TEXT[]      NOT NULL DEFAULT '{}',
			concern             TEXT[]      NOT NULL DEFAULT '{}',
			key_ingredients     TEXT[]      NOT NULL DEFAULT '{}',
			target_area         TEXT[]      NOT NULL DEFAULT '{}',
			product_form        TEXT,
			texture             TEXT,
			routine_step        TEXT,
			routine_time        TEXT,
			application_method  TEXT,
			free_from           TEXT[]      NOT NULL DEFAULT '{}',
			scent               TEXT,
			spf                 INTEGER,
			marketing_claim     TEXT,
			benefits            TEXT[]      NOT NULL DEFAULT '{}',
			how_to_use          TEXT,
			volume_ml           INTEGER,
			weight_g            INTEGER,
			unit_count          SMALLINT,
			extra               JSONB       NOT NULL DEFAULT '{}'::jsonb,
			updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		`CREATE INDEX IF NOT EXISTS idx_mc_skin_type       ON catalog.master_cosmetics USING GIN (skin_type);`,
		`CREATE INDEX IF NOT EXISTS idx_mc_concern         ON catalog.master_cosmetics USING GIN (concern);`,
		`CREATE INDEX IF NOT EXISTS idx_mc_key_ingredients ON catalog.master_cosmetics USING GIN (key_ingredients);`,
		`CREATE INDEX IF NOT EXISTS idx_mc_free_from       ON catalog.master_cosmetics USING GIN (free_from);`,
		`CREATE INDEX IF NOT EXISTS idx_mc_benefits        ON catalog.master_cosmetics USING GIN (benefits);`,

		// --- catalog.products: listing-as-overrides extensions ---
		// New columns let the listing carry per-tenant deltas while master holds
		// the shared truth. Render = COALESCE(listing.field, master.field).
		`ALTER TABLE catalog.products ADD COLUMN IF NOT EXISTS master_variant_id UUID REFERENCES catalog.master_variants(id);`,
		`ALTER TABLE catalog.products ADD COLUMN IF NOT EXISTS original_name TEXT;`,
		`ALTER TABLE catalog.products ADD COLUMN IF NOT EXISTS display_name TEXT;`,
		`ALTER TABLE catalog.products ADD COLUMN IF NOT EXISTS raw_attributes JSONB NOT NULL DEFAULT '{}'::jsonb;`,
		`ALTER TABLE catalog.products ADD COLUMN IF NOT EXISTS media JSONB NOT NULL DEFAULT '[]'::jsonb;`,
		`ALTER TABLE catalog.products ADD COLUMN IF NOT EXISTS source_system TEXT;`,
		`ALTER TABLE catalog.products ADD COLUMN IF NOT EXISTS source_id TEXT;`,
		`ALTER TABLE catalog.products ADD COLUMN IF NOT EXISTS payload_hash TEXT;`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_products_master_variant ON catalog.products(master_variant_id);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_products_tenant_source
			ON catalog.products(tenant_id, source_system, source_id)
			WHERE source_system IS NOT NULL AND source_id IS NOT NULL;`,

		// --- master_categories: clean global taxonomy curated by Vlad ---
		`CREATE TABLE IF NOT EXISTS catalog.master_categories (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			parent_id UUID REFERENCES catalog.master_categories(id),
			slug TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			vertical TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_mc_parent ON catalog.master_categories(parent_id);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_mc_vertical ON catalog.master_categories(vertical);`,

		// --- master_product_categories: M:N (one master can be in many categories) ---
		`CREATE TABLE IF NOT EXISTS catalog.master_product_categories (
			master_product_id UUID NOT NULL REFERENCES catalog.master_products(id) ON DELETE CASCADE,
			master_category_id UUID NOT NULL REFERENCES catalog.master_categories(id) ON DELETE CASCADE,
			PRIMARY KEY (master_product_id, master_category_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_mpc_category ON catalog.master_product_categories(master_category_id);`,

		// --- tenant_categories: copy of tenant's collection tree (their names) ---
		// kind distinguishes regular categories from showcase ("Best Sellers")
		// or promo ("On Sale 30%") collections that don't map to master taxonomy.
		`CREATE TABLE IF NOT EXISTS catalog.tenant_categories (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id UUID NOT NULL REFERENCES catalog.tenants(id) ON DELETE CASCADE,
			parent_id UUID REFERENCES catalog.tenant_categories(id),
			external_id TEXT,
			slug TEXT NOT NULL,
			name TEXT NOT NULL,
			kind TEXT NOT NULL DEFAULT 'category'
				CHECK (kind IN ('category', 'showcase', 'promo')),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_tc_tenant ON catalog.tenant_categories(tenant_id);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_tc_parent ON catalog.tenant_categories(parent_id);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_catalog_tc_tenant_external
			ON catalog.tenant_categories(tenant_id, external_id)
			WHERE external_id IS NOT NULL;`,

		// --- category_mapping: tenant_category -> master_category (N:1, NULLable) ---
		// NULL = tenant category is showcase/promo, doesn't belong in master tree.
		`CREATE TABLE IF NOT EXISTS catalog.category_mapping (
			tenant_category_id UUID PRIMARY KEY REFERENCES catalog.tenant_categories(id) ON DELETE CASCADE,
			master_category_id UUID REFERENCES catalog.master_categories(id) ON DELETE SET NULL,
			confidence TEXT NOT NULL DEFAULT 'unverified',
			mapped_by TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_cmap_master ON catalog.category_mapping(master_category_id);`,

		// --- tenant_listing_categories: M:N listing -> tenant_categories ---
		`CREATE TABLE IF NOT EXISTS catalog.tenant_listing_categories (
			listing_id UUID NOT NULL REFERENCES catalog.products(id) ON DELETE CASCADE,
			tenant_category_id UUID NOT NULL REFERENCES catalog.tenant_categories(id) ON DELETE CASCADE,
			PRIMARY KEY (listing_id, tenant_category_id)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_tlc_category ON catalog.tenant_listing_categories(tenant_category_id);`,

		// --- master_attribute_candidates: staging for promotion to typed columns ---
		// Tenant-imported fields that aren't in any master schema yet land here.
		// Curator promotes when seen_in_tenants >= 2. Embedding includes candidate
		// values so search works even before promotion.
		`CREATE TABLE IF NOT EXISTS catalog.master_attribute_candidates (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			key TEXT NOT NULL,
			vertical TEXT NOT NULL,
			seen_in_tenants INTEGER NOT NULL DEFAULT 1,
			sample_values JSONB NOT NULL DEFAULT '[]'::jsonb,
			proposed_type TEXT,
			agent_meta TEXT,
			embedding vector(384),
			status TEXT NOT NULL DEFAULT 'pending'
				CHECK (status IN ('pending', 'promoted', 'dismissed', 'merged')),
			merged_into_key TEXT,
			promoted_to_column TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (key, vertical)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_mac_status ON catalog.master_attribute_candidates(status);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_mac_vertical ON catalog.master_attribute_candidates(vertical);`,

		// --- master_category_candidates: staging for new master categories ---
		`CREATE TABLE IF NOT EXISTS catalog.master_category_candidates (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name TEXT NOT NULL,
			proposed_parent TEXT,
			seen_in_tenants INTEGER NOT NULL DEFAULT 1,
			vertical TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending'
				CHECK (status IN ('pending', 'promoted', 'dismissed')),
			promoted_to_id UUID REFERENCES catalog.master_categories(id),
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (name, vertical)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_mcc_status ON catalog.master_category_candidates(status);`,

		// --- tenant_variant_candidates_junk: gift wrap / engraving / addon detection ---
		`CREATE TABLE IF NOT EXISTS catalog.tenant_variant_candidates_junk (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id UUID NOT NULL REFERENCES catalog.tenants(id) ON DELETE CASCADE,
			listing_id UUID NOT NULL REFERENCES catalog.products(id) ON DELETE CASCADE,
			master_variant_id UUID REFERENCES catalog.master_variants(id) ON DELETE SET NULL,
			detected_reason JSONB NOT NULL DEFAULT '{}'::jsonb,
			classification TEXT NOT NULL DEFAULT 'pending'
				CHECK (classification IN ('pending', 'confirmed_addon', 'false_positive')),
			classified_at TIMESTAMPTZ,
			classified_by TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_tvcj_tenant_status
			ON catalog.tenant_variant_candidates_junk(tenant_id, classification);`,

		// --- audit_log: append-only history of who changed what when ---
		// System bulk ops use one row with aggregate_meta (batch_id, count).
		// Human actions use detailed field_changes per entity.
		`CREATE TABLE IF NOT EXISTS catalog.audit_log (
			id BIGSERIAL PRIMARY KEY,
			tenant_id UUID,
			actor_kind TEXT NOT NULL,
			actor_id TEXT,
			entity_kind TEXT NOT NULL,
			entity_id UUID NOT NULL,
			action TEXT NOT NULL,
			field_changes JSONB,
			aggregate_meta JSONB,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_audit_entity
			ON catalog.audit_log(entity_kind, entity_id, created_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_audit_tenant
			ON catalog.audit_log(tenant_id, created_at DESC) WHERE tenant_id IS NOT NULL;`,

		// --- tenant_catalog_schema: per-tenant mapping artifact (1 LLM call result) ---
		// Stores the JSON artifact produced by the agent normalizer at first import.
		// Re-imports apply the artifact via plain code, no LLM. Stale = needs regen.
		`CREATE TABLE IF NOT EXISTS catalog.tenant_catalog_schema (
			tenant_id UUID PRIMARY KEY REFERENCES catalog.tenants(id) ON DELETE CASCADE,
			mapping_artifact JSONB NOT NULL,
			artifact_version INTEGER NOT NULL DEFAULT 1,
			status TEXT NOT NULL DEFAULT 'active'
				CHECK (status IN ('active', 'stale', 'needs_human_review')),
			discovered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			validated_at TIMESTAMPTZ,
			re_discover_after TIMESTAMPTZ
		);`,

		// --- unit_aliases: tenant-aware alias resolution for unit parser ---
		// Unit conversions live in code (internal/units/). DB stores aliases
		// only — "мл" → mL, "MILLILITERS" → mL. tenant_id NULL = global seed.
		`CREATE TABLE IF NOT EXISTS catalog.unit_aliases (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id UUID REFERENCES catalog.tenants(id) ON DELETE CASCADE,
			raw_token TEXT NOT NULL,
			canonical_unit TEXT NOT NULL,
			confidence TEXT NOT NULL DEFAULT 'auto',
			source TEXT,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			UNIQUE (tenant_id, raw_token)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_unit_aliases_global
			ON catalog.unit_aliases(raw_token) WHERE tenant_id IS NULL;`,

		// Seed global alias rows (idempotent via ON CONFLICT). Lowercase raw_token
		// is the lookup key — parser lowercases before lookup.
		// English-only seed (spec §1.14 — MVP no multi-language).
		`INSERT INTO catalog.unit_aliases (tenant_id, raw_token, canonical_unit, source) VALUES
			(NULL, 'ml', 'mL', 'seed'),
			(NULL, 'milliliter', 'mL', 'seed'),
			(NULL, 'milliliters', 'mL', 'seed'),
			(NULL, 'l', 'mL', 'seed'),
			(NULL, 'liter', 'mL', 'seed'),
			(NULL, 'liters', 'mL', 'seed'),
			(NULL, 'fl oz', 'mL', 'seed'),
			(NULL, 'oz', 'mL', 'seed'),
			(NULL, 'g', 'g', 'seed'),
			(NULL, 'gr', 'g', 'seed'),
			(NULL, 'gram', 'g', 'seed'),
			(NULL, 'grams', 'g', 'seed'),
			(NULL, 'kg', 'g', 'seed'),
			(NULL, 'kilogram', 'g', 'seed'),
			(NULL, 'lb', 'g', 'seed'),
			(NULL, 'lbs', 'g', 'seed'),
			(NULL, 'mm', 'mm', 'seed'),
			(NULL, 'cm', 'mm', 'seed'),
			(NULL, 'm', 'mm', 'seed'),
			(NULL, 'inch', 'mm', 'seed'),
			(NULL, 'in', 'mm', 'seed'),
			(NULL, '"', 'mm', 'seed'),
			(NULL, 'pcs', 'pcs', 'seed'),
			(NULL, 'pieces', 'pcs', 'seed')
		ON CONFLICT (tenant_id, raw_token) DO NOTHING;`,

		// --- api_keys: public REST API authentication ---
		// key_hash is bcrypt of plain key; plain shown to user once on creation.
		// label lets tenants distinguish "production" vs "staging" keys.
		`CREATE TABLE IF NOT EXISTS catalog.api_keys (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id UUID NOT NULL REFERENCES catalog.tenants(id) ON DELETE CASCADE,
			key_hash TEXT NOT NULL,
			label TEXT,
			last_used_at TIMESTAMPTZ,
			revoked_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_api_keys_tenant
			ON catalog.api_keys(tenant_id) WHERE revoked_at IS NULL;`,
		// M10: key_prefix lets the API-key middleware narrow by prefix before
		// bcrypt-comparing the full hash. Without this, every request scans
		// every active key and runs bcrypt — fine on MVP, painful at scale.
		`ALTER TABLE catalog.api_keys ADD COLUMN IF NOT EXISTS key_prefix TEXT;`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_api_keys_prefix
			ON catalog.api_keys(key_prefix) WHERE revoked_at IS NULL;`,

		// =========================================================================
		// M4a (2026-04-26) — Foundation for metadata-first Shopify import
		// =========================================================================

		// shopify_raw_imports table was removed 2026-05-17: the catalog v2
		// rebuild replaced its role with catalog.inbox_items. No live writer
		// references it anymore. The CREATE TABLE IF NOT EXISTS block was
		// dropped here to stop recreating the empty table on each startup;
		// the prod row will be DROPped in a coordinated migration with backup.

		// --- master_variants.embedding: per-variant vectors ---
		// Spec §8 decision: parent + per-variant from launch (no A/B). Variant
		// embeddings power "473ml ceramide cleanser"-style queries that the
		// parent text alone can't disambiguate.
		`DO $$ BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = 'catalog' AND table_name = 'master_variants' AND column_name = 'embedding'
			) THEN
				ALTER TABLE catalog.master_variants ADD COLUMN embedding vector(384);
			END IF;
		END $$;`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_mv_embedding
			ON catalog.master_variants USING hnsw (embedding vector_cosine_ops);`,

		// --- tenant_catalog_schema.validation_report: agent artifact validation ---
		// Records coverage %, parser failures, dry-run match rate from the
		// validation pass that follows the discovery agent. needs_human_review
		// status carries this report so curator UI can show specifics.
		`ALTER TABLE catalog.tenant_catalog_schema ADD COLUMN IF NOT EXISTS validation_report JSONB;`,

		// --- unit_aliases cleanup: drop cyrillic seeds ---
		// First admin deploy seeded мл/г/м/шт. The in-code rawTokenFactors map
		// is English-only (spec §1.14 — MVP no multi-language), so these rows
		// never matched anything. Harmless but messy; remove them.
		`DELETE FROM catalog.unit_aliases
			WHERE tenant_id IS NULL AND raw_token IN ('мл', 'г', 'м', 'шт');`,

		// =========================================================================
		// M4b (2026-04-26) — Match cascade fuzzy step needs pg_trgm
		// =========================================================================

		// pg_trgm enables similarity() / % operator for trigram-based fuzzy
		// title matching. Step 3 of the cascade (vendor + similarity(name) > 0.85
		// + axes) relies on this. Extension is widely available on managed
		// Postgres including Neon — no licensing concerns.
		`CREATE EXTENSION IF NOT EXISTS pg_trgm;`,

		// GIN index on master_products.name with trigram ops makes the
		// similarity scan acceptable even at 100k+ rows. Without it the
		// match cascade would table-scan on every variant.
		`CREATE INDEX IF NOT EXISTS idx_catalog_mp_name_trgm
			ON catalog.master_products USING gin (name gin_trgm_ops);`,
	}

	// Curator-driven pivot 2026-04-27, Этап 2.2 — harvester-lite needs
	// upsert-by-source for catalog.products. Existing idx_catalog_products_tenant_source
	// is non-unique; add a unique partial index that matches the harvester's
	// ON CONFLICT target (tenant + source_system + source_id, skipping rows
	// without source — those are manual / CSV imports keyed differently).
	migrations = append(migrations,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_catalog_products_tenant_source_unique
			ON catalog.products (tenant_id, source_system, source_id)
			WHERE source_system IS NOT NULL AND source_id IS NOT NULL;`,
	)

	// Catalog completion 2026-04-28, Phase A4 — UNIQUE(tenant_id, external_id) on
	// tenant_categories. The original M8 schema declared the constraint in the
	// design doc but the actual CREATE TABLE shipped without it; the
	// CategoriesV2Adapter.UpsertTenantCategory uses
	// ON CONFLICT (tenant_id, external_id) which requires this index. Partial
	// index because external_id is NULL for manually-created categories.
	migrations = append(migrations,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_tenant_categories_tenant_external
			ON catalog.tenant_categories (tenant_id, external_id)
			WHERE external_id IS NOT NULL;`,
	)

	// Catalog completion 2026-04-28, Phase A3 — replace ALTER TABLE per-vertical
	// promotion with declarative master_field_definitions + tier2 JSONB.
	//
	// Why: in production, runtime DDL via ALTER TABLE under chat-widget read load
	// risks AccessExclusiveLock contention; per-vertical typed tables also push the
	// schema out of code into the DB-mutation log, making the system harder to
	// reason about for new engineers.
	//
	// Approach: curator "promotes" a candidate by INSERTing into
	// master_field_definitions (vertical, key, label, type, enum_values). Values
	// flow into master_products.tier2 JSONB. Application code reads
	// master_field_definitions to render typed UI / serialize chat fields.
	// Backfill on promote rewrites tier2 from listings.raw_attributes.
	//
	// Naming note: the table is `master_field_definitions`, not `field_definitions`,
	// to avoid collision with V4's per-tenant rendering schema (project_v4 owns
	// `catalog.field_definitions` for chat-engine atom/slot metadata, see
	// project_v4/.../catalog_migrations.go::migrationCatalogFieldDefinitions).
	// Different concept (master-side typed registry vs per-tenant render schema),
	// different shape, different writer; co-existing in the same schema requires
	// distinct names. The two tables are intentionally NOT joined or unified.
	//
	// Backwards-compat: existing master_cosmetics / master_laptops typed tables stay
	// in place — read paths continue using them. New promotions go through tier2.
	// Migration of legacy typed columns into tier2 is a separate, deferred task.
	migrations = append(migrations,
		`CREATE TABLE IF NOT EXISTS catalog.master_field_definitions (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			vertical TEXT NOT NULL,
			key TEXT NOT NULL,
			label TEXT NOT NULL,
			type TEXT NOT NULL CHECK (type IN ('text','number','enum','array','boolean')),
			enum_values TEXT[],
			source TEXT NOT NULL DEFAULT 'curator'
				CHECK (source IN ('seed','curator','migration')),
			promoted_from_candidate UUID REFERENCES catalog.master_attribute_candidates(id),
			promoted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			promoted_by TEXT NOT NULL,
			UNIQUE(vertical, key)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_master_field_definitions_vertical
			ON catalog.master_field_definitions(vertical);`,
		`ALTER TABLE catalog.master_products
			ADD COLUMN IF NOT EXISTS tier2 JSONB NOT NULL DEFAULT '{}'::jsonb;`,
		// Generic GIN over the whole tier2 — useful as long as we don't have
		// thousands of fields per vertical. Per-key functional indexes are added
		// at promotion time inside the curator transaction (see promote_attribute).
		`CREATE INDEX IF NOT EXISTS idx_master_products_tier2_gin
			ON catalog.master_products USING gin (tier2);`,
	)

	// Catalog completion 2026-04-28, Phase D2 — merge_reports table.
	//
	// One row per applier run for a tenant. Proposals are an array stored as
	// JSONB rather than a normalized child table because they're always read
	// together (per-tenant flow), the applier writes them once, and curator
	// mutations are typically bulk approve/reject — joins would buy nothing.
	//
	// agent_meta_summary captures provenance (the agent notes + cost from the
	// MappingArtifact this report was generated against) so the curator UI
	// can show context without re-fetching the schema.
	migrations = append(migrations,
		`CREATE TABLE IF NOT EXISTS catalog.merge_reports (
			id BIGSERIAL PRIMARY KEY,
			tenant_id UUID NOT NULL REFERENCES catalog.tenants(id) ON DELETE CASCADE,
			generated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			schema_version INTEGER NOT NULL DEFAULT 1,
			status TEXT NOT NULL DEFAULT 'pending'
				CHECK (status IN ('pending','reviewed','applied','partial','reverted','superseded')),
			agent_meta_summary TEXT,
			total_listings INTEGER NOT NULL DEFAULT 0,
			auto_link_count INTEGER NOT NULL DEFAULT 0,
			new_master_count INTEGER NOT NULL DEFAULT 0,
			needs_review_count INTEGER NOT NULL DEFAULT 0,
			skip_count INTEGER NOT NULL DEFAULT 0,
			already_linked_count INTEGER NOT NULL DEFAULT 0,
			proposals JSONB NOT NULL DEFAULT '[]'::jsonb,
			applied_at TIMESTAMPTZ,
			applied_by TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_merge_reports_tenant_generated
			ON catalog.merge_reports(tenant_id, generated_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_merge_reports_status
			ON catalog.merge_reports(status) WHERE status IN ('pending','reviewed');`,
		// Already_linked column added after first iteration (the applier was
		// missing the early-return for already-merged listings, which produced a
		// 998-row "create new master" garbage report against heybabes — see
		// docs/Updates/main-catalog-completion-phaseD2_*.md). IF NOT EXISTS keeps
		// the migration idempotent on fresh + existing schemas.
		`ALTER TABLE catalog.merge_reports
			ADD COLUMN IF NOT EXISTS already_linked_count INTEGER NOT NULL DEFAULT 0;`,
	)

	// Auth-dedup 2026-05-13 — soft-delete for catalog.tenants. Set when an
	// email-merged Shopify install cleans up an empty Google-auto-provisioned
	// orphan workspace (auth_magic_link.go:cleanupEmptyOrphanTenants). All
	// tenant-list queries must filter `WHERE deleted_at IS NULL`.
	migrations = append(migrations,
		`ALTER TABLE catalog.tenants ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_tenants_deleted_at
			ON catalog.tenants(deleted_at) WHERE deleted_at IS NOT NULL;`,
	)

	// Auth-dedup 2026-05-13 — extend auth_challenges.kind CHECK to include
	// 'shop_pending_link' (Shopify install without owner email → token to
	// link tenant after user signs in via standard methods). Drop-and-add
	// guarded by a versioned constraint name so re-runs are idempotent.
	migrations = append(migrations,
		`DO $$ BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint WHERE conname = 'auth_challenges_kind_check_v2'
			) THEN
				ALTER TABLE admin.auth_challenges DROP CONSTRAINT IF EXISTS auth_challenges_kind_check;
				ALTER TABLE admin.auth_challenges ADD CONSTRAINT auth_challenges_kind_check_v2
					CHECK (kind IN ('email_verify','password_reset','totp_setup','email_2fa','magic_link','shop_pending_link'));
			END IF;
		END $$;`,
	)

	// pipeline_traces table (idempotent, same as chat backend)
	migrations = append(migrations,
		`CREATE TABLE IF NOT EXISTS pipeline_traces (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			session_id TEXT NOT NULL,
			query TEXT NOT NULL,
			turn_id TEXT,
			timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			trace_data JSONB NOT NULL,
			total_ms INTEGER NOT NULL DEFAULT 0,
			cost_usd DOUBLE PRECISION NOT NULL DEFAULT 0,
			error TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_pipeline_traces_timestamp
			ON pipeline_traces(timestamp DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_pipeline_traces_session_id
			ON pipeline_traces(session_id);`,
	)

	// inbox_items — universal raw-tenant-data buffer (6-step catalog flow,
	// see /Users/starknight/.claude/plans/structured-discovering-lollipop.md).
	// Schema-agnostic: any source (Shopify/CSV/Sheets/manual) writes here as
	// JSONB. Discovery agent reads via narrow tools, apply consumes it.
	migrations = append(migrations,
		`CREATE TABLE IF NOT EXISTS catalog.inbox_items (
			id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id    UUID NOT NULL REFERENCES catalog.tenants(id) ON DELETE CASCADE,
			source_kind  TEXT NOT NULL CHECK (source_kind IN ('shopify','csv','sheets','manual')),
			external_id  TEXT NOT NULL,
			raw          JSONB NOT NULL,
			payload_hash TEXT NOT NULL,
			fetched_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			applied_at   TIMESTAMPTZ
		);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_inbox_items_unique_source
			ON catalog.inbox_items(tenant_id, source_kind, external_id);`,
		`CREATE INDEX IF NOT EXISTS idx_inbox_items_raw_gin
			ON catalog.inbox_items USING GIN (raw);`,
		`CREATE INDEX IF NOT EXISTS idx_inbox_items_tenant_unapplied
			ON catalog.inbox_items(tenant_id) WHERE applied_at IS NULL;`,
	)

	// Catalog v2 hardening — master matching + per-row vertical + soft-delete.
	// See: /Users/starknight/.claude/plans/structured-discovering-lollipop.md
	// (Addendum: 2026-05-15 evening).
	//   - vertical_aliases: shopify product_type → class lookup, seeded with
	//     ~45 starter aliases. Curator-editable later.
	//   - master_products.gtin / normalized_match_key: secondary match keys
	//     for MatchOrCreateMaster cascade.
	//   - products.tenant_overrides: reserved JSONB pocket for tenant-specific
	//     marketing payload (tags like "HIT SALE", custom images, video links).
	//     Column added now; first writer wired in a later pass.
	//   - pg_trgm GIN indexes on master_products(brand, name) for future
	//     curator-side fuzzy search. Not used by the apply_v2 match cascade
	//     (which is strict-exact only).
	//   - Backfill of normalized_match_key for any existing master_products
	//     (hey-babes seed) so the new cascade can hit them.
	migrations = append(migrations,
		`CREATE EXTENSION IF NOT EXISTS pg_trgm;`,
		`CREATE TABLE IF NOT EXISTS catalog.vertical_aliases (
			alias       TEXT PRIMARY KEY,
			vertical    TEXT NOT NULL,
			source      TEXT NOT NULL DEFAULT 'seed',
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		`INSERT INTO catalog.vertical_aliases (alias, vertical) VALUES
			('skincare','cosmetics'), ('skin care','cosmetics'), ('cosmetics','cosmetics'),
			('beauty','cosmetics'), ('makeup','cosmetics'), ('fragrance','cosmetics'),
			('perfume','cosmetics'), ('sunscreen','cosmetics'), ('body care','cosmetics'),
			('serum','cosmetics'), ('moisturizer','cosmetics'), ('cleanser','cosmetics'),
			('hair care','haircare'), ('haircare','haircare'), ('shampoo','haircare'),
			('conditioner','haircare'), ('hair styling','haircare'), ('hair dryer','haircare'),
			('laptop','electronics'), ('laptops','electronics'), ('computer','electronics'),
			('pc','electronics'), ('phone','electronics'), ('smartphone','electronics'),
			('tablet','electronics'), ('camera','electronics'), ('headphones','electronics'),
			('tv','electronics'), ('monitor','electronics'), ('keyboard','electronics'),
			('mouse','electronics'),
			('sofa','furniture'), ('chair','furniture'), ('table','furniture'),
			('desk','furniture'), ('bed','furniture'), ('wardrobe','furniture'),
			('bookshelf','furniture'), ('cabinet','furniture'), ('dresser','furniture'),
			('shirt','apparel'), ('t-shirt','apparel'), ('dress','apparel'),
			('pants','apparel'), ('jeans','apparel'), ('jacket','apparel'),
			('coat','apparel'), ('sweater','apparel'),
			('shoes','footwear'), ('boots','footwear'), ('sneakers','footwear'),
			('sandals','footwear'),
			('skis','ski'), ('snowboard','ski'), ('ski boots','ski'),
			('ski goggles','ski'), ('ski poles','ski')
		ON CONFLICT (alias) DO NOTHING;`,
		`ALTER TABLE catalog.master_products
			ADD COLUMN IF NOT EXISTS gtin TEXT,
			ADD COLUMN IF NOT EXISTS normalized_match_key TEXT;`,
		`ALTER TABLE catalog.products
			ADD COLUMN IF NOT EXISTS tenant_overrides JSONB NOT NULL DEFAULT '{}'::jsonb;`,
		`CREATE INDEX IF NOT EXISTS idx_master_products_gtin
			ON catalog.master_products(gtin) WHERE gtin IS NOT NULL;`,
		`CREATE INDEX IF NOT EXISTS idx_master_products_normalized_match_key
			ON catalog.master_products(normalized_match_key) WHERE normalized_match_key IS NOT NULL;`,
		// Functional index on LOWER(sku) for the BulkMatchOrCreateMaster
		// cascade JOIN. Without it, the 3-key match did a full scan of
		// master_products on every batch (dominant cost in apply throughput).
		`CREATE INDEX IF NOT EXISTS idx_master_products_lower_sku
			ON catalog.master_products(LOWER(sku)) WHERE sku IS NOT NULL AND sku <> '';`,
		`CREATE INDEX IF NOT EXISTS idx_master_products_brand_trgm
			ON catalog.master_products USING gin (brand gin_trgm_ops);`,
		`CREATE INDEX IF NOT EXISTS idx_master_products_name_trgm
			ON catalog.master_products USING gin (name gin_trgm_ops);`,
		`CREATE INDEX IF NOT EXISTS idx_catalog_products_active
			ON catalog.products(tenant_id, master_product_id) WHERE deleted_at IS NULL;`,
		`UPDATE catalog.master_products
			SET normalized_match_key = LOWER(REGEXP_REPLACE(
				REGEXP_REPLACE(COALESCE(brand,'') || ' ' || COALESCE(name,''),
					'[^[:alnum:][:space:]]', '', 'g'),
				'\s+', ' ', 'g'))
			WHERE normalized_match_key IS NULL;`,

		// --- pending approval scaffolding (bundled milestone) ---
		// New mastered rows + enrichments to existing mastered rows are
		// staged for 30 days before they become production-visible. Curator
		// (Vlad) approves/rejects via UI; expired rows are swept by a
		// background runner. Existing master_products default to 'approved'
		// so the migration is non-disruptive.
		`CREATE TABLE IF NOT EXISTS catalog.master_pending_changes (
			id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			master_product_id     UUID NOT NULL REFERENCES catalog.master_products(id) ON DELETE CASCADE,
			op_kind               TEXT NOT NULL CHECK (op_kind IN ('enrich_array_union','enrich_scalar_fill')),
			field_name            TEXT NOT NULL,
			pending_value         JSONB NOT NULL,
			source_inbox_item_id  UUID REFERENCES catalog.inbox_items(id) ON DELETE SET NULL,
			tenant_id             UUID NOT NULL REFERENCES catalog.tenants(id) ON DELETE CASCADE,
			status                TEXT NOT NULL DEFAULT 'pending'
			                      CHECK (status IN ('pending','approved','rejected','expired')),
			created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			expires_at            TIMESTAMPTZ NOT NULL DEFAULT NOW() + INTERVAL '30 days',
			decided_at            TIMESTAMPTZ,
			decided_by            TEXT,
			decided_reason        TEXT
		);`,
		`CREATE INDEX IF NOT EXISTS idx_master_pending_changes_master_status
			ON catalog.master_pending_changes(master_product_id, status);`,
		`CREATE INDEX IF NOT EXISTS idx_master_pending_changes_tenant_status
			ON catalog.master_pending_changes(tenant_id, status);`,
		`CREATE INDEX IF NOT EXISTS idx_master_pending_changes_expires
			ON catalog.master_pending_changes(expires_at) WHERE status='pending';`,

		// Whole-master staging flags. Master rows created by first apply for
		// a tenant land here as 'pending_approval' until curator approves.
		// Existing rows keep 'approved' by default (non-disruptive).
		`ALTER TABLE catalog.master_products
			ADD COLUMN IF NOT EXISTS approval_status TEXT NOT NULL DEFAULT 'approved'
				CHECK (approval_status IN ('approved','pending_approval','rejected','expired')),
			ADD COLUMN IF NOT EXISTS pending_approval_expires_at TIMESTAMPTZ,
			ADD COLUMN IF NOT EXISTS approved_at TIMESTAMPTZ,
			ADD COLUMN IF NOT EXISTS approved_by TEXT;`,
		`CREATE INDEX IF NOT EXISTS idx_master_products_pending_approval
			ON catalog.master_products(approval_status, pending_approval_expires_at)
			WHERE approval_status='pending_approval';`,

		// alpha-0.9.5 — schema drift findings. After each apply we publish
		// a classify job to Redis; workers populate this table with the
		// LLM's classification of each unmapped inbox field (typo / alias
		// of existing attribute / genuinely new). Owner triggers
		// patch-discovery from the admin tab to apply a finding.
		`CREATE TABLE IF NOT EXISTS catalog.schema_drift_findings (
			id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id         UUID NOT NULL REFERENCES catalog.tenants(id) ON DELETE CASCADE,
			apply_run_id      TEXT NOT NULL,
			field_name        TEXT NOT NULL,
			type_guess        TEXT,
			stats             JSONB NOT NULL,
			decision          TEXT
				CHECK (decision IS NULL OR decision IN ('typo_of_existing','alias_of_attribute','new')),
			confidence        NUMERIC(3,2),
			suggested_action  JSONB,
			status            TEXT NOT NULL DEFAULT 'pending'
				CHECK (status IN ('pending','classified','applied','dismissed')),
			created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			classified_at     TIMESTAMPTZ,
			decided_at        TIMESTAMPTZ,
			UNIQUE (tenant_id, apply_run_id, field_name)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_schema_drift_findings_tenant_status
			ON catalog.schema_drift_findings(tenant_id, status);`,
	)

	// =========================================================================
	// Group D Phase 1 (2026-05-25) — radical catalog cleanup.
	// Destructive drops; Neon is the dev stand (owner-approved). Runs last so any
	// dependent objects are already present. Idempotent via IF EXISTS.
	// =========================================================================
	migrations = append(migrations,
		// D4 — dead services tables. The service ingest path (import.go) and all
		// readers were removed in the same change; no live code references these.
		`DROP TABLE IF EXISTS catalog.services CASCADE;`,
		`DROP TABLE IF EXISTS catalog.master_services CASCADE;`,
	)

	// D1a/D2 — tier2 becomes the canonical home for typed per-vertical attributes
	// (vertical-agnostic). Seed the cosmetics key registry, backfill the legacy
	// master_cosmetics typed table into master_products.tier2, then drop it.
	migrations = append(migrations,
		// Key registry for the cosmetics vertical (tier2 allowlist).
		`INSERT INTO catalog.master_field_definitions (vertical, key, label, type, source, promoted_by) VALUES
			('cosmetics','skin_type','Skin Type','array','migration','groupd'),
			('cosmetics','concern','Concern','array','migration','groupd'),
			('cosmetics','key_ingredients','Key Ingredients','array','migration','groupd'),
			('cosmetics','target_area','Target Area','array','migration','groupd'),
			('cosmetics','free_from','Free From','array','migration','groupd'),
			('cosmetics','benefits','Benefits','array','migration','groupd'),
			('cosmetics','product_form','Product Form','text','migration','groupd'),
			('cosmetics','texture','Texture','text','migration','groupd'),
			('cosmetics','routine_step','Routine Step','text','migration','groupd'),
			('cosmetics','routine_time','Routine Time','text','migration','groupd'),
			('cosmetics','application_method','Application Method','text','migration','groupd'),
			('cosmetics','scent','Scent','text','migration','groupd'),
			('cosmetics','marketing_claim','Marketing Claim','text','migration','groupd'),
			('cosmetics','how_to_use','How To Use','text','migration','groupd'),
			('cosmetics','spf','SPF','number','migration','groupd'),
			('cosmetics','volume_ml','Volume (ml)','number','migration','groupd'),
			('cosmetics','weight_g','Weight (g)','number','migration','groupd'),
			('cosmetics','unit_count','Unit Count','number','migration','groupd')
		ON CONFLICT (vertical, key) DO NOTHING;`,

		// Backfill master_cosmetics → master_products.tier2. Guarded by table
		// existence so re-runs after the drop are a no-op (idempotent).
		`DO $$ BEGIN
			IF EXISTS (SELECT 1 FROM information_schema.tables
			           WHERE table_schema='catalog' AND table_name='master_cosmetics') THEN
				UPDATE catalog.master_products mp
				SET tier2 = COALESCE(mp.tier2, '{}'::jsonb) || mc.patch,
				    updated_at = NOW()
				FROM (
					SELECT master_product_id,
					       jsonb_strip_nulls(
					           COALESCE(extra, '{}'::jsonb) || jsonb_build_object(
					               'skin_type',          CASE WHEN array_length(skin_type,1)       > 0 THEN to_jsonb(skin_type) END,
					               'concern',            CASE WHEN array_length(concern,1)         > 0 THEN to_jsonb(concern) END,
					               'key_ingredients',    CASE WHEN array_length(key_ingredients,1) > 0 THEN to_jsonb(key_ingredients) END,
					               'target_area',        CASE WHEN array_length(target_area,1)     > 0 THEN to_jsonb(target_area) END,
					               'free_from',          CASE WHEN array_length(free_from,1)       > 0 THEN to_jsonb(free_from) END,
					               'benefits',           CASE WHEN array_length(benefits,1)        > 0 THEN to_jsonb(benefits) END,
					               'product_form',       to_jsonb(NULLIF(product_form,'')),
					               'texture',            to_jsonb(NULLIF(texture,'')),
					               'routine_step',       to_jsonb(NULLIF(routine_step,'')),
					               'routine_time',       to_jsonb(NULLIF(routine_time,'')),
					               'application_method', to_jsonb(NULLIF(application_method,'')),
					               'scent',              to_jsonb(NULLIF(scent,'')),
					               'marketing_claim',    to_jsonb(NULLIF(marketing_claim,'')),
					               'how_to_use',         to_jsonb(NULLIF(how_to_use,'')),
					               'spf',                to_jsonb(spf),
					               'volume_ml',          to_jsonb(volume_ml),
					               'weight_g',           to_jsonb(weight_g),
					               'unit_count',         to_jsonb(unit_count)
					           )
					       ) AS patch
					FROM catalog.master_cosmetics
				) mc
				WHERE mp.id = mc.master_product_id AND mc.patch <> '{}'::jsonb;
			END IF;
		END $$;`,

		`DROP TABLE IF EXISTS catalog.master_cosmetics CASCADE;`,
	)

	// D1a (cont.) — backfill the 17 legacy typed columns ON master_products into
	// tier2 (cosmetic attrs) / tier3 (provenance), then drop them. tier2 already
	// holds master_cosmetics-derived values from the step above, which win on
	// conflict (`legacy || existing_tier2`). Guarded so re-runs after the drop
	// are a no-op.
	migrations = append(migrations,
		`DO $$ BEGIN
			IF EXISTS (SELECT 1 FROM information_schema.columns
			           WHERE table_schema='catalog' AND table_name='master_products' AND column_name='skin_type') THEN
				UPDATE catalog.master_products mp
				SET tier2 = jsonb_strip_nulls(jsonb_build_object(
						'skin_type',          CASE WHEN array_length(skin_type,1)>0       THEN to_jsonb(skin_type) END,
						'concern',            CASE WHEN array_length(concern,1)>0         THEN to_jsonb(concern) END,
						'key_ingredients',    CASE WHEN array_length(key_ingredients,1)>0 THEN to_jsonb(key_ingredients) END,
						'target_area',        CASE WHEN array_length(target_area,1)>0     THEN to_jsonb(target_area) END,
						'free_from',          CASE WHEN array_length(free_from,1)>0       THEN to_jsonb(free_from) END,
						'benefits',           CASE WHEN array_length(benefits,1)>0        THEN to_jsonb(benefits) END,
						'product_form',       to_jsonb(NULLIF(product_form,'')),
						'texture',            to_jsonb(NULLIF(texture,'')),
						'routine_step',       to_jsonb(NULLIF(routine_step,'')),
						'routine_time',       to_jsonb(NULLIF(routine_time,'')),
						'application_method', to_jsonb(NULLIF(application_method,'')),
						'marketing_claim',    to_jsonb(NULLIF(marketing_claim,'')),
						'how_to_use',         to_jsonb(NULLIF(how_to_use,''))
					)) || COALESCE(mp.tier2,'{}'::jsonb),
				    tier3 = COALESCE(mp.tier3,'{}'::jsonb) || jsonb_strip_nulls(jsonb_build_object(
						'original_name', to_jsonb(NULLIF(original_name,'')),
						'product_line',  to_jsonb(NULLIF(product_line,''))
					)),
				    updated_at = NOW()
				WHERE array_length(skin_type,1)>0 OR array_length(concern,1)>0 OR array_length(key_ingredients,1)>0
				   OR array_length(target_area,1)>0 OR array_length(free_from,1)>0 OR array_length(benefits,1)>0
				   OR COALESCE(product_form,'')<>'' OR COALESCE(texture,'')<>'' OR COALESCE(routine_step,'')<>''
				   OR COALESCE(routine_time,'')<>'' OR COALESCE(application_method,'')<>'' OR COALESCE(marketing_claim,'')<>''
				   OR COALESCE(how_to_use,'')<>'' OR COALESCE(original_name,'')<>'' OR COALESCE(product_line,'')<>'';
			END IF;
		END $$;`,
		`ALTER TABLE catalog.master_products
			DROP COLUMN IF EXISTS skin_type, DROP COLUMN IF EXISTS concern, DROP COLUMN IF EXISTS key_ingredients,
			DROP COLUMN IF EXISTS target_area, DROP COLUMN IF EXISTS free_from, DROP COLUMN IF EXISTS benefits,
			DROP COLUMN IF EXISTS marketing_claim, DROP COLUMN IF EXISTS how_to_use, DROP COLUMN IF EXISTS product_form,
			DROP COLUMN IF EXISTS texture, DROP COLUMN IF EXISTS routine_step, DROP COLUMN IF EXISTS routine_time,
			DROP COLUMN IF EXISTS application_method, DROP COLUMN IF EXISTS original_name, DROP COLUMN IF EXISTS product_line,
			DROP COLUMN IF EXISTS enrichment_version;`,
	)

	for i, m := range migrations {
		if _, err := c.pool.Exec(ctx, m); err != nil {
			return fmt.Errorf("catalog migration %d failed: %w", i+1, err)
		}
	}
	return nil
}
