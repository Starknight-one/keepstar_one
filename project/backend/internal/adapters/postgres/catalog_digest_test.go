package postgres_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"keepstar/internal/adapters/postgres"
	"keepstar/internal/domain"
)

// setupDigestTest connects to DB and returns a CatalogAdapter.
// Skips test if DATABASE_URL is not set.
func setupDigestTest(t *testing.T) *postgres.CatalogAdapter {
	t.Helper()
	_ = godotenv.Load("../../../../.env")

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set — skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dbClient, err := postgres.NewClient(ctx, dbURL)
	if err != nil {
		t.Fatalf("DB connection failed: %v", err)
	}
	t.Cleanup(func() { dbClient.Close() })

	if err := dbClient.RunCatalogMigrations(ctx); err != nil {
		t.Fatalf("catalog migrations failed: %v", err)
	}

	return postgres.NewCatalogAdapter(dbClient)
}

// digestGetTenantID resolves a tenant slug to its UUID.
func digestGetTenantID(t *testing.T, catalog *postgres.CatalogAdapter, slug string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tenant, err := catalog.GetTenantBySlug(ctx, slug)
	if err != nil {
		t.Fatalf("get tenant %s: %v", slug, err)
	}
	return tenant.ID
}

// digestFindCategory finds a category by name (case-insensitive contains).
func digestFindCategory(digest *domain.CatalogDigest, name string) *domain.DigestCategory {
	lower := strings.ToLower(name)
	for i, cat := range digest.Categories {
		if strings.Contains(strings.ToLower(cat.Name), lower) {
			return &digest.Categories[i]
		}
	}
	return nil
}

// digestFindParam finds a param by key in a category.
func digestFindParam(cat *domain.DigestCategory, key string) *domain.DigestParam {
	for i, p := range cat.Params {
		if p.Key == key {
			return &cat.Params[i]
		}
	}
	return nil
}

// --- Nike: single-brand tenant, shoe-heavy ---

func TestGenerateCatalogDigest_Nike(t *testing.T) {
	catalog := setupDigestTest(t)
	tenantID := digestGetTenantID(t, catalog, "nike")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	digest, err := catalog.GenerateCatalogDigest(ctx, tenantID)
	if err != nil {
		t.Fatalf("generate digest: %v", err)
	}

	// Nike has ~116 products across ~13 categories from seed
	if digest.TotalProducts < 100 {
		t.Errorf("expected 100+ products for Nike, got %d", digest.TotalProducts)
	}
	if len(digest.Categories) < 10 {
		t.Errorf("expected 10+ categories for Nike, got %d", len(digest.Categories))
	}

	// Must have shoe categories
	shoeCategories := []string{"Casual Shoes", "Running Shoes", "Basketball Shoes"}
	for _, name := range shoeCategories {
		cat := digestFindCategory(digest, name)
		if cat == nil {
			t.Errorf("expected category %q in Nike digest", name)
			continue
		}
		if cat.Count == 0 {
			t.Errorf("category %q has 0 products", name)
		}
		if cat.PriceRange[1] < cat.PriceRange[0] {
			t.Errorf("category %s: max price (%d) < min price (%d)", cat.Name, cat.PriceRange[1], cat.PriceRange[0])
		}
	}

	// Nike is single-brand: brand param should have cardinality 1 in most categories
	singleBrandCount := 0
	for _, cat := range digest.Categories {
		bp := digestFindParam(&cat, "brand")
		if bp != nil && bp.Cardinality == 1 {
			singleBrandCount++
		}
	}
	if singleBrandCount == 0 {
		t.Error("expected at least some categories with brand cardinality=1 for single-brand Nike tenant")
	}

	t.Logf("Nike: %d products, %d categories, %d single-brand categories",
		digest.TotalProducts, len(digest.Categories), singleBrandCount)
	for _, cat := range digest.Categories {
		t.Logf("  %s (%d): %d-%d kop, %d params", cat.Name, cat.Count, cat.PriceRange[0], cat.PriceRange[1], len(cat.Params))
	}
}

// --- Sportmaster: multi-brand, largest tenant ---

