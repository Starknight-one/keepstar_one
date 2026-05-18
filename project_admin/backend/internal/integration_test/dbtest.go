//go:build integration

// Package integration_test — integration tests that hit a real Postgres
// instance. Runs with:
//
//   DATABASE_URL=$NEON_URL go test -tags=integration -v ./internal/integration_test/...
//
// Tests reuse the shared admin.* + catalog.* schemas on the prod-as-dev-stand
// Neon database. Isolation is per-test via UUID-based tenant rows that get
// cleaned up in t.Cleanup. We do NOT spin up a parallel schema — the
// migrations infra hardcodes admin./catalog. names, so isolation by random
// tenant_id is the path with least friction.
package integration_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// setupDB returns a live connection pool or skips the test when DATABASE_URL
// is not set (e.g. local dev, plain CI). The skip is loud enough that you
// notice the env var is missing without making the test pretend to pass.
func setupDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set — skipping integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// seedTenant inserts a one-off catalog.tenants row with a random slug + name
// and returns its ID. Caller gets automatic cleanup via t.Cleanup that
// cascades through FK to remove all rows referencing this tenant.
//
// We never reuse a tenant across tests — each gets a fresh UUID-named row
// so parallel test runs don't collide on UNIQUE(slug).
func seedTenant(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	id := uuid.NewString()
	slug := "itest-" + id[:8]
	_, err := pool.Exec(ctx, `
		INSERT INTO catalog.tenants (id, slug, name, created_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
	`, id, slug, "Integration Test "+slug)
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// Cascading order: child rows first, then tenant. FK cascade handles
		// most but we explicitly delete the rows we may have created so
		// integration runs never leave stray data.
		_, _ = pool.Exec(ctx, `DELETE FROM catalog.products WHERE tenant_id = $1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM catalog.master_products WHERE owner_tenant_id = $1`, id)
		_, _ = pool.Exec(ctx, `DELETE FROM catalog.tenants WHERE id = $1`, id)
	})
	return id
}

// seedMasterProduct inserts a minimal catalog.master_products row owned by
// the given tenant and returns its ID. Useful for tests that need an
// existing master to bind a listing against.
func seedMasterProduct(t *testing.T, pool *pgxpool.Pool, tenantID, sku string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO catalog.master_products
			(name, brand, vertical, sku, owner_tenant_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		RETURNING id::text
	`, fmt.Sprintf("Integration Master %s", sku), "ITest Brand", "cosmetics", sku, tenantID).Scan(&id)
	if err != nil {
		t.Fatalf("seed master_product: %v", err)
	}
	return id
}
