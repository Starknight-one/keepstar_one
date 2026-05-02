package usecases

import (
	"context"
	"testing"

	"keepstar_v5/internal/domain"
)

// TestStateLifecycleSmoke wires together the full chunk-2 surface:
// CreateState → UpdateData → UpdateTemplate (scene-graph Document) →
// PushView → PopView → Rollback. Verifies the delta stream length and final
// state match expectations end-to-end. The mock port mirrors V4 zone-write
// semantics (zone update + delta append).
func TestStateLifecycleSmoke(t *testing.T) {
	port := newMockStatePort()
	ctx := context.Background()
	sess := "smoke"

	if _, err := port.CreateState(ctx, sess); err != nil {
		t.Fatalf("create: %v", err)
	}

	// 1. Agent1 loads products.
	if _, err := port.UpdateData(ctx, sess,
		domain.StateData{Products: []domain.Product{{ID: "p1"}, {ID: "p2"}, {ID: "p3"}}},
		domain.StateMeta{Count: 3, Fields: []string{"id", "name"}},
		domain.DeltaInfo{
			Trigger: domain.TriggerUserQuery, Source: domain.SourceLLM, ActorID: "agent1",
			DeltaType: domain.DeltaTypeAdd, Path: "data.products",
			Action: domain.Action{Type: domain.ActionSearch},
			Result: domain.ResultMeta{Count: 3, Fields: []string{"id", "name"}},
		},
	); err != nil {
		t.Fatalf("update data: %v", err)
	}

	// 2. Agent2 renders a scene-graph Document into the Template zone.
	sceneGraph := map[string]interface{}{
		"version": "2.10",
		"children": []interface{}{
			map[string]interface{}{"type": "frame", "id": "root"},
		},
	}
	if _, err := port.UpdateTemplate(ctx, sess, sceneGraph,
		domain.DeltaInfo{
			Trigger: domain.TriggerUserQuery, Source: domain.SourceLLM, ActorID: "agent2",
			DeltaType: domain.DeltaTypeUpdate, Path: "current.template",
			Action: domain.Action{Type: domain.ActionLayout},
			Result: domain.ResultMeta{Count: 3, Fields: []string{"id", "name"}},
		},
	); err != nil {
		t.Fatalf("update template: %v", err)
	}

	// 3. User clicks an item — push grid snapshot, switch to detail.
	gridSnap := &domain.ViewSnapshot{
		Mode: domain.ViewModeGrid, Step: 2,
		Refs: []domain.EntityRef{{Type: domain.EntityTypeProduct, ID: "p1"}},
	}
	if err := port.PushView(ctx, sess, gridSnap); err != nil {
		t.Fatalf("push view: %v", err)
	}
	stack, _ := port.GetViewStack(ctx, sess)
	if len(stack) != 1 {
		t.Fatalf("stack after push: %d", len(stack))
	}

	// 4. User hits back — pop returns the snapshot.
	popped, err := port.PopView(ctx, sess)
	if err != nil || popped == nil || popped.Step != 2 {
		t.Fatalf("pop view: popped=%+v err=%v", popped, err)
	}

	// 5. Reconstruct at step 2 — should yield 3 products in meta and the
	// scene-graph Document.
	uc := NewReconstructStateUseCase(port)
	rec, err := uc.Execute(ctx, ReconstructRequest{SessionID: sess, ToStep: 2})
	if err != nil {
		t.Fatalf("reconstruct: %v", err)
	}
	if rec.State.Current.Meta.Count != 3 {
		t.Errorf("meta count after replay: %d, want 3", rec.State.Current.Meta.Count)
	}
	if rec.State.Current.Template == nil || rec.State.Current.Template["version"] != "2.10" {
		t.Errorf("scene-graph not preserved: %+v", rec.State.Current.Template)
	}

	// 6. Rollback to step 1 — drops the Template, keeps the data.
	rb := NewRollbackUseCase(port)
	rbResp, err := rb.Execute(ctx, RollbackRequest{
		SessionID: sess, ToStep: 1,
		Source: domain.SourceUser, ActorID: "user_back",
	})
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if rbResp.RollbackDelta.Step != 3 {
		t.Errorf("rollback delta step: %d, want 3", rbResp.RollbackDelta.Step)
	}
	if rbResp.State.Current.Template != nil {
		t.Errorf("step-1 replay should have no template: %+v", rbResp.State.Current.Template)
	}

	// Final delta count: 2 zone-writes + 1 rollback = 3.
	all, _ := port.GetDeltas(ctx, sess)
	if len(all) != 3 {
		t.Errorf("delta stream length: %d, want 3", len(all))
	}
}
