package usecases_test

import (
	"context"
	"os"
	"testing"
	"time"

	"keepstar/internal/adapters/postgres"
	"keepstar/internal/domain"
	"keepstar/internal/logger"
	"keepstar/internal/usecases"

	"github.com/google/uuid"
)

var rollbackTestLog = logger.New("error")

func testDatabaseURL() string {
	return os.Getenv("DATABASE_URL")
}

func setupTestSession(t *testing.T, client *postgres.Client) string {
	ctx := context.Background()
	tenantID := "test-tenant"
	sessionID := uuid.New().String()

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
	_, _ = client.Pool().Exec(ctx, `DELETE FROM chat_sessions WHERE id = $1`, sessionID)
}

// =============================================================================
// User Scenario: Search -> Filter -> Rollback
// =============================================================================
// User searches for "ноутбуки", then filters by "Apple", then wants to go back
// to see all laptops again (rollback to step 1)

func TestRollbackUseCase_UserRollbackAfterFilter(t *testing.T) {
	dbURL := testDatabaseURL()
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()

	client, err := postgres.NewClient(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	if err := client.RunMigrations(ctx); err != nil {
		t.Fatalf("Migrations failed: %v", err)
	}
	if err := client.RunStateMigrations(ctx); err != nil {
		t.Fatalf("State migrations failed: %v", err)
	}

	stateAdapter := postgres.NewStateAdapter(client, rollbackTestLog)
	sessionID := setupTestSession(t, client)
	defer cleanupTestSession(t, client, sessionID)

	// Create initial state
	_, err = stateAdapter.CreateState(ctx, sessionID)
	if err != nil {
		t.Fatalf("CreateState failed: %v", err)
	}

	// Step 1: Agent1 searches for laptops - found 50 products
	delta1 := &domain.Delta{
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
			Count:  50,
			Fields: []string{"name", "price", "brand", "rating"},
		},
		CreatedAt: time.Now(),
	}
	if _, err := stateAdapter.AddDelta(ctx, sessionID, delta1); err != nil {
		t.Fatalf("AddDelta 1 failed: %v", err)
	}

	// Update state meta (step is auto-synced by AddDelta)
	state, _ := stateAdapter.GetState(ctx, sessionID)
	state.Current.Meta.Count = 50
	state.Current.Meta.Fields = []string{"name", "price", "brand", "rating"}
	stateAdapter.UpdateState(ctx, state)

	// Step 2: User asks to filter by Apple - now 5 products
	delta2 := &domain.Delta{
		Trigger:   domain.TriggerUserQuery,
		Source:    domain.SourceLLM,
		ActorID:   "agent1",
		DeltaType: domain.DeltaTypeUpdate,
		Path:      "data.products",
		Action: domain.Action{
			Type:   domain.ActionFilter,
			Tool:   "filter_products",
			Params: map[string]interface{}{"brand": "Apple"},
		},
		Result: domain.ResultMeta{
			Count:  5,
			Fields: []string{"name", "price", "brand", "rating"},
		},
		CreatedAt: time.Now(),
	}
	if _, err := stateAdapter.AddDelta(ctx, sessionID, delta2); err != nil {
		t.Fatalf("AddDelta 2 failed: %v", err)
	}

	state, _ = stateAdapter.GetState(ctx, sessionID)
	state.Current.Meta.Count = 5
	stateAdapter.UpdateState(ctx, state)

	// Step 3: User says "go back" or clicks back - rollback to step 1
	rollbackUC := usecases.NewRollbackUseCase(stateAdapter)

	resp, err := rollbackUC.Execute(ctx, usecases.RollbackRequest{
		SessionID: sessionID,
		ToStep:    1,
		Source:    domain.SourceUser,
		ActorID:   "user_back",
	})
	if err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	// Verify rollback results
	if resp.FromStep != 2 {
		t.Errorf("Expected FromStep 2, got %d", resp.FromStep)
	}

	if resp.ToStep != 1 {
		t.Errorf("Expected ToStep 1, got %d", resp.ToStep)
	}

	if resp.RolledBack != 1 {
		t.Errorf("Expected RolledBack 1, got %d", resp.RolledBack)
	}

	// Verify rollback delta was created
	if resp.RollbackDelta == nil {
		t.Fatal("Expected rollback delta to be created")
	}

	if resp.RollbackDelta.Source != domain.SourceUser {
		t.Errorf("Expected rollback source 'user', got '%s'", resp.RollbackDelta.Source)
	}

	if resp.RollbackDelta.ActorID != "user_back" {
		t.Errorf("Expected rollback actor_id 'user_back', got '%s'", resp.RollbackDelta.ActorID)
	}

	if resp.RollbackDelta.DeltaType != domain.DeltaTypeRollback {
		t.Errorf("Expected delta_type 'rollback', got '%s'", resp.RollbackDelta.DeltaType)
	}

	// Verify we now have 3 deltas (original 2 + rollback)
	deltas, _ := stateAdapter.GetDeltas(ctx, sessionID)
	if len(deltas) != 3 {
		t.Errorf("Expected 3 deltas after rollback, got %d", len(deltas))
	}
}

