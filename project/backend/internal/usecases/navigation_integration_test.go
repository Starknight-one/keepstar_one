package usecases_test

import (
	"context"
	"testing"

	"keepstar/internal/adapters/postgres"
	"keepstar/internal/domain"
	"keepstar/internal/logger"
	"keepstar/internal/presets"
	"keepstar/internal/testutil"
	"keepstar/internal/usecases"
)

// navSetup creates DB-backed adapters for navigation integration tests.
func navSetup(t *testing.T) (context.Context, *postgres.StateAdapter, string) {
	t.Helper()
	client := testutil.TestDB(t)
	log := logger.New("error")
	adapter := postgres.NewStateAdapter(client, log)
	sessionID := testutil.TestStateWithProducts(t, client, 4)
	return context.Background(), adapter, sessionID
}

// TestNavIntegration_ExpandWritesDB verifies Expand writes to real Postgres.
func TestNavIntegration_ExpandWritesDB(t *testing.T) {
	ctx, adapter, sessionID := navSetup(t)
	presetRegistry := presets.NewPresetRegistry()
	expandUC := usecases.NewExpandUseCase(adapter, presetRegistry)

	resp, err := expandUC.Execute(ctx, usecases.ExpandRequest{
		SessionID:  sessionID,
		EntityType: domain.EntityTypeProduct,
		EntityID:   "prod-001",
		TurnID:     "turn-nav-1",
	})
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if resp.ViewMode != domain.ViewModeDetail {
		t.Errorf("want detail, got %s", resp.ViewMode)
	}

	// Verify state persisted to DB
	state, err := adapter.GetState(ctx, sessionID)
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if state.View.Mode != domain.ViewModeDetail {
		t.Errorf("DB view: want detail, got %s", state.View.Mode)
	}
	if state.View.Focused == nil || state.View.Focused.ID != "prod-001" {
		t.Error("DB focused: expected prod-001")
	}

	// Verify deltas written
	deltas, err := adapter.GetDeltas(ctx, sessionID)
	if err != nil {
		t.Fatalf("get deltas: %v", err)
	}
	// UpdateData from testutil + 2 zone-writes from Expand (view + template)
	if len(deltas) < 3 {
		t.Errorf("want >= 3 deltas, got %d", len(deltas))
	}
}

// TestNavIntegration_ExpandBackRoundtrip verifies Expand → Back restores grid in DB.
func TestNavIntegration_ExpandBackRoundtrip(t *testing.T) {
	ctx, adapter, sessionID := navSetup(t)
	presetRegistry := presets.NewPresetRegistry()
	expandUC := usecases.NewExpandUseCase(adapter, presetRegistry)
	backUC := usecases.NewBackUseCase(adapter, presetRegistry)

	// Expand
	_, err := expandUC.Execute(ctx, usecases.ExpandRequest{
		SessionID:  sessionID,
		EntityType: domain.EntityTypeProduct,
		EntityID:   "prod-002",
		TurnID:     "turn-nav-2",
	})
	if err != nil {
		t.Fatalf("expand: %v", err)
	}

	// Back
	backResp, err := backUC.Execute(ctx, usecases.BackRequest{
		SessionID: sessionID,
		TurnID:    "turn-nav-3",
	})
	if err != nil {
		t.Fatalf("back: %v", err)
	}
	if backResp.ViewMode != domain.ViewModeGrid {
		t.Errorf("back: want grid, got %s", backResp.ViewMode)
	}

	// Verify DB state
	state, err := adapter.GetState(ctx, sessionID)
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if state.View.Mode != domain.ViewModeGrid {
		t.Errorf("DB: want grid, got %s", state.View.Mode)
	}
}

// TestNavIntegration_DeepStack tests 5 expand → 5 back.
func TestNavIntegration_DeepStack(t *testing.T) {
	ctx, adapter, sessionID := navSetup(t)
	presetRegistry := presets.NewPresetRegistry()
	expandUC := usecases.NewExpandUseCase(adapter, presetRegistry)
	backUC := usecases.NewBackUseCase(adapter, presetRegistry)

	productIDs := []string{"prod-001", "prod-002", "prod-003", "prod-004", "prod-001"}

	// 5 expands
	for i, pid := range productIDs {
		_, err := expandUC.Execute(ctx, usecases.ExpandRequest{
			SessionID:  sessionID,
			EntityType: domain.EntityTypeProduct,
			EntityID:   pid,
			TurnID:     "turn-deep-" + pid,
		})
		if err != nil {
			t.Fatalf("expand[%d] %s: %v", i, pid, err)
		}
	}

	// Verify stack depth
	stack, err := adapter.GetViewStack(ctx, sessionID)
	if err != nil {
		t.Fatalf("get stack: %v", err)
	}
	if len(stack) != 5 {
		t.Errorf("want stack size 5, got %d", len(stack))
	}

	// 5 backs
	for i := 0; i < 5; i++ {
		resp, err := backUC.Execute(ctx, usecases.BackRequest{
			SessionID: sessionID,
			TurnID:    "turn-back-deep",
		})
		if err != nil {
			t.Fatalf("back[%d]: %v", i, err)
		}
		if i < 4 && !resp.CanGoBack {
			t.Errorf("back[%d]: expected canGoBack=true", i)
		}
	}

	// Final state should be grid
	state, err := adapter.GetState(ctx, sessionID)
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if state.View.Mode != domain.ViewModeGrid {
		t.Errorf("after all backs: want grid, got %s", state.View.Mode)
	}
}

// TestNavIntegration_ExpandBackExpand verifies expand→back→expand on different product.
func TestNavIntegration_ExpandBackExpand(t *testing.T) {
	ctx, adapter, sessionID := navSetup(t)
	presetRegistry := presets.NewPresetRegistry()
	expandUC := usecases.NewExpandUseCase(adapter, presetRegistry)
	backUC := usecases.NewBackUseCase(adapter, presetRegistry)

	// Expand prod-001
	_, err := expandUC.Execute(ctx, usecases.ExpandRequest{
		SessionID: sessionID, EntityType: domain.EntityTypeProduct,
		EntityID: "prod-001", TurnID: "t1",
	})
	if err != nil {
		t.Fatalf("expand 1: %v", err)
	}

	// Back
	_, err = backUC.Execute(ctx, usecases.BackRequest{SessionID: sessionID, TurnID: "t2"})
	if err != nil {
		t.Fatalf("back: %v", err)
	}

	// Expand different product
	resp, err := expandUC.Execute(ctx, usecases.ExpandRequest{
		SessionID: sessionID, EntityType: domain.EntityTypeProduct,
		EntityID: "prod-003", TurnID: "t3",
	})
	if err != nil {
		t.Fatalf("expand 2: %v", err)
	}
	if resp.Focused == nil || resp.Focused.ID != "prod-003" {
		t.Error("expected focused prod-003")
	}
}

// TestNavIntegration_BackEmptyStack tests back on fresh state.
func TestNavIntegration_BackEmptyStack(t *testing.T) {
	ctx, adapter, sessionID := navSetup(t)
	presetRegistry := presets.NewPresetRegistry()
	backUC := usecases.NewBackUseCase(adapter, presetRegistry)

	resp, err := backUC.Execute(ctx, usecases.BackRequest{
		SessionID: sessionID, TurnID: "t-empty",
	})
	if err != nil {
		t.Fatalf("back empty: %v", err)
	}
	if resp.CanGoBack {
		t.Error("expected canGoBack=false on empty stack")
	}
}
