package postgres

import (
	"context"
	"fmt"
)

// RunCatalogMigrations executes catalog schema migrations
func (c *Client) RunCatalogMigrations(ctx context.Context) error {
	migrations := []string{
		migrationCatalogSchema,
		migrationCatalogTenants,
		migrationCatalogCategories,
		migrationCatalogMasterProducts,
		migrationCatalogProducts,
		migrationCatalogIndexes,
		migrationCatalogCategorySlugUnique,
		migrationCatalogPgvector,
		migrationCatalogDigest,
		migrationCatalogStock,
		migrationCatalogStockSeed,
		migrationCatalogServices,
		migrationCatalogTags,
		migrationCatalogPIMColumns,
		migrationCatalogIngredients,
		migrationCatalogPIMIndexes,
		migrationCatalogVolumeColumns,
		migrationCatalogDropLegacyColumns,
		migrationCatalogFieldDefinitions,
		migrationCatalogFieldDefinitionsSeed,
		migrationCatalogProductsExtra,
		migrationCatalogTestElectronicsSeed,
	}

	for i, migration := range migrations {
		if _, err := c.pool.Exec(ctx, migration); err != nil {
			return fmt.Errorf("catalog migration %d failed: %w", i+1, err)
		}
	}

	return nil
}

const migrationCatalogSchema = `
CREATE SCHEMA IF NOT EXISTS catalog;
`

const migrationCatalogTenants = `
CREATE TABLE IF NOT EXISTS catalog.tenants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug VARCHAR(100) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL,
    settings JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
`

const migrationCatalogCategories = `
CREATE TABLE IF NOT EXISTS catalog.categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(100) NOT NULL,
    parent_id UUID REFERENCES catalog.categories(id),
    created_at TIMESTAMPTZ DEFAULT NOW()
);
`

const migrationCatalogMasterProducts = `
CREATE TABLE IF NOT EXISTS catalog.master_products (
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
);
`

const migrationCatalogProducts = `
CREATE TABLE IF NOT EXISTS catalog.products (
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
);
`

const migrationCatalogIndexes = `
CREATE INDEX IF NOT EXISTS idx_catalog_tenants_slug ON catalog.tenants(slug);
CREATE INDEX IF NOT EXISTS idx_catalog_products_tenant ON catalog.products(tenant_id);
CREATE INDEX IF NOT EXISTS idx_catalog_products_master ON catalog.products(master_product_id);
CREATE INDEX IF NOT EXISTS idx_catalog_master_products_category ON catalog.master_products(category_id);
CREATE INDEX IF NOT EXISTS idx_catalog_master_products_owner ON catalog.master_products(owner_tenant_id);
CREATE INDEX IF NOT EXISTS idx_catalog_master_products_sku ON catalog.master_products(sku);
CREATE INDEX IF NOT EXISTS idx_catalog_categories_slug ON catalog.categories(slug);
CREATE INDEX IF NOT EXISTS idx_catalog_categories_parent ON catalog.categories(parent_id);
`

const migrationCatalogCategorySlugUnique = `
CREATE UNIQUE INDEX IF NOT EXISTS idx_catalog_categories_slug_unique ON catalog.categories(slug);
`

const migrationCatalogPgvector = `
CREATE EXTENSION IF NOT EXISTS vector;
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'catalog'
        AND table_name = 'master_products'
        AND column_name = 'embedding'
    ) THEN
        ALTER TABLE catalog.master_products ADD COLUMN embedding vector(384);
    END IF;
END $$;
CREATE INDEX IF NOT EXISTS idx_master_products_embedding
    ON catalog.master_products USING hnsw (embedding vector_cosine_ops);
`

const migrationCatalogDigest = `
ALTER TABLE catalog.tenants ADD COLUMN IF NOT EXISTS catalog_digest JSONB DEFAULT NULL;
`

