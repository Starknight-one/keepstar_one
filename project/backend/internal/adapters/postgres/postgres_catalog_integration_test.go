package postgres_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"keepstar/internal/adapters/postgres"
	"keepstar/internal/domain"
	"keepstar/internal/ports"
)

// ---------- helpers ----------

func catalogTestSetup(t *testing.T) (context.Context, *postgres.Client, *postgres.CatalogAdapter) {
	t.Helper()
	client := getSharedClient(t)
	catalog := postgres.NewCatalogAdapter(client)
	return context.Background(), client, catalog
}

func ensureTestTenant(t *testing.T, client *postgres.Client, slug, name string) string {
	t.Helper()
	ctx := context.Background()
	var id string
	err := client.Pool().QueryRow(ctx,
		`SELECT id FROM catalog.tenants WHERE slug = $1`, slug).Scan(&id)
	if err == nil {
		return id
	}
	id = uuid.New().String()
	_, err = client.Pool().Exec(ctx, `
		INSERT INTO catalog.tenants (id, slug, name, type, settings, created_at, updated_at)
		VALUES ($1, $2, $3, 'retailer', '{}', NOW(), NOW())
	`, id, slug, name)
	if err != nil {
		t.Fatalf("create test tenant: %v", err)
	}
	t.Cleanup(func() {
		pool := client.Pool()
		_, _ = pool.Exec(context.Background(), `DELETE FROM catalog.products WHERE tenant_id = $1`, id)
		_, _ = pool.Exec(context.Background(), `DELETE FROM catalog.stock WHERE tenant_id = $1`, id)
		_, _ = pool.Exec(context.Background(), `DELETE FROM catalog.tenants WHERE id = $1`, id)
	})
	return id
}

func seedTestProducts(t *testing.T, client *postgres.Client, tenantID string, n int) []string {
	t.Helper()
	ctx := context.Background()

	// Create a test category so digest JOIN works
	categoryID := uuid.New().String()
	categorySlug := fmt.Sprintf("test-cat-%s", uuid.New().String()[:8])
	_, err := client.Pool().Exec(ctx, `
		INSERT INTO catalog.categories (id, name, slug, created_at)
		VALUES ($1, $2, $3, NOW())
	`, categoryID, "Test Category", categorySlug)
	if err != nil {
		t.Fatalf("seed category: %v", err)
	}
	t.Cleanup(func() {
		_, _ = client.Pool().Exec(context.Background(), `DELETE FROM catalog.categories WHERE id = $1`, categoryID)
	})

	var productIDs []string
	var masterIDs []string
	brands := []string{"Nike", "Adidas", "Puma", "Reebok"}
	for i := 0; i < n; i++ {
		masterID := uuid.New().String()
		sku := fmt.Sprintf("TEST-SKU-%s", uuid.New().String()[:8])
		masterIDs = append(masterIDs, masterID)
		_, err = client.Pool().Exec(ctx, `
			INSERT INTO catalog.master_products (id, sku, name, description, brand, category_id, images, owner_tenant_id, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, '[]', $7, NOW(), NOW())
		`, masterID, sku,
			fmt.Sprintf("Test Product %d", i+1),
			fmt.Sprintf("Description %d", i+1),
			brands[i%len(brands)],
			categoryID,
			tenantID)
		if err != nil {
			t.Fatalf("seed master product: %v", err)
		}

		productID := uuid.New().String()
		_, err = client.Pool().Exec(ctx, `
			INSERT INTO catalog.products (id, tenant_id, master_product_id, price, currency, created_at, updated_at)
			VALUES ($1, $2, $3, $4, 'RUB', NOW(), NOW())
		`, productID, tenantID, masterID, (i+1)*10000)
		if err != nil {
			t.Fatalf("seed product: %v", err)
		}
		productIDs = append(productIDs, productID)
	}
	t.Cleanup(func() {
		for _, id := range productIDs {
			_, _ = client.Pool().Exec(context.Background(), `DELETE FROM catalog.products WHERE id = $1`, id)
		}
		for _, id := range masterIDs {
			_, _ = client.Pool().Exec(context.Background(), `DELETE FROM catalog.master_products WHERE id = $1`, id)
		}
	})
	return productIDs
}

