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
	}

	for i, m := range migrations {
		if _, err := c.pool.Exec(ctx, m); err != nil {
			return fmt.Errorf("admin migration %d failed: %w", i+1, err)
		}
	}
	return nil
}