const migrationCatalogStock = `
CREATE TABLE IF NOT EXISTS catalog.stock (
    tenant_id UUID NOT NULL REFERENCES catalog.tenants(id),
    product_id UUID NOT NULL REFERENCES catalog.products(id) ON DELETE CASCADE,
    quantity INTEGER NOT NULL DEFAULT 0,
    reserved INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (tenant_id, product_id)
);
CREATE INDEX IF NOT EXISTS idx_catalog_stock_tenant ON catalog.stock(tenant_id);
`

const migrationCatalogStockSeed = `
INSERT INTO catalog.stock (tenant_id, product_id, quantity)
SELECT tenant_id, id, stock_quantity FROM catalog.products
WHERE stock_quantity > 0
ON CONFLICT DO NOTHING;
`

const migrationCatalogServices = `
CREATE TABLE IF NOT EXISTS catalog.master_services (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sku VARCHAR(100) NOT NULL UNIQUE,
    name VARCHAR(500) NOT NULL,
    description TEXT,
    brand VARCHAR(255),
    category_id UUID REFERENCES catalog.categories(id),
    images JSONB DEFAULT '[]',
    attributes JSONB DEFAULT '{}',
    duration VARCHAR(100),
    provider VARCHAR(255),
    owner_tenant_id UUID REFERENCES catalog.tenants(id),
    embedding vector(384),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS catalog.services (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES catalog.tenants(id),
    master_service_id UUID REFERENCES catalog.master_services(id),
    name VARCHAR(500),
    description TEXT,
    price INTEGER NOT NULL DEFAULT 0,
    currency VARCHAR(10) DEFAULT 'RUB',
    rating NUMERIC(2,1) DEFAULT 0,
    images JSONB DEFAULT '[]',
    availability VARCHAR(50) DEFAULT 'available',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_catalog_services_tenant ON catalog.services(tenant_id);
CREATE INDEX IF NOT EXISTS idx_catalog_services_master ON catalog.services(master_service_id);
CREATE INDEX IF NOT EXISTS idx_catalog_master_services_category ON catalog.master_services(category_id);
CREATE INDEX IF NOT EXISTS idx_catalog_master_services_sku ON catalog.master_services(sku);
CREATE INDEX IF NOT EXISTS idx_catalog_master_services_embedding
    ON catalog.master_services USING hnsw (embedding vector_cosine_ops);
CREATE UNIQUE INDEX IF NOT EXISTS idx_catalog_services_tenant_master
    ON catalog.services(tenant_id, master_service_id);
`

const migrationCatalogTags = `
ALTER TABLE catalog.products ADD COLUMN IF NOT EXISTS tags JSONB DEFAULT '[]';
ALTER TABLE catalog.services ADD COLUMN IF NOT EXISTS tags JSONB DEFAULT '[]';
CREATE INDEX IF NOT EXISTS idx_catalog_products_tags ON catalog.products USING gin(tags);
CREATE INDEX IF NOT EXISTS idx_catalog_services_tags ON catalog.services USING gin(tags);
`

// --- PIM Redesign migrations ---

const migrationCatalogPIMColumns = `
-- Identification
ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS short_name VARCHAR(200);
ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS original_name VARCHAR(500);
ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS product_line VARCHAR(200);

-- Structured enum fields
ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS product_form VARCHAR(30);
ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS texture VARCHAR(30);
ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS routine_step VARCHAR(30);
ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS routine_time VARCHAR(10);
ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS application_method VARCHAR(30);

-- Array fields
ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS skin_type TEXT[];
ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS concern TEXT[];
ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS key_ingredients TEXT[];
ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS target_area TEXT[];
ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS free_from TEXT[];

-- Text fields
ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS marketing_claim VARCHAR(300);
ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS benefits TEXT[];
ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS how_to_use TEXT;
ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS volume VARCHAR(50);
ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS inci_text TEXT;

-- Versioning
ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS enrichment_version SMALLINT DEFAULT 0;
`

