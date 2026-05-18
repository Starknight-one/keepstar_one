//go:build integration

package integration_test

import (
	"context"
	"testing"
	"time"

	"keepstar-admin/internal/adapters/postgres"
	"keepstar-admin/internal/ports"
)

// TestScenario_156_SoftDelete_StampsDeletedAt_OnRealPostgres verifies the
// SQL-level half of sc 156 (the unit test in apply_v2_test.go ассертит only
// the writer call was made; this one verifies the actual UPDATE lands a
// deleted_at value in the row).
func TestScenario_156_SoftDelete_StampsDeletedAt_OnRealPostgres(t *testing.T) {
	pool := setupDB(t)
	tenantID := seedTenant(t, pool)

	// Build a Client wrapper so we can use the existing CatalogV2WriterAdapter
	// without re-implementing the SQL.
	clientCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := pool.Config().ConnString()
	client, err := postgres.NewClient(clientCtx, url)
	if err != nil {
		t.Fatalf("postgres.NewClient: %v", err)
	}
	t.Cleanup(client.Close)
	writer := postgres.NewCatalogV2WriterAdapter(client)

	// Seed a master + listing for this tenant.
	masterID := seedMasterProduct(t, pool, tenantID, "ITEST-SD-1")

	listingID, err := writer.UpsertListing(clientCtx, &ports.ListingUpsert{
		TenantID:        tenantID,
		MasterProductID: masterID,
		Price:           1999,
		Currency:        "USD",
		Stock:           5,
		SourceSystem:    "shopify",
		SourceID:        "gid://shopify/Product/itest-1",
	})
	if err != nil {
		t.Fatalf("UpsertListing: %v", err)
	}
	if listingID == "" {
		t.Fatal("UpsertListing returned empty id")
	}

	// Soft-delete it.
	if err := writer.SoftDeleteListing(clientCtx, tenantID, "shopify", "gid://shopify/Product/itest-1"); err != nil {
		t.Fatalf("SoftDeleteListing: %v", err)
	}

	// Verify deleted_at is now non-null + master_products row is untouched.
	var deletedAt *time.Time
	err = pool.QueryRow(clientCtx, `
		SELECT deleted_at FROM catalog.products
		WHERE tenant_id = $1 AND source_system = $2 AND source_id = $3
	`, tenantID, "shopify", "gid://shopify/Product/itest-1").Scan(&deletedAt)
	if err != nil {
		t.Fatalf("select deleted_at: %v", err)
	}
	if deletedAt == nil {
		t.Error("deleted_at is still NULL after SoftDeleteListing — SQL UPDATE didn't land")
	}

	// Master should not have been touched.
	var masterName string
	err = pool.QueryRow(clientCtx, `
		SELECT name FROM catalog.master_products WHERE id::text = $1
	`, masterID).Scan(&masterName)
	if err != nil {
		t.Fatalf("select master: %v", err)
	}
	if masterName == "" {
		t.Error("master_products row missing after soft-delete — should never touch master")
	}

	// Idempotency: re-call should leave deleted_at unchanged (COALESCE keeps
	// original timestamp).
	firstDeleted := *deletedAt
	if err := writer.SoftDeleteListing(clientCtx, tenantID, "shopify", "gid://shopify/Product/itest-1"); err != nil {
		t.Fatalf("re-SoftDeleteListing: %v", err)
	}
	var deletedAfterSecond *time.Time
	_ = pool.QueryRow(clientCtx, `
		SELECT deleted_at FROM catalog.products
		WHERE tenant_id = $1 AND source_system = $2 AND source_id = $3
	`, tenantID, "shopify", "gid://shopify/Product/itest-1").Scan(&deletedAfterSecond)
	if deletedAfterSecond == nil || !deletedAfterSecond.Equal(firstDeleted) {
		t.Errorf("idempotency broken: first=%v after-second=%v", firstDeleted, deletedAfterSecond)
	}
}
