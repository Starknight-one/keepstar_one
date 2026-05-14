-- =========================================================================
-- Catalog Flow Rebuild — Part 1 (destructive but V5-chat-safe)
-- =========================================================================
-- Date: 2026-05-14
-- Plan: /Users/starknight/.claude/plans/structured-discovering-lollipop.md
--
-- What this does (all in one transaction; ROLLBACK on any error):
--
--   A. master_cosmetics reshape (0 rows lost, structure changes)
--      A1. DROP TABLE master_cosmetics            -- legacy (master_variant_id PK)
--      A2. CREATE TABLE master_cosmetics          -- new shape (master_product_id PK)
--      A3. Backfill from master_products.<cosmetic-cols> for hey-babes
--          (and any other vertical='cosmetics' master rows — currently 987)
--
--   B. DROP unused tables (no V5 chat references; 0 active rows in most)
--      master_services, candidates/junk/merge_reports family, categories M:N,
--      legacy raw imports + per-tenant schema crud
--
--   C. DELETE 11 empty seed tenants from Feb-2026 PoCs
--
-- What this does NOT do (saved for part 2, after V5 chat is switched to
-- reading master_cosmetics):
--   - DROP COLUMN on master_products (V5 still reads mp.skin_type etc.)
--   - DROP TABLE master_variants (V5 SELECT does LEFT JOIN it via products.master_variant_id)
--   - DROP COLUMN catalog.products.master_variant_id
--
-- Reversibility: structural; if we drop the wrong table by accident, we
-- restore from a Neon point-in-time backup. ROLLBACK protects single-tx
-- errors. Vlad's call: prod is a dev-stand, no real users, acceptable risk.
-- =========================================================================

BEGIN;

-- -------------------------------------------------------------------------
-- A1. DROP legacy master_cosmetics (0 rows; legacy master_variant_id PK).
-- -------------------------------------------------------------------------
DROP TABLE IF EXISTS catalog.master_cosmetics CASCADE;

-- -------------------------------------------------------------------------
-- A2. CREATE new master_cosmetics (master_product_id PK, 17 typed columns
--     for cosmetic Tier-2 attributes + small extra JSONB slot).
-- -------------------------------------------------------------------------
CREATE TABLE catalog.master_cosmetics (
    master_product_id   uuid PRIMARY KEY REFERENCES catalog.master_products(id) ON DELETE CASCADE,
    skin_type           text[]      NOT NULL DEFAULT '{}',
    concern             text[]      NOT NULL DEFAULT '{}',
    key_ingredients     text[]      NOT NULL DEFAULT '{}',
    target_area         text[]      NOT NULL DEFAULT '{}',
    product_form        text,
    texture             text,
    routine_step        text,
    routine_time        text,
    application_method  text,
    free_from           text[]      NOT NULL DEFAULT '{}',
    scent               text,
    spf                 integer,
    marketing_claim     text,
    benefits            text[]      NOT NULL DEFAULT '{}',
    how_to_use          text,
    volume_ml           integer,
    weight_g            integer,
    unit_count          smallint,
    extra               jsonb       NOT NULL DEFAULT '{}'::jsonb,
    updated_at          timestamptz NOT NULL DEFAULT now()
);

-- GIN indexes on array-valued columns for chat-side filtering
-- (BuildCatalogDigest reads distinct skin_type / concern values; the GIN
-- keeps that cheap as the table grows).
CREATE INDEX idx_mc_skin_type       ON catalog.master_cosmetics USING GIN (skin_type);
CREATE INDEX idx_mc_concern         ON catalog.master_cosmetics USING GIN (concern);
CREATE INDEX idx_mc_key_ingredients ON catalog.master_cosmetics USING GIN (key_ingredients);
CREATE INDEX idx_mc_free_from       ON catalog.master_cosmetics USING GIN (free_from);
CREATE INDEX idx_mc_benefits        ON catalog.master_cosmetics USING GIN (benefits);

