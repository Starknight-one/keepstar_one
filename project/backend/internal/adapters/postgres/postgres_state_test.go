package postgres_test

import (
	"context"
	"testing"
	"time"

	"keepstar/internal/adapters/postgres"
	"keepstar/internal/domain"
	"keepstar/internal/logger"

	"github.com/google/uuid"
)

var testLog = logger.New("error")

func testSessionID(t *testing.T, client *postgres.Client) string {
	ctx := context.Background()
	tenantID := "test-tenant"
	sessionID := uuid.New().String()

	// Create a chat session first (required for foreign key)
	_, err := client.Pool().Exec(ctx, `
		INSERT INTO chat_sessions (id, tenant_id, status)
		VALUES ($1, $2, 'active')
	`, sessionID, tenantID)
	if err != nil {
		t.Fatalf("Failed to create test session: %v", err)
	}

	return sessionID
}

func cleanupTestSession(t *testing.T, client *postgres.Client, sessionID string) {
	ctx := context.Background()
	_, _ = client.Pool().Exec(ctx, `DELETE FROM chat_session_deltas WHERE session_id = $1`, sessionID)
	_, _ = client.Pool().Exec(ctx, `DELETE FROM chat_session_state WHERE session_id = $1`, sessionID)
	_, _ = client.Pool().Exec(ctx, `DELETE FROM chat_sessions WHERE id = $1`, sessionID)
}

func TestStateAdapter_CreateAndGetState(t *testing.T) {
	client := getSharedClient(t)
	ctx := context.Background()

	adapter := postgres.NewStateAdapter(client, testLog)
	sessionID := testSessionID(t, client)
	defer cleanupTestSession(t, client, sessionID)

	// Create state
	state, err := adapter.CreateState(ctx, sessionID)
	if err != nil {
		t.Fatalf("CreateState failed: %v", err)
	}

	if state.Step != 0 {
		t.Errorf("Expected step 0, got %d", state.Step)
	}

	if state.ID == "" {
		t.Error("Expected state ID to be set")
	}

	// Get state
	retrieved, err := adapter.GetState(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}

	if retrieved.ID != state.ID {
		t.Errorf("ID mismatch: expected %s, got %s", state.ID, retrieved.ID)
	}

	if retrieved.SessionID != sessionID {
		t.Errorf("SessionID mismatch: expected %s, got %s", sessionID, retrieved.SessionID)
	}
}

func TestStateAdapter_UpdateState(t *testing.T) {
	client := getSharedClient(t)
	ctx := context.Background()

	adapter := postgres.NewStateAdapter(client, testLog)
	sessionID := testSessionID(t, client)
	defer cleanupTestSession(t, client, sessionID)

	// Create state
	state, err := adapter.CreateState(ctx, sessionID)
	if err != nil {
		t.Fatalf("CreateState failed: %v", err)
	}

	// Update state
	state.Step = 3
	state.Current.Meta = domain.StateMeta{
		Count:  10,
		Fields: []string{"name", "price", "rating"},
	}
	state.Current.Data = domain.StateData{
		Products: []domain.Product{
			{ID: "p1", Name: "Test Product", Price: 1000},
		},
	}

	err = adapter.UpdateState(ctx, state)
	if err != nil {
		t.Fatalf("UpdateState failed: %v", err)
	}

	// Verify update
	retrieved, err := adapter.GetState(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}

	if retrieved.Step != 3 {
		t.Errorf("Expected step 3, got %d", retrieved.Step)
	}

	if retrieved.Current.Meta.Count != 10 {
		t.Errorf("Expected meta count 10, got %d", retrieved.Current.Meta.Count)
	}

	if len(retrieved.Current.Data.Products) != 1 {
		t.Errorf("Expected 1 product, got %d", len(retrieved.Current.Data.Products))
	}
}