const migrationCatalogIngredients = `
CREATE TABLE IF NOT EXISTS catalog.ingredients (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    inci_name VARCHAR(500) NOT NULL,
    name_ru VARCHAR(500),
    slug VARCHAR(200) NOT NULL,
    function VARCHAR(100),
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_catalog_ingredients_inci_name
    ON catalog.ingredients (LOWER(inci_name));
CREATE UNIQUE INDEX IF NOT EXISTS idx_catalog_ingredients_slug
    ON catalog.ingredients (slug);

CREATE TABLE IF NOT EXISTS catalog.product_ingredients (
    master_product_id UUID NOT NULL REFERENCES catalog.master_products(id) ON DELETE CASCADE,
    ingredient_id UUID NOT NULL REFERENCES catalog.ingredients(id) ON DELETE CASCADE,
    position SMALLINT NOT NULL DEFAULT 0,
    is_key BOOLEAN NOT NULL DEFAULT false,
    PRIMARY KEY (master_product_id, ingredient_id)
);
CREATE INDEX IF NOT EXISTS idx_catalog_product_ingredients_ingredient
    ON catalog.product_ingredients (ingredient_id);
`

const migrationCatalogPIMIndexes = `
-- B-tree indexes for enum/scalar lookups
CREATE INDEX IF NOT EXISTS idx_catalog_mp_product_form ON catalog.master_products (product_form);
CREATE INDEX IF NOT EXISTS idx_catalog_mp_routine_step ON catalog.master_products (routine_step);
CREATE INDEX IF NOT EXISTS idx_catalog_mp_brand ON catalog.master_products (brand);
CREATE INDEX IF NOT EXISTS idx_catalog_mp_short_name ON catalog.master_products (short_name);
CREATE INDEX IF NOT EXISTS idx_catalog_mp_cat_form ON catalog.master_products (category_id, product_form);

-- GIN indexes for array lookups
CREATE INDEX IF NOT EXISTS idx_catalog_mp_skin_type ON catalog.master_products USING gin (skin_type);
CREATE INDEX IF NOT EXISTS idx_catalog_mp_concern ON catalog.master_products USING gin (concern);
CREATE INDEX IF NOT EXISTS idx_catalog_mp_key_ingredients ON catalog.master_products USING gin (key_ingredients);
CREATE INDEX IF NOT EXISTS idx_catalog_mp_target_area ON catalog.master_products USING gin (target_area);
CREATE INDEX IF NOT EXISTS idx_catalog_mp_free_from ON catalog.master_products USING gin (free_from);
`

const migrationCatalogVolumeColumns = `
ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS volume_ml INT;
ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS weight_g INT;
ALTER TABLE catalog.master_products ADD COLUMN IF NOT EXISTS unit_count SMALLINT NOT NULL DEFAULT 1;
`

const migrationCatalogDropLegacyColumns = `
ALTER TABLE catalog.master_products DROP COLUMN IF EXISTS short_name;
ALTER TABLE catalog.master_products DROP COLUMN IF EXISTS volume;
ALTER TABLE catalog.master_products DROP COLUMN IF EXISTS attributes;
ALTER TABLE catalog.master_products DROP COLUMN IF EXISTS inci_text;
DROP INDEX IF EXISTS idx_catalog_mp_short_name;
`

// --- Engine V2: field_definitions table ---

const migrationCatalogFieldDefinitions = `
CREATE TABLE IF NOT EXISTS catalog.field_definitions (
    tenant_id UUID NOT NULL REFERENCES catalog.tenants(id),
    field_name VARCHAR(100) NOT NULL,
    entity_type VARCHAR(20) NOT NULL DEFAULT 'product',
    atom_type VARCHAR(20) NOT NULL DEFAULT 'text',
    atom_subtype VARCHAR(20) NOT NULL DEFAULT 'string',
    unit VARCHAR(20),
    label VARCHAR(200) NOT NULL,
    default_display VARCHAR(50) NOT NULL DEFAULT 'body',
    default_slot VARCHAR(50) NOT NULL DEFAULT 'primary',
    priority INT NOT NULL DEFAULT 100,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (tenant_id, entity_type, field_name)
);
CREATE INDEX IF NOT EXISTS idx_field_defs_tenant_type
    ON catalog.field_definitions(tenant_id, entity_type, priority);
`