// ---------- Tenant tests ----------

func TestCatalogIntegration_GetTenantBySlug_Found(t *testing.T) {
	ctx, client, catalog := catalogTestSetup(t)
	slug := fmt.Sprintf("inttest-%d", time.Now().UnixNano())
	ensureTestTenant(t, client, slug, "Integration Test Store")

	tenant, err := catalog.GetTenantBySlug(ctx, slug)
	if err != nil {
		t.Fatalf("GetTenantBySlug: %v", err)
	}
	if tenant.Slug != slug {
		t.Errorf("want slug %s, got %s", slug, tenant.Slug)
	}
}

func TestCatalogIntegration_GetTenantBySlug_NotFound(t *testing.T) {
	ctx, _, catalog := catalogTestSetup(t)

	_, err := catalog.GetTenantBySlug(ctx, "nonexistent-slug-xyz")
	if err != domain.ErrTenantNotFound {
		t.Errorf("want ErrTenantNotFound, got %v", err)
	}
}

// ---------- ListProducts tests ----------

func TestCatalogIntegration_ListProducts_Basic(t *testing.T) {
	ctx, client, catalog := catalogTestSetup(t)
	slug := fmt.Sprintf("list-%d", time.Now().UnixNano())
	tenantID := ensureTestTenant(t, client, slug, "List Test Store")
	seedTestProducts(t, client, tenantID, 5)

	products, count, err := catalog.ListProducts(ctx, tenantID, ports.ProductFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListProducts: %v", err)
	}
	if count < 5 {
		t.Errorf("want count >= 5, got %d", count)
	}
	if len(products) < 5 {
		t.Errorf("want >= 5 products, got %d", len(products))
	}
}

func TestCatalogIntegration_ListProducts_MasterMerge(t *testing.T) {
	ctx, client, catalog := catalogTestSetup(t)
	slug := fmt.Sprintf("merge-%d", time.Now().UnixNano())
	tenantID := ensureTestTenant(t, client, slug, "Merge Test Store")
	seedTestProducts(t, client, tenantID, 2)

	products, _, err := catalog.ListProducts(ctx, tenantID, ports.ProductFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListProducts: %v", err)
	}
	if len(products) < 2 {
		t.Fatalf("want >= 2 products, got %d", len(products))
	}

	// Products should have merged name/brand from master
	for i, p := range products {
		if p.Name == "" {
			t.Errorf("product[%d]: name should be merged from master", i)
		}
		if p.Brand == "" {
			t.Errorf("product[%d]: brand should be merged from master", i)
		}
	}
}

func TestCatalogIntegration_ListProducts_FilterBrand(t *testing.T) {
	ctx, client, catalog := catalogTestSetup(t)
	slug := fmt.Sprintf("brand-%d", time.Now().UnixNano())
	tenantID := ensureTestTenant(t, client, slug, "Brand Test Store")
	seedTestProducts(t, client, tenantID, 8) // creates Nike, Adidas, Puma, Reebok cycle

	products, _, err := catalog.ListProducts(ctx, tenantID, ports.ProductFilter{
		Brand: "Nike",
		Limit: 50,
	})
	if err != nil {
		t.Fatalf("ListProducts with brand filter: %v", err)
	}

	for _, p := range products {
		if p.Brand != "Nike" {
			t.Errorf("brand filter leak: got product with brand %s", p.Brand)
		}
	}
}

func TestCatalogIntegration_ListProducts_FilterPriceRange(t *testing.T) {
	ctx, client, catalog := catalogTestSetup(t)
	slug := fmt.Sprintf("price-%d", time.Now().UnixNano())
	tenantID := ensureTestTenant(t, client, slug, "Price Test Store")
	seedTestProducts(t, client, tenantID, 5) // prices: 10000, 20000, 30000, 40000, 50000

	products, _, err := catalog.ListProducts(ctx, tenantID, ports.ProductFilter{
		MinPrice: 20000,
		MaxPrice: 40000,
		Limit:    50,
	})
	if err != nil {
		t.Fatalf("ListProducts with price filter: %v", err)
	}

	for _, p := range products {
		if p.Price < 20000 || p.Price > 40000 {
			t.Errorf("price filter leak: got product with price %d", p.Price)
		}
	}
}