func TestStateAdapter_AddAndGetDeltas(t *testing.T) {
	client := getSharedClient(t)
	ctx := context.Background()

	adapter := postgres.NewStateAdapter(client, testLog)
	sessionID := testSessionID(t, client)
	defer cleanupTestSession(t, client, sessionID)

	// Create state first
	_, err := adapter.CreateState(ctx, sessionID)
	if err != nil {
		t.Fatalf("CreateState failed: %v", err)
	}

	// Add deltas
	delta1 := &domain.Delta{
		Step:    1,
		Trigger: domain.TriggerUserQuery,
		Action: domain.Action{
			Type:   domain.ActionSearch,
			Tool:   "search_products",
			Params: map[string]interface{}{"query": "ноутбуки"},
		},
		Result: domain.ResultMeta{
			Count:  10,
			Fields: []string{"name", "price", "rating"},
		},
		CreatedAt: time.Now(),
	}

	_, err = adapter.AddDelta(ctx, sessionID, delta1)
	if err != nil {
		t.Fatalf("AddDelta failed: %v", err)
	}

	delta2 := &domain.Delta{
		Step:    2,
		Trigger: domain.TriggerWidgetAction,
		Action: domain.Action{
			Type:   domain.ActionFilter,
			Tool:   "filter_products",
			Params: map[string]interface{}{"brand": "Apple"},
		},
		Result: domain.ResultMeta{
			Count:  3,
			Fields: []string{"name", "price", "rating"},
		},
		CreatedAt: time.Now(),
	}

	_, err = adapter.AddDelta(ctx, sessionID, delta2)
	if err != nil {
		t.Fatalf("AddDelta failed: %v", err)
	}

	// Get all deltas
	deltas, err := adapter.GetDeltas(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetDeltas failed: %v", err)
	}

	if len(deltas) != 2 {
		t.Errorf("Expected 2 deltas, got %d", len(deltas))
	}

	if deltas[0].Action.Tool != "search_products" {
		t.Errorf("Expected tool 'search_products', got '%s'", deltas[0].Action.Tool)
	}

	if deltas[1].Result.Count != 3 {
		t.Errorf("Expected result count 3, got %d", deltas[1].Result.Count)
	}

	// Get deltas since step 2
	deltasSince, err := adapter.GetDeltasSince(ctx, sessionID, 2)
	if err != nil {
		t.Fatalf("GetDeltasSince failed: %v", err)
	}

	if len(deltasSince) != 1 {
		t.Errorf("Expected 1 delta since step 2, got %d", len(deltasSince))
	}

	if deltasSince[0].Step != 2 {
		t.Errorf("Expected step 2, got %d", deltasSince[0].Step)
	}
}

func TestStateAdapter_GetState_NotFound(t *testing.T) {
	client := getSharedClient(t)
	ctx := context.Background()

	adapter := postgres.NewStateAdapter(client, testLog)

	_, err := adapter.GetState(ctx, uuid.New().String())
	if err != domain.ErrSessionNotFound {
		t.Errorf("Expected ErrSessionNotFound, got %v", err)
	}
}

// =============================================================================
// Delta State Management Tests (x7k9m2p)
// =============================================================================

