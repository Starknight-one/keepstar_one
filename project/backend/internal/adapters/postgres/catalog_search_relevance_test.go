package postgres_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/joho/godotenv"
	openaiAdapter "keepstar/internal/adapters/openai"
	"keepstar/internal/adapters/postgres"
	"keepstar/internal/domain"
	"keepstar/internal/ports"
)

// --- Setup ---

// setupRelevanceTest connects to DB and returns catalog + embedding clients.
// Skips test if DATABASE_URL or OPENAI_API_KEY are not set.
func setupRelevanceTest(t *testing.T) (*postgres.CatalogAdapter, ports.EmbeddingPort) {
	t.Helper()
	_ = godotenv.Load("../../../../.env")

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set — skipping integration test")
	}
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set — skipping integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	dbClient, err := postgres.NewClient(ctx, dbURL)
	if err != nil {
		t.Fatalf("DB connection failed: %v", err)
	}
	t.Cleanup(func() { dbClient.Close() })

	catalog := postgres.NewCatalogAdapter(dbClient)

	model := os.Getenv("EMBEDDING_MODEL")
	if model == "" {
		model = "text-embedding-3-small"
	}
	emb := openaiAdapter.NewEmbeddingClient(apiKey, model, 384)

	return catalog, emb
}

// embed generates embedding for a single text. Tracks cost.
func embed(t *testing.T, emb ports.EmbeddingPort, text string) []float32 {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	vecs, err := emb.Embed(ctx, []string{text})
	if err != nil {
		t.Fatalf("embed %q: %v", text, err)
	}
	// text-embedding-3-small: ~$0.02/1M tokens, ~1 token per 4 chars
	tokens := len(text) / 4
	cost := float64(tokens) * 0.02 / 1_000_000
	t.Logf("  embed: %q → %d tokens, ~$%.6f", text, tokens, cost)
	return vecs[0]
}

// getTenantID resolves a tenant slug to its UUID.
func getTenantID(t *testing.T, catalog *postgres.CatalogAdapter, slug string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tenant, err := catalog.GetTenantBySlug(ctx, slug)
	if err != nil {
		t.Fatalf("get tenant %s: %v", slug, err)
	}
	return tenant.ID
}

// logResults logs search results for debugging.
func logResults(t *testing.T, label string, products []domain.Product) {
	t.Helper()
	t.Logf("%s: %d results", label, len(products))
	for i, p := range products {
		if i >= 5 {
			t.Logf("  ... and %d more", len(products)-5)
			break
		}
		t.Logf("  [%d] %s | %s | %s | %d kop", i, p.Name, p.Brand, p.Category, p.Price)
	}
}

// --- Vector search: brand + category precision ---

func TestRelevance_NikeSneakers(t *testing.T) {
	catalog, emb := setupRelevanceTest(t)
	tenantID := getTenantID(t, catalog, "nike")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	vec := embed(t, emb, "кроссовки Nike")
	vf := &ports.VectorFilter{Brand: "Nike"}

	products, err := catalog.VectorSearch(ctx, tenantID, vec, 10, vf)
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	if len(products) == 0 {
		t.Fatal("expected results for Nike sneakers, got 0")
	}

	shoeCategories := map[string]bool{
		"sneakers": true, "running": true, "basketball": true, "lifestyle": true,
		"running shoes": true, "basketball shoes": true, "casual shoes": true,
	}
	for _, p := range products {
		if !strings.Contains(strings.ToLower(p.Brand), "nike") && !strings.Contains(strings.ToLower(p.Brand), "jordan") {
			t.Errorf("expected Nike/Jordan brand, got %q for %q", p.Brand, p.Name)
		}
		catLower := strings.ToLower(p.Category)
		if !shoeCategories[catLower] {
			t.Errorf("expected shoe category, got %q for %q", p.Category, p.Name)
		}
	}
	logResults(t, "Nike sneakers", products)
}

func TestRelevance_AppleLaptop(t *testing.T) {
	catalog, emb := setupRelevanceTest(t)
	tenantID := getTenantID(t, catalog, "techstore")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	vec := embed(t, emb, "ноутбук Apple")
	vf := &ports.VectorFilter{Brand: "Apple", CategoryName: "Laptops"}

	products, err := catalog.VectorSearch(ctx, tenantID, vec, 10, vf)
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	if len(products) == 0 {
		t.Fatal("expected results for Apple laptops, got 0")
	}

	for _, p := range products {
		if !strings.Contains(strings.ToLower(p.Brand), "apple") {
			t.Errorf("expected Apple brand, got %q for %q", p.Brand, p.Name)
		}
		if !strings.EqualFold(p.Category, "Laptops") {
			t.Errorf("expected Laptops category, got %q for %q", p.Category, p.Name)
		}
	}
	logResults(t, "Apple laptops", products)
}