// Seed all currently hardcoded fields for every existing tenant.
// Uses INSERT ... ON CONFLICT DO NOTHING so it's idempotent.
// Excludes test-* tenants — those have their own tailored field definitions
// (e.g. migrationCatalogTestElectronicsSeed) and should not inherit the
// hey-babes-shaped catalog.
const migrationCatalogFieldDefinitionsSeed = `
INSERT INTO catalog.field_definitions (tenant_id, field_name, entity_type, atom_type, atom_subtype, unit, label, default_display, default_slot, priority)
SELECT t.id, v.field_name, v.entity_type, v.atom_type, v.atom_subtype, v.unit, v.label, v.default_display, v.default_slot, v.priority
FROM catalog.tenants t
CROSS JOIN (VALUES
    -- Product fields
    ('images',         'product', 'image',  'url',      NULL,  'Images',         'image-cover',    'hero',      0),
    ('name',           'product', 'text',   'string',   NULL,  'Name',           'h2',             'title',     1),
    ('price',          'product', 'number', 'currency', NULL,  'Price',          'price',          'price',     2),
    ('rating',         'product', 'number', 'rating',   NULL,  'Rating',         'rating-compact', 'primary',   3),
    ('brand',          'product', 'text',   'string',   NULL,  'Brand',          'tag',            'primary',   4),
    ('category',       'product', 'text',   'string',   NULL,  'Category',       'tag',            'primary',   5),
    ('description',    'product', 'text',   'string',   NULL,  'Description',    'body-sm',        'secondary', 6),
    ('tags',           'product', 'text',   'string',   NULL,  'Tags',           'tag',            'secondary', 7),
    ('stockQuantity',  'product', 'number', 'int',      NULL,  'Stock',          'body-sm',        'secondary', 8),
    ('productForm',    'product', 'text',   'string',   NULL,  'Product Form',   'tag',            'secondary', 9),
    ('skinType',       'product', 'text',   'string',   NULL,  'Skin Type',      'tag',            'secondary', 10),
    ('concern',        'product', 'text',   'string',   NULL,  'Concern',        'tag',            'secondary', 11),
    ('keyIngredients', 'product', 'text',   'string',   NULL,  'Key Ingredients','body-sm',        'secondary', 12),
    -- Service fields
    ('images',         'service', 'image',  'url',      NULL,  'Images',         'image-cover',    'hero',      0),
    ('name',           'service', 'text',   'string',   NULL,  'Name',           'h2',             'title',     1),
    ('price',          'service', 'number', 'currency', NULL,  'Price',          'price',          'price',     2),
    ('rating',         'service', 'number', 'rating',   NULL,  'Rating',         'rating-compact', 'primary',   3),
    ('duration',       'service', 'text',   'string',   NULL,  'Duration',       'body',           'primary',   4),
    ('provider',       'service', 'text',   'string',   NULL,  'Provider',       'body',           'primary',   5),
    ('availability',   'service', 'text',   'string',   NULL,  'Availability',   'body',           'primary',   6),
    ('description',    'service', 'text',   'string',   NULL,  'Description',    'body-sm',        'secondary', 7),
    ('attributes',     'service', 'text',   'string',   NULL,  'Attributes',     'body-sm',        'secondary', 8)
) AS v(field_name, entity_type, atom_type, atom_subtype, unit, label, default_display, default_slot, priority)
WHERE t.slug NOT LIKE 'test-%'
ON CONFLICT DO NOTHING;
`

