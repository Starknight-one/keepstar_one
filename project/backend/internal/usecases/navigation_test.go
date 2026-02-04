package usecases_test

import (
	"context"
	"testing"
	"time"

	"keepstar/internal/domain"
	"keepstar/internal/presets"
	"keepstar/internal/usecases"
)

// mockStatePort implements ports.StatePort for testing without database
type mockStatePort struct {
	state     *domain.SessionState
	deltas    []domain.Delta
	viewStack []domain.ViewSnapshot
}

func newMockStatePort() *mockStatePort {
	return &mockStatePort{
		deltas:    []domain.Delta{},
		viewStack: []domain.ViewSnapshot{},
	}
}

func (m *mockStatePort) CreateState(ctx context.Context, sessionID string) (*domain.SessionState, error) {
	m.state = &domain.SessionState{
		ID:        "state-1",
		SessionID: sessionID,
		Current: domain.StateCurrent{
			Data: domain.StateData{},
			Meta: domain.StateMeta{},
		},
		View: domain.ViewState{
			Mode: domain.ViewModeGrid,
		},
		ViewStack: []domain.ViewSnapshot{},
		Step:      0,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
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
	m.state = state
	return nil
}

func (m *mockStatePort) AddDelta(ctx context.Context, sessionID string, delta *domain.Delta) (int, error) {
	step := len(m.deltas) + 1
	delta.Step = step
	m.deltas = append(m.deltas, *delta)
	return step, nil
}

func (m *mockStatePort) GetDeltas(ctx context.Context, sessionID string) ([]domain.Delta, error) {
	return m.deltas, nil
}

func (m *mockStatePort) GetDeltasSince(ctx context.Context, sessionID string, fromStep int) ([]domain.Delta, error) {
	var result []domain.Delta
	for _, d := range m.deltas {
		if d.Step >= fromStep {
			result = append(result, d)
		}
	}
	return result, nil
}

func (m *mockStatePort) GetDeltasUntil(ctx context.Context, sessionID string, toStep int) ([]domain.Delta, error) {
	var result []domain.Delta
	for _, d := range m.deltas {
		if d.Step <= toStep {
			result = append(result, d)
		}
	}
	return result, nil
}

func (m *mockStatePort) PushView(ctx context.Context, sessionID string, snapshot *domain.ViewSnapshot) error {
	m.viewStack = append(m.viewStack, *snapshot)
	if m.state != nil {
		m.state.ViewStack = m.viewStack
	}
	return nil
}

func (m *mockStatePort) PopView(ctx context.Context, sessionID string) (*domain.ViewSnapshot, error) {
	if len(m.viewStack) == 0 {
		return nil, nil
	}
	last := m.viewStack[len(m.viewStack)-1]
	m.viewStack = m.viewStack[:len(m.viewStack)-1]
	if m.state != nil {
		m.state.ViewStack = m.viewStack
	}
	return &last, nil
}

func (m *mockStatePort) GetViewStack(ctx context.Context, sessionID string) ([]domain.ViewSnapshot, error) {
	return m.viewStack, nil
}

// =============================================================================
// Test: ExpandUseCase
// =============================================================================

func TestExpandUseCase_Success(t *testing.T) {
	ctx := context.Background()
	statePort := newMockStatePort()
	presetRegistry := presets.NewPresetRegistry()

	// Setup: create state with products
	statePort.CreateState(ctx, "session-1")
	statePort.state.Current.Data.Products = []domain.Product{
		{
			ID:          "product-1",
			Name:        "Nike Air Max 90",
			Price:       12990,
			Currency:    "$",
			Images:      []string{"https://example.com/nike1.jpg"},
			Rating:      4.5,
			Brand:       "Nike",
			Category:    "Sneakers",
			Description: "Classic sneakers",
		},
		{
			ID:       "product-2",
			Name:     "Nike Air Force 1",
			Price:    9990,
			Currency: "$",
			Images:   []string{"https://example.com/nike2.jpg"},
			Rating:   4.8,
			Brand:    "Nike",
		},
	}
	statePort.state.View.Mode = domain.ViewModeGrid

	// Create use case
	expandUC := usecases.NewExpandUseCase(statePort, presetRegistry)

	// Execute expand
	resp, err := expandUC.Execute(ctx, usecases.ExpandRequest{
		SessionID:  "session-1",
		EntityType: domain.EntityTypeProduct,
		EntityID:   "product-1",
	})

	// Verify
	if err != nil {
		t.Fatalf("Expand failed: %v", err)
	}

	if !resp.Success {
		t.Error("Expected success=true")
	}

	if resp.ViewMode != domain.ViewModeDetail {
		t.Errorf("Expected viewMode=detail, got %s", resp.ViewMode)
	}

	if resp.Focused == nil || resp.Focused.ID != "product-1" {
		t.Error("Expected focused to be product-1")
	}

	if resp.StackSize != 1 {
		t.Errorf("Expected stackSize=1, got %d", resp.StackSize)
	}

	// Verify formation
	if resp.Formation == nil {
		t.Fatal("Expected formation to be returned")
	}

	if resp.Formation.Mode != domain.FormationTypeSingle {
		t.Errorf("Expected formation mode=single, got %s", resp.Formation.Mode)
	}

	if len(resp.Formation.Widgets) != 1 {
		t.Errorf("Expected 1 widget, got %d", len(resp.Formation.Widgets))
	}

	// Verify widget has ProductDetail template
	widget := resp.Formation.Widgets[0]
	if widget.Template != "ProductDetail" {
		t.Errorf("Expected template=ProductDetail, got %s", widget.Template)
	}

	// Verify entityRef
	if widget.EntityRef == nil || widget.EntityRef.ID != "product-1" {
		t.Error("Expected widget to have entityRef with product-1")
	}

	// Verify delta was created
	if len(statePort.deltas) != 1 {
		t.Errorf("Expected 1 delta, got %d", len(statePort.deltas))
	}

	delta := statePort.deltas[0]
	if delta.DeltaType != domain.DeltaTypePush {
		t.Errorf("Expected deltaType=push, got %s", delta.DeltaType)
	}
	if delta.Source != domain.SourceUser {
		t.Errorf("Expected source=user, got %s", delta.Source)
	}

	t.Logf("Expand successful: viewMode=%s, stackSize=%d, widget=%s",
		resp.ViewMode, resp.StackSize, widget.Template)
}

func TestExpandUseCase_EntityNotFound(t *testing.T) {
	ctx := context.Background()
	statePort := newMockStatePort()
	presetRegistry := presets.NewPresetRegistry()

	// Setup: create state with products (but not the one we're looking for)
	statePort.CreateState(ctx, "session-1")
	statePort.state.Current.Data.Products = []domain.Product{
		{ID: "product-1", Name: "Nike Air Max 90"},
	}

	expandUC := usecases.NewExpandUseCase(statePort, presetRegistry)

	// Try to expand non-existent product
	_, err := expandUC.Execute(ctx, usecases.ExpandRequest{
		SessionID:  "session-1",
		EntityType: domain.EntityTypeProduct,
		EntityID:   "product-999",
	})

	if err == nil {
		t.Error("Expected error for non-existent entity")
	}

	t.Logf("Got expected error: %v", err)
}

// =============================================================================
// Test: BackUseCase
// =============================================================================

func TestBackUseCase_Success(t *testing.T) {
	ctx := context.Background()
	statePort := newMockStatePort()
	presetRegistry := presets.NewPresetRegistry()

	// Setup: create state with products and a view in the stack
	statePort.CreateState(ctx, "session-1")
	statePort.state.Current.Data.Products = []domain.Product{
		{ID: "product-1", Name: "Nike Air Max 90", Price: 12990, Currency: "$", Images: []string{"img1.jpg"}},
		{ID: "product-2", Name: "Nike Air Force 1", Price: 9990, Currency: "$", Images: []string{"img2.jpg"}},
	}
	statePort.state.View.Mode = domain.ViewModeDetail
	statePort.state.View.Focused = &domain.EntityRef{Type: domain.EntityTypeProduct, ID: "product-1"}

	// Push a view to the stack (simulating previous grid view)
	statePort.PushView(ctx, "session-1", &domain.ViewSnapshot{
		Mode:      domain.ViewModeGrid,
		Focused:   nil,
		Refs:      []domain.EntityRef{{Type: domain.EntityTypeProduct, ID: "product-1"}, {Type: domain.EntityTypeProduct, ID: "product-2"}},
		Step:      1,
		CreatedAt: time.Now(),
	})

	// Create use case
	backUC := usecases.NewBackUseCase(statePort, presetRegistry)

	// Execute back
	resp, err := backUC.Execute(ctx, usecases.BackRequest{
		SessionID: "session-1",
	})

	// Verify
	if err != nil {
		t.Fatalf("Back failed: %v", err)
	}

	if !resp.Success {
		t.Error("Expected success=true")
	}

	if resp.ViewMode != domain.ViewModeGrid {
		t.Errorf("Expected viewMode=grid, got %s", resp.ViewMode)
	}

	if resp.StackSize != 0 {
		t.Errorf("Expected stackSize=0, got %d", resp.StackSize)
	}

	if resp.CanGoBack {
		t.Error("Expected canGoBack=false when stack is empty")
	}

	// Verify formation (should be grid with 2 products)
	if resp.Formation == nil {
		t.Fatal("Expected formation to be returned")
	}

	if resp.Formation.Mode != domain.FormationTypeGrid {
		t.Errorf("Expected formation mode=grid, got %s", resp.Formation.Mode)
	}

	if len(resp.Formation.Widgets) != 2 {
		t.Errorf("Expected 2 widgets, got %d", len(resp.Formation.Widgets))
	}

	// Verify delta was created
	if len(statePort.deltas) != 1 {
		t.Errorf("Expected 1 delta, got %d", len(statePort.deltas))
	}

	delta := statePort.deltas[0]
	if delta.DeltaType != domain.DeltaTypePop {
		t.Errorf("Expected deltaType=pop, got %s", delta.DeltaType)
	}

	t.Logf("Back successful: viewMode=%s, stackSize=%d, widgets=%d",
		resp.ViewMode, resp.StackSize, len(resp.Formation.Widgets))
}

func TestBackUseCase_EmptyStack(t *testing.T) {
	ctx := context.Background()
	statePort := newMockStatePort()
	presetRegistry := presets.NewPresetRegistry()

	// Setup: create state with empty view stack
	statePort.CreateState(ctx, "session-1")
	statePort.state.View.Mode = domain.ViewModeGrid

	backUC := usecases.NewBackUseCase(statePort, presetRegistry)

	// Execute back on empty stack
	resp, err := backUC.Execute(ctx, usecases.BackRequest{
		SessionID: "session-1",
	})

	// Should succeed but indicate can't go back
	if err != nil {
		t.Fatalf("Back failed: %v", err)
	}

	if !resp.Success {
		t.Error("Expected success=true")
	}

	if resp.CanGoBack {
		t.Error("Expected canGoBack=false for empty stack")
	}

	// No delta should be created when stack is empty
	if len(statePort.deltas) != 0 {
		t.Errorf("Expected 0 deltas for empty stack, got %d", len(statePort.deltas))
	}

	t.Logf("Back on empty stack: canGoBack=%v", resp.CanGoBack)
}

// =============================================================================
// Test: Full navigation flow (expand -> back)
// =============================================================================

func TestNavigationFlow_ExpandAndBack(t *testing.T) {
	ctx := context.Background()
	statePort := newMockStatePort()
	presetRegistry := presets.NewPresetRegistry()

	// Setup: create state with products
	statePort.CreateState(ctx, "session-1")
	statePort.state.Current.Data.Products = []domain.Product{
		{ID: "product-1", Name: "Nike Air Max 90", Price: 12990, Currency: "$", Images: []string{"img1.jpg"}, Brand: "Nike"},
		{ID: "product-2", Name: "Nike Air Force 1", Price: 9990, Currency: "$", Images: []string{"img2.jpg"}, Brand: "Nike"},
	}
	statePort.state.View.Mode = domain.ViewModeGrid

	expandUC := usecases.NewExpandUseCase(statePort, presetRegistry)
	backUC := usecases.NewBackUseCase(statePort, presetRegistry)

	t.Log("=== Step 1: Initial state (grid view) ===")
	t.Logf("ViewMode: %s, Products: %d", statePort.state.View.Mode, len(statePort.state.Current.Data.Products))

	// Step 2: Expand product-1
	t.Log("\n=== Step 2: Expand product-1 ===")
	expandResp, err := expandUC.Execute(ctx, usecases.ExpandRequest{
		SessionID:  "session-1",
		EntityType: domain.EntityTypeProduct,
		EntityID:   "product-1",
	})
	if err != nil {
		t.Fatalf("Expand failed: %v", err)
	}

	t.Logf("ViewMode: %s, StackSize: %d, CanGoBack: %v",
		expandResp.ViewMode, expandResp.StackSize, expandResp.StackSize > 0)
	t.Logf("Formation: mode=%s, widgets=%d, template=%s",
		expandResp.Formation.Mode, len(expandResp.Formation.Widgets), expandResp.Formation.Widgets[0].Template)

	if expandResp.ViewMode != domain.ViewModeDetail {
		t.Errorf("After expand: expected detail mode")
	}
	if expandResp.StackSize != 1 {
		t.Errorf("After expand: expected stack size 1")
	}

	// Step 3: Go back
	t.Log("\n=== Step 3: Go back ===")
	backResp, err := backUC.Execute(ctx, usecases.BackRequest{
		SessionID: "session-1",
	})
	if err != nil {
		t.Fatalf("Back failed: %v", err)
	}

	t.Logf("ViewMode: %s, StackSize: %d, CanGoBack: %v",
		backResp.ViewMode, backResp.StackSize, backResp.CanGoBack)
	t.Logf("Formation: mode=%s, widgets=%d",
		backResp.Formation.Mode, len(backResp.Formation.Widgets))

	if backResp.ViewMode != domain.ViewModeGrid {
		t.Errorf("After back: expected grid mode")
	}
	if backResp.StackSize != 0 {
		t.Errorf("After back: expected stack size 0")
	}
	if backResp.CanGoBack {
		t.Errorf("After back: expected canGoBack=false")
	}

	// Verify deltas
	t.Log("\n=== Deltas ===")
	for i, d := range statePort.deltas {
		t.Logf("Delta %d: step=%d, type=%s, source=%s, actor=%s",
			i, d.Step, d.DeltaType, d.Source, d.ActorID)
	}

	if len(statePort.deltas) != 2 {
		t.Errorf("Expected 2 deltas (push + pop), got %d", len(statePort.deltas))
	}
}

// =============================================================================
// Test: Service expand (not just products)
// =============================================================================

func TestExpandUseCase_Service(t *testing.T) {
	ctx := context.Background()
	statePort := newMockStatePort()
	presetRegistry := presets.NewPresetRegistry()

	// Setup: create state with services
	statePort.CreateState(ctx, "session-1")
	statePort.state.Current.Data.Services = []domain.Service{
		{
			ID:           "service-1",
			Name:         "Haircut",
			Price:        1500,
			Currency:     "$",
			Duration:     "30 min",
			Provider:     "John",
			Availability: "available",
			Description:  "Professional haircut",
		},
	}
	statePort.state.View.Mode = domain.ViewModeGrid

	expandUC := usecases.NewExpandUseCase(statePort, presetRegistry)

	resp, err := expandUC.Execute(ctx, usecases.ExpandRequest{
		SessionID:  "session-1",
		EntityType: domain.EntityTypeService,
		EntityID:   "service-1",
	})

	if err != nil {
		t.Fatalf("Expand service failed: %v", err)
	}

	if resp.Formation.Widgets[0].Template != "ServiceDetail" {
		t.Errorf("Expected template=ServiceDetail, got %s", resp.Formation.Widgets[0].Template)
	}

	t.Logf("Service expand successful: template=%s", resp.Formation.Widgets[0].Template)
}
