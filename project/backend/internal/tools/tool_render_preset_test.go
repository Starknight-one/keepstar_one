package tools_test

import (
	"context"
	"testing"

	"keepstar/internal/domain"
	"keepstar/internal/presets"
	"keepstar/internal/tools"
)

func TestRenderProductPreset_UsesUpdateTemplate(t *testing.T) {
	state := &domain.SessionState{
		ID: "s1", SessionID: "sess-1",
		Current: domain.StateCurrent{
			Data: domain.StateData{
				Products: []domain.Product{
					{ID: "p1", Name: "Nike Air Max", Price: 12990, Currency: "$", Brand: "Nike"},
					{ID: "p2", Name: "Nike Dunk", Price: 9990, Currency: "$", Brand: "Nike"},
				},
			},
			Meta: domain.StateMeta{Count: 2, Fields: []string{"id", "name", "price", "brand"}},
		},
	}
	sp := newMockStatePort(state)
	pr := presets.NewPresetRegistry()
	tool := tools.NewRenderProductPresetTool(sp, pr)

	ctx := context.Background()
	toolCtx := tools.ToolContext{SessionID: "sess-1", TurnID: "turn-1", ActorID: "agent2"}
	result, err := tool.Execute(ctx, toolCtx, map[string]interface{}{"preset": "product_grid"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}

	// UpdateTemplate must be called, NOT UpdateState
	if sp.UpdateTemplateCalls != 1 {
		t.Errorf("expected 1 UpdateTemplate call, got %d", sp.UpdateTemplateCalls)
	}
	if sp.UpdateStateCalls != 0 {
		t.Errorf("expected 0 UpdateState calls, got %d", sp.UpdateStateCalls)
	}
}

func TestRenderProductPreset_DataZoneUntouched(t *testing.T) {
	originalProducts := []domain.Product{
		{ID: "p1", Name: "Nike Air Max", Price: 12990, Currency: "$", Brand: "Nike"},
	}
	state := &domain.SessionState{
		ID: "s1", SessionID: "sess-1",
		Current: domain.StateCurrent{
			Data: domain.StateData{Products: originalProducts},
			Meta: domain.StateMeta{Count: 1, Fields: []string{"id", "name", "price"}},
		},
	}
	sp := newMockStatePort(state)
	pr := presets.NewPresetRegistry()
	tool := tools.NewRenderProductPresetTool(sp, pr)

	ctx := context.Background()
	toolCtx := tools.ToolContext{SessionID: "sess-1", TurnID: "turn-1", ActorID: "agent2"}
	_, err := tool.Execute(ctx, toolCtx, map[string]interface{}{"preset": "product_grid"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Data zone must NOT be modified by render tool
	if sp.UpdateDataCalls != 0 {
		t.Errorf("expected 0 UpdateData calls, got %d", sp.UpdateDataCalls)
	}

	// Products must still be the same
	if len(state.Current.Data.Products) != 1 {
		t.Errorf("expected 1 product unchanged, got %d", len(state.Current.Data.Products))
	}
	if state.Current.Data.Products[0].ID != "p1" {
		t.Errorf("expected product ID p1, got %s", state.Current.Data.Products[0].ID)
	}
}

func TestRenderServicePreset_UsesUpdateTemplate(t *testing.T) {
	state := &domain.SessionState{
		ID: "s1", SessionID: "sess-1",
		Current: domain.StateCurrent{
			Data: domain.StateData{
				Services: []domain.Service{
					{ID: "s1", Name: "Massage", Price: 5000, Currency: "$"},
				},
			},
			Meta: domain.StateMeta{ServiceCount: 1},
		},
	}
	sp := newMockStatePort(state)
	pr := presets.NewPresetRegistry()
	tool := tools.NewRenderServicePresetTool(sp, pr)

	ctx := context.Background()
	toolCtx := tools.ToolContext{SessionID: "sess-1", TurnID: "turn-1", ActorID: "agent2"}
	result, err := tool.Execute(ctx, toolCtx, map[string]interface{}{"preset": "service_card"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", result.Content)
	}

	if sp.UpdateTemplateCalls != 1 {
		t.Errorf("expected 1 UpdateTemplate call, got %d", sp.UpdateTemplateCalls)
	}
	if sp.UpdateStateCalls != 0 {
		t.Errorf("expected 0 UpdateState calls, got %d", sp.UpdateStateCalls)
	}
}
