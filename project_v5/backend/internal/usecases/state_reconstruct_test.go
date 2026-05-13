package usecases

import (
	"context"
	"reflect"
	"testing"

	"keepstar_v5/internal/domain"
)

// TestReconstructAcrossFiveSteps replays a 5-delta sequence (search → filter →
// push → pop → layout) and verifies the reconstructed state is deterministic.
// Replaying twice produces an identical state.
func TestReconstructAcrossFiveSteps(t *testing.T) {
	port := newMockStatePort()
	ctx := context.Background()
	sess := "sess-replay"
	if _, err := port.CreateState(ctx, sess); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Step 1 — add 5 products
	if _, err := port.UpdateData(ctx, sess,
		domain.StateData{Products: []domain.Product{{ID: "p1"}, {ID: "p2"}, {ID: "p3"}, {ID: "p4"}, {ID: "p5"}}},
		domain.StateMeta{Count: 5, Fields: []string{"id", "name"}},
		domain.DeltaInfo{Trigger: domain.TriggerUserQuery, Source: domain.SourceLLM, ActorID: "agent1",
			DeltaType: domain.DeltaTypeAdd, Path: "data.products",
			Action: domain.Action{Type: domain.ActionSearch},
			Result: domain.ResultMeta{Count: 5, Fields: []string{"id", "name"}}},
	); err != nil {
		t.Fatalf("step 1: %v", err)
	}

	// Step 2 — filter to 2
	if _, err := port.UpdateData(ctx, sess,
		domain.StateData{Products: []domain.Product{{ID: "p1"}, {ID: "p2"}}},
		domain.StateMeta{Count: 2, Fields: []string{"id", "name"}},
		domain.DeltaInfo{Trigger: domain.TriggerUserQuery, Source: domain.SourceLLM, ActorID: "agent1",
			DeltaType: domain.DeltaTypeUpdate, Path: "data.products",
			Action: domain.Action{Type: domain.ActionFilter},
			Result: domain.ResultMeta{Count: 2, Fields: []string{"id", "name"}}},
	); err != nil {
		t.Fatalf("step 2: %v", err)
	}

	// Step 3 — push view (delta records a push, no payload)
	if _, err := port.UpdateView(ctx, sess,
		domain.ViewState{Mode: domain.ViewModeDetail, Focused: &domain.EntityRef{Type: domain.EntityTypeProduct, ID: "p1"}},
		[]domain.ViewSnapshot{{Mode: domain.ViewModeGrid, Step: 2}},
		domain.DeltaInfo{Trigger: domain.TriggerWidgetAction, Source: domain.SourceUser, ActorID: "user_click",
			DeltaType: domain.DeltaTypePush, Path: "viewStack",
			Action: domain.Action{Type: domain.ActionLayout},
			Result: domain.ResultMeta{Count: 2, Fields: []string{"id", "name"}}},
	); err != nil {
		t.Fatalf("step 3: %v", err)
	}

	// Step 4 — pop back
	if _, err := port.UpdateView(ctx, sess,
		domain.ViewState{Mode: domain.ViewModeGrid},
		[]domain.ViewSnapshot{},
		domain.DeltaInfo{Trigger: domain.TriggerWidgetAction, Source: domain.SourceUser, ActorID: "user_back",
			DeltaType: domain.DeltaTypePop, Path: "viewStack",
			Action: domain.Action{Type: domain.ActionLayout},
			Result: domain.ResultMeta{Count: 2, Fields: []string{"id", "name"}}},
	); err != nil {
		t.Fatalf("step 4: %v", err)
	}

	// Step 5 — change layout (template zone — scene-graph Document JSON)
	if _, err := port.UpdateTemplate(ctx, sess,
		map[string]interface{}{"version": "2.10", "children": []interface{}{}},
		domain.DeltaInfo{Trigger: domain.TriggerUserQuery, Source: domain.SourceLLM, ActorID: "agent2",
			DeltaType: domain.DeltaTypeUpdate, Path: "current.template",
			Action: domain.Action{Type: domain.ActionLayout},
			Result: domain.ResultMeta{Count: 2, Fields: []string{"id", "name"}}},
	); err != nil {
		t.Fatalf("step 5: %v", err)
	}

	uc := NewReconstructStateUseCase(port)

	// Replay to step 5
	resp, err := uc.Execute(ctx, ReconstructRequest{SessionID: sess, ToStep: 5})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if resp.DeltaCount != 5 {
		t.Errorf("expected 5 deltas, got %d", resp.DeltaCount)
	}
	if resp.State.Step != 5 {
		t.Errorf("expected final step 5, got %d", resp.State.Step)
	}
	if resp.State.Current.Meta.Count != 2 {
		t.Errorf("expected meta.count=2 after filter, got %d", resp.State.Current.Meta.Count)
	}
	if resp.State.Current.Template == nil || resp.State.Current.Template["version"] != "2.10" {
		t.Errorf("template not replayed: %+v", resp.State.Current.Template)
	}

	// Determinism — replay twice
	resp2, err := uc.Execute(ctx, ReconstructRequest{SessionID: sess, ToStep: 5})
	if err != nil {
		t.Fatalf("execute 2: %v", err)
	}
	if !reflect.DeepEqual(resp.State.Current.Meta, resp2.State.Current.Meta) {
		t.Errorf("non-deterministic replay: %+v vs %+v", resp.State.Current.Meta, resp2.State.Current.Meta)
	}

	// Replay to step 2 — should yield count=2 (filter result), no template.
	resp3, err := uc.Execute(ctx, ReconstructRequest{SessionID: sess, ToStep: 2})
	if err != nil {
		t.Fatalf("execute 3: %v", err)
	}
	if resp3.State.Step != 2 || resp3.State.Current.Meta.Count != 2 {
		t.Errorf("partial replay drift: step=%d count=%d", resp3.State.Step, resp3.State.Current.Meta.Count)
	}
	if resp3.State.Current.Template != nil {
		t.Errorf("step-2 replay should have no template (set at step 5): %+v", resp3.State.Current.Template)
	}
}

func TestReconstructEmptyHistory(t *testing.T) {
	port := newMockStatePort()
	ctx := context.Background()
	if _, err := port.CreateState(ctx, "empty"); err != nil {
		t.Fatalf("create: %v", err)
	}
	uc := NewReconstructStateUseCase(port)
	resp, err := uc.Execute(ctx, ReconstructRequest{SessionID: "empty", ToStep: 0})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if resp.State.Step != 0 || resp.DeltaCount != 0 {
		t.Errorf("empty replay: step=%d count=%d", resp.State.Step, resp.DeltaCount)
	}
}
