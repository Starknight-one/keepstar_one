package postgres_test

import (
	"context"
	"testing"

	"keepstar/internal/adapters/postgres"
	"keepstar/internal/domain"
	"keepstar/internal/logger"

	"github.com/google/uuid"
)

func sessionAdapter(t *testing.T) (context.Context, *postgres.StateAdapter, *postgres.Client) {
	t.Helper()
	client := getSharedClient(t)
	log := logger.New("error")
	return context.Background(), postgres.NewStateAdapter(client, log), client
}

func TestSessionIntegration_CreateState_RequiresSession(t *testing.T) {
	ctx, adapter, _ := sessionAdapter(t)
	_, err := adapter.CreateState(ctx, "nonexistent-session-"+uuid.New().String())
	if err == nil {
		t.Error("expected FK constraint error when session doesn't exist")
	}
}

func TestSessionIntegration_SessionCRUD(t *testing.T) {
	ctx, adapter, client := sessionAdapter(t)
	sessionID := testSessionID(t, client)
	defer cleanupTestSession(t, client, sessionID)

	state, err := adapter.CreateState(ctx, sessionID)
	if err != nil {
		t.Fatalf("CreateState: %v", err)
	}
	if state.SessionID != sessionID {
		t.Errorf("SessionID mismatch: want %s, got %s", sessionID, state.SessionID)
	}
	if state.Step != 0 {
		t.Errorf("initial step: want 0, got %d", state.Step)
	}

	got, err := adapter.GetState(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if got.ID != state.ID {
		t.Errorf("ID mismatch: want %s, got %s", state.ID, got.ID)
	}
}

func TestSessionIntegration_GetState_NotFound(t *testing.T) {
	ctx, adapter, _ := sessionAdapter(t)
	_, err := adapter.GetState(ctx, "nonexistent-"+uuid.New().String())
	if err != domain.ErrSessionNotFound {
		t.Errorf("want ErrSessionNotFound, got %v", err)
	}
}

func TestSessionIntegration_DeltaStepSequential(t *testing.T) {
	ctx, adapter, client := sessionAdapter(t)
	sessionID := testSessionID(t, client)
	defer cleanupTestSession(t, client, sessionID)

	if _, err := adapter.CreateState(ctx, sessionID); err != nil {
		t.Fatalf("CreateState: %v", err)
	}

	for i := 0; i < 3; i++ {
		delta := &domain.Delta{
			TurnID:    "turn-1",
			Trigger:   domain.TriggerUserQuery,
			Source:    domain.SourceLLM,
			ActorID:   "agent1",
			DeltaType: domain.DeltaTypeAdd,
			Path:      "data.products",
		}
		step, err := adapter.AddDelta(ctx, sessionID, delta)
		if err != nil {
			t.Fatalf("AddDelta[%d]: %v", i, err)
		}
		if step != i+1 {
			t.Errorf("delta %d: want step %d, got %d", i, i+1, step)
		}
	}

	state, err := adapter.GetState(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if state.Step != 3 {
		t.Errorf("state step: want 3, got %d", state.Step)
	}
}

func TestSessionIntegration_ViewStackPushPop(t *testing.T) {
	ctx, adapter, client := sessionAdapter(t)
	sessionID := testSessionID(t, client)
	defer cleanupTestSession(t, client, sessionID)

	if _, err := adapter.CreateState(ctx, sessionID); err != nil {
		t.Fatalf("CreateState: %v", err)
	}

	snap1 := &domain.ViewSnapshot{Mode: domain.ViewModeGrid, Step: 1}
	snap2 := &domain.ViewSnapshot{Mode: domain.ViewModeDetail, Step: 2,
		Focused: &domain.EntityRef{Type: domain.EntityTypeProduct, ID: "p1"}}

	if err := adapter.PushView(ctx, sessionID, snap1); err != nil {
		t.Fatalf("PushView 1: %v", err)
	}
	if err := adapter.PushView(ctx, sessionID, snap2); err != nil {
		t.Fatalf("PushView 2: %v", err)
	}

	stack, err := adapter.GetViewStack(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetViewStack: %v", err)
	}
	if len(stack) != 2 {
		t.Fatalf("want stack size 2, got %d", len(stack))
	}

	popped, err := adapter.PopView(ctx, sessionID)
	if err != nil {
		t.Fatalf("PopView: %v", err)
	}
	if popped.Mode != domain.ViewModeDetail {
		t.Errorf("want popped mode detail, got %s", popped.Mode)
	}
	if popped.Focused == nil || popped.Focused.ID != "p1" {
		t.Error("popped snapshot should have focused p1")
	}

	popped2, err := adapter.PopView(ctx, sessionID)
	if err != nil {
		t.Fatalf("PopView 2: %v", err)
	}
	if popped2.Mode != domain.ViewModeGrid {
		t.Errorf("want popped mode grid, got %s", popped2.Mode)
	}

	popped3, err := adapter.PopView(ctx, sessionID)
	if err != nil {
		t.Fatalf("PopView empty: %v", err)
	}
	if popped3 != nil {
		t.Error("pop empty stack should return nil")
	}
}

func TestSessionIntegration_ZoneWriteUpdateData(t *testing.T) {
	ctx, adapter, client := sessionAdapter(t)
	sessionID := testSessionID(t, client)
	defer cleanupTestSession(t, client, sessionID)

	if _, err := adapter.CreateState(ctx, sessionID); err != nil {
		t.Fatalf("CreateState: %v", err)
	}

	data := domain.StateData{
		Products: []domain.Product{
			{ID: "p1", Name: "Nike Air", Price: 150000},
			{ID: "p2", Name: "Adidas Ultra", Price: 180000},
		},
	}
	meta := domain.StateMeta{Count: 2, Fields: []string{"id", "name", "price"}}
	info := domain.DeltaInfo{
		TurnID:    "turn-1",
		Trigger:   domain.TriggerUserQuery,
		Source:    domain.SourceLLM,
		ActorID:   "agent1",
		DeltaType: domain.DeltaTypeAdd,
		Path:      "data.products",
		Result:    domain.ResultMeta{Count: 2},
	}

	step, err := adapter.UpdateData(ctx, sessionID, data, meta, info)
	if err != nil {
		t.Fatalf("UpdateData: %v", err)
	}
	if step < 1 {
		t.Errorf("want step >= 1, got %d", step)
	}

	state, err := adapter.GetState(ctx, sessionID)
	if err != nil {
		t.Fatalf("GetState: %v", err)
	}
	if len(state.Current.Data.Products) != 2 {
		t.Errorf("want 2 products, got %d", len(state.Current.Data.Products))
	}
	if state.Current.Meta.Count != 2 {
		t.Errorf("want meta count 2, got %d", state.Current.Meta.Count)
	}
}