// TestStateAdapter_DeltaWithSourceTracking tests new delta fields: Source, ActorID, DeltaType, Path
func TestStateAdapter_DeltaWithSourceTracking(t *testing.T) {
	client := getSharedClient(t)
	ctx := context.Background()

	adapter := postgres.NewStateAdapter(client, testLog)
	sessionID := testSessionID(t, client)
	defer cleanupTestSession(t, client, sessionID)

	// Create state
	_, err := adapter.CreateState(ctx, sessionID)
	if err != nil {
		t.Fatalf("CreateState failed: %v", err)
	}

	// Scenario: Agent1 searches for products (LLM action)
	delta1 := &domain.Delta{
		Step:      1,
		Trigger:   domain.TriggerUserQuery,
		Source:    domain.SourceLLM,
		ActorID:   "agent1",
		DeltaType: domain.DeltaTypeAdd,
		Path:      "data.products",
		Action: domain.Action{
			Type:   domain.ActionSearch,
			Tool:   "search_products",
			Params: map[string]interface{}{"query": "ноутбуки"},
		},
		Result: domain.ResultMeta{
			Count:  10,
			Fields: []string{"name", "price", "rating"},
		},
		CreatedAt: time.Now(),
	}

	_, err = adapter.AddDelta(ctx, sessionID, delta1)
	if err != nil {
		t.Fatalf("AddDelta failed: %v", err)
	}

	// Scenario: User clicks on a product (user action)
	delta2 := &domain.Delta{
		Step:      2,
		Trigger:   domain.TriggerWidgetAction,
		Source:    domain.SourceUser,
		ActorID:   "user_click",
		DeltaType: domain.DeltaTypePush,
		Path:      "viewStack",
		Action: domain.Action{
			Type:   domain.ActionLayout,
			Params: map[string]interface{}{"product_id": "p123", "action": "expand"},
		},
		Result: domain.ResultMeta{Count: 1},
		CreatedAt: time.Now(),
	}

	_, err = adapter.AddDelta(ctx, sessionID, delta2)
	if err != nil {
		t.Fatalf("AddDelta failed: %v", err)
	}

	// Retrieve and verify
	deltas, err := adapter.GetDeltas(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetDeltas failed: %v", err)
	}

	if len(deltas) != 2 {
		t.Fatalf("Expected 2 deltas, got %d", len(deltas))
	}

	// Verify delta1 (LLM action)
	if deltas[0].Source != domain.SourceLLM {
		t.Errorf("Delta1: expected source 'llm', got '%s'", deltas[0].Source)
	}
	if deltas[0].ActorID != "agent1" {
		t.Errorf("Delta1: expected actor_id 'agent1', got '%s'", deltas[0].ActorID)
	}
	if deltas[0].DeltaType != domain.DeltaTypeAdd {
		t.Errorf("Delta1: expected delta_type 'add', got '%s'", deltas[0].DeltaType)
	}
	if deltas[0].Path != "data.products" {
		t.Errorf("Delta1: expected path 'data.products', got '%s'", deltas[0].Path)
	}

	// Verify delta2 (User action)
	if deltas[1].Source != domain.SourceUser {
		t.Errorf("Delta2: expected source 'user', got '%s'", deltas[1].Source)
	}
	if deltas[1].ActorID != "user_click" {
		t.Errorf("Delta2: expected actor_id 'user_click', got '%s'", deltas[1].ActorID)
	}
	if deltas[1].DeltaType != domain.DeltaTypePush {
		t.Errorf("Delta2: expected delta_type 'push', got '%s'", deltas[1].DeltaType)
	}
}

// TestStateAdapter_GetDeltasUntil tests retrieving deltas up to a specific step
func TestStateAdapter_GetDeltasUntil(t *testing.T) {
	client := getSharedClient(t)
	ctx := context.Background()

	adapter := postgres.NewStateAdapter(client, testLog)
	sessionID := testSessionID(t, client)
	defer cleanupTestSession(t, client, sessionID)

	_, err := adapter.CreateState(ctx, sessionID)
	if err != nil {
		t.Fatalf("CreateState failed: %v", err)
	}

	// Create 5 deltas simulating a user session
	for i := 1; i <= 5; i++ {
		delta := &domain.Delta{
			Step:      i,
			Trigger:   domain.TriggerUserQuery,
			Source:    domain.SourceLLM,
			ActorID:   "agent1",
			DeltaType: domain.DeltaTypeAdd,
			Path:      "data.products",
			Action: domain.Action{
				Type: domain.ActionSearch,
				Tool: "search_products",
			},
			Result:    domain.ResultMeta{Count: i * 10},
			CreatedAt: time.Now(),
		}
		if _, err := adapter.AddDelta(ctx, sessionID, delta); err != nil {
			t.Fatalf("AddDelta step %d failed: %v", i, err)
		}
	}

	// Get deltas until step 3
	deltas, err := adapter.GetDeltasUntil(ctx, sessionID, 3)
	if err != nil {
		t.Fatalf("GetDeltasUntil failed: %v", err)
	}

	if len(deltas) != 3 {
		t.Errorf("Expected 3 deltas (steps 1-3), got %d", len(deltas))
	}

	// Verify steps
	for i, d := range deltas {
		expectedStep := i + 1
		if d.Step != expectedStep {
			t.Errorf("Delta %d: expected step %d, got %d", i, expectedStep, d.Step)
		}
	}
}