// =============================================================================
// User Scenario: Multi-step session with reconstruct
// =============================================================================
// User does: search -> filter -> sort -> wants to see state at step 2

func TestReconstructStateUseCase_ReplayToSpecificStep(t *testing.T) {
	dbURL := testDatabaseURL()
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()

	client, err := postgres.NewClient(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	if err := client.RunMigrations(ctx); err != nil {
		t.Fatalf("Migrations failed: %v", err)
	}
	if err := client.RunStateMigrations(ctx); err != nil {
		t.Fatalf("State migrations failed: %v", err)
	}

	stateAdapter := postgres.NewStateAdapter(client, rollbackTestLog)
	sessionID := setupTestSession(t, client)
	defer cleanupTestSession(t, client, sessionID)

	_, err = stateAdapter.CreateState(ctx, sessionID)
	if err != nil {
		t.Fatalf("CreateState failed: %v", err)
	}

	// Build session history (steps auto-assigned: 1, 2, 3, 4)
	actions := []struct {
		action domain.ActionType
		count  int
	}{
		{domain.ActionSearch, 100}, // Step 1: Search: found 100 products
		{domain.ActionFilter, 30},  // Step 2: Filter by brand: 30 products
		{domain.ActionFilter, 10},  // Step 3: Filter by price: 10 products
		{domain.ActionSort, 10},    // Step 4: Sort by rating: still 10
	}

	for _, s := range actions {
		delta := &domain.Delta{
			Trigger:   domain.TriggerUserQuery,
			Source:    domain.SourceLLM,
			ActorID:   "agent1",
			DeltaType: domain.DeltaTypeUpdate,
			Path:      "data.products",
			Action:    domain.Action{Type: s.action},
			Result:    domain.ResultMeta{Count: s.count, Fields: []string{"name", "price"}},
			CreatedAt: time.Now(),
		}
		if _, err := stateAdapter.AddDelta(ctx, sessionID, delta); err != nil {
			t.Fatalf("AddDelta failed: %v", err)
		}
	}

	// Reconstruct state at step 2 (after first filter, before price filter)
	reconstructUC := usecases.NewReconstructStateUseCase(stateAdapter)

	resp, err := reconstructUC.Execute(ctx, usecases.ReconstructRequest{
		SessionID: sessionID,
		ToStep:    2,
	})
	if err != nil {
		t.Fatalf("Reconstruct failed: %v", err)
	}

	// Should have 2 deltas applied
	if resp.DeltaCount != 2 {
		t.Errorf("Expected 2 deltas applied, got %d", resp.DeltaCount)
	}

	if resp.StepNow != 2 {
		t.Errorf("Expected step 2, got %d", resp.StepNow)
	}

	// Verify state reflects step 2 (30 products after brand filter)
	if resp.State.Current.Meta.Count != 30 {
		t.Errorf("Expected count 30 at step 2, got %d", resp.State.Current.Meta.Count)
	}
}

// =============================================================================
// User Scenario: Drill-down with ViewStack
// =============================================================================
// User: sees grid -> clicks product -> sees detail -> clicks back -> sees grid again

func TestRollbackUseCase_ViewStackNavigation(t *testing.T) {
	dbURL := testDatabaseURL()
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()

	client, err := postgres.NewClient(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	if err := client.RunMigrations(ctx); err != nil {
		t.Fatalf("Migrations failed: %v", err)
	}
	if err := client.RunStateMigrations(ctx); err != nil {
		t.Fatalf("State migrations failed: %v", err)
	}

	stateAdapter := postgres.NewStateAdapter(client, rollbackTestLog)
	sessionID := setupTestSession(t, client)
	defer cleanupTestSession(t, client, sessionID)

	_, err = stateAdapter.CreateState(ctx, sessionID)
	if err != nil {
		t.Fatalf("CreateState failed: %v", err)
	}

	// Step 1: Agent1 searches, user sees grid with 10 products
	delta1 := &domain.Delta{
		Trigger:   domain.TriggerUserQuery,
		Source:    domain.SourceLLM,
		ActorID:   "agent1",
		DeltaType: domain.DeltaTypeAdd,
		Path:      "data.products",
		Action:    domain.Action{Type: domain.ActionSearch},
		Result:    domain.ResultMeta{Count: 10},
		CreatedAt: time.Now(),
	}
	stateAdapter.AddDelta(ctx, sessionID, delta1)

	// Update state to grid mode (step auto-synced)
	state, _ := stateAdapter.GetState(ctx, sessionID)
	state.View.Mode = domain.ViewModeGrid
	state.Current.Meta.Count = 10
	stateAdapter.UpdateState(ctx, state)

	// Step 2: User clicks product "p5" - push grid to stack, switch to detail
	gridSnapshot := &domain.ViewSnapshot{
		Mode: domain.ViewModeGrid,
		Refs: []domain.EntityRef{
			{Type: domain.EntityTypeProduct, ID: "p1"},
			{Type: domain.EntityTypeProduct, ID: "p2"},
			{Type: domain.EntityTypeProduct, ID: "p5"},
		},
		Step:      1,
		CreatedAt: time.Now(),
	}
	stateAdapter.PushView(ctx, sessionID, gridSnapshot)

	// Record the expand action
	delta2 := &domain.Delta{
		Trigger:   domain.TriggerWidgetAction,
		Source:    domain.SourceUser,
		ActorID:   "user_click",
		DeltaType: domain.DeltaTypePush,
		Path:      "view",
		Action: domain.Action{
			Type:   domain.ActionLayout,
			Params: map[string]interface{}{"product_id": "p5", "action": "expand"},
		},
		Result:    domain.ResultMeta{Count: 1},
		CreatedAt: time.Now(),
	}
	stateAdapter.AddDelta(ctx, sessionID, delta2)

	// Update state to detail mode (step auto-synced)
	state, _ = stateAdapter.GetState(ctx, sessionID)
	state.View.Mode = domain.ViewModeDetail
	state.View.Focused = &domain.EntityRef{Type: domain.EntityTypeProduct, ID: "p5"}
	stateAdapter.UpdateState(ctx, state)

	// Step 3: User clicks back
	poppedView, err := stateAdapter.PopView(ctx, sessionID)
	if err != nil {
		t.Fatalf("PopView failed: %v", err)
	}

	// Record the back action
	delta3 := &domain.Delta{
		Trigger:   domain.TriggerWidgetAction,
		Source:    domain.SourceUser,
		ActorID:   "user_back",
		DeltaType: domain.DeltaTypePop,
		Path:      "view",
		Action: domain.Action{
			Type:   domain.ActionLayout,
			Params: map[string]interface{}{"action": "back"},
		},
		Result:    domain.ResultMeta{Count: 10},
		CreatedAt: time.Now(),
	}
	stateAdapter.AddDelta(ctx, sessionID, delta3)

	// Restore view from popped snapshot (step auto-synced)
	state, _ = stateAdapter.GetState(ctx, sessionID)
	state.View.Mode = poppedView.Mode
	state.View.Focused = poppedView.Focused
	stateAdapter.UpdateState(ctx, state)

	// Verify: user should be back on grid
	finalState, _ := stateAdapter.GetState(ctx, sessionID)

	if finalState.View.Mode != domain.ViewModeGrid {
		t.Errorf("Expected view mode 'grid' after back, got '%s'", finalState.View.Mode)
	}

	if finalState.View.Focused != nil {
		t.Error("Expected no focused item after back to grid")
	}

	// Verify delta history shows the navigation
	deltas, _ := stateAdapter.GetDeltas(ctx, sessionID)
	if len(deltas) != 3 {
		t.Errorf("Expected 3 deltas, got %d", len(deltas))
	}

	// Delta 2 should be user push (expand)
	if deltas[1].Source != domain.SourceUser || deltas[1].DeltaType != domain.DeltaTypePush {
		t.Error("Delta 2 should be user push action")
	}

	// Delta 3 should be user pop (back)
	if deltas[2].Source != domain.SourceUser || deltas[2].DeltaType != domain.DeltaTypePop {
		t.Error("Delta 3 should be user pop action")
	}
}

// =============================================================================
// Edge Case: Rollback validation
// =============================================================================

func TestRollbackUseCase_CannotRollbackForward(t *testing.T) {
	dbURL := testDatabaseURL()
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()

	client, err := postgres.NewClient(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	if err := client.RunMigrations(ctx); err != nil {
		t.Fatalf("Migrations failed: %v", err)
	}
	if err := client.RunStateMigrations(ctx); err != nil {
		t.Fatalf("State migrations failed: %v", err)
	}

	stateAdapter := postgres.NewStateAdapter(client, rollbackTestLog)
	sessionID := setupTestSession(t, client)
	defer cleanupTestSession(t, client, sessionID)

	_, _ = stateAdapter.CreateState(ctx, sessionID)

	// Add one delta (step auto-assigned = 1)
	delta := &domain.Delta{
		Trigger:   domain.TriggerUserQuery,
		Source:    domain.SourceLLM,
		ActorID:   "agent1",
		DeltaType: domain.DeltaTypeAdd,
		Path:      "data.products",
		Action:    domain.Action{Type: domain.ActionSearch},
		Result:    domain.ResultMeta{Count: 10},
		CreatedAt: time.Now(),
	}
	stateAdapter.AddDelta(ctx, sessionID, delta)

	// Try to rollback to step 5 (doesn't exist yet)
	rollbackUC := usecases.NewRollbackUseCase(stateAdapter)

	_, err = rollbackUC.Execute(ctx, usecases.RollbackRequest{
		SessionID: sessionID,
		ToStep:    5, // Current step is 1, can't go forward
		Source:    domain.SourceUser,
		ActorID:   "user_back",
	})

	if err == nil {
		t.Error("Expected error when trying to rollback forward")
	}
}

func TestRollbackUseCase_CannotRollbackToNegativeStep(t *testing.T) {
	dbURL := testDatabaseURL()
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()

	client, err := postgres.NewClient(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	if err := client.RunMigrations(ctx); err != nil {
		t.Fatalf("Migrations failed: %v", err)
	}
	if err := client.RunStateMigrations(ctx); err != nil {
		t.Fatalf("State migrations failed: %v", err)
	}

	stateAdapter := postgres.NewStateAdapter(client, rollbackTestLog)
	sessionID := setupTestSession(t, client)
	defer cleanupTestSession(t, client, sessionID)

	_, _ = stateAdapter.CreateState(ctx, sessionID)

	// Add a delta so state.Step = 1
	delta := &domain.Delta{
		Trigger:   domain.TriggerUserQuery,
		Source:    domain.SourceLLM,
		ActorID:   "agent1",
		DeltaType: domain.DeltaTypeAdd,
		Path:      "data.products",
		Action:    domain.Action{Type: domain.ActionSearch},
		Result:    domain.ResultMeta{Count: 10},
		CreatedAt: time.Now(),
	}
	stateAdapter.AddDelta(ctx, sessionID, delta)

	rollbackUC := usecases.NewRollbackUseCase(stateAdapter)

	_, err = rollbackUC.Execute(ctx, usecases.RollbackRequest{
		SessionID: sessionID,
		ToStep:    -1,
		Source:    domain.SourceUser,
		ActorID:   "user_back",
	})

	if err == nil {
		t.Error("Expected error when trying to rollback to negative step")
	}
}
