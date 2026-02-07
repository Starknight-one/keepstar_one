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