// TestStateAdapter_ViewStack tests PushView, PopView, GetViewStack operations
func TestStateAdapter_ViewStack(t *testing.T) {
	client := getSharedClient(t)
	ctx := context.Background()

	adapter := postgres.NewStateAdapter(client, testLog)
	sessionID := testSessionID(t, client)
	defer cleanupTestSession(t, client, sessionID)

	_, err := adapter.CreateState(ctx, sessionID)
	if err != nil {
		t.Fatalf("CreateState failed: %v", err)
	}

	// Scenario: User is on grid view, clicks product to expand (push)
	snapshot1 := &domain.ViewSnapshot{
		Mode: domain.ViewModeGrid,
		Refs: []domain.EntityRef{
			{Type: domain.EntityTypeProduct, ID: "p1"},
			{Type: domain.EntityTypeProduct, ID: "p2"},
			{Type: domain.EntityTypeProduct, ID: "p3"},
		},
		Step:      1,
		CreatedAt: time.Now(),
	}

	err = adapter.PushView(ctx, sessionID, snapshot1)
	if err != nil {
		t.Fatalf("PushView 1 failed: %v", err)
	}

	// User expands product p2 to detail view, then drills deeper
	snapshot2 := &domain.ViewSnapshot{
		Mode:    domain.ViewModeDetail,
		Focused: &domain.EntityRef{Type: domain.EntityTypeProduct, ID: "p2"},
		Refs: []domain.EntityRef{
			{Type: domain.EntityTypeProduct, ID: "p2"},
		},
		Step:      2,
		CreatedAt: time.Now(),
	}

	err = adapter.PushView(ctx, sessionID, snapshot2)
	if err != nil {
		t.Fatalf("PushView 2 failed: %v", err)
	}

	// Verify stack has 2 items
	stack, err := adapter.GetViewStack(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetViewStack failed: %v", err)
	}

	if len(stack) != 2 {
		t.Errorf("Expected stack size 2, got %d", len(stack))
	}

	// User clicks back - should pop detail view
	popped, err := adapter.PopView(ctx, sessionID)
	if err != nil {
		t.Fatalf("PopView failed: %v", err)
	}

	if popped == nil {
		t.Fatal("PopView returned nil")
	}

	if popped.Mode != domain.ViewModeDetail {
		t.Errorf("Expected popped mode 'detail', got '%s'", popped.Mode)
	}

	if popped.Focused == nil || popped.Focused.ID != "p2" {
		t.Error("Expected popped focused to be p2")
	}

	// Verify stack now has 1 item
	stack, err = adapter.GetViewStack(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetViewStack after pop failed: %v", err)
	}

	if len(stack) != 1 {
		t.Errorf("Expected stack size 1 after pop, got %d", len(stack))
	}

	// Pop again - should get grid view
	popped, err = adapter.PopView(ctx, sessionID)
	if err != nil {
		t.Fatalf("PopView 2 failed: %v", err)
	}

	if popped.Mode != domain.ViewModeGrid {
		t.Errorf("Expected popped mode 'grid', got '%s'", popped.Mode)
	}

	// Pop empty stack - should return nil
	popped, err = adapter.PopView(ctx, sessionID)
	if err != nil {
		t.Fatalf("PopView empty failed: %v", err)
	}

	if popped != nil {
		t.Error("Expected nil when popping empty stack")
	}
}

// TestStateAdapter_ViewStateInSessionState tests view state fields in SessionState
func TestStateAdapter_ViewStateInSessionState(t *testing.T) {
	client := getSharedClient(t)
	ctx := context.Background()

	adapter := postgres.NewStateAdapter(client, testLog)
	sessionID := testSessionID(t, client)
	defer cleanupTestSession(t, client, sessionID)

	// Create state - should have default grid mode
	state, err := adapter.CreateState(ctx, sessionID)
	if err != nil {
		t.Fatalf("CreateState failed: %v", err)
	}

	if state.View.Mode != domain.ViewModeGrid {
		t.Errorf("Expected default view mode 'grid', got '%s'", state.View.Mode)
	}

	// Update to detail mode with focused product
	state.View.Mode = domain.ViewModeDetail
	state.View.Focused = &domain.EntityRef{
		Type: domain.EntityTypeProduct,
		ID:   "product-123",
	}
	state.ViewStack = []domain.ViewSnapshot{
		{Mode: domain.ViewModeGrid, Step: 0},
	}

	err = adapter.UpdateState(ctx, state)
	if err != nil {
		t.Fatalf("UpdateState failed: %v", err)
	}

	// Retrieve and verify
	retrieved, err := adapter.GetState(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}

	if retrieved.View.Mode != domain.ViewModeDetail {
		t.Errorf("Expected view mode 'detail', got '%s'", retrieved.View.Mode)
	}

	if retrieved.View.Focused == nil {
		t.Fatal("Expected focused to be set")
	}

	if retrieved.View.Focused.ID != "product-123" {
		t.Errorf("Expected focused ID 'product-123', got '%s'", retrieved.View.Focused.ID)
	}

	if len(retrieved.ViewStack) != 1 {
		t.Errorf("Expected ViewStack length 1, got %d", len(retrieved.ViewStack))
	}
}

