package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"keepstar/internal/adapters/postgres"
	"keepstar/internal/domain"

	"github.com/google/uuid"
)

func testDatabaseURL() string {
	return os.Getenv("DATABASE_URL")
}

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
	_, _ = client.Pool().Exec(ctx, `DELETE FROM chat_sessions WHERE id = $1`, sessionID)
}

func TestStateAdapter_CreateAndGetState(t *testing.T) {
	dbURL := testDatabaseURL()
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()

	client, err := postgres.NewClient(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer client.Close()

	// Run migrations
	if err := client.RunMigrations(ctx); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}
	if err := client.RunStateMigrations(ctx); err != nil {
		t.Fatalf("Failed to run state migrations: %v", err)
	}

	adapter := postgres.NewStateAdapter(client)
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
	dbURL := testDatabaseURL()
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()

	client, err := postgres.NewClient(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer client.Close()

	if err := client.RunMigrations(ctx); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}
	if err := client.RunStateMigrations(ctx); err != nil {
		t.Fatalf("Failed to run state migrations: %v", err)
	}

	adapter := postgres.NewStateAdapter(client)
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
	dbURL := testDatabaseURL()
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()

	client, err := postgres.NewClient(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer client.Close()

	if err := client.RunMigrations(ctx); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}
	if err := client.RunStateMigrations(ctx); err != nil {
		t.Fatalf("Failed to run state migrations: %v", err)
	}

	adapter := postgres.NewStateAdapter(client)
	sessionID := testSessionID(t, client)
	defer cleanupTestSession(t, client, sessionID)

	// Create state first
	_, err = adapter.CreateState(ctx, sessionID)
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

	err = adapter.AddDelta(ctx, sessionID, delta1)
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

	err = adapter.AddDelta(ctx, sessionID, delta2)
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
	dbURL := testDatabaseURL()
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}

	ctx := context.Background()

	client, err := postgres.NewClient(ctx, dbURL)
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer client.Close()

	if err := client.RunStateMigrations(ctx); err != nil {
		t.Fatalf("Failed to run state migrations: %v", err)
	}

	adapter := postgres.NewStateAdapter(client)

	_, err = adapter.GetState(ctx, uuid.New().String())
	if err != domain.ErrSessionNotFound {
		t.Errorf("Expected ErrSessionNotFound, got %v", err)
	}
}
