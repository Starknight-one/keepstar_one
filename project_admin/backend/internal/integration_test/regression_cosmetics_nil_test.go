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

// TestBulkUpsertCosmetics_NilArrays locks in the A1 fix: nil slice fields on
// MasterCosmeticsUpsert must not crash the bulk insert. Prior to the fix the
// row was JSON-marshalled with nil slices serialized as `null`, which SQL
// `jsonb_array_elements_text(null)` rejected with SQLSTATE 22023.
//
// After the fix, coerceStringSlice maps nil to []string{} before marshal,
// so SQL sees `[]` and lands an empty Postgres text[] in master_cosmetics.
func TestBulkUpsertCosmetics_NilArrays(t *testing.T) {
	pool := setupDB(t)
	tenantID := seedTenant(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := postgres.NewClient(ctx, pool.Config().ConnString())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(client.Close)
	writer := postgres.NewCatalogV2WriterAdapter(client, logger.New("error"))

	// Three masters: two all-nil, one mixed.
	masterAllNil1 := seedMasterProduct(t, pool, tenantID, "ITEST-COSM-NIL-1")
	masterAllNil2 := seedMasterProduct(t, pool, tenantID, "ITEST-COSM-NIL-2")
	masterMixed := seedMasterProduct(t, pool, tenantID, "ITEST-COSM-MIX-1")

	items := []ports.BulkCosmeticsItem{
		{
			MasterProductID: masterAllNil1,
			Fields: &ports.MasterCosmeticsUpsert{
				SkinType:       nil,
				Concern:        nil,
				KeyIngredients: nil,
				TargetArea:     nil,
				FreeFrom:       nil,
				Benefits:       nil,
			},
		},
		{
			MasterProductID: masterAllNil2,
			Fields:          &ports.MasterCosmeticsUpsert{},
		},
		{
			MasterProductID: masterMixed,
			Fields: &ports.MasterCosmeticsUpsert{
				SkinType:       []string{"oily", "combination"},
				KeyIngredients: []string{"niacinamide", "salicylic acid"},
			},
		},
	}

	written, err := writer.BulkUpsertCosmetics(ctx, items)
	if err != nil {
		t.Fatalf("BulkUpsertCosmetics with nil arrays: %v", err)
	}
	if written != 3 {
		t.Errorf("written = %d, want 3", written)
	}

	cases := []struct {
		mid      string
		wantSkin int
		wantIngr int
		wantConc int
	}{
		{masterAllNil1, 0, 0, 0},
		{masterAllNil2, 0, 0, 0},
		{masterMixed, 2, 2, 0},
	}
	for _, c := range cases {
		var skinLen, ingrLen, concLen int
		err := pool.QueryRow(ctx, `
			SELECT
				COALESCE(array_length(skin_type, 1), 0),
				COALESCE(array_length(key_ingredients, 1), 0),
				COALESCE(array_length(concern, 1), 0)
			FROM catalog.master_cosmetics
			WHERE master_product_id::text = $1
		`, c.mid).Scan(&skinLen, &ingrLen, &concLen)
		if err != nil {
			t.Fatalf("select cosmetics for master %s: %v", c.mid, err)
		}
		if skinLen != c.wantSkin {
			t.Errorf("master %s: skin_type len = %d, want %d", c.mid, skinLen, c.wantSkin)
		}
		if ingrLen != c.wantIngr {
			t.Errorf("master %s: key_ingredients len = %d, want %d", c.mid, ingrLen, c.wantIngr)
		}
		if concLen != c.wantConc {
			t.Errorf("master %s: concern len = %d, want %d", c.mid, concLen, c.wantConc)
		}
	}

	// All array columns should be empty arrays, never NULL.
	var nullCount int
	err = pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM catalog.master_cosmetics
		WHERE master_product_id::text = ANY($1::text[])
		  AND (skin_type IS NULL OR concern IS NULL OR key_ingredients IS NULL
		    OR target_area IS NULL OR free_from IS NULL OR benefits IS NULL)
	`, []string{masterAllNil1, masterAllNil2, masterMixed}).Scan(&nullCount)
	if err != nil {
		t.Fatalf("count null arrays: %v", err)
	}
	if nullCount != 0 {
		t.Errorf("found %d rows with NULL array columns — should all be empty arrays", nullCount)
	}
}