// =============================================================================
// Zone-Write Tests (z8v4q1w)
// =============================================================================

// TestStateAdapter_UpdateData tests zone-write for data zone
func TestStateAdapter_UpdateData(t *testing.T) {
	client := getSharedClient(t)
	ctx := context.Background()

	adapter := postgres.NewStateAdapter(client, testLog)
	sessionID := testSessionID(t, client)
	defer cleanupTestSession(t, client, sessionID)

	_, err := adapter.CreateState(ctx, sessionID)
	if err != nil {
		t.Fatalf("CreateState failed: %v", err)
	}

	// Zone-write: update data + create delta atomically
	data := domain.StateData{
		Products: []domain.Product{
			{ID: "p1", Name: "Nike Air Max", Price: 12990},
			{ID: "p2", Name: "Nike Dunk Low", Price: 10990},
		},
	}
	meta := domain.StateMeta{
		Count:  2,
		Fields: []string{"id", "name", "price"},
	}
	info := domain.DeltaInfo{
		TurnID:    "turn-data-1",
		Trigger:   domain.TriggerUserQuery,
		Source:    domain.SourceLLM,
		ActorID:   "agent1",
		DeltaType: domain.DeltaTypeAdd,
		Path:      "data.products",
		Action:    domain.Action{Type: domain.ActionSearch, Tool: "search_products"},
		Result:    domain.ResultMeta{Count: 2, Fields: []string{"id", "name", "price"}},
	}

	step, err := adapter.UpdateData(ctx, sessionID, data, meta, info)
	if err != nil {
		t.Fatalf("UpdateData failed: %v", err)
	}
	if step != 1 {
		t.Errorf("Expected step 1, got %d", step)
	}

	// Verify state was updated
	state, err := adapter.GetState(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	if len(state.Current.Data.Products) != 2 {
		t.Errorf("Expected 2 products, got %d", len(state.Current.Data.Products))
	}
	if state.Current.Meta.Count != 2 {
		t.Errorf("Expected meta count 2, got %d", state.Current.Meta.Count)
	}

	// Verify delta was created with TurnID
	deltas, err := adapter.GetDeltas(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetDeltas failed: %v", err)
	}
	if len(deltas) != 1 {
		t.Fatalf("Expected 1 delta, got %d", len(deltas))
	}
	if deltas[0].TurnID != "turn-data-1" {
		t.Errorf("Expected TurnID 'turn-data-1', got '%s'", deltas[0].TurnID)
	}
	if deltas[0].ActorID != "agent1" {
		t.Errorf("Expected actor_id 'agent1', got '%s'", deltas[0].ActorID)
	}

	// Verify template and view were NOT affected
	if state.Current.Template != nil {
		t.Error("Expected template to remain nil after UpdateData")
	}
	if state.View.Mode != domain.ViewModeGrid {
		t.Errorf("Expected view mode 'grid' unchanged, got '%s'", state.View.Mode)
	}
}

// TestStateAdapter_UpdateTemplate tests zone-write for template zone
func TestStateAdapter_UpdateTemplate(t *testing.T) {
	client := getSharedClient(t)
	ctx := context.Background()

	adapter := postgres.NewStateAdapter(client, testLog)
	sessionID := testSessionID(t, client)
	defer cleanupTestSession(t, client, sessionID)

	_, err := adapter.CreateState(ctx, sessionID)
	if err != nil {
		t.Fatalf("CreateState failed: %v", err)
	}

	// First, set some data to verify it's not overwritten
	state, _ := adapter.GetState(ctx, sessionID)
	state.Current.Data.Products = []domain.Product{{ID: "p1", Name: "Test"}}
	state.Current.Meta.Count = 1
	adapter.UpdateState(ctx, state)

	// Zone-write: update template
	template := map[string]interface{}{
		"formation": map[string]interface{}{"mode": "grid", "widgets": []interface{}{}},
	}
	info := domain.DeltaInfo{
		TurnID:    "turn-tmpl-1",
		Trigger:   domain.TriggerUserQuery,
		Source:    domain.SourceLLM,
		ActorID:   "agent2",
		DeltaType: domain.DeltaTypeUpdate,
		Path:      "template",
		Action:    domain.Action{Type: domain.ActionLayout, Tool: "render_product_preset"},
	}

	step, err := adapter.UpdateTemplate(ctx, sessionID, template, info)
	if err != nil {
		t.Fatalf("UpdateTemplate failed: %v", err)
	}
	if step != 1 {
		t.Errorf("Expected step 1, got %d", step)
	}

	// Verify template was updated
	state, _ = adapter.GetState(ctx, sessionID)
	if state.Current.Template == nil {
		t.Fatal("Expected template to be set")
	}
	if _, ok := state.Current.Template["formation"]; !ok {
		t.Error("Expected 'formation' key in template")
	}

	// Verify data was NOT affected
	if len(state.Current.Data.Products) != 1 {
		t.Errorf("Expected data unchanged (1 product), got %d", len(state.Current.Data.Products))
	}

	// Verify delta
	deltas, _ := adapter.GetDeltas(ctx, sessionID)
	if len(deltas) != 1 {
		t.Fatalf("Expected 1 delta, got %d", len(deltas))
	}
	if deltas[0].TurnID != "turn-tmpl-1" {
		t.Errorf("Expected TurnID 'turn-tmpl-1', got '%s'", deltas[0].TurnID)
	}
	if deltas[0].ActorID != "agent2" {
		t.Errorf("Expected actor_id 'agent2', got '%s'", deltas[0].ActorID)
	}
}

// TestStateAdapter_UpdateView tests zone-write for view zone
func TestStateAdapter_UpdateView(t *testing.T) {
	client := getSharedClient(t)
	ctx := context.Background()

	adapter := postgres.NewStateAdapter(client, testLog)
	sessionID := testSessionID(t, client)
	defer cleanupTestSession(t, client, sessionID)

	_, err := adapter.CreateState(ctx, sessionID)
	if err != nil {
		t.Fatalf("CreateState failed: %v", err)
	}

	// Zone-write: switch to detail mode
	view := domain.ViewState{
		Mode:    domain.ViewModeDetail,
		Focused: &domain.EntityRef{Type: domain.EntityTypeProduct, ID: "p1"},
	}
	stack := []domain.ViewSnapshot{
		{Mode: domain.ViewModeGrid, Step: 0, CreatedAt: time.Now()},
	}
	info := domain.DeltaInfo{
		TurnID:    "turn-view-1",
		Trigger:   domain.TriggerWidgetAction,
		Source:    domain.SourceUser,
		ActorID:   "user_expand",
		DeltaType: domain.DeltaTypePush,
		Path:      "view",
	}

	step, err := adapter.UpdateView(ctx, sessionID, view, stack, info)
	if err != nil {
		t.Fatalf("UpdateView failed: %v", err)
	}
	if step != 1 {
		t.Errorf("Expected step 1, got %d", step)
	}

	// Verify view was updated
	state, _ := adapter.GetState(ctx, sessionID)
	if state.View.Mode != domain.ViewModeDetail {
		t.Errorf("Expected view mode 'detail', got '%s'", state.View.Mode)
	}
	if state.View.Focused == nil || state.View.Focused.ID != "p1" {
		t.Error("Expected focused to be p1")
	}
	if len(state.ViewStack) != 1 {
		t.Errorf("Expected ViewStack length 1, got %d", len(state.ViewStack))
	}

	// Verify delta
	deltas, _ := adapter.GetDeltas(ctx, sessionID)
	if len(deltas) != 1 {
		t.Fatalf("Expected 1 delta, got %d", len(deltas))
	}
	if deltas[0].TurnID != "turn-view-1" {
		t.Errorf("Expected TurnID 'turn-view-1', got '%s'", deltas[0].TurnID)
	}
	if deltas[0].Source != domain.SourceUser {
		t.Errorf("Expected source 'user', got '%s'", deltas[0].Source)
	}
}

// TestStateAdapter_AppendConversation tests conversation history zone-write
func TestStateAdapter_AppendConversation(t *testing.T) {
	client := getSharedClient(t)
	ctx := context.Background()

	adapter := postgres.NewStateAdapter(client, testLog)
	sessionID := testSessionID(t, client)
	defer cleanupTestSession(t, client, sessionID)

	_, err := adapter.CreateState(ctx, sessionID)
	if err != nil {
		t.Fatalf("CreateState failed: %v", err)
	}

	// Append conversation messages
	messages := []domain.LLMMessage{
		{Role: "user", Content: "покажи кроссовки nike"},
		{Role: "assistant", Content: "tool_use"},
	}

	err = adapter.AppendConversation(ctx, sessionID, messages)
	if err != nil {
		t.Fatalf("AppendConversation failed: %v", err)
	}

	// Verify conversation was saved
	state, _ := adapter.GetState(ctx, sessionID)
	if len(state.ConversationHistory) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(state.ConversationHistory))
	}
	if state.ConversationHistory[0].Role != "user" {
		t.Errorf("Expected first message role 'user', got '%s'", state.ConversationHistory[0].Role)
	}
	if state.ConversationHistory[0].Content != "покажи кроссовки nike" {
		t.Errorf("Expected first message content, got '%s'", state.ConversationHistory[0].Content)
	}

	// Verify no delta was created (AppendConversation doesn't create deltas)
	deltas, _ := adapter.GetDeltas(ctx, sessionID)
	if len(deltas) != 0 {
		t.Errorf("Expected 0 deltas for AppendConversation, got %d", len(deltas))
	}
}

