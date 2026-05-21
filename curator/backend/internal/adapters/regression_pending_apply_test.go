//go:build integration

package adapters_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"keepstar-curator/internal/adapters"
)

// TestApplyPendingChanges_RoutesByFieldName locks in A3 from alpha-0.9.4:
// approve now actually writes the proposed values into master_products /
// master_cosmetics / tier3 instead of just flipping the status flag.
//
// Coverage:
//   - master.<col>      → master_products text column (fill-when-empty)
//   - cosmetics scalar  → master_cosmetics text via INSERT … ON CONFLICT
//   - cosmetics array   → master_cosmetics text[] via DISTINCT union
//   - tier3.<key>       → master_products.tier3 jsonb merge
//   - unknown field     → counted as skipped, no error
func TestApplyPendingChanges_RoutesByFieldName(t *testing.T) {
	pool := setupCuratorDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	tenantID := seedCuratorTenant(t, pool)
	masterID := seedCuratorMaster(t, pool, tenantID, "Original Name", "" /* brand empty */)

	// Seed five pending changes covering each routing branch.
	pcs := []struct {
		op    string
		field string
		value any
	}{
		{"enrich_scalar_fill", "brand", "L'Oreal"},                                    // master text fill — current empty
		{"enrich_scalar_fill", "name", "Should Not Overwrite"},                        // master text fill — current NON-empty → skipped
		{"enrich_scalar_fill", "product_form", "cream"},                               // cosmetics text fill
		{"enrich_array_union", "skin_type", []string{"oily", "combination"}},          // cosmetics array
		{"enrich_array_union", "skin_type", []string{"combination", "sensitive"}},     // cosmetics array (union with prev)
		{"enrich_scalar_fill", "tier3.primary_category", "Skincare"},                  // tier3
		{"enrich_scalar_fill", "definitely_not_a_known_field", "ignored"},             // unknown — should be skipped
	}
	changeIDs := make([]string, 0, len(pcs))
	for _, p := range pcs {
		valJSON, _ := json.Marshal(p.value)
		var id string
		err := pool.QueryRow(ctx, `
			INSERT INTO catalog.master_pending_changes
				(master_product_id, tenant_id, op_kind, field_name, pending_value, status)
			VALUES ($1::uuid, $2::uuid, $3, $4, $5::jsonb, 'pending')
			RETURNING id::text
		`, masterID, tenantID, p.op, p.field, string(valJSON)).Scan(&id)
		if err != nil {
			t.Fatalf("seed pending change %s: %v", p.field, err)
		}
		changeIDs = append(changeIDs, id)
	}

	client := &adapters.Client{Pool: pool}
	res, err := client.ApplyPendingChanges(ctx, changeIDs)
	if err != nil {
		t.Fatalf("ApplyPendingChanges: %v", err)
	}

	// applied = 6 (all known fields, including the name-overwrite which routed
	// to master_products but had a no-op WHERE clause). Skipped = 1 (unknown).
	if res.Applied != 6 {
		t.Errorf("Applied = %d, want 6 (5 mutations + 1 no-op route)", res.Applied)
	}
	if res.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1 (the unknown field)", res.Skipped)
	}

	// master_products: brand filled, name unchanged.
	var name, brand string
	if err := pool.QueryRow(ctx, `SELECT name, COALESCE(brand,'') FROM catalog.master_products WHERE id::text = $1`, masterID).Scan(&name, &brand); err != nil {
		t.Fatalf("read master row: %v", err)
	}
	if name != "Original Name" {
		t.Errorf("name = %q, want unchanged 'Original Name' (fill-when-empty contract broken)", name)
	}
	if brand != "L'Oreal" {
		t.Errorf("brand = %q, want 'L'Oreal'", brand)
	}

	// tier3: primary_category landed.
	var tier3Cat string
	if err := pool.QueryRow(ctx, `SELECT COALESCE(tier3->>'primary_category','') FROM catalog.master_products WHERE id::text = $1`, masterID).Scan(&tier3Cat); err != nil {
		t.Fatalf("read tier3: %v", err)
	}
	if tier3Cat != "Skincare" {
		t.Errorf("tier3.primary_category = %q, want 'Skincare'", tier3Cat)
	}

	// master_cosmetics: product_form filled + skin_type union (DISTINCT-merge of 3 elements).
	var productForm string
	var skinType []string
	err = pool.QueryRow(ctx, `
		SELECT COALESCE(product_form,''), COALESCE(skin_type, '{}'::text[])
		FROM catalog.master_cosmetics WHERE master_product_id::text = $1
	`, masterID).Scan(&productForm, &skinType)
	if err != nil {
		t.Fatalf("read cosmetics row: %v", err)
	}
	if productForm != "cream" {
		t.Errorf("product_form = %q, want 'cream'", productForm)
	}
	// Order-insensitive comparison of {"oily","combination","sensitive"}.
	seen := map[string]bool{}
	for _, v := range skinType {
		seen[v] = true
	}
	for _, want := range []string{"oily", "combination", "sensitive"} {
		if !seen[want] {
			t.Errorf("skin_type missing %q (got %v)", want, skinType)
		}
	}
	if len(skinType) != 3 {
		t.Errorf("skin_type len = %d (got %v), want 3 distinct elements after union", len(skinType), skinType)
	}
}

// --- minimal fixtures (no shared dbtest helper in curator yet) ---

func setupCuratorDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set — skipping curator integration test")
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

func randomSuffix() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func seedCuratorTenant(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	suf := randomSuffix()
	slug := "curator-itest-" + suf
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO catalog.tenants (slug, name, created_at, updated_at)
		VALUES ($1, $2, NOW(), NOW())
		RETURNING id::text
	`, slug, "Curator Integration Test "+slug).Scan(&id)
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	t.Cleanup(func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer ccancel()
		_, _ = pool.Exec(cctx, `DELETE FROM catalog.master_pending_changes WHERE tenant_id = $1`, id)
		_, _ = pool.Exec(cctx, `DELETE FROM catalog.master_cosmetics
			WHERE master_product_id IN (SELECT id FROM catalog.master_products WHERE owner_tenant_id = $1)`, id)
		_, _ = pool.Exec(cctx, `DELETE FROM catalog.master_products WHERE owner_tenant_id = $1`, id)
		_, _ = pool.Exec(cctx, `DELETE FROM catalog.tenants WHERE id = $1`, id)
	})
	return id
}

func seedCuratorMaster(t *testing.T, pool *pgxpool.Pool, tenantID, name, brand string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var id string
	err := pool.QueryRow(ctx, `
		INSERT INTO catalog.master_products
			(name, brand, vertical, sku, owner_tenant_id, created_at, updated_at)
		VALUES ($1, NULLIF($2,''), $3, $4, $5, NOW(), NOW())
		RETURNING id::text
	`, name, brand, "cosmetics", fmt.Sprintf("CURATOR-ITEST-%s", randomSuffix()), tenantID).Scan(&id)
	if err != nil {
		t.Fatalf("seed master: %v", err)
	}
	return id
}