-- -------------------------------------------------------------------------
-- A3. Backfill hey-babes (+ any other cosmetics master) — copy 17 typed
--     columns from master_products into new master_cosmetics rows.
--
-- Source columns currently sit on master_products as a result of the
-- February PIM redesign that pre-dates the spec. After step 1c they get
-- dropped from master_products; until then the columns coexist (data
-- duplicated, but apply_v2 + V5 chat will read master_cosmetics first).
-- -------------------------------------------------------------------------
INSERT INTO catalog.master_cosmetics (
    master_product_id,
    skin_type, concern, key_ingredients, target_area,
    product_form, texture, routine_step, routine_time, application_method,
    free_from, marketing_claim, benefits, how_to_use,
    volume_ml, weight_g, unit_count
)
SELECT
    id,
    COALESCE(skin_type,       '{}'::text[]),
    COALESCE(concern,         '{}'::text[]),
    COALESCE(key_ingredients, '{}'::text[]),
    COALESCE(target_area,     '{}'::text[]),
    product_form, texture, routine_step, routine_time, application_method,
    COALESCE(free_from, '{}'::text[]),
    marketing_claim,
    COALESCE(benefits, '{}'::text[]),
    how_to_use,
    volume_ml, weight_g, unit_count
FROM catalog.master_products
WHERE vertical = 'cosmetics'
ON CONFLICT (master_product_id) DO NOTHING;

-- -------------------------------------------------------------------------
-- B. DROP unused tables (no V5 chat references; rolled-back features).
--    CASCADE absorbs any forgotten FKs (none expected, but cheap belt+suspenders).
-- -------------------------------------------------------------------------

-- B1. Master services — 0 rows, never wired
DROP TABLE IF EXISTS catalog.master_services CASCADE;

-- B2. Candidates / promotion machinery (curator-driven Tier-2 promotion,
--     dropped in plan — promotion becomes a regular dev migration)
DROP TABLE IF EXISTS catalog.master_attribute_candidates CASCADE;
DROP TABLE IF EXISTS catalog.master_category_candidates  CASCADE;
DROP TABLE IF EXISTS catalog.master_field_definitions    CASCADE;

-- B3. M:N categories (replaced by 1:N category_id legacy column on master_products + tenant_categories)
DROP TABLE IF EXISTS catalog.master_product_categories CASCADE;
DROP TABLE IF EXISTS catalog.master_categories         CASCADE;
DROP TABLE IF EXISTS catalog.tenant_listing_categories CASCADE;
DROP TABLE IF EXISTS catalog.category_mapping          CASCADE;

-- B4. Junk + merge-report machinery (curator flow rolled back)
DROP TABLE IF EXISTS catalog.tenant_variant_candidates_junk CASCADE;
DROP TABLE IF EXISTS catalog.merge_reports                  CASCADE;

-- B5. Legacy staging + per-tenant field schemas (inbox_items replaces both)
DROP TABLE IF EXISTS catalog.shopify_raw_imports CASCADE;
DROP TABLE IF EXISTS catalog.field_definitions   CASCADE;

-- B6. Auxiliary normalization tables (no longer needed without unit-parser flow)
DROP TABLE IF EXISTS catalog.unit_aliases        CASCADE;
DROP TABLE IF EXISTS catalog.product_ingredients CASCADE;
DROP TABLE IF EXISTS catalog.ingredients         CASCADE;

-- -------------------------------------------------------------------------
-- C. DELETE 11 empty seed tenants from February PoCs (0 listings, 0 masters,
--    no integrations attached). Cascade removes admin.user_tenants /
--    admin.tenant_integrations rows referencing them — but all checks show
--    they're orphans already.
-- -------------------------------------------------------------------------
DELETE FROM catalog.tenants
WHERE slug IN (
    'nike',
    'sportmaster',
    'techstore',
    'fashionhub',
    'autofix',
    'beautylab',
    'fitzone',
    'homeservice',
    'keepstart',
    'list-1770980182692261000',
    'test-electronics'
)
  AND id NOT IN (
      -- safety: only delete if the tenant truly has no listings / masters / integrations
      SELECT tenant_id FROM catalog.products
      UNION
      SELECT owner_tenant_id FROM catalog.master_products WHERE owner_tenant_id IS NOT NULL
      UNION
      SELECT tenant_id FROM admin.tenant_integrations
  );

-- -------------------------------------------------------------------------
-- Verification block — printed at COMMIT time.
-- -------------------------------------------------------------------------
SELECT 'master_cosmetics rows backfilled' AS check, COUNT(*)::text AS value FROM catalog.master_cosmetics
UNION ALL SELECT 'tenants remaining',                 COUNT(*)::text FROM catalog.tenants
UNION ALL SELECT 'master_products remaining',         COUNT(*)::text FROM catalog.master_products
UNION ALL SELECT 'catalog.products remaining',        COUNT(*)::text FROM catalog.products
UNION ALL SELECT 'inbox_items',                       COUNT(*)::text FROM catalog.inbox_items;

COMMIT;