// TestStateAdapter_DeltaWithTurnID tests that turn_id is stored and retrieved correctly
func TestStateAdapter_DeltaWithTurnID(t *testing.T) {
	client := getSharedClient(t)
	ctx := context.Background()

	adapter := postgres.NewStateAdapter(client, testLog)
	sessionID := testSessionID(t, client)
	defer cleanupTestSession(t, client, sessionID)

	_, err := adapter.CreateState(ctx, sessionID)
	if err != nil {
		t.Fatalf("CreateState failed: %v", err)
	}

	turnID := "turn-abc-123"

	// Add delta with TurnID
	delta := &domain.Delta{
		TurnID:    turnID,
		Trigger:   domain.TriggerUserQuery,
		Source:    domain.SourceLLM,
		ActorID:   "agent1",
		DeltaType: domain.DeltaTypeAdd,
		Path:      "data.products",
		Action:    domain.Action{Type: domain.ActionSearch},
		Result:    domain.ResultMeta{Count: 5},
		CreatedAt: time.Now(),
	}

	_, err = adapter.AddDelta(ctx, sessionID, delta)
	if err != nil {
		t.Fatalf("AddDelta failed: %v", err)
	}

	// Add another delta with same TurnID (grouped turn)
	delta2 := &domain.Delta{
		TurnID:    turnID,
		Trigger:   domain.TriggerUserQuery,
		Source:    domain.SourceLLM,
		ActorID:   "agent2",
		DeltaType: domain.DeltaTypeUpdate,
		Path:      "template",
		Action:    domain.Action{Type: domain.ActionLayout},
		Result:    domain.ResultMeta{},
		CreatedAt: time.Now(),
	}

	_, err = adapter.AddDelta(ctx, sessionID, delta2)
	if err != nil {
		t.Fatalf("AddDelta 2 failed: %v", err)
	}

	// Add delta without TurnID
	delta3 := &domain.Delta{
		Trigger:   domain.TriggerSystem,
		Source:    domain.SourceSystem,
		ActorID:   "system",
		DeltaType: domain.DeltaTypeUpdate,
		Path:      "state",
		Action:    domain.Action{Type: domain.ActionRollback},
		Result:    domain.ResultMeta{},
		CreatedAt: time.Now(),
	}

	_, err = adapter.AddDelta(ctx, sessionID, delta3)
	if err != nil {
		t.Fatalf("AddDelta 3 failed: %v", err)
	}

	// Retrieve and verify
	deltas, err := adapter.GetDeltas(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetDeltas failed: %v", err)
	}

	if len(deltas) != 3 {
		t.Fatalf("Expected 3 deltas, got %d", len(deltas))
	}

	// First two should have TurnID
	if deltas[0].TurnID != turnID {
		t.Errorf("Delta 1: expected TurnID '%s', got '%s'", turnID, deltas[0].TurnID)
	}
	if deltas[1].TurnID != turnID {
		t.Errorf("Delta 2: expected TurnID '%s', got '%s'", turnID, deltas[1].TurnID)
	}

	// Third should have empty TurnID
	if deltas[2].TurnID != "" {
		t.Errorf("Delta 3: expected empty TurnID, got '%s'", deltas[2].TurnID)
	}

	t.Logf("TurnID grouping works: deltas 1,2 share '%s', delta 3 has no TurnID", turnID)
}