func TestGenerateCatalogDigest_Sportmaster(t *testing.T) {
	catalog := setupDigestTest(t)
	tenantID := digestGetTenantID(t, catalog, "sportmaster")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	digest, err := catalog.GenerateCatalogDigest(ctx, tenantID)
	if err != nil {
		t.Fatalf("generate digest: %v", err)
	}

	// Sportmaster ~254 products, ~18 categories
	if digest.TotalProducts < 200 {
		t.Errorf("expected 200+ products for Sportmaster, got %d", digest.TotalProducts)
	}
	if len(digest.Categories) < 15 {
		t.Errorf("expected 15+ categories for Sportmaster, got %d", len(digest.Categories))
	}

	// Multi-brand: at least one category should have brand cardinality > 1
	multiBrandFound := false
	for _, cat := range digest.Categories {
		bp := digestFindParam(&cat, "brand")
		if bp != nil && bp.Cardinality > 1 {
			multiBrandFound = true
			// If cardinality > 15, should have Top, not Values
			if bp.Cardinality > 15 && len(bp.Top) == 0 && len(bp.Families) == 0 {
				t.Errorf("category %s: brand cardinality %d > 15 but no Top/Families", cat.Name, bp.Cardinality)
			}
			break
		}
	}
	if !multiBrandFound {
		t.Error("expected multi-brand categories in Sportmaster (cardinality > 1)")
	}

	// Should have color params in clothing/shoe categories
	colorFound := false
	for _, cat := range digest.Categories {
		cp := digestFindParam(&cat, "color")
		if cp != nil {
			colorFound = true
			if cp.Cardinality == 0 {
				t.Errorf("category %s: color cardinality should be > 0", cat.Name)
			}
		}
	}
	if !colorFound {
		t.Error("expected color param in at least one Sportmaster category")
	}

	t.Logf("Sportmaster: %d products, %d categories", digest.TotalProducts, len(digest.Categories))
}

// --- TechStore: electronics with numeric params ---

func TestGenerateCatalogDigest_TechStore(t *testing.T) {
	catalog := setupDigestTest(t)
	tenantID := digestGetTenantID(t, catalog, "techstore")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	digest, err := catalog.GenerateCatalogDigest(ctx, tenantID)
	if err != nil {
		t.Fatalf("generate digest: %v", err)
	}

	// TechStore ~220 products, ~10 categories
	if digest.TotalProducts < 150 {
		t.Errorf("expected 150+ products for TechStore, got %d", digest.TotalProducts)
	}

	// Must have electronics categories
	electronicsCategories := []string{"Smartphones", "Laptops", "Headphones"}
	for _, name := range electronicsCategories {
		cat := digestFindCategory(digest, name)
		if cat == nil {
			t.Errorf("expected category %q in TechStore digest", name)
			continue
		}
		if cat.Count == 0 {
			t.Errorf("category %q has 0 products", name)
		}
	}

	// Laptops should have numeric/tech params: ram, storage, display
	laptops := digestFindCategory(digest, "Laptops")
	if laptops != nil {
		techParams := []string{"ram", "storage"}
		for _, key := range techParams {
			p := digestFindParam(laptops, key)
			if p == nil {
				t.Errorf("Laptops missing param %q", key)
			}
		}
		t.Logf("Laptops (%d): %d params", laptops.Count, len(laptops.Params))
		for _, p := range laptops.Params {
			t.Logf("  %s: card=%d values=%d top=%d families=%d range=%q",
				p.Key, p.Cardinality, len(p.Values), len(p.Top), len(p.Families), p.Range)
		}
	}
}

// --- Service tenants ---

func TestGenerateCatalogDigest_ServiceTenants(t *testing.T) {
	catalog := setupDigestTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	services := []struct {
		slug       string
		minProducts int
		minCategories int
	}{
		{"beautylab", 20, 2},
		{"autofix", 20, 1},
		{"fitzone", 15, 1},
		{"homeservice", 15, 1},
	}

	for _, s := range services {
		t.Run(s.slug, func(t *testing.T) {
			tenantID := digestGetTenantID(t, catalog, s.slug)
			digest, err := catalog.GenerateCatalogDigest(ctx, tenantID)
			if err != nil {
				t.Fatalf("generate digest for %s: %v", s.slug, err)
			}

			if digest.TotalProducts < s.minProducts {
				t.Errorf("%s: expected %d+ products, got %d", s.slug, s.minProducts, digest.TotalProducts)
			}
			if len(digest.Categories) < s.minCategories {
				t.Errorf("%s: expected %d+ categories, got %d", s.slug, s.minCategories, len(digest.Categories))
			}

			// Service categories should have "type" param
			typeFound := false
			for _, cat := range digest.Categories {
				if digestFindParam(&cat, "type") != nil {
					typeFound = true
					break
				}
			}
			if !typeFound {
				t.Logf("%s: no 'type' param found (may be expected for some service tenants)", s.slug)
			}

			t.Logf("%s: %d products, %d categories", s.slug, digest.TotalProducts, len(digest.Categories))
			for _, cat := range digest.Categories {
				t.Logf("  %s (%d): %d params", cat.Name, cat.Count, len(cat.Params))
			}
		})
	}
}

