package usecases

import (
	"context"
	"testing"

	"keepstar_v5/internal/domain"
)

// TestRollbackToStep2 builds a 4-step session, rolls back to step 2, and
// verifies: state matches step-2 reconstruction, a rollback delta is
// appended, and the new step number is 5 (not 2 — rollback is forward in
// delta-stream history).
func TestRollbackToStep2(t *testing.T) {
	port := newMockStatePort()
	ctx := context.Background()
	sess := "sess-rollback"
	if _, err := port.CreateState(ctx, sess); err != nil {
		t.Fatalf("create: %v", err)
	}

	infoOf := func(t domain.DeltaType, n int, action domain.ActionType) domain.DeltaInfo {
		return domain.DeltaInfo{
			Trigger:   domain.TriggerUserQuery,
			Source:    domain.SourceLLM,
			ActorID:   "agent1",
			DeltaType: t,
			Path:      "data.products",
			Action:    domain.Action{Type: action},
			Result:    domain.ResultMeta{Count: n, Fields: []string{"id"}},
		}
	}

	for i, n := range []int{5, 3, 2, 1} {
		dt := domain.DeltaTypeAdd
		if i > 0 {
			dt = domain.DeltaTypeUpdate
		}
		if _, err := port.UpdateData(ctx, sess,
			domain.StateData{}, domain.StateMeta{Count: n, Fields: []string{"id"}},
			infoOf(dt, n, domain.ActionFilter),
		); err != nil {
			t.Fatalf("step %d: %v", i+1, err)
		}
	}

	// state.step should now be 4
	cur, _ := port.GetState(ctx, sess)
	if cur.Step != 4 {
		t.Fatalf("pre-rollback step: %d, want 4", cur.Step)
	}

	uc := NewRollbackUseCase(port)
	resp, err := uc.Execute(ctx, RollbackRequest{
		SessionID: sess,
		ToStep:    2,
		Source:    domain.SourceUser,
		ActorID:   "user_back",
	})
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}

	if resp.FromStep != 4 || resp.ToStep != 2 || resp.RolledBack != 2 {
		t.Errorf("rollback delta accounting: from=%d to=%d rolled=%d", resp.FromStep, resp.ToStep, resp.RolledBack)
	}
	if resp.RollbackDelta == nil || resp.RollbackDelta.DeltaType != domain.DeltaTypeRollback {
		t.Errorf("rollback delta missing or wrong type: %+v", resp.RollbackDelta)
	}
	if resp.RollbackDelta.Step != 5 {
		t.Errorf("rollback delta step: %d, want 5 (next in stream)", resp.RollbackDelta.Step)
	}
	// State after rollback: meta from step 2 (count=3), step bumped to 5.
	if resp.State.Step != 5 {
		t.Errorf("post-rollback state.step: %d, want 5", resp.State.Step)
	}
	if resp.State.Current.Meta.Count != 3 {
		t.Errorf("post-rollback meta.count: %d, want 3 (state at step 2)", resp.State.Current.Meta.Count)
	}

	// Rollback delta is in the stream now.
	all, _ := port.GetDeltas(ctx, sess)
	if len(all) != 5 {
		t.Errorf("delta stream length after rollback: %d, want 5", len(all))
	}
}

func TestRollbackForwardRejected(t *testing.T) {
	port := newMockStatePort()
	ctx := context.Background()
	sess := "sess-fw"
	if _, err := port.CreateState(ctx, sess); err != nil {
		t.Fatalf("create: %v", err)
	}
	uc := NewRollbackUseCase(port)

	if _, err := uc.Execute(ctx, RollbackRequest{SessionID: sess, ToStep: 5}); err == nil {
		t.Error("expected error when rolling back forward (current=0, target=5)")
	}
	if _, err := uc.Execute(ctx, RollbackRequest{SessionID: sess, ToStep: -1}); err == nil {
		t.Error("expected error on negative target step")
	}
}