func TestCatalogIntegration_ListProducts_Pagination(t *testing.T) {
	ctx, client, catalog := catalogTestSetup(t)
	slug := fmt.Sprintf("page-%d", time.Now().UnixNano())
	tenantID := ensureTestTenant(t, client, slug, "Page Test Store")
	seedTestProducts(t, client, tenantID, 5)

	page1, total, err := catalog.ListProducts(ctx, tenantID, ports.ProductFilter{Limit: 2, Offset: 0})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	page2, _, err := catalog.ListProducts(ctx, tenantID, ports.ProductFilter{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}

	if total < 5 {
		t.Errorf("want total >= 5, got %d", total)
	}
	if len(page1) != 2 {
		t.Errorf("page 1: want 2, got %d", len(page1))
	}
	if len(page2) != 2 {
		t.Errorf("page 2: want 2, got %d", len(page2))
	}

	// Pages should not overlap
	if len(page1) > 0 && len(page2) > 0 && page1[0].ID == page2[0].ID {
		t.Error("pages overlap: same first product")
	}
}

func TestCatalogIntegration_ListProducts_PriceFormatted(t *testing.T) {
	ctx, client, catalog := catalogTestSetup(t)
	slug := fmt.Sprintf("fmt-%d", time.Now().UnixNano())
	tenantID := ensureTestTenant(t, client, slug, "Format Test Store")
	seedTestProducts(t, client, tenantID, 1) // price = 10000 kopecks = 100 rubles

	products, _, err := catalog.ListProducts(ctx, tenantID, ports.ProductFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListProducts: %v", err)
	}
	if len(products) < 1 {
		t.Fatal("no products")
	}

	if products[0].PriceFormatted == "" {
		t.Error("PriceFormatted should not be empty")
	}
	// Should contain ₽ for RUB
	if products[0].Currency == "RUB" && products[0].PriceFormatted != "" {
		// Just verify it's not empty — exact format tested in unit test
		t.Logf("PriceFormatted: %q", products[0].PriceFormatted)
	}
}

func TestCatalogIntegration_ListProducts_Search(t *testing.T) {
	ctx, client, catalog := catalogTestSetup(t)
	slug := fmt.Sprintf("search-%d", time.Now().UnixNano())
	tenantID := ensureTestTenant(t, client, slug, "Search Test Store")
	seedTestProducts(t, client, tenantID, 5)

	products, _, err := catalog.ListProducts(ctx, tenantID, ports.ProductFilter{
		Search: "Test Product 1",
		Limit:  50,
	})
	if err != nil {
		t.Fatalf("ListProducts search: %v", err)
	}

	if len(products) == 0 {
		t.Error("search should find at least 1 product matching 'Test Product 1'")
	}
}

func TestCatalogIntegration_ListProducts_SortByPrice(t *testing.T) {
	ctx, client, catalog := catalogTestSetup(t)
	slug := fmt.Sprintf("sort-%d", time.Now().UnixNano())
	tenantID := ensureTestTenant(t, client, slug, "Sort Test Store")
	seedTestProducts(t, client, tenantID, 5)

	products, _, err := catalog.ListProducts(ctx, tenantID, ports.ProductFilter{
		SortField: "price",
		SortOrder: "asc",
		Limit:     50,
	})
	if err != nil {
		t.Fatalf("ListProducts sort: %v", err)
	}
	if len(products) < 2 {
		t.Skip("not enough products to test sort")
	}

	for i := 1; i < len(products); i++ {
		if products[i].Price < products[i-1].Price {
			t.Errorf("sort broken: products[%d].Price=%d < products[%d].Price=%d",
				i, products[i].Price, i-1, products[i-1].Price)
		}
	}
}

// ---------- GetProduct tests ----------

func TestCatalogIntegration_GetProduct_Found(t *testing.T) {
	ctx, client, catalog := catalogTestSetup(t)
	slug := fmt.Sprintf("get-%d", time.Now().UnixNano())
	tenantID := ensureTestTenant(t, client, slug, "Get Test Store")
	ids := seedTestProducts(t, client, tenantID, 1)

	product, err := catalog.GetProduct(ctx, tenantID, ids[0])
	if err != nil {
		t.Fatalf("GetProduct: %v", err)
	}
	if product.ID != ids[0] {
		t.Errorf("want ID %s, got %s", ids[0], product.ID)
	}
	if product.Name == "" {
		t.Error("name should be merged from master")
	}
}

func TestCatalogIntegration_GetProduct_NotFound(t *testing.T) {
	ctx, client, catalog := catalogTestSetup(t)
	slug := fmt.Sprintf("notfound-%d", time.Now().UnixNano())
	tenantID := ensureTestTenant(t, client, slug, "NotFound Test Store")

	_, err := catalog.GetProduct(ctx, tenantID, "nonexistent-product-xyz")
	if err == nil {
		t.Error("expected error for nonexistent product")
	}
}

// ---------- CatalogDigest tests ----------

func TestCatalogIntegration_DigestSaveAndGet(t *testing.T) {
	ctx, client, catalog := catalogTestSetup(t)
	slug := fmt.Sprintf("digest-%d", time.Now().UnixNano())
	tenantID := ensureTestTenant(t, client, slug, "Digest Test Store")

	digest := &domain.CatalogDigest{
		GeneratedAt:   time.Now(),
		TotalProducts: 42,
		CategoryTree: []domain.DigestCategoryGroup{
			{Name: "Shoes", Slug: "shoes", Children: []domain.DigestCategoryLeaf{
				{Name: "Sneakers", Slug: "sneakers", Count: 42},
			}},
		},
	}

	if err := catalog.SaveCatalogDigest(ctx, tenantID, digest); err != nil {
		t.Fatalf("SaveCatalogDigest: %v", err)
	}

	loaded, err := catalog.GetCatalogDigest(ctx, tenantID)
	if err != nil {
		t.Fatalf("GetCatalogDigest: %v", err)
	}

	if loaded.TotalProducts != 42 {
		t.Errorf("want TotalProducts=42, got %d", loaded.TotalProducts)
	}
	if len(loaded.CategoryTree) != 1 {
		t.Errorf("want 1 category group, got %d", len(loaded.CategoryTree))
	}
}

func TestCatalogIntegration_DigestGenerate(t *testing.T) {
	ctx, client, catalog := catalogTestSetup(t)
	slug := fmt.Sprintf("digen-%d", time.Now().UnixNano())
	tenantID := ensureTestTenant(t, client, slug, "DigestGen Test Store")
	seedTestProducts(t, client, tenantID, 4)

	digest, err := catalog.GenerateCatalogDigest(ctx, tenantID)
	if err != nil {
		t.Fatalf("GenerateCatalogDigest: %v", err)
	}
	if digest.TotalProducts < 4 {
		t.Errorf("want TotalProducts >= 4, got %d", digest.TotalProducts)
	}
	if len(digest.CategoryTree) < 1 {
		t.Errorf("want >= 1 category group, got %d", len(digest.CategoryTree))
	}
}

// ---------- GetAllTenants ----------

func TestCatalogIntegration_GetAllTenants(t *testing.T) {
	ctx, client, catalog := catalogTestSetup(t)
	slug := fmt.Sprintf("all-%d", time.Now().UnixNano())
	ensureTestTenant(t, client, slug, "All Test Store")

	tenants, err := catalog.GetAllTenants(ctx)
	if err != nil {
		t.Fatalf("GetAllTenants: %v", err)
	}
	if len(tenants) == 0 {
		t.Error("expected at least 1 tenant")
	}
}