// --- Cross-tenant isolation ---

func TestGenerateCatalogDigest_TenantIsolation(t *testing.T) {
	catalog := setupDigestTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// beautylab is service-only, should NOT have electronics or shoes
	tenantID := digestGetTenantID(t, catalog, "beautylab")
	digest, err := catalog.GenerateCatalogDigest(ctx, tenantID)
	if err != nil {
		t.Fatalf("generate digest: %v", err)
	}

	for _, cat := range digest.Categories {
		lower := strings.ToLower(cat.Name)
		if strings.Contains(lower, "sneakers") || strings.Contains(lower, "laptop") ||
			strings.Contains(lower, "smartphone") || strings.Contains(lower, "running") {
			t.Errorf("beautylab should NOT have category %q — tenant isolation broken", cat.Name)
		}
	}
}

// --- Empty tenant edge case ---

func TestGenerateCatalogDigest_EmptyTenant(t *testing.T) {
	catalog := setupDigestTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	digest, err := catalog.GenerateCatalogDigest(ctx, "00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatalf("generate digest for empty tenant: %v", err)
	}

	if digest.TotalProducts != 0 {
		t.Errorf("expected 0 total_products for empty tenant, got %d", digest.TotalProducts)
	}
	if len(digest.Categories) != 0 {
		t.Errorf("expected 0 categories for empty tenant, got %d", len(digest.Categories))
	}
}

// --- Cardinality rules ---

func TestGenerateCatalogDigest_CardinalityRules(t *testing.T) {
	catalog := setupDigestTest(t)
	tenantID := digestGetTenantID(t, catalog, "sportmaster")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	digest, err := catalog.GenerateCatalogDigest(ctx, tenantID)
	if err != nil {
		t.Fatalf("generate digest: %v", err)
	}

	for _, cat := range digest.Categories {
		for _, p := range cat.Params {
			switch {
			case p.Range != "":
				// Numeric range — valid regardless of cardinality
				if p.Type != "range" {
					t.Errorf("%s/%s: has Range but type=%q, expected 'range'", cat.Name, p.Key, p.Type)
				}
			case p.Cardinality <= 15:
				if len(p.Values) == 0 && len(p.Top) == 0 && len(p.Families) == 0 {
					t.Errorf("%s/%s: cardinality %d ≤ 15 but no Values/Top/Families", cat.Name, p.Key, p.Cardinality)
				}
			case p.Cardinality > 15 && p.Cardinality <= 50:
				if len(p.Top) == 0 && len(p.Families) == 0 {
					// Values fallback is also OK
					if len(p.Values) == 0 {
						t.Errorf("%s/%s: cardinality %d (16-50) but no Top/Families/Values", cat.Name, p.Key, p.Cardinality)
					}
				}
				if len(p.Top) > 0 && p.More == 0 {
					t.Errorf("%s/%s: has Top but More=0 (expected remaining count)", cat.Name, p.Key)
				}
			case p.Cardinality > 50:
				if len(p.Families) == 0 && len(p.Top) == 0 {
					t.Errorf("%s/%s: cardinality %d > 50 but no Families/Top", cat.Name, p.Key, p.Cardinality)
				}
			}
		}
	}
}

// --- ToPromptText with real data ---

