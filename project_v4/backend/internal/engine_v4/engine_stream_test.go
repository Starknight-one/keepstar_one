package engine_v4

import (
	"testing"
)

// TestExecuteWidgetPartial_SingleWidget verifies a single widget group
// produces one widget with bound data, no cross-widget work performed.
func TestExecuteWidgetPartial_SingleWidget(t *testing.T) {
	e := NewEngine()
	ops := []Op{
		{Type: OpInsert, Ref: "w", Parent: "formation", Props: map[string]interface{}{"type": "widget", "size": "medium"}},
		{Type: OpInsert, Parent: "$w", Props: map[string]interface{}{"type": "text", "fieldName": "name"}},
		{Type: OpInsert, Parent: "$w", Props: map[string]interface{}{"type": "number", "fieldName": "price", "format": "currency"}},
	}
	out := e.ExecuteWidgetPartial(PartialInput{
		Ops:        ops,
		Data:       []map[string]interface{}{{"name": "Alpha", "price": 9.99}},
		EntityType: "product",
	})
	if len(out.Widgets) != 1 {
		t.Fatalf("widgets=%d, want 1", len(out.Widgets))
	}
	w := out.Widgets[0]
	if len(w.Atoms) != 2 {
		t.Errorf("atoms=%d, want 2", len(w.Atoms))
	}
	// Data should be bound
	if w.Atoms[0].Value != "Alpha" {
		t.Errorf("name=%v, want Alpha", w.Atoms[0].Value)
	}
}

// TestFinalizeFormation_CrossWidgetNoGroups verifies finalize runs cleanly
// on literal widgets (no GroupID) — no cross-widget removal applies.
func TestFinalizeFormation_CrossWidgetNoGroups(t *testing.T) {
	e := NewEngine()
	w1 := e.ExecuteWidgetPartial(PartialInput{
		Ops: []Op{
			{Type: OpInsert, Ref: "h", Parent: "formation", Props: map[string]interface{}{"type": "widget", "size": "large"}},
			{Type: OpInsert, Parent: "$h", Props: map[string]interface{}{"type": "text", "value": "Hero"}},
		},
	})
	w2 := e.ExecuteWidgetPartial(PartialInput{
		Ops: []Op{
			{Type: OpInsert, Ref: "c", Parent: "formation", Props: map[string]interface{}{"type": "widget", "size": "small"}},
			{Type: OpInsert, Parent: "$c", Props: map[string]interface{}{"type": "text", "fieldName": "name"}},
		},
		Data: []map[string]interface{}{{"name": "Card"}},
	})
	all := append(w1.Widgets, w2.Widgets...)
	out := e.FinalizeFormation(all, "grid", 3)
	if out.Formation == nil {
		t.Fatal("nil formation")
	}
	// Both widgets should survive (no cross-widget group to apply C1)
	total := len(out.Formation.Widgets)
	for _, s := range out.Formation.Sections {
		total += len(s.Widgets)
	}
	if total != 2 {
		t.Errorf("total widgets after finalize=%d, want 2", total)
	}
	if out.TreeMap == nil {
		t.Error("tree map not built")
	}
}
