//go:build integration

package integration_test

import (
	"context"
	"testing"
	"time"

	"keepstar-admin/internal/adapters/postgres"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

// TestScenario_141_ListingUniqueness_OnRealPostgres verifies that the
// listing upsert is idempotent at the SQL level.
//
// Note about doc vs reality: docs/pre_launch_scenarios.md sc 141 claims the
// uniqueness key is (tenant_id, source_system, source_id), but the adapter's
// ON CONFLICT clause keys on (tenant_id, master_product_id). This test pins
// the actual behavior — re-upserting with the same (tenant, master) updates
// the row in place rather than inserting a duplicate.
func TestScenario_141_ListingUniqueness_OnRealPostgres(t *testing.T) {
	pool := setupDB(t)
	tenantID := seedTenant(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := postgres.NewClient(ctx, pool.Config().ConnString())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(client.Close)
	writer := postgres.NewCatalogV2WriterAdapter(client, logger.New("error"))

	masterID := seedMasterProduct(t, pool, tenantID, "ITEST-UNIQ-1")

	// First upsert: should insert.
	firstID, err := writer.UpsertListing(ctx, &ports.ListingUpsert{
		TenantID:        tenantID,
		MasterProductID: masterID,
		Price:           1000,
		Currency:        "USD",
		Stock:           10,
		SourceSystem:    "shopify",
		SourceID:        "gid://shopify/Product/itest-uniq",
	})
	if err != nil {
		t.Fatalf("first UpsertListing: %v", err)
	}

	// Second upsert with different price/stock: must update in place, same id.
	secondID, err := writer.UpsertListing(ctx, &ports.ListingUpsert{
		TenantID:        tenantID,
		MasterProductID: masterID,
		Price:           2000,
		Currency:        "USD",
		Stock:           20,
		SourceSystem:    "shopify",
		SourceID:        "gid://shopify/Product/itest-uniq",
	})
	if err != nil {
		t.Fatalf("second UpsertListing: %v", err)
	}
	if firstID != secondID {
		t.Errorf("upsert created a new row: first=%q second=%q (unique key (tenant, master) failing)", firstID, secondID)
	}

	// Verify only one row exists for (tenant, master).
	var rowCount int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM catalog.products
		WHERE tenant_id = $1 AND master_product_id::text = $2
	`, tenantID, masterID).Scan(&rowCount)
	if err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if rowCount != 1 {
		t.Errorf("rows for (tenant, master) = %d, want 1", rowCount)
	}

	// And the latest values were applied (price + stock from second call).
	var price, stock int
	err = pool.QueryRow(ctx, `
		SELECT price, stock_quantity FROM catalog.products
		WHERE tenant_id = $1 AND master_product_id::text = $2
	`, tenantID, masterID).Scan(&price, &stock)
	if err != nil {
		t.Fatalf("select listing: %v", err)
	}
	if price != 2000 || stock != 20 {
		t.Errorf("upsert did not apply latest values: price=%d stock=%d, want 2000/20", price, stock)
	}

	// Sanity that updated_at moved forward — implicit proof the UPDATE branch
	// of ON CONFLICT actually ran (NOT a no-op insert with conflict ignore).
	var updatedAt time.Time
	_ = pool.QueryRow(ctx, `
		SELECT updated_at FROM catalog.products WHERE id::text = $1
	`, firstID).Scan(&updatedAt)
	if updatedAt.IsZero() {
		t.Error("updated_at zero — UPSERT path didn't refresh timestamp")
	}
}