func TestGenerateCatalogDigest_ToPromptText_Integration(t *testing.T) {
	catalog := setupDigestTest(t)
	tenantID := digestGetTenantID(t, catalog, "sportmaster")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	digest, err := catalog.GenerateCatalogDigest(ctx, tenantID)
	if err != nil {
		t.Fatalf("generate digest: %v", err)
	}

	text := digest.ToPromptText()
	if text == "" {
		t.Fatal("ToPromptText returned empty string for non-empty digest")
	}

	// Must contain header
	if !strings.Contains(text, "Tenant catalog:") {
		t.Error("missing 'Tenant catalog:' header")
	}

	// Must contain search strategy block
	if !strings.Contains(text, "Search strategy:") {
		t.Error("missing 'Search strategy:' block")
	}

	// Must have filter hints
	if !strings.Contains(text, "→ filter") {
		t.Error("missing '→ filter' hints")
	}

	// Must have at least one category name from the real data
	foundCategory := false
	for _, cat := range digest.Categories {
		if strings.Contains(text, cat.Name) {
			foundCategory = true
			break
		}
	}
	if !foundCategory {
		t.Error("no category names found in prompt text")
	}

	// Price should be in rubles, not kopecks (no 7-digit numbers)
	if strings.Contains(text, "000000") {
		t.Error("prompt text may contain kopecks instead of rubles")
	}

	t.Logf("ToPromptText length: %d chars, %d lines", len(text), strings.Count(text, "\n")+1)
	t.Log(text)
}

// --- Save/Get roundtrip ---

func TestSaveCatalogDigest_RoundTrip(t *testing.T) {
	catalog := setupDigestTest(t)
	tenantID := digestGetTenantID(t, catalog, "nike")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	original := &domain.CatalogDigest{
		GeneratedAt:   time.Now().Truncate(time.Second),
		TotalProducts: 42,
		Categories: []domain.DigestCategory{
			{
				Name:       "Test Category",
				Slug:       "test-cat",
				Count:      42,
				PriceRange: [2]int{100000, 500000},
				Params: []domain.DigestParam{
					{Key: "brand", Type: "enum", Cardinality: 3, Values: []string{"A", "B", "C"}},
				},
			},
		},
	}

	if err := catalog.SaveCatalogDigest(ctx, tenantID, original); err != nil {
		t.Fatalf("save digest: %v", err)
	}

	loaded, err := catalog.GetCatalogDigest(ctx, tenantID)
	if err != nil {
		t.Fatalf("get digest: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil digest after save")
	}

	if loaded.TotalProducts != original.TotalProducts {
		t.Errorf("total_products: got %d, want %d", loaded.TotalProducts, original.TotalProducts)
	}
	if len(loaded.Categories) != len(original.Categories) {
		t.Fatalf("categories count: got %d, want %d", len(loaded.Categories), len(original.Categories))
	}
	if loaded.Categories[0].Name != "Test Category" {
		t.Errorf("category name: got %q, want %q", loaded.Categories[0].Name, "Test Category")
	}
	if loaded.Categories[0].Params[0].Key != "brand" {
		t.Errorf("param key: got %q, want %q", loaded.Categories[0].Params[0].Key, "brand")
	}
	if len(loaded.Categories[0].Params[0].Values) != 3 {
		t.Errorf("param values count: got %d, want 3", len(loaded.Categories[0].Params[0].Values))
	}

	// Cleanup: restore real digest
	realDigest, _ := catalog.GenerateCatalogDigest(ctx, tenantID)
	if realDigest != nil {
		_ = catalog.SaveCatalogDigest(ctx, tenantID, realDigest)
	}
}

func TestGetCatalogDigest_Exists(t *testing.T) {
	catalog := setupDigestTest(t)
	tenantID := digestGetTenantID(t, catalog, "nike")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	digest, err := catalog.GenerateCatalogDigest(ctx, tenantID)
	if err != nil {
		t.Fatalf("generate digest: %v", err)
	}
	if err := catalog.SaveCatalogDigest(ctx, tenantID, digest); err != nil {
		t.Fatalf("save digest: %v", err)
	}

	loaded, err := catalog.GetCatalogDigest(ctx, tenantID)
	if err != nil {
		t.Fatalf("get digest: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil digest")
	}
	if loaded.TotalProducts != digest.TotalProducts {
		t.Errorf("total_products mismatch: got %d, want %d", loaded.TotalProducts, digest.TotalProducts)
	}
}

func TestGetCatalogDigest_NotGenerated(t *testing.T) {
	catalog := setupDigestTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	digest, err := catalog.GetCatalogDigest(ctx, "00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatalf("expected no error for non-existent tenant, got: %v", err)
	}
	if digest != nil {
		t.Errorf("expected nil digest for non-existent tenant, got: %+v", digest)
	}
}