func TestRelevance_CategoryPrecision(t *testing.T) {
	catalog, emb := setupRelevanceTest(t)
	tenantID := getTenantID(t, catalog, "techstore")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	vec := embed(t, emb, "наушники Sony")
	vf := &ports.VectorFilter{Brand: "Sony", CategoryName: "Headphones"}

	products, err := catalog.VectorSearch(ctx, tenantID, vec, 10, vf)
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	if len(products) == 0 {
		t.Fatal("expected Sony headphone results, got 0")
	}

	for _, p := range products {
		if !strings.Contains(strings.ToLower(p.Brand), "sony") {
			t.Errorf("expected Sony brand, got %q for %q", p.Brand, p.Name)
		}
		if !strings.EqualFold(p.Category, "Headphones") {
			t.Errorf("expected Headphones, got %q for %q (cross-category leak!)", p.Category, p.Name)
		}
	}
	logResults(t, "Sony headphones", products)
}

// --- Brand exclusion ---

func TestRelevance_BrandExclusion(t *testing.T) {
	catalog, emb := setupRelevanceTest(t)
	tenantID := getTenantID(t, catalog, "sportmaster")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	vec := embed(t, emb, "кроссовки Adidas")
	vf := &ports.VectorFilter{Brand: "Adidas"}

	products, err := catalog.VectorSearch(ctx, tenantID, vec, 10, vf)
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}

	for _, p := range products {
		if strings.EqualFold(p.Brand, "Nike") {
			t.Errorf("Nike product %q should NOT appear in Adidas-filtered results", p.Name)
		}
	}
	logResults(t, "Adidas (no Nike)", products)
}

// --- Category-only filter ---

func TestRelevance_RunningShoes(t *testing.T) {
	catalog, emb := setupRelevanceTest(t)
	tenantID := getTenantID(t, catalog, "sportmaster")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	vec := embed(t, emb, "беговые кроссовки")
	vf := &ports.VectorFilter{CategoryName: "Running"}

	products, err := catalog.VectorSearch(ctx, tenantID, vec, 10, vf)
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	if len(products) == 0 {
		t.Fatal("expected running shoe results, got 0")
	}

	runningCategories := map[string]bool{
		"running": true, "running shoes": true,
	}
	top5 := min(5, len(products))
	runningCount := 0
	for _, p := range products[:top5] {
		if runningCategories[strings.ToLower(p.Category)] {
			runningCount++
		}
	}
	if runningCount < top5/2 {
		t.Errorf("expected running shoes to dominate top-5, only got %d/%d", runningCount, top5)
	}
	logResults(t, "Running shoes", products)
}

// --- Keyword + price filtering ---

func TestRelevance_CheapSmartphone(t *testing.T) {
	catalog, emb := setupRelevanceTest(t)
	tenantID := getTenantID(t, catalog, "techstore")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	vec := embed(t, emb, "недорогой смартфон")
	vf := &ports.VectorFilter{CategoryName: "Smartphones"}

	vectorProducts, err := catalog.VectorSearch(ctx, tenantID, vec, 20, vf)
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}

	keywordProducts, _, err := catalog.ListProducts(ctx, tenantID, ports.ProductFilter{
		CategoryName: "Smartphones",
		MaxPrice:     5000000, // 50,000 RUB
		Limit:        20,
	})
	if err != nil {
		t.Fatalf("keyword search: %v", err)
	}

	if len(keywordProducts) == 0 && len(vectorProducts) == 0 {
		t.Fatal("expected some smartphone results from at least one search path")
	}

	for _, p := range keywordProducts {
		if p.Price > 5000000 {
			t.Errorf("price filter violated: %d > 5000000 kopecks for %s", p.Price, p.Name)
		}
	}

	logResults(t, "Cheap smartphones (vector)", vectorProducts)
	logResults(t, "Cheap smartphones (keyword)", keywordProducts)
}

// --- Service search (semantic, no filters) ---

func TestRelevance_HaircutService(t *testing.T) {
	catalog, emb := setupRelevanceTest(t)
	tenantID := getTenantID(t, catalog, "beautylab")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	vec := embed(t, emb, "мужская стрижка")
	products, err := catalog.VectorSearch(ctx, tenantID, vec, 10, nil)
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	if len(products) == 0 {
		t.Fatal("expected haircut service results, got 0")
	}

	hasHaircut := false
	for _, p := range products[:min(5, len(products))] {
		nameLower := strings.ToLower(p.Name)
		if strings.Contains(nameLower, "haircut") || strings.Contains(nameLower, "trim") ||
			strings.Contains(nameLower, "styling") || strings.Contains(nameLower, "hair") {
			hasHaircut = true
		}
	}
	if !hasHaircut {
		t.Error("expected haircut/hair service in top-5")
	}
	logResults(t, "Haircut service", products)
}

