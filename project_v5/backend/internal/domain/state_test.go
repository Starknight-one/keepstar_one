package domain

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestSessionStateRoundTrip verifies that a non-trivial SessionState — with
// scene-graph Template, view stack, actions, and an LLMMessage in
// ConversationHistory — survives Marshal → Unmarshal with the same shape.
func TestSessionStateRoundTrip(t *testing.T) {
	src := SessionState{
		ID:        "state-1",
		SessionID: "sess-1",
		Current: StateCurrent{
			Data: StateData{
				Products: []Product{{ID: "p1", Name: "Cream"}},
			},
			Meta: StateMeta{Count: 1, Fields: []string{"id", "name"}},
			Template: map[string]interface{}{
				"version":  "2.10",
				"children": []interface{}{map[string]interface{}{"type": "frame", "id": "root"}},
			},
		},
		View:      ViewState{Mode: ViewModeGrid},
		ViewStack: []ViewSnapshot{{Mode: ViewModeGrid, Refs: []EntityRef{{Type: EntityTypeProduct, ID: "p1"}}, Step: 1}},
		Actions: StateActions{
			LikedIds:  []string{"p1"},
			CartItems: []CartItem{{EntityType: EntityTypeProduct, EntityId: "p1", Quantity: 2}},
		},
		ConversationHistory: []LLMMessage{
			{Role: "user", Content: "show me creams"},
			{Role: "assistant", ToolCalls: []ToolCall{{ID: "t1", Name: "catalog_search", Input: map[string]interface{}{"q": "cream"}}}},
		},
		Step: 1,
	}

	raw, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var dst SessionState
	if err := json.Unmarshal(raw, &dst); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if dst.ID != src.ID || dst.SessionID != src.SessionID {
		t.Errorf("identity drift: %+v vs %+v", dst, src)
	}
	if dst.Current.Meta.Count != 1 || len(dst.Current.Meta.Fields) != 2 {
		t.Errorf("meta drift: %+v", dst.Current.Meta)
	}
	if dst.Current.Template["version"] != "2.10" {
		t.Errorf("template version drift: %+v", dst.Current.Template)
	}
	if len(dst.Actions.LikedIds) != 1 || dst.Actions.LikedIds[0] != "p1" {
		t.Errorf("liked ids drift: %+v", dst.Actions.LikedIds)
	}
	if len(dst.Actions.CartItems) != 1 || dst.Actions.CartItems[0].Quantity != 2 {
		t.Errorf("cart drift: %+v", dst.Actions.CartItems)
	}
	if len(dst.ConversationHistory) != 2 {
		t.Errorf("conversation history len: %d", len(dst.ConversationHistory))
	}
	if dst.ConversationHistory[1].ToolCalls[0].Name != "catalog_search" {
		t.Errorf("tool call drift: %+v", dst.ConversationHistory[1].ToolCalls)
	}
	if !reflect.DeepEqual(dst.ViewStack[0].Refs, src.ViewStack[0].Refs) {
		t.Errorf("view stack refs drift: %+v vs %+v", dst.ViewStack[0].Refs, src.ViewStack[0].Refs)
	}
}

func TestStateActionsZeroValueIsUsable(t *testing.T) {
	var a StateActions
	if a.LikedIds != nil || a.CartItems != nil {
		t.Errorf("zero value should be nil slices: %+v", a)
	}
	// Marshalling a zero-value StateActions should still produce valid JSON.
	if _, err := json.Marshal(a); err != nil {
		t.Fatalf("marshal zero value: %v", err)
	}
}
