package tools_test

import (
	"context"
	"testing"
	"time"

	"keepstar/internal/domain"
	"keepstar/internal/ports"
	"keepstar/internal/tools"
)

// mockStatePort implements ports.StatePort for tool testing
type mockStatePort struct {
	state *domain.SessionState

	// Call tracking
	UpdateDataCalls     int
	UpdateTemplateCalls int
	UpdateStateCalls    int
	AddDeltaCalls       int
	LastDeltaInfo       *domain.Delta
}

func newMockStatePort(state *domain.SessionState) *mockStatePort {
	return &mockStatePort{state: state}
}

func (m *mockStatePort) CreateState(ctx context.Context, sessionID string) (*domain.SessionState, error) {
	m.state = &domain.SessionState{
		ID: "state-1", SessionID: sessionID,
		Current:   domain.StateCurrent{},
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	return m.state, nil
}
func (m *mockStatePort) GetState(ctx context.Context, sessionID string) (*domain.SessionState, error) {
	if m.state == nil {
		return nil, domain.ErrSessionNotFound
	}
	return m.state, nil
}
func (m *mockStatePort) UpdateState(ctx context.Context, state *domain.SessionState) error {
	m.UpdateStateCalls++
	m.state = state
	return nil
}
func (m *mockStatePort) AddDelta(ctx context.Context, sessionID string, delta *domain.Delta) (int, error) {
	m.AddDeltaCalls++
	m.LastDeltaInfo = delta
	return 1, nil
}
func (m *mockStatePort) GetDeltas(ctx context.Context, sessionID string) ([]domain.Delta, error) {
	return nil, nil
}
func (m *mockStatePort) GetDeltasSince(ctx context.Context, sessionID string, fromStep int) ([]domain.Delta, error) {
	return nil, nil
}
func (m *mockStatePort) GetDeltasUntil(ctx context.Context, sessionID string, toStep int) ([]domain.Delta, error) {
	return nil, nil
}
func (m *mockStatePort) UpdateData(ctx context.Context, sessionID string, data domain.StateData, meta domain.StateMeta, info domain.DeltaInfo) (int, error) {
	m.UpdateDataCalls++
	if m.state != nil {
		m.state.Current.Data = data
		m.state.Current.Meta = meta
	}
	m.LastDeltaInfo = info.ToDelta()
	return 1, nil
}
func (m *mockStatePort) UpdateTemplate(ctx context.Context, sessionID string, template map[string]interface{}, info domain.DeltaInfo) (int, error) {
	m.UpdateTemplateCalls++
	if m.state != nil {
		m.state.Current.Template = template
	}
	m.LastDeltaInfo = info.ToDelta()
	return 1, nil
}
func (m *mockStatePort) UpdateView(ctx context.Context, sessionID string, view domain.ViewState, stack []domain.ViewSnapshot, info domain.DeltaInfo) (int, error) {
	return 1, nil
}
func (m *mockStatePort) AppendConversation(ctx context.Context, sessionID string, messages []domain.LLMMessage) error {
	return nil
}
func (m *mockStatePort) PushView(ctx context.Context, sessionID string, snapshot *domain.ViewSnapshot) error {
	return nil
}
func (m *mockStatePort) PopView(ctx context.Context, sessionID string) (*domain.ViewSnapshot, error) {
	return nil, nil
}
func (m *mockStatePort) GetViewStack(ctx context.Context, sessionID string) ([]domain.ViewSnapshot, error) {
	return nil, nil
}

// mockCatalogPort implements ports.CatalogPort for tool testing
type mockCatalogPort struct {
	products []domain.Product
	total    int
}

func (m *mockCatalogPort) GetTenantBySlug(ctx context.Context, slug string) (*domain.Tenant, error) {
	return &domain.Tenant{ID: "t1", Slug: slug}, nil
}
func (m *mockCatalogPort) GetCategories(ctx context.Context) ([]domain.Category, error) {
	return nil, nil
}
func (m *mockCatalogPort) GetMasterProduct(ctx context.Context, id string) (*domain.MasterProduct, error) {
	return nil, nil
}
func (m *mockCatalogPort) ListProducts(ctx context.Context, tenantID string, filter ports.ProductFilter) ([]domain.Product, int, error) {
	return m.products, m.total, nil
}
func (m *mockCatalogPort) GetProduct(ctx context.Context, tenantID string, productID string) (*domain.Product, error) {
	return nil, nil
}

func TestSearchProducts_SuccessUsesUpdateData(t *testing.T) {
	state := &domain.SessionState{
		ID: "s1", SessionID: "sess-1",
		Current: domain.StateCurrent{
			Meta: domain.StateMeta{Aliases: map[string]string{"tenant_slug": "nike"}},
		},
	}
	sp := newMockStatePort(state)
	cp := &mockCatalogPort{
		products: []domain.Product{
			{ID: "p1", Name: "Nike Air Max", Price: 12990, Brand: "Nike"},
			{ID: "p2", Name: "Nike Dunk", Price: 9990, Brand: "Nike"},
		},
		total: 2,
	}
	tool := tools.NewSearchProductsTool(sp, cp)

	ctx := context.Background()
	toolCtx := tools.ToolContext{SessionID: "sess-1", TurnID: "turn-1", ActorID: "agent1"}
	result, err := tool.Execute(ctx, toolCtx, map[string]interface{}{"query": "Nike"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}

	// UpdateData must be called, NOT UpdateState
	if sp.UpdateDataCalls != 1 {
		t.Errorf("expected 1 UpdateData call, got %d", sp.UpdateDataCalls)
	}
	if sp.UpdateStateCalls != 0 {
		t.Errorf("expected 0 UpdateState calls, got %d", sp.UpdateStateCalls)
	}
}

func TestSearchProducts_EmptyPreservesData(t *testing.T) {
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
	tool := tools.NewSearchProductsTool(sp, cp)

	ctx := context.Background()
	toolCtx := tools.ToolContext{SessionID: "sess-1", TurnID: "turn-1", ActorID: "agent1"}
	result, err := tool.Execute(ctx, toolCtx, map[string]interface{}{"query": "nonexistent"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Data zone must NOT be overwritten
	if sp.UpdateDataCalls != 0 {
		t.Errorf("expected 0 UpdateData calls (empty should not mutate data), got %d", sp.UpdateDataCalls)
	}
	if sp.UpdateStateCalls != 0 {
		t.Errorf("expected 0 UpdateState calls, got %d", sp.UpdateStateCalls)
	}

	// AddDelta must be called with count:0
	if sp.AddDeltaCalls != 1 {
		t.Errorf("expected 1 AddDelta call for empty marker, got %d", sp.AddDeltaCalls)
	}
	if sp.LastDeltaInfo != nil && sp.LastDeltaInfo.Result.Count != 0 {
		t.Errorf("expected delta result count=0, got %d", sp.LastDeltaInfo.Result.Count)
	}

	// State products must still be there
	if len(state.Current.Data.Products) != 1 {
		t.Errorf("expected 1 product preserved, got %d", len(state.Current.Data.Products))
	}

	// Result content should indicate empty
	if result.Content != "empty: 0 results, previous data preserved" {
		t.Errorf("unexpected result content: %s", result.Content)
	}
}

func TestSearchProducts_AliasesPreserved(t *testing.T) {
	state := &domain.SessionState{
		ID: "s1", SessionID: "sess-1",
		Current: domain.StateCurrent{
			Meta: domain.StateMeta{
				Aliases: map[string]string{"tenant_slug": "nike"},
			},
		},
	}
	sp := newMockStatePort(state)
	cp := &mockCatalogPort{
		products: []domain.Product{{ID: "p1", Name: "Nike Air Max", Price: 12990}},
		total:    1,
	}
	tool := tools.NewSearchProductsTool(sp, cp)

	ctx := context.Background()
	toolCtx := tools.ToolContext{SessionID: "sess-1", TurnID: "turn-1", ActorID: "agent1"}
	_, err := tool.Execute(ctx, toolCtx, map[string]interface{}{"query": "Nike"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Aliases must be preserved after UpdateData
	if state.Current.Meta.Aliases == nil {
		t.Fatal("expected Aliases to be preserved, got nil")
	}
	if state.Current.Meta.Aliases["tenant_slug"] != "nike" {
		t.Errorf("expected tenant_slug=nike, got %s", state.Current.Meta.Aliases["tenant_slug"])
	}
}

func TestSearchProducts_TurnIDInDelta(t *testing.T) {
	state := &domain.SessionState{
		ID: "s1", SessionID: "sess-1",
		Current: domain.StateCurrent{
			Meta: domain.StateMeta{Aliases: map[string]string{"tenant_slug": "nike"}},
		},
	}
	sp := newMockStatePort(state)
	cp := &mockCatalogPort{
		products: []domain.Product{{ID: "p1", Name: "Nike Air Max", Price: 12990}},
		total:    1,
	}
	tool := tools.NewSearchProductsTool(sp, cp)

	ctx := context.Background()
	toolCtx := tools.ToolContext{SessionID: "sess-1", TurnID: "test-turn-123", ActorID: "agent1"}
	_, err := tool.Execute(ctx, toolCtx, map[string]interface{}{"query": "Nike"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sp.LastDeltaInfo == nil {
		t.Fatal("expected delta to be created")
	}
	if sp.LastDeltaInfo.TurnID != "test-turn-123" {
		t.Errorf("expected TurnID=test-turn-123, got %s", sp.LastDeltaInfo.TurnID)
	}
}