func TestRelevance_NailCareService(t *testing.T) {
	catalog, emb := setupRelevanceTest(t)
	tenantID := getTenantID(t, catalog, "beautylab")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	vec := embed(t, emb, "маникюр")
	products, err := catalog.VectorSearch(ctx, tenantID, vec, 10, nil)
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	if len(products) == 0 {
		t.Fatal("expected nail care results, got 0")
	}

	hasNail := false
	for _, p := range products[:min(5, len(products))] {
		nameLower := strings.ToLower(p.Name)
		if strings.Contains(nameLower, "nail") || strings.Contains(nameLower, "manicure") ||
			strings.Contains(nameLower, "pedicure") || strings.Contains(nameLower, "gel") {
			hasNail = true
		}
	}
	if !hasNail {
		t.Error("expected nail care service in top-5 for 'маникюр' query")
	}
	logResults(t, "Nail care (маникюр)", products)
}

// --- Cross-category semantic search (no category filter) ---

func TestRelevance_CrossCategory_ForRunning(t *testing.T) {
	catalog, emb := setupRelevanceTest(t)
	tenantID := getTenantID(t, catalog, "sportmaster")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Broad query — should NOT be limited to one category
	vec := embed(t, emb, "для бега")
	products, err := catalog.VectorSearch(ctx, tenantID, vec, 20, nil)
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	if len(products) == 0 {
		t.Fatal("expected cross-category results for 'для бега', got 0")
	}

	// Should find items from multiple categories (shoes, clothing, etc.)
	categorySet := make(map[string]int)
	for _, p := range products {
		categorySet[p.Category]++
	}

	if len(categorySet) < 2 {
		t.Errorf("expected results from 2+ categories for broad query, got %d categories: %v", len(categorySet), categorySet)
	}

	t.Logf("Cross-category 'для бега': %d categories found", len(categorySet))
	for cat, count := range categorySet {
		t.Logf("  %s: %d results", cat, count)
	}
	logResults(t, "Cross-category running", products)
}

// --- Color semantic search ---

func TestRelevance_ColorSemantic(t *testing.T) {
	catalog, emb := setupRelevanceTest(t)
	tenantID := getTenantID(t, catalog, "sportmaster")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// "чёрные кроссовки" — color in vector, category in filter
	vec := embed(t, emb, "чёрные кроссовки")
	vf := &ports.VectorFilter{CategoryName: "Casual Shoes"}

	products, err := catalog.VectorSearch(ctx, tenantID, vec, 10, vf)
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	if len(products) == 0 {
		t.Fatal("expected results for 'чёрные кроссовки', got 0")
	}

	// At least some results should be shoe-related
	shoeCount := 0
	for _, p := range products {
		catLower := strings.ToLower(p.Category)
		if strings.Contains(catLower, "casual") || strings.Contains(catLower, "shoe") ||
			strings.Contains(catLower, "running") || strings.Contains(catLower, "basketball") {
			shoeCount++
		}
	}
	if shoeCount == 0 {
		t.Error("expected at least some shoe products for 'чёрные кроссовки'")
	}
	logResults(t, "Black sneakers (color semantic)", products)
}

// --- Russian synonym search ---

func TestRelevance_RussianSynonym_Nout(t *testing.T) {
	catalog, emb := setupRelevanceTest(t)
	tenantID := getTenantID(t, catalog, "techstore")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// "ноут" is colloquial Russian for "laptop"
	vec := embed(t, emb, "ноут для работы")
	products, err := catalog.VectorSearch(ctx, tenantID, vec, 10, nil)
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	if len(products) == 0 {
		t.Fatal("expected results for 'ноут для работы', got 0")
	}

	// At least some results should be laptops
	laptopCount := 0
	for _, p := range products[:min(5, len(products))] {
		catLower := strings.ToLower(p.Category)
		nameLower := strings.ToLower(p.Name)
		if strings.Contains(catLower, "laptop") || strings.Contains(nameLower, "macbook") ||
			strings.Contains(nameLower, "thinkpad") || strings.Contains(nameLower, "zenbook") ||
			strings.Contains(nameLower, "book") {
			laptopCount++
		}
	}
	if laptopCount == 0 {
		t.Error("expected laptops in top-5 for 'ноут для работы'")
	}
	logResults(t, "Russian 'ноут' → laptops", products)
}

// --- Tenant isolation ---

