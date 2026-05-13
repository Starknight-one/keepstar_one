package domain

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDeltaRoundTrip(t *testing.T) {
	src := Delta{
		Step:      3,
		TurnID:    "turn-abc",
		Trigger:   TriggerUserQuery,
		Source:    SourceLLM,
		ActorID:   "agent1",
		DeltaType: DeltaTypeAdd,
		Path:      "data.products",
		Action: Action{
			Type:   ActionSearch,
			Tool:   "catalog_search",
			Params: map[string]interface{}{"q": "cream"},
		},
		Result: ResultMeta{
			Count:   12,
			Fields:  []string{"id", "name", "price"},
			Aliases: map[string]string{"name": "displayName"},
		},
		Template:  map[string]interface{}{"version": "2.10"},
		CreatedAt: time.Date(2026, 5, 2, 18, 0, 0, 0, time.UTC),
	}

	raw, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var dst Delta
	if err := json.Unmarshal(raw, &dst); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if dst.Step != 3 || dst.TurnID != "turn-abc" {
		t.Errorf("identity drift: %+v", dst)
	}
	if dst.Trigger != TriggerUserQuery || dst.Source != SourceLLM {
		t.Errorf("trigger/source drift: %+v", dst)
	}
	if dst.DeltaType != DeltaTypeAdd || dst.Path != "data.products" {
		t.Errorf("type/path drift: %+v", dst)
	}
	if dst.Action.Type != ActionSearch || dst.Action.Tool != "catalog_search" {
		t.Errorf("action drift: %+v", dst.Action)
	}
	if dst.Result.Count != 12 || len(dst.Result.Fields) != 3 {
		t.Errorf("result drift: %+v", dst.Result)
	}
	if dst.Template["version"] != "2.10" {
		t.Errorf("template drift: %+v", dst.Template)
	}
	if !dst.CreatedAt.Equal(src.CreatedAt) {
		t.Errorf("created_at drift: %v vs %v", dst.CreatedAt, src.CreatedAt)
	}
}

func TestDeltaInfoToDelta(t *testing.T) {
	info := DeltaInfo{
		TurnID:    "turn-1",
		Trigger:   TriggerWidgetAction,
		Source:    SourceUser,
		ActorID:   "user_click",
		DeltaType: DeltaTypeUpdate,
		Path:      "actions.likedIds",
		Action:    Action{Type: ActionLike},
		Result:    ResultMeta{Count: 1, Fields: []string{"id"}},
	}
	before := time.Now()
	d := info.ToDelta()
	after := time.Now()

	if d.TurnID != "turn-1" || d.Trigger != TriggerWidgetAction {
		t.Errorf("info → delta drift: %+v", d)
	}
	if d.Source != SourceUser || d.ActorID != "user_click" {
		t.Errorf("source/actor drift: %+v", d)
	}
	if d.Action.Type != ActionLike {
		t.Errorf("action drift: %+v", d.Action)
	}
	if d.CreatedAt.Before(before) || d.CreatedAt.After(after) {
		t.Errorf("created_at not stamped at call time: %v not in [%v, %v]", d.CreatedAt, before, after)
	}
	if d.Step != 0 {
		t.Errorf("ToDelta should leave Step=0 (assigned later by AddDelta), got %d", d.Step)
	}
}
