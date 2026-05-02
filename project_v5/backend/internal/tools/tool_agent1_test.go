package tools

import (
	"context"
	"testing"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// agent1StatePort is a focused fake covering only the StatePort methods the
// three Agent1 tools call. Other methods panic — any accidental call is a
// regression in the tool's contract.
type agent1StatePort struct {
	state    *domain.SessionState
	deltas   []*domain.Delta
	updateOK bool
	dataSeen domain.StateData
	metaSeen domain.StateMeta
}

func newAgent1StatePort(products []domain.Product) *agent1StatePort {
	return &agent1StatePort{
		state: &domain.SessionState{
			SessionID: "sess-1",
			Current: domain.StateCurrent{
				Data: domain.StateData{Products: products},
				Meta: domain.StateMeta{
					Aliases: map[string]string{"tenant_slug": "acme"},
				},
			},
		},
	}
}

func (m *agent1StatePort) GetState(_ context.Context, _ string) (*domain.SessionState, error) {
	cp := *m.state
	return &cp, nil
}

func (m *agent1StatePort) AddDelta(_ context.Context, _ string, d *domain.Delta) (int, error) {
	d.Step = len(m.deltas) + 1
	m.deltas = append(m.deltas, d)
	return d.Step, nil
}

func (m *agent1StatePort) UpdateData(_ context.Context, _ string, data domain.StateData, meta domain.StateMeta, info domain.DeltaInfo) (int, error) {
	m.updateOK = true
	m.dataSeen = data
	m.metaSeen = meta
	d := info.ToDelta()
	d.Step = len(m.deltas) + 1
	m.deltas = append(m.deltas, d)
	m.state.Current.Data = data
	m.state.Current.Meta = meta
	return d.Step, nil
}

func (m *agent1StatePort) GetDeltas(_ context.Context, _ string) ([]domain.Delta, error) {
	out := make([]domain.Delta, 0, len(m.deltas))
	for _, d := range m.deltas {
		out = append(out, *d)
	}
	return out, nil
}

// All other StatePort methods panic — these tools must not call them.
func (m *agent1StatePort) CreateState(context.Context, string) (*domain.SessionState, error) {
	panic("not used")
}
func (m *agent1StatePort) UpdateState(context.Context, *domain.SessionState) error {
	panic("not used")
}
func (m *agent1StatePort) GetDeltasSince(context.Context, string, int) ([]domain.Delta, error) {
	panic("not used")
}
func (m *agent1StatePort) GetDeltasUntil(context.Context, string, int) ([]domain.Delta, error) {
	panic("not used")
}
func (m *agent1StatePort) UpdateTemplate(context.Context, string, map[string]interface{}, domain.DeltaInfo) (int, error) {
	panic("not used")
}
func (m *agent1StatePort) UpdateView(context.Context, string, domain.ViewState, []domain.ViewSnapshot, domain.DeltaInfo) (int, error) {
	panic("not used")
}
func (m *agent1StatePort) UpdateActions(context.Context, string, domain.StateActions, domain.DeltaInfo) (int, error) {
	panic("not used")
}
func (m *agent1StatePort) AppendConversation(context.Context, string, []domain.LLMMessage) error {
	panic("not used")
}
func (m *agent1StatePort) AppendAgent2History(context.Context, string, []domain.LLMMessage) error {
	panic("not used")
}
func (m *agent1StatePort) PushView(context.Context, string, *domain.ViewSnapshot) error {
	panic("not used")
}
func (m *agent1StatePort) PopView(context.Context, string) (*domain.ViewSnapshot, error) {
	panic("not used")
}
func (m *agent1StatePort) GetViewStack(context.Context, string) ([]domain.ViewSnapshot, error) {
	panic("not used")
}

// agent1CatalogPort is a fake CatalogPort that returns a fixed tenant +
// canned product list. ListProducts records the filter it received so tests
// can assert on it.
type agent1CatalogPort struct {
	tenant     *domain.Tenant
	products   []domain.Product
	lastFilter ports.ProductFilter
}

func (c *agent1CatalogPort) GetTenantBySlug(_ context.Context, _ string) (*domain.Tenant, error) {
	return c.tenant, nil
}

func (c *agent1CatalogPort) ListProducts(_ context.Context, _ string, f ports.ProductFilter) ([]domain.Product, int, error) {
	c.lastFilter = f
	return c.products, len(c.products), nil
}

func (c *agent1CatalogPort) GetProduct(_ context.Context, _ string, id string) (*domain.Product, error) {
	for _, p := range c.products {
		if p.ID == id {
			return &p, nil
		}
	}
	return nil, domain.ErrProductNotFound
}

// ─── catalog_search ──────────────────────────────────────────────────────

func TestCatalogSearchHappyPath(t *testing.T) {
	state := newAgent1StatePort(nil)
	cat := &agent1CatalogPort{
		tenant: &domain.Tenant{ID: "tnt-1", Slug: "acme"},
		products: []domain.Product{
			{ID: "p1", Name: "Hyaluronic Serum", Brand: "COSRX", Price: 250000, Rating: 4.5},
			{ID: "p2", Name: "Snail Cream", Brand: "COSRX", Price: 350000},
		},
	}
	tool := NewCatalogSearchTool(state, cat)
	res, err := tool.Execute(context.Background(),
		domain.ToolContext{SessionID: "sess-1", TenantSlug: "acme"},
		map[string]interface{}{
			"vector_query": "увлажнение для лица",
			"filters": map[string]interface{}{
				"brand":     "COSRX",
				"min_price": float64(1000),
				"max_price": float64(5000),
			},
			"limit": float64(20),
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected IsError: %s", res.Content)
	}
	if !state.updateOK {
		t.Fatal("expected UpdateData to be called")
	}
	if len(state.dataSeen.Products) != 2 {
		t.Errorf("expected 2 products in state, got %d", len(state.dataSeen.Products))
	}
	if state.metaSeen.ProductCount != 2 {
		t.Errorf("ProductCount=%d, want 2", state.metaSeen.ProductCount)
	}
	// Brand stripped from search; min/max converted to kopecks.
	if cat.lastFilter.Search != "увлажнение для лица" {
		t.Errorf("Search=%q, want unchanged (brand %q is not a substring)", cat.lastFilter.Search, cat.lastFilter.Brand)
	}
	if cat.lastFilter.Brand != "COSRX" {
		t.Errorf("Brand=%q, want COSRX", cat.lastFilter.Brand)
	}
	if cat.lastFilter.MinPrice != 100000 || cat.lastFilter.MaxPrice != 500000 {
		t.Errorf("price kopecks = %d/%d, want 100000/500000", cat.lastFilter.MinPrice, cat.lastFilter.MaxPrice)
	}
	if got, _ := res.Metadata["search_type"].(string); got != "keyword" {
		t.Errorf("search_type=%q, want keyword", got)
	}
}

func TestCatalogSearchEmptyResultPreservesState(t *testing.T) {
	original := []domain.Product{{ID: "p1", Name: "Old Product"}}
	state := newAgent1StatePort(original)
	cat := &agent1CatalogPort{
		tenant:   &domain.Tenant{ID: "tnt-1", Slug: "acme"},
		products: []domain.Product{},
	}
	tool := NewCatalogSearchTool(state, cat)
	res, err := tool.Execute(context.Background(),
		domain.ToolContext{SessionID: "sess-1", TenantSlug: "acme"},
		map[string]interface{}{"vector_query": "xyz"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.updateOK {
		t.Error("UpdateData must NOT be called on empty result — state should be preserved")
	}
	if len(state.deltas) != 1 {
		t.Errorf("expected exactly 1 delta (empty record), got %d", len(state.deltas))
	}
	if state.deltas[0].Result.Count != 0 {
		t.Errorf("delta count=%d, want 0", state.deltas[0].Result.Count)
	}
	if got := state.state.Current.Data.Products; len(got) != 1 || got[0].ID != "p1" {
		t.Errorf("original products clobbered: %+v", got)
	}
	if !contains(res.Content, "preserved") {
		t.Errorf("expected 'preserved' in content, got %q", res.Content)
	}
}

func TestCatalogSearchBrandStrippedFromSearch(t *testing.T) {
	state := newAgent1StatePort(nil)
	cat := &agent1CatalogPort{
		tenant:   &domain.Tenant{ID: "tnt-1", Slug: "acme"},
		products: []domain.Product{{ID: "p1", Name: "X", Brand: "COSRX"}},
	}
	tool := NewCatalogSearchTool(state, cat)
	_, err := tool.Execute(context.Background(),
		domain.ToolContext{SessionID: "sess-1", TenantSlug: "acme"},
		map[string]interface{}{
			"vector_query": "COSRX серум",
			"filters":      map[string]interface{}{"brand": "COSRX"},
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// "COSRX" should be stripped from Search; "серум" remains.
	if got := cat.lastFilter.Search; got == "COSRX серум" {
		t.Errorf("brand not stripped: Search=%q", got)
	}
	if got := cat.lastFilter.Search; !contains(got, "серум") {
		t.Errorf("non-brand keyword lost: Search=%q", got)
	}
}

// ─── _internal_state_filter ──────────────────────────────────────────────

func TestStateFilterByBrand(t *testing.T) {
	state := newAgent1StatePort([]domain.Product{
		{ID: "p1", Name: "X", Brand: "COSRX", Price: 250000},
		{ID: "p2", Name: "Y", Brand: "MEDI-PEEL", Price: 350000},
		{ID: "p3", Name: "Z", Brand: "cosrx", Price: 450000}, // case-insensitive
	})
	tool := NewStateFilterTool(state)
	res, err := tool.Execute(context.Background(),
		domain.ToolContext{SessionID: "sess-1"},
		map[string]interface{}{"brand": "COSRX"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("unexpected IsError: %s", res.Content)
	}
	if len(state.dataSeen.Products) != 2 {
		t.Errorf("expected 2 (case-insensitive), got %d", len(state.dataSeen.Products))
	}
}

func TestStateFilterByPriceConvertsRubles(t *testing.T) {
	state := newAgent1StatePort([]domain.Product{
		{ID: "p1", Name: "X", Price: 250000}, // 2500 руб
		{ID: "p2", Name: "Y", Price: 600000}, // 6000 руб
	})
	tool := NewStateFilterTool(state)
	_, err := tool.Execute(context.Background(),
		domain.ToolContext{SessionID: "sess-1"},
		map[string]interface{}{"max_price": float64(5000)}, // 5000 руб = 500_000 коп
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(state.dataSeen.Products) != 1 || state.dataSeen.Products[0].ID != "p1" {
		t.Errorf("expected only p1 (2500 руб ≤ 5000 руб); got %+v", state.dataSeen.Products)
	}
}

func TestStateFilterEmptyPreservesData(t *testing.T) {
	original := []domain.Product{{ID: "p1", Brand: "COSRX"}, {ID: "p2", Brand: "COSRX"}}
	state := newAgent1StatePort(original)
	tool := NewStateFilterTool(state)
	res, err := tool.Execute(context.Background(),
		domain.ToolContext{SessionID: "sess-1"},
		map[string]interface{}{"brand": "MEDI-PEEL"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.updateOK {
		t.Error("UpdateData must NOT be called on empty filter result")
	}
	if len(state.deltas) != 1 || state.deltas[0].Result.Count != 0 {
		t.Errorf("expected exactly 1 empty delta, got %+v", state.deltas)
	}
	if !contains(res.Content, "preserved") {
		t.Errorf("expected 'preserved' in content, got %q", res.Content)
	}
}

func TestStateFilterByTextMatchHitsName(t *testing.T) {
	state := newAgent1StatePort([]domain.Product{
		{ID: "p1", Name: "Hyaluronic Serum"},
		{ID: "p2", Name: "Snail Cream", Description: "with hyaluronic"},
		{ID: "p3", Name: "Toner"},
	})
	tool := NewStateFilterTool(state)
	_, err := tool.Execute(context.Background(),
		domain.ToolContext{SessionID: "sess-1"},
		map[string]interface{}{"text_match": "hyaluronic"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(state.dataSeen.Products) != 2 {
		t.Errorf("expected 2 (matches name + description), got %d", len(state.dataSeen.Products))
	}
}

// ─── _internal_history_lookup ────────────────────────────────────────────

func TestHistoryLookupMatchesToolName(t *testing.T) {
	state := newAgent1StatePort(nil)
	state.deltas = []*domain.Delta{
		{Step: 1, Action: domain.Action{Tool: "catalog_search"}, Path: "data.products", Result: domain.ResultMeta{Count: 20, Fields: []string{"id", "name"}}},
		{Step: 2, Action: domain.Action{Tool: "_internal_state_filter"}, Path: "data.products", Result: domain.ResultMeta{Count: 5}},
		{Step: 3, Action: domain.Action{Tool: "visual_assembly"}, Path: "current.template", Result: domain.ResultMeta{Count: 1}},
	}
	tool := NewHistoryLookupTool(state)
	res, err := tool.Execute(context.Background(),
		domain.ToolContext{SessionID: "sess-1"},
		map[string]interface{}{"query": "search"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(res.Content, "catalog_search") {
		t.Errorf("expected catalog_search in result, got %q", res.Content)
	}
	if contains(res.Content, "visual_assembly") {
		t.Errorf("query 'search' should NOT match 'visual_assembly': %q", res.Content)
	}
}

func TestHistoryLookupLastN(t *testing.T) {
	state := newAgent1StatePort(nil)
	state.deltas = []*domain.Delta{
		{Step: 1, Action: domain.Action{Tool: "catalog_search"}},
		{Step: 2, Action: domain.Action{Tool: "catalog_search"}},
		{Step: 3, Action: domain.Action{Tool: "catalog_search"}},
	}
	tool := NewHistoryLookupTool(state)
	res, err := tool.Execute(context.Background(),
		domain.ToolContext{SessionID: "sess-1"},
		map[string]interface{}{"query": "catalog", "last_n": float64(2)},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should match steps 2+3 only.
	if contains(res.Content, "step 1:") {
		t.Errorf("last_n=2 must skip step 1; got %q", res.Content)
	}
	if !contains(res.Content, "step 2:") || !contains(res.Content, "step 3:") {
		t.Errorf("expected steps 2+3, got %q", res.Content)
	}
}

func TestHistoryLookupNoMatch(t *testing.T) {
	state := newAgent1StatePort(nil)
	state.deltas = []*domain.Delta{
		{Step: 1, Action: domain.Action{Tool: "catalog_search"}},
	}
	tool := NewHistoryLookupTool(state)
	res, err := tool.Execute(context.Background(),
		domain.ToolContext{SessionID: "sess-1"},
		map[string]interface{}{"query": "xyz_does_not_exist"},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(res.Content, "no matching") {
		t.Errorf("expected 'no matching' message, got %q", res.Content)
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || (len(s) > 0 && (containsCI(s, sub))))
}
