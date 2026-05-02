//go:build integration

// Run with: TEST_DATABASE_URL=$DATABASE_URL go test -tags=integration ./internal/adapters/postgres/...
//
// The test creates a temporary v5_chat_sessions row then verifies
// CreateState → AddDelta → zone-write → reconstruct semantics. It cleans up
// after itself using ON DELETE CASCADE on session_id.

package postgres

import (
	"context"
	"os"
	"testing"

	"keepstar_v5/internal/domain"
)

func setupClient(t *testing.T) *Client {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()
	c, err := NewClient(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(c.Close)
	if err := c.RunStateMigrations(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return c
}

func newTestSession(t *testing.T, c *Client) string {
	t.Helper()
	ctx := context.Background()
	var id string
	err := c.pool.QueryRow(ctx, `
		INSERT INTO v5_chat_sessions (tenant_id) VALUES ($1) RETURNING id
	`, "test-tenant").Scan(&id)
	if err != nil {
		t.Fatalf("create test session: %v", err)
	}
	t.Cleanup(func() {
		_, _ = c.pool.Exec(context.Background(), `DELETE FROM v5_chat_sessions WHERE id = $1`, id)
	})
	return id
}

func TestStateAdapterLifecycle(t *testing.T) {
	c := setupClient(t)
	a := NewStateAdapter(c, nil)
	ctx := context.Background()
	sessID := newTestSession(t, c)

	state, err := a.CreateState(ctx, sessID)
	if err != nil {
		t.Fatalf("create state: %v", err)
	}
	if state.Step != 0 || state.View.Mode != domain.ViewModeGrid {
		t.Errorf("fresh state shape: %+v", state)
	}

	// UpdateData → step 1
	step, err := a.UpdateData(ctx, sessID,
		domain.StateData{Products: []domain.Product{{ID: "p1", Name: "Cream"}}},
		domain.StateMeta{Count: 1, Fields: []string{"id", "name"}},
		domain.DeltaInfo{
			Trigger:   domain.TriggerUserQuery,
			Source:    domain.SourceLLM,
			ActorID:   "agent1",
			DeltaType: domain.DeltaTypeAdd,
			Path:      "data.products",
			Action:    domain.Action{Type: domain.ActionSearch},
			Result:    domain.ResultMeta{Count: 1, Fields: []string{"id", "name"}},
		})
	if err != nil {
		t.Fatalf("update data: %v", err)
	}
	if step != 1 {
		t.Errorf("expected step 1, got %d", step)
	}

	// UpdateTemplate → step 2 (scene-graph Document JSON)
	step, err = a.UpdateTemplate(ctx, sessID,
		map[string]interface{}{
			"version":  "2.10",
			"children": []interface{}{map[string]interface{}{"type": "frame", "id": "root"}},
		},
		domain.DeltaInfo{
			Trigger:   domain.TriggerUserQuery,
			Source:    domain.SourceLLM,
			ActorID:   "agent2",
			DeltaType: domain.DeltaTypeUpdate,
			Path:      "current.template",
			Action:    domain.Action{Type: domain.ActionLayout},
			Result:    domain.ResultMeta{Count: 1, Fields: []string{"id"}},
		})
	if err != nil {
		t.Fatalf("update template: %v", err)
	}
	if step != 2 {
		t.Errorf("expected step 2, got %d", step)
	}

	// Read back
	got, err := a.GetState(ctx, sessID)
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if got.Step != 2 {
		t.Errorf("state.step after 2 zone writes: %d, want 2", got.Step)
	}
	if got.Current.Template["version"] != "2.10" {
		t.Errorf("template content lost: %+v", got.Current.Template)
	}
	if len(got.Current.Data.Products) != 1 {
		t.Errorf("data lost: %+v", got.Current.Data)
	}

	// Delta stream
	deltas, err := a.GetDeltas(ctx, sessID)
	if err != nil {
		t.Fatalf("get deltas: %v", err)
	}
	if len(deltas) != 2 {
		t.Fatalf("expected 2 deltas, got %d", len(deltas))
	}
	if deltas[0].Step != 1 || deltas[1].Step != 2 {
		t.Errorf("delta step ordering: %d, %d", deltas[0].Step, deltas[1].Step)
	}
	if deltas[0].DeltaType != domain.DeltaTypeAdd || deltas[1].DeltaType != domain.DeltaTypeUpdate {
		t.Errorf("delta type mismatch: %s, %s", deltas[0].DeltaType, deltas[1].DeltaType)
	}

	// GetDeltasUntil(1) returns just the first
	until, err := a.GetDeltasUntil(ctx, sessID, 1)
	if err != nil {
		t.Fatalf("get deltas until: %v", err)
	}
	if len(until) != 1 || until[0].Step != 1 {
		t.Errorf("until 1 should yield 1 delta, got %d", len(until))
	}
}

func TestStateAdapterViewStack(t *testing.T) {
	c := setupClient(t)
	a := NewStateAdapter(c, nil)
	ctx := context.Background()
	sessID := newTestSession(t, c)
	if _, err := a.CreateState(ctx, sessID); err != nil {
		t.Fatalf("create: %v", err)
	}

	snap1 := &domain.ViewSnapshot{Mode: domain.ViewModeGrid, Step: 1, Refs: []domain.EntityRef{{Type: domain.EntityTypeProduct, ID: "p1"}}}
	if err := a.PushView(ctx, sessID, snap1); err != nil {
		t.Fatalf("push: %v", err)
	}
	stack, err := a.GetViewStack(ctx, sessID)
	if err != nil {
		t.Fatalf("get stack: %v", err)
	}
	if len(stack) != 1 || stack[0].Step != 1 {
		t.Errorf("stack after push: %+v", stack)
	}
	popped, err := a.PopView(ctx, sessID)
	if err != nil {
		t.Fatalf("pop: %v", err)
	}
	if popped == nil || popped.Step != 1 {
		t.Errorf("popped: %+v", popped)
	}
	stack, err = a.GetViewStack(ctx, sessID)
	if err != nil {
		t.Fatalf("get stack 2: %v", err)
	}
	if len(stack) != 0 {
		t.Errorf("stack after pop should be empty: %+v", stack)
	}
}
