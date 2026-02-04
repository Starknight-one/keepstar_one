package domain

import (
	"testing"
	"time"
)

func TestDeltaInfo_ToDelta(t *testing.T) {
	info := DeltaInfo{
		TurnID:    "turn-123",
		Trigger:   TriggerUserQuery,
		Source:    SourceLLM,
		ActorID:   "agent1",
		DeltaType: DeltaTypeAdd,
		Path:      "data.products",
		Action: Action{
			Type:   ActionSearch,
			Tool:   "search_products",
			Params: map[string]interface{}{"query": "nike"},
		},
		Result: ResultMeta{
			Count:  10,
			Fields: []string{"name", "price"},
		},
	}

	before := time.Now()
	delta := info.ToDelta()
	after := time.Now()

	// Verify all fields transferred
	if delta.TurnID != "turn-123" {
		t.Errorf("TurnID: expected turn-123, got %s", delta.TurnID)
	}
	if delta.Trigger != TriggerUserQuery {
		t.Errorf("Trigger: expected USER_QUERY, got %s", delta.Trigger)
	}
	if delta.Source != SourceLLM {
		t.Errorf("Source: expected llm, got %s", delta.Source)
	}
	if delta.ActorID != "agent1" {
		t.Errorf("ActorID: expected agent1, got %s", delta.ActorID)
	}
	if delta.DeltaType != DeltaTypeAdd {
		t.Errorf("DeltaType: expected add, got %s", delta.DeltaType)
	}
	if delta.Path != "data.products" {
		t.Errorf("Path: expected data.products, got %s", delta.Path)
	}
	if delta.Action.Type != ActionSearch {
		t.Errorf("Action.Type: expected SEARCH, got %s", delta.Action.Type)
	}
	if delta.Action.Tool != "search_products" {
		t.Errorf("Action.Tool: expected search_products, got %s", delta.Action.Tool)
	}
	if delta.Result.Count != 10 {
		t.Errorf("Result.Count: expected 10, got %d", delta.Result.Count)
	}
	if len(delta.Result.Fields) != 2 {
		t.Errorf("Result.Fields: expected 2, got %d", len(delta.Result.Fields))
	}

	// Step should be zero (not set by ToDelta — assigned by AddDelta)
	if delta.Step != 0 {
		t.Errorf("Step: expected 0 (unset), got %d", delta.Step)
	}

	// Template should be nil (not in DeltaInfo)
	if delta.Template != nil {
		t.Error("Template: expected nil")
	}

	// CreatedAt should be approximately now
	if delta.CreatedAt.Before(before) || delta.CreatedAt.After(after) {
		t.Errorf("CreatedAt: expected between %v and %v, got %v", before, after, delta.CreatedAt)
	}
}

func TestDeltaInfo_ToDelta_EmptyTurnID(t *testing.T) {
	info := DeltaInfo{
		Trigger:   TriggerWidgetAction,
		Source:    SourceUser,
		ActorID:   "user_expand",
		DeltaType: DeltaTypePush,
		Path:      "view",
	}

	delta := info.ToDelta()

	if delta.TurnID != "" {
		t.Errorf("TurnID: expected empty, got %s", delta.TurnID)
	}
	if delta.Source != SourceUser {
		t.Errorf("Source: expected user, got %s", delta.Source)
	}
}

func TestDeltaInfo_ToDelta_IndependentCalls(t *testing.T) {
	// Two ToDelta calls from same DeltaInfo should create independent deltas
	info := DeltaInfo{
		TurnID:    "turn-same",
		Trigger:   TriggerUserQuery,
		Source:    SourceLLM,
		ActorID:   "agent1",
		DeltaType: DeltaTypeAdd,
		Path:      "data.products",
	}

	delta1 := info.ToDelta()
	delta2 := info.ToDelta()

	// They should have same values but be different pointers
	if delta1 == delta2 {
		t.Error("Expected different pointers from separate ToDelta calls")
	}
	if delta1.TurnID != delta2.TurnID {
		t.Error("Expected same TurnID from same DeltaInfo")
	}
}