// --- Metadata-driven binding: tenant-specific field storage ---

// Adds a JSONB column to catalog.products so tenants can store catalog fields
// that don't fit the hey-babes shape (model/manufacturer/cpu/ram for electronics,
// title/author/isbn for books, etc.). ListProducts scans this column into
// domain.Product.Extra and ProductToMap spreads it into the data[i] map for
// Agent2 binding. SampleFieldValues pulls samples from it for the <fields> block.
const migrationCatalogProductsExtra = `
ALTER TABLE catalog.products ADD COLUMN IF NOT EXISTS extra JSONB DEFAULT '{}'::jsonb;
`

// Seeds a synthetic test-electronics tenant for the metadata-driven binding
// PoC. Proves one engine + one preset works across domains without code
// changes: the LLM reads the <fields> block, sees model/manufacturer/cover_image
// instead of name/brand/images, and emits override ops that remap atom
// fieldNames at ApplyOps time.
//
// Idempotency:
//   - tenant inserted via ON CONFLICT (slug) DO NOTHING.
//   - field_definitions protected by composite PK (tenant_id, entity_type, field_name).
//   - products only inserted if the tenant has zero products yet (WHERE NOT EXISTS).
//
// Closes: docs/PRE_LAUNCH_TASKS.md → 4.3 B7 (role-based field resolution, replaced
// by metadata-driven binding). Design doc: docs/New features/METADATA_DRIVEN_BINDING_2026-04-09.md
const migrationCatalogTestElectronicsSeed = `
-- 1. Tenant
INSERT INTO catalog.tenants (slug, name, type)
VALUES ('test-electronics', 'Test Electronics', 'b2c')
ON CONFLICT (slug) DO NOTHING;

-- 2. Field definitions (9 tenant-specific fields — no images/name/brand)
INSERT INTO catalog.field_definitions (tenant_id, field_name, entity_type, atom_type, atom_subtype, unit, label, default_display, default_slot, priority)
SELECT t.id, v.field_name, v.entity_type, v.atom_type, v.atom_subtype, v.unit, v.label, v.default_display, v.default_slot, v.priority
FROM catalog.tenants t
CROSS JOIN (VALUES
    ('cover_image',   'product', 'image',  'url',      NULL,   'Cover Image',   'image-cover',    'hero',      0),
    ('model',         'product', 'text',   'string',   NULL,   'Model',         'h2',             'title',     1),
    ('price',         'product', 'number', 'currency', 'RUB',  'Price',         'price',          'price',     2),
    ('manufacturer',  'product', 'text',   'string',   NULL,   'Manufacturer',  'tag',            'primary',   3),
    ('rating',        'product', 'number', 'rating',   NULL,   'Rating',        'rating-compact', 'secondary', 4),
    ('cpu',           'product', 'text',   'string',   NULL,   'CPU',           'tag',            'primary',   5),
    ('ram',           'product', 'text',   'string',   NULL,   'RAM',           'tag',            'primary',   6),
    ('display_size',  'product', 'text',   'string',   NULL,   'Display',       'tag',            'primary',   7),
    ('battery_life',  'product', 'text',   'string',   NULL,   'Battery',       'body-sm',        'secondary', 8)
) AS v(field_name, entity_type, atom_type, atom_subtype, unit, label, default_display, default_slot, priority)
WHERE t.slug = 'test-electronics'
ON CONFLICT DO NOTHING;

-- 3. Products (8 laptops, tenant-specific fields go into extra JSONB).
--    "Ноутбук" prefix lets keyword search match; empty images + everything
--    non-hey-babes into extra forces the LLM to emit override ops.
INSERT INTO catalog.products (tenant_id, name, description, price, currency, rating, images, extra)
SELECT t.id, v.name, v.description, v.price, 'RUB', v.rating, '[]'::jsonb, v.extra::jsonb
FROM catalog.tenants t
CROSS JOIN (VALUES
    ('Ноутбук Apple MacBook Pro 14',
     'Флагманский ноутбук для профессионалов на чипе Apple Silicon',
     24999000, 4.9,
     '{"model":"MacBook Pro 14","manufacturer":"Apple","cpu":"Apple M3 Pro","ram":"18GB","display_size":"14.2\" Liquid Retina XDR","battery_life":"18 hours","cover_image":"https://cdn.example.com/mbp14-m3.webp"}'),
    ('Ноутбук Apple MacBook Air 13',
     'Тонкий и лёгкий ноутбук для повседневных задач',
     11999000, 4.8,
     '{"model":"MacBook Air 13","manufacturer":"Apple","cpu":"Apple M2","ram":"8GB","display_size":"13.6\" Liquid Retina","battery_life":"18 hours","cover_image":"https://cdn.example.com/mba13-m2.webp"}'),
    ('Ноутбук Lenovo ThinkPad X1 Carbon Gen 11',
     'Бизнес-ноутбук с лёгким углеволоконным корпусом',
     18999000, 4.6,
     '{"model":"ThinkPad X1 Carbon Gen 11","manufacturer":"Lenovo","cpu":"Intel Core i7-1355U","ram":"16GB","display_size":"14\" WUXGA IPS","battery_life":"15 hours","cover_image":"https://cdn.example.com/x1c-gen11.webp"}'),
    ('Ноутбук Dell XPS 15 9530',
     'Производительный ноутбук для работы с графикой и видео',
     21999000, 4.5,
     '{"model":"XPS 15 9530","manufacturer":"Dell","cpu":"Intel Core i7-13700H","ram":"32GB","display_size":"15.6\" OLED 3.5K","battery_life":"13 hours","cover_image":"https://cdn.example.com/xps15-9530.webp"}'),
    ('Ноутбук Asus ROG Strix G16',
     'Игровой ноутбук с RTX 4070 для требовательных игр',
     17999000, 4.4,
     '{"model":"ROG Strix G16","manufacturer":"Asus","cpu":"Intel Core i9-13980HX","ram":"32GB","display_size":"16\" QHD 240Hz","battery_life":"6 hours","cover_image":"https://cdn.example.com/rog-strix-g16.webp"}'),
    ('Ноутбук HP Spectre x360 14',
     'Премиум-ноутбук 2-в-1 с сенсорным OLED-экраном',
     16999000, 4.7,
     '{"model":"Spectre x360 14","manufacturer":"HP","cpu":"Intel Core Ultra 7 155H","ram":"16GB","display_size":"14\" OLED 2.8K touch","battery_life":"17 hours","cover_image":"https://cdn.example.com/spectre-x360-14.webp"}'),
    ('Ноутбук Acer Swift 5',
     'Ультрапортативный ноутбук с антимикробным покрытием',
     9999000, 4.3,
     '{"model":"Swift 5 SF514","manufacturer":"Acer","cpu":"Intel Core i7-1260P","ram":"16GB","display_size":"14\" 2560x1600 IPS","battery_life":"14 hours","cover_image":"https://cdn.example.com/swift-5.webp"}'),
    ('Ноутбук Microsoft Surface Laptop 5',
     'Элегантный ноутбук от Microsoft с корпусом из магния',
     14999000, 4.5,
     '{"model":"Surface Laptop 5","manufacturer":"Microsoft","cpu":"Intel Core i7-1255U","ram":"16GB","display_size":"13.5\" PixelSense touch","battery_life":"18 hours","cover_image":"https://cdn.example.com/surface-laptop-5.webp"}')
) AS v(name, description, price, rating, extra)
WHERE t.slug = 'test-electronics'
  AND NOT EXISTS (
    SELECT 1 FROM catalog.products p2 WHERE p2.tenant_id = t.id
  );
`
