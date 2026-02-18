package tools_test

import (
	"context"
	"testing"
	"time"

	"keepstar/internal/domain"
	"keepstar/internal/ports"
	"keepstar/internal/tools"
)

// --- Mock StatePort ---

type mockStatePort struct {
	state *domain.SessionState

	UpdateDataCalls     int
	UpdateTemplateCalls int
	UpdateStateCalls    int
	AddDeltaCalls       int
	LastDeltaInfo       *domain.Delta
}

func newMockStatePort(state *domain.SessionState) *mockStatePort {
	return &mockStatePort{state: state}
}

func (m *mockStatePort) CreateState(_ context.Context, sessionID string) (*domain.SessionState, error) {
	m.state = &domain.SessionState{
		ID: "state-1", SessionID: sessionID,
		Current:   domain.StateCurrent{},
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	return m.state, nil
}
func (m *mockStatePort) GetState(_ context.Context, _ string) (*domain.SessionState, error) {
	if m.state == nil {
		return nil, domain.ErrSessionNotFound
	}
	return m.state, nil
}
func (m *mockStatePort) UpdateState(_ context.Context, state *domain.SessionState) error {
	m.UpdateStateCalls++
	m.state = state
	return nil
}
func (m *mockStatePort) AddDelta(_ context.Context, _ string, delta *domain.Delta) (int, error) {
	m.AddDeltaCalls++
	m.LastDeltaInfo = delta
	return 1, nil
}
func (m *mockStatePort) GetDeltas(_ context.Context, _ string) ([]domain.Delta, error) {
	return nil, nil
}
func (m *mockStatePort) GetDeltasSince(_ context.Context, _ string, _ int) ([]domain.Delta, error) {
	return nil, nil
}
func (m *mockStatePort) GetDeltasUntil(_ context.Context, _ string, _ int) ([]domain.Delta, error) {
	return nil, nil
}
func (m *mockStatePort) UpdateData(_ context.Context, _ string, data domain.StateData, meta domain.StateMeta, info domain.DeltaInfo) (int, error) {
	m.UpdateDataCalls++
	if m.state != nil {
		m.state.Current.Data = data
		m.state.Current.Meta = meta
	}
	m.LastDeltaInfo = info.ToDelta()
	return 1, nil
}
func (m *mockStatePort) UpdateTemplate(_ context.Context, _ string, template map[string]interface{}, info domain.DeltaInfo) (int, error) {
	m.UpdateTemplateCalls++
	if m.state != nil {
		m.state.Current.Template = template
	}
	m.LastDeltaInfo = info.ToDelta()
	return 1, nil
}
func (m *mockStatePort) UpdateView(_ context.Context, _ string, _ domain.ViewState, _ []domain.ViewSnapshot, _ domain.DeltaInfo) (int, error) {
	return 1, nil
}
func (m *mockStatePort) AppendConversation(_ context.Context, _ string, _ []domain.LLMMessage) error {
	return nil
}
func (m *mockStatePort) PushView(_ context.Context, _ string, _ *domain.ViewSnapshot) error {
	return nil
}
func (m *mockStatePort) PopView(_ context.Context, _ string) (*domain.ViewSnapshot, error) {
	return nil, nil
}
func (m *mockStatePort) GetViewStack(_ context.Context, _ string) ([]domain.ViewSnapshot, error) {
	return nil, nil
}

// --- Mock CatalogPort ---

type mockCatalogPort struct {
	products       []domain.Product
	total          int
	vectorProducts []domain.Product
}

func (m *mockCatalogPort) GetTenantBySlug(_ context.Context, slug string) (*domain.Tenant, error) {
	return &domain.Tenant{ID: "t1", Slug: slug}, nil
}
func (m *mockCatalogPort) GetCategories(_ context.Context) ([]domain.Category, error) {
	return nil, nil
}
func (m *mockCatalogPort) GetMasterProduct(_ context.Context, _ string) (*domain.MasterProduct, error) {
	return nil, nil
}
func (m *mockCatalogPort) ListProducts(_ context.Context, _ string, _ ports.ProductFilter) ([]domain.Product, int, error) {
	return m.products, m.total, nil
}
func (m *mockCatalogPort) GetProduct(_ context.Context, _ string, _ string) (*domain.Product, error) {
	return nil, nil
}
func (m *mockCatalogPort) VectorSearch(_ context.Context, _ string, _ []float32, _ int, _ *ports.VectorFilter) ([]domain.Product, error) {
	return m.vectorProducts, nil
}
func (m *mockCatalogPort) SeedEmbedding(_ context.Context, _ string, _ []float32) error {
	return nil
}
func (m *mockCatalogPort) GetMasterProductsWithoutEmbedding(_ context.Context) ([]domain.MasterProduct, error) {
	return nil, nil
}
func (m *mockCatalogPort) GenerateCatalogDigest(_ context.Context, _ string) (*domain.CatalogDigest, error) {
	return nil, nil
}
func (m *mockCatalogPort) GetCatalogDigest(_ context.Context, _ string) (*domain.CatalogDigest, error) {
	return nil, nil
}
func (m *mockCatalogPort) SaveCatalogDigest(_ context.Context, _ string, _ *domain.CatalogDigest) error {
	return nil
}
func (m *mockCatalogPort) GetAllTenants(_ context.Context) ([]domain.Tenant, error) {
	return nil, nil
}
func (m *mockCatalogPort) GetStock(_ context.Context, _ string, _ string) (*domain.Stock, error) {
	return nil, nil
}
func (m *mockCatalogPort) ListServices(_ context.Context, _ string, _ ports.ProductFilter) ([]domain.Service, int, error) {
	return nil, 0, nil
}
func (m *mockCatalogPort) GetService(_ context.Context, _ string, _ string) (*domain.Service, error) {
	return nil, nil
}
func (m *mockCatalogPort) VectorSearchServices(_ context.Context, _ string, _ []float32, _ int, _ *ports.VectorFilter) ([]domain.Service, error) {
	return nil, nil
}
func (m *mockCatalogPort) GetMasterServicesWithoutEmbedding(_ context.Context) ([]domain.MasterService, error) {
	return nil, nil
}
func (m *mockCatalogPort) SeedServiceEmbedding(_ context.Context, _ string, _ []float32) error {
	return nil
}

// --- Mock CatalogPort with capture ---

type mockCatalogPortCapture struct {
	products       []domain.Product
	total          int
	vectorProducts []domain.Product
	captureFilter  *ports.ProductFilter
	captureVF      *ports.VectorFilter // captured vector filter
}

func (m *mockCatalogPortCapture) GetTenantBySlug(_ context.Context, slug string) (*domain.Tenant, error) {
	return &domain.Tenant{ID: "t1", Slug: slug}, nil
}
func (m *mockCatalogPortCapture) GetCategories(_ context.Context) ([]domain.Category, error) {
	return nil, nil
}
func (m *mockCatalogPortCapture) GetMasterProduct(_ context.Context, _ string) (*domain.MasterProduct, error) {
	return nil, nil
}
func (m *mockCatalogPortCapture) ListProducts(_ context.Context, _ string, filter ports.ProductFilter) ([]domain.Product, int, error) {
	if m.captureFilter != nil {
		*m.captureFilter = filter
	}
	return m.products, m.total, nil
}
func (m *mockCatalogPortCapture) GetProduct(_ context.Context, _ string, _ string) (*domain.Product, error) {
	return nil, nil
}
func (m *mockCatalogPortCapture) VectorSearch(_ context.Context, _ string, _ []float32, _ int, vf *ports.VectorFilter) ([]domain.Product, error) {
	m.captureVF = vf
	return m.vectorProducts, nil
}
func (m *mockCatalogPortCapture) SeedEmbedding(_ context.Context, _ string, _ []float32) error {
	return nil
}
func (m *mockCatalogPortCapture) GetMasterProductsWithoutEmbedding(_ context.Context) ([]domain.MasterProduct, error) {
	return nil, nil
}
func (m *mockCatalogPortCapture) GenerateCatalogDigest(_ context.Context, _ string) (*domain.CatalogDigest, error) {
	return nil, nil
}
func (m *mockCatalogPortCapture) GetCatalogDigest(_ context.Context, _ string) (*domain.CatalogDigest, error) {
	return nil, nil
}
func (m *mockCatalogPortCapture) SaveCatalogDigest(_ context.Context, _ string, _ *domain.CatalogDigest) error {
	return nil
}
func (m *mockCatalogPortCapture) GetAllTenants(_ context.Context) ([]domain.Tenant, error) {
	return nil, nil
}
func (m *mockCatalogPortCapture) GetStock(_ context.Context, _ string, _ string) (*domain.Stock, error) {
	return nil, nil
}
func (m *mockCatalogPortCapture) ListServices(_ context.Context, _ string, _ ports.ProductFilter) ([]domain.Service, int, error) {
	return nil, 0, nil
}
func (m *mockCatalogPortCapture) GetService(_ context.Context, _ string, _ string) (*domain.Service, error) {
	return nil, nil
}
func (m *mockCatalogPortCapture) VectorSearchServices(_ context.Context, _ string, _ []float32, _ int, _ *ports.VectorFilter) ([]domain.Service, error) {
	return nil, nil
}
func (m *mockCatalogPortCapture) GetMasterServicesWithoutEmbedding(_ context.Context) ([]domain.MasterService, error) {
	return nil, nil
}
func (m *mockCatalogPortCapture) SeedServiceEmbedding(_ context.Context, _ string, _ []float32) error {
	return nil
}

// --- Mock EmbeddingPort ---

type mockEmbeddingPort struct{}

func (m *mockEmbeddingPort) Embed(_ context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i := range texts {
		result[i] = make([]float32, 384)
		result[i][0] = float32(i) + 0.1 // unique-ish
	}
	return result, nil
}

// --- Helpers ---

func defaultState() *domain.SessionState {
	return &domain.SessionState{
		ID: "s1", SessionID: "sess-1",
		Current: domain.StateCurrent{
			Meta: domain.StateMeta{Aliases: map[string]string{"tenant_slug": "nike"}},
		},
	}
}

func defaultToolCtx() tools.ToolContext {
	return tools.ToolContext{SessionID: "sess-1", TurnID: "turn-1", ActorID: "agent1"}
}

// =============================================================================
// Tests
// =============================================================================

func TestCatalogSearch_HybridMerge(t *testing.T) {
	sp := newMockStatePort(defaultState())
	cp := &mockCatalogPort{
		products: []domain.Product{
			{ID: "p1", Name: "Nike Pegasus 41", Price: 1399000, Brand: "Nike", Category: "Running Shoes"},
			{ID: "p2", Name: "Nike Vomero 17", Price: 1599000, Brand: "Nike", Category: "Running Shoes"},
		},
		total: 2,
		vectorProducts: []domain.Product{
			{ID: "p1", Name: "Nike Pegasus 41", Price: 1399000, Brand: "Nike", Category: "Running Shoes"},
			{ID: "p3", Name: "Adidas Ultraboost 24", Price: 1899000, Brand: "Adidas", Category: "Running Shoes"},
		},
	}
	emb := &mockEmbeddingPort{}
	tool := tools.NewCatalogSearchTool(sp, cp, emb)

	result, err := tool.Execute(context.Background(), defaultToolCtx(), map[string]interface{}{
		"vector_query": "running shoes Nike",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error: %s", result.Content)
	}

	// UpdateData must be called (hybrid merge produced results)
	if sp.UpdateDataCalls != 1 {
		t.Errorf("expected 1 UpdateData call, got %d", sp.UpdateDataCalls)
	}

	// p1 appears in BOTH keyword and vector → highest RRF score
	products := sp.state.Current.Data.Products
	if len(products) == 0 {
		t.Fatal("expected merged products, got 0")
	}
	if products[0].ID != "p1" {
		t.Errorf("expected p1 (appears in both lists) to be ranked #1, got %s", products[0].ID)
	}
}

func TestCatalogSearch_KeywordOnly(t *testing.T) {
	sp := newMockStatePort(defaultState())
	cp := &mockCatalogPort{
		products: []domain.Product{
			{ID: "p1", Name: "Nike Air Max 90", Price: 1299000, Brand: "Nike"},
		},
		total: 1,
	}
	// nil embedding port → keyword-only mode
	tool := tools.NewCatalogSearchTool(sp, cp, nil)

	result, err := tool.Execute(context.Background(), defaultToolCtx(), map[string]interface{}{
		"vector_query": "air max",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool error: %s", result.Content)
	}

	if sp.UpdateDataCalls != 1 {
		t.Errorf("expected 1 UpdateData call, got %d", sp.UpdateDataCalls)
	}
	if len(sp.state.Current.Data.Products) != 1 {
		t.Errorf("expected 1 product, got %d", len(sp.state.Current.Data.Products))
	}
}

func TestCatalogSearch_EmptyPreservesState(t *testing.T) {
	state := &domain.SessionState{
		ID: "s1", SessionID: "sess-1",
		Current: domain.StateCurrent{
			Data: domain.StateData{
				Products: []domain.Product{{ID: "p1", Name: "Existing Product"}},
			},
			Meta: domain.StateMeta{Count: 1, Aliases: map[string]string{"tenant_slug": "nike"}},
		},
	}
	sp := newMockStatePort(state)
	cp := &mockCatalogPort{products: nil, total: 0}
	tool := tools.NewCatalogSearchTool(sp, cp, nil)

	result, err := tool.Execute(context.Background(), defaultToolCtx(), map[string]interface{}{
		"vector_query": "nonexistent product xyz",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Data zone must NOT be overwritten
	if sp.UpdateDataCalls != 0 {
		t.Errorf("expected 0 UpdateData calls (empty preserves data), got %d", sp.UpdateDataCalls)
	}

	// AddDelta must be called with count:0
	if sp.AddDeltaCalls != 1 {
		t.Errorf("expected 1 AddDelta call for empty marker, got %d", sp.AddDeltaCalls)
	}

	// State products must still be there
	if len(state.Current.Data.Products) != 1 {
		t.Errorf("expected 1 product preserved, got %d", len(state.Current.Data.Products))
	}

	if result.Content != "empty: 0 results, previous data preserved" {
		t.Errorf("unexpected result content: %s", result.Content)
	}
}

func TestCatalogSearch_BrandStrippedFromSearch(t *testing.T) {
	sp := newMockStatePort(defaultState())
	var capturedFilter ports.ProductFilter
	cp := &mockCatalogPortCapture{
		products: []domain.Product{
			{ID: "p1", Name: "Nike Pegasus 41", Price: 1399000, Brand: "Nike"},
		},
		total:         1,
		captureFilter: &capturedFilter,
	}
	tool := tools.NewCatalogSearchTool(sp, cp, nil)

	_, err := tool.Execute(context.Background(), defaultToolCtx(), map[string]interface{}{
		"vector_query": "Nike Pegasus",
		"filters": map[string]interface{}{
			"brand": "Nike",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedFilter.Brand != "Nike" {
		t.Errorf("expected Brand=Nike, got %s", capturedFilter.Brand)
	}
	if capturedFilter.Search != "Pegasus" {
		t.Errorf("expected Search=Pegasus (brand stripped), got %q", capturedFilter.Search)
	}
}

func TestCatalogSearch_FiltersPassed(t *testing.T) {
	sp := newMockStatePort(defaultState())
	var capturedFilter ports.ProductFilter
	cp := &mockCatalogPortCapture{
		products: []domain.Product{
			{ID: "p1", Name: "Nike Pegasus 41", Price: 1399000, Brand: "Nike", Category: "Running Shoes"},
		},
		total:         1,
		captureFilter: &capturedFilter,
	}
	tool := tools.NewCatalogSearchTool(sp, cp, nil)

	_, err := tool.Execute(context.Background(), defaultToolCtx(), map[string]interface{}{
		"vector_query": "running shoes",
		"filters": map[string]interface{}{
			"brand":     "Nike",
			"category":  "Running Shoes",
			"min_price": float64(5000),
			"max_price": float64(20000),
			"color":     "Blue",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedFilter.Brand != "Nike" {
		t.Errorf("expected Brand=Nike, got %s", capturedFilter.Brand)
	}
	if capturedFilter.CategoryName != "Running Shoes" {
		t.Errorf("expected CategoryName=Running Shoes, got %s", capturedFilter.CategoryName)
	}
	if capturedFilter.MinPrice != 500000 {
		t.Errorf("expected MinPrice=500000 (kopecks), got %d", capturedFilter.MinPrice)
	}
	if capturedFilter.MaxPrice != 2000000 {
		t.Errorf("expected MaxPrice=2000000 (kopecks), got %d", capturedFilter.MaxPrice)
	}
}

func TestCatalogSearch_FiltersPassedToVector(t *testing.T) {
	sp := newMockStatePort(defaultState())
	cp := &mockCatalogPortCapture{
		products: []domain.Product{
			{ID: "p1", Name: "Nike Pegasus 41", Price: 1399000, Brand: "Nike"},
		},
		total:          1,
		vectorProducts: []domain.Product{},
	}
	emb := &mockEmbeddingPort{}
	tool := tools.NewCatalogSearchTool(sp, cp, emb)

	_, err := tool.Execute(context.Background(), defaultToolCtx(), map[string]interface{}{
		"vector_query": "running shoes",
		"filters": map[string]interface{}{
			"brand":    "Nike",
			"category": "Running Shoes",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// VectorFilter must have been passed
	if cp.captureVF == nil {
		t.Fatal("expected VectorFilter to be passed, got nil")
	}
	if cp.captureVF.Brand != "Nike" {
		t.Errorf("expected VectorFilter.Brand=Nike, got %s", cp.captureVF.Brand)
	}
	if cp.captureVF.CategoryName != "Running Shoes" {
		t.Errorf("expected VectorFilter.CategoryName=Running Shoes, got %s", cp.captureVF.CategoryName)
	}
}

func TestCatalogSearch_RRFWeightsKeywordHigher(t *testing.T) {
	sp := newMockStatePort(defaultState())

	// p1 is rank 0 in keyword only, p2 is rank 0 in vector only
	// With keyword weight 1.5x, p1 should rank higher than p2
	cp := &mockCatalogPort{
		products: []domain.Product{
			{ID: "p1", Name: "Nike Pegasus 41", Price: 1399000, Brand: "Nike"},
		},
		total: 1,
		vectorProducts: []domain.Product{
			{ID: "p2", Name: "Adidas Ultraboost 24", Price: 1899000, Brand: "Adidas"},
		},
	}
	emb := &mockEmbeddingPort{}
	tool := tools.NewCatalogSearchTool(sp, cp, emb)

	_, err := tool.Execute(context.Background(), defaultToolCtx(), map[string]interface{}{
		"vector_query": "running shoes",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	products := sp.state.Current.Data.Products
	if len(products) < 2 {
		t.Fatalf("expected at least 2 products, got %d", len(products))
	}

	// Keyword rank-0 product (p1) should be first because keyword weight > vector weight
	// keyword score for p1: 1.5/(60+0+1) = 1.5/61 ≈ 0.02459
	// vector score for p2: 1.0/(60+0+1) = 1.0/61 ≈ 0.01639
	if products[0].ID != "p1" {
		t.Errorf("expected keyword-matched p1 to rank first (keyword weight 1.5x), got %s", products[0].ID)
	}
}

func TestCatalogSearch_PriceConversion(t *testing.T) {
	sp := newMockStatePort(defaultState())
	var capturedFilter ports.ProductFilter
	cp := &mockCatalogPortCapture{
		products:      nil,
		total:         0,
		captureFilter: &capturedFilter,
	}
	tool := tools.NewCatalogSearchTool(sp, cp, nil)

	_, err := tool.Execute(context.Background(), defaultToolCtx(), map[string]interface{}{
		"vector_query": "shoes",
		"filters": map[string]interface{}{
			"min_price": float64(5000),
			"max_price": float64(15000),
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Prices should be converted from rubles to kopecks (×100)
	if capturedFilter.MinPrice != 500000 {
		t.Errorf("expected MinPrice=500000 kopecks, got %d", capturedFilter.MinPrice)
	}
	if capturedFilter.MaxPrice != 1500000 {
		t.Errorf("expected MaxPrice=1500000 kopecks, got %d", capturedFilter.MaxPrice)
	}
}

func TestCatalogSearch_TenantFromAliases(t *testing.T) {
	state := &domain.SessionState{
		ID: "s1", SessionID: "sess-1",
		Current: domain.StateCurrent{
			Meta: domain.StateMeta{Aliases: map[string]string{"tenant_slug": "techstore"}},
		},
	}
	sp := newMockStatePort(state)

	var tenantSlugSeen string
	cp := &mockCatalogPort{
		products: []domain.Product{{ID: "p1", Name: "MacBook Air", Price: 14999000, Brand: "Apple"}},
		total:    1,
	}
	// Wrap to capture tenant slug
	wrappedCP := &tenantCaptureCatalogPort{inner: cp, slugCapture: &tenantSlugSeen}

	tool := tools.NewCatalogSearchTool(sp, wrappedCP, nil)

	_, err := tool.Execute(context.Background(), defaultToolCtx(), map[string]interface{}{
		"vector_query": "macbook",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tenantSlugSeen != "techstore" {
		t.Errorf("expected tenant slug=techstore, got %s", tenantSlugSeen)
	}
}

// tenantCaptureCatalogPort wraps a CatalogPort and captures the tenant slug passed to GetTenantBySlug
type tenantCaptureCatalogPort struct {
	inner       *mockCatalogPort
	slugCapture *string
}

func (m *tenantCaptureCatalogPort) GetTenantBySlug(ctx context.Context, slug string) (*domain.Tenant, error) {
	*m.slugCapture = slug
	return m.inner.GetTenantBySlug(ctx, slug)
}
func (m *tenantCaptureCatalogPort) GetCategories(ctx context.Context) ([]domain.Category, error) {
	return m.inner.GetCategories(ctx)
}
func (m *tenantCaptureCatalogPort) GetMasterProduct(ctx context.Context, id string) (*domain.MasterProduct, error) {
	return m.inner.GetMasterProduct(ctx, id)
}
func (m *tenantCaptureCatalogPort) ListProducts(ctx context.Context, tenantID string, filter ports.ProductFilter) ([]domain.Product, int, error) {
	return m.inner.ListProducts(ctx, tenantID, filter)
}
func (m *tenantCaptureCatalogPort) GetProduct(ctx context.Context, tenantID string, productID string) (*domain.Product, error) {
	return m.inner.GetProduct(ctx, tenantID, productID)
}
func (m *tenantCaptureCatalogPort) VectorSearch(ctx context.Context, tenantID string, embedding []float32, limit int, filter *ports.VectorFilter) ([]domain.Product, error) {
	return m.inner.VectorSearch(ctx, tenantID, embedding, limit, filter)
}
func (m *tenantCaptureCatalogPort) SeedEmbedding(ctx context.Context, masterProductID string, embedding []float32) error {
	return m.inner.SeedEmbedding(ctx, masterProductID, embedding)
}
func (m *tenantCaptureCatalogPort) GetMasterProductsWithoutEmbedding(ctx context.Context) ([]domain.MasterProduct, error) {
	return m.inner.GetMasterProductsWithoutEmbedding(ctx)
}
func (m *tenantCaptureCatalogPort) GenerateCatalogDigest(ctx context.Context, tenantID string) (*domain.CatalogDigest, error) {
	return m.inner.GenerateCatalogDigest(ctx, tenantID)
}
func (m *tenantCaptureCatalogPort) GetCatalogDigest(ctx context.Context, tenantID string) (*domain.CatalogDigest, error) {
	return m.inner.GetCatalogDigest(ctx, tenantID)
}
func (m *tenantCaptureCatalogPort) SaveCatalogDigest(ctx context.Context, tenantID string, digest *domain.CatalogDigest) error {
	return m.inner.SaveCatalogDigest(ctx, tenantID, digest)
}
func (m *tenantCaptureCatalogPort) GetAllTenants(ctx context.Context) ([]domain.Tenant, error) {
	return m.inner.GetAllTenants(ctx)
}
func (m *tenantCaptureCatalogPort) GetStock(ctx context.Context, tenantID string, productID string) (*domain.Stock, error) {
	return m.inner.GetStock(ctx, tenantID, productID)
}
func (m *tenantCaptureCatalogPort) ListServices(ctx context.Context, tenantID string, filter ports.ProductFilter) ([]domain.Service, int, error) {
	return m.inner.ListServices(ctx, tenantID, filter)
}
func (m *tenantCaptureCatalogPort) GetService(ctx context.Context, tenantID string, serviceID string) (*domain.Service, error) {
	return m.inner.GetService(ctx, tenantID, serviceID)
}
func (m *tenantCaptureCatalogPort) VectorSearchServices(ctx context.Context, tenantID string, embedding []float32, limit int, filter *ports.VectorFilter) ([]domain.Service, error) {
	return m.inner.VectorSearchServices(ctx, tenantID, embedding, limit, filter)
}
func (m *tenantCaptureCatalogPort) GetMasterServicesWithoutEmbedding(ctx context.Context) ([]domain.MasterService, error) {
	return m.inner.GetMasterServicesWithoutEmbedding(ctx)
}
func (m *tenantCaptureCatalogPort) SeedServiceEmbedding(ctx context.Context, masterServiceID string, embedding []float32) error {
	return m.inner.SeedServiceEmbedding(ctx, masterServiceID, embedding)
}
