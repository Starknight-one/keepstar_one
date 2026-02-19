package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"keepstar/internal/adapters/postgres"
	"keepstar/internal/domain"
)

// setupDigestTest creates a CatalogAdapter. Skips if DATABASE_URL not set.
func setupDigestTest(t *testing.T) *postgres.CatalogAdapter {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set — skipping digest integration test")
	}
	client := getSharedClient(t)
	return postgres.NewCatalogAdapter(client)
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

// --- Generate digest (basic) ---

func TestGenerateCatalogDigest_Basic(t *testing.T) {
	catalog := setupDigestTest(t)

	// Use any existing tenant
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tenants, err := catalog.GetAllTenants(ctx)
	if err != nil {
		t.Fatalf("GetAllTenants: %v", err)
	}
	if len(tenants) == 0 {
		t.Skip("no tenants in DB")
	}

	tenantID := tenants[0].ID
	digest, err := catalog.GenerateCatalogDigest(ctx, tenantID)
	if err != nil {
		t.Fatalf("GenerateCatalogDigest: %v", err)
	}

	if digest.TotalProducts == 0 {
		t.Skip("tenant has no products")
	}

	// Should have category tree
	if len(digest.CategoryTree) == 0 {
		t.Error("expected non-empty category tree")
	}

	// Should have shared filters
	if len(digest.SharedFilters) == 0 {
		t.Log("warning: no shared filters found (PIM columns may not be populated)")
	}

	// ToPromptText should produce compact output
	text := digest.ToPromptText()
	t.Logf("Digest: %d chars, %d category groups, %d filters, %d brands, %d ingredients",
		len(text), len(digest.CategoryTree), len(digest.SharedFilters), len(digest.TopBrands), len(digest.TopIngredients))
	t.Logf("Content:\n%s", text)

	if len(text) > 3000 {
		t.Errorf("digest text too large: %d chars", len(text))
	}
}

func TestGenerateCatalogDigest_EmptyTenant(t *testing.T) {
	catalog := setupDigestTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	digest, err := catalog.GenerateCatalogDigest(ctx, "00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatalf("GenerateCatalogDigest: %v", err)
	}
	if digest.TotalProducts != 0 {
		t.Errorf("expected 0 products for fake tenant, got %d", digest.TotalProducts)
	}
}

// --- Save/Get round-trip ---

func TestSaveCatalogDigest_RoundTrip(t *testing.T) {
	catalog := setupDigestTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tenants, err := catalog.GetAllTenants(ctx)
	if err != nil || len(tenants) == 0 {
		t.Skip("no tenants in DB")
	}
	tenantID := tenants[0].ID

	original := &domain.CatalogDigest{
		GeneratedAt:   time.Now().Truncate(time.Second),
		TotalProducts: 42,
		CategoryTree: []domain.DigestCategoryGroup{
			{Name: "test", Slug: "test", Children: []domain.DigestCategoryLeaf{
				{Name: "Sub", Slug: "sub", Count: 42},
			}},
		},
		SharedFilters: []domain.DigestSharedFilter{
			{Key: "skin_type", Values: []string{"dry", "oily"}},
		},
		TopBrands:      []string{"BrandA", "BrandB"},
		TopIngredients: []string{"IngredA"},
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
	if loaded.TotalProducts != 42 {
		t.Errorf("total_products: got %d, want 42", loaded.TotalProducts)
	}
	if len(loaded.CategoryTree) != 1 {
		t.Errorf("category_tree: got %d groups, want 1", len(loaded.CategoryTree))
	}
	if len(loaded.SharedFilters) != 1 {
		t.Errorf("shared_filters: got %d, want 1", len(loaded.SharedFilters))
	}
	if len(loaded.TopBrands) != 2 {
		t.Errorf("top_brands: got %d, want 2", len(loaded.TopBrands))
	}

	// Restore real digest
	realDigest, _ := catalog.GenerateCatalogDigest(ctx, tenantID)
	if realDigest != nil {
		_ = catalog.SaveCatalogDigest(ctx, tenantID, realDigest)
	}
}

func TestGetCatalogDigest_NotGenerated(t *testing.T) {
	catalog := setupDigestTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	digest, err := catalog.GetCatalogDigest(ctx, "00000000-0000-0000-0000-000000000000")
	if err != nil {
		t.Fatalf("GetCatalogDigest: %v", err)
	}
	if digest != nil {
		t.Errorf("expected nil for non-existent tenant, got %+v", digest)
	}
}