// TestStateAdapter_ZoneWriteIsolation tests that zone-writes don't affect other zones
func TestStateAdapter_ZoneWriteIsolation(t *testing.T) {
	client := getSharedClient(t)
	ctx := context.Background()

	adapter := postgres.NewStateAdapter(client, testLog)
	sessionID := testSessionID(t, client)
	defer cleanupTestSession(t, client, sessionID)

	_, err := adapter.CreateState(ctx, sessionID)
	if err != nil {
		t.Fatalf("CreateState failed: %v", err)
	}

	turnID := "turn-isolation"

	// 1. Write data zone
	data := domain.StateData{Products: []domain.Product{{ID: "p1", Name: "Test", Price: 100}}}
	meta := domain.StateMeta{Count: 1, Fields: []string{"id", "name", "price"}}
	adapter.UpdateData(ctx, sessionID, data, meta, domain.DeltaInfo{
		TurnID: turnID, Trigger: domain.TriggerUserQuery, Source: domain.SourceLLM,
		ActorID: "agent1", DeltaType: domain.DeltaTypeAdd, Path: "data.products",
	})

	// 2. Write template zone
	template := map[string]interface{}{"formation": "grid"}
	adapter.UpdateTemplate(ctx, sessionID, template, domain.DeltaInfo{
		TurnID: turnID, Trigger: domain.TriggerUserQuery, Source: domain.SourceLLM,
		ActorID: "agent2", DeltaType: domain.DeltaTypeUpdate, Path: "template",
	})

	// 3. Write view zone
	view := domain.ViewState{Mode: domain.ViewModeDetail, Focused: &domain.EntityRef{Type: domain.EntityTypeProduct, ID: "p1"}}
	stack := []domain.ViewSnapshot{{Mode: domain.ViewModeGrid, Step: 0, CreatedAt: time.Now()}}
	adapter.UpdateView(ctx, sessionID, view, stack, domain.DeltaInfo{
		TurnID: turnID, Trigger: domain.TriggerWidgetAction, Source: domain.SourceUser,
		ActorID: "user_expand", DeltaType: domain.DeltaTypePush, Path: "view",
	})

	// 4. Write conversation
	adapter.AppendConversation(ctx, sessionID, []domain.LLMMessage{{Role: "user", Content: "test"}})

	// Verify ALL zones are intact
	state, err := adapter.GetState(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}

	// Data zone
	if len(state.Current.Data.Products) != 1 || state.Current.Data.Products[0].ID != "p1" {
		t.Error("Data zone corrupted after other zone-writes")
	}
	if state.Current.Meta.Count != 1 {
		t.Errorf("Meta corrupted: expected count 1, got %d", state.Current.Meta.Count)
	}

	// Template zone
	if state.Current.Template == nil || state.Current.Template["formation"] != "grid" {
		t.Error("Template zone corrupted after other zone-writes")
	}

	// View zone
	if state.View.Mode != domain.ViewModeDetail {
		t.Errorf("View zone corrupted: expected detail, got %s", state.View.Mode)
	}
	if state.View.Focused == nil || state.View.Focused.ID != "p1" {
		t.Error("View focused corrupted")
	}
	if len(state.ViewStack) != 1 {
		t.Errorf("ViewStack corrupted: expected 1, got %d", len(state.ViewStack))
	}

	// Conversation zone
	if len(state.ConversationHistory) != 1 || state.ConversationHistory[0].Content != "test" {
		t.Error("Conversation zone corrupted after other zone-writes")
	}

	// Verify 3 deltas with same TurnID (data + template + view, conversation has no delta)
	deltas, _ := adapter.GetDeltas(ctx, sessionID)
	if len(deltas) != 3 {
		t.Errorf("Expected 3 deltas, got %d", len(deltas))
	}
	for i, d := range deltas {
		if d.TurnID != turnID {
			t.Errorf("Delta %d: expected TurnID '%s', got '%s'", i, turnID, d.TurnID)
		}
	}

	t.Log("Zone isolation verified: all 4 zones written independently, no cross-contamination")
}