func TestRelevance_TenantIsolation(t *testing.T) {
	catalog, emb := setupRelevanceTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// beautylab is services-only, searching for "Nike" should return 0 or irrelevant results
	tenantID := getTenantID(t, catalog, "beautylab")
	vec := embed(t, emb, "кроссовки Nike")
	products, err := catalog.VectorSearch(ctx, tenantID, vec, 10, nil)
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}

	for _, p := range products {
		if strings.EqualFold(p.Brand, "Nike") {
			t.Errorf("Nike product %q found in beautylab tenant — isolation broken!", p.Name)
		}
		catLower := strings.ToLower(p.Category)
		if strings.Contains(catLower, "sneaker") || strings.Contains(catLower, "running") {
			t.Errorf("shoe category %q found in beautylab — isolation broken!", p.Category)
		}
	}
	logResults(t, "beautylab (no Nike expected)", products)
}

// --- Nonexistent brand ---

func TestRelevance_NonexistentBrand(t *testing.T) {
	catalog, emb := setupRelevanceTest(t)
	tenantID := getTenantID(t, catalog, "sportmaster")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	vec := embed(t, emb, "кроссовки Balenciaga")
	vf := &ports.VectorFilter{Brand: "Balenciaga"}

	products, err := catalog.VectorSearch(ctx, tenantID, vec, 10, vf)
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}

	if len(products) > 0 {
		t.Logf("got %d results for nonexistent brand Balenciaga (may be semantic noise)", len(products))
		for _, p := range products {
			if strings.EqualFold(p.Brand, "Balenciaga") {
				t.Errorf("Balenciaga should not exist in seed data, but found %q", p.Name)
			}
		}
	} else {
		t.Log("correctly returned 0 results for nonexistent brand")
	}
}

// --- Combined filters (brand + category + price) ---

func TestRelevance_CombinedFilters(t *testing.T) {
	catalog, emb := setupRelevanceTest(t)
	tenantID := getTenantID(t, catalog, "sportmaster")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Vector search with brand + category filter
	vec := embed(t, emb, "Nike кроссовки")
	vf := &ports.VectorFilter{Brand: "Nike", CategoryName: "Casual Shoes"}
	vectorProducts, err := catalog.VectorSearch(ctx, tenantID, vec, 10, vf)
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}

	// Keyword search with brand + category + price
	keywordProducts, total, err := catalog.ListProducts(ctx, tenantID, ports.ProductFilter{
		Brand:        "Nike",
		CategoryName: "Casual Shoes",
		MaxPrice:     1500000, // 15,000 RUB
		Limit:        10,
	})
	if err != nil {
		t.Fatalf("keyword search: %v", err)
	}

	t.Logf("Combined: vector=%d, keyword=%d (total matching: %d)", len(vectorProducts), len(keywordProducts), total)

	// Keyword results must respect ALL filters
	for _, p := range keywordProducts {
		if !strings.Contains(strings.ToLower(p.Brand), "nike") {
			t.Errorf("brand filter broken: got %q", p.Brand)
		}
		if p.Price > 1500000 {
			t.Errorf("price filter broken: %d > 1500000 for %s", p.Price, p.Name)
		}
	}

	logResults(t, "Combined vector", vectorProducts)
	logResults(t, "Combined keyword", keywordProducts)
}

// --- Keyword search: JSONB attribute filter ---

func TestRelevance_AttributeFilter_Color(t *testing.T) {
	catalog, _ := setupRelevanceTest(t)
	tenantID := getTenantID(t, catalog, "sportmaster")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	products, total, err := catalog.ListProducts(ctx, tenantID, ports.ProductFilter{
		CategoryName: "Casual Shoes",
		Attributes:   map[string]string{"color": "Black"},
		Limit:        20,
	})
	if err != nil {
		t.Fatalf("keyword search with color attribute: %v", err)
	}

	if len(products) == 0 {
		t.Fatal("expected results for color=Black in Casual Shoes, got 0")
	}

	t.Logf("Color=Black casual shoes: %d results (total: %d)", len(products), total)
	logResults(t, "Black sneakers (JSONB filter)", products)
}

// --- Cost summary ---

func TestRelevance_CostSummary(t *testing.T) {
	// This test doesn't call APIs — it just summarizes expected costs
	// based on the number of embedding calls in this test suite.

	embeddingTests := 14 // approximate count of tests that call embed()
	avgTokensPerQuery := 10
	pricePerMToken := 0.02 // text-embedding-3-small

	totalTokens := embeddingTests * avgTokensPerQuery
	totalCost := float64(totalTokens) * pricePerMToken / 1_000_000

	t.Logf("=== Cost Summary ===")
	t.Logf("Embedding calls: ~%d", embeddingTests)
	t.Logf("Avg tokens/call: ~%d", avgTokensPerQuery)
	t.Logf("Total tokens: ~%d", totalTokens)
	t.Logf("Estimated cost: ~$%.6f (practically free)", totalCost)
	t.Logf("Model: text-embedding-3-small @ $0.02/1M tokens")
	t.Logf("Note: DB queries to Neon are included in free tier")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
