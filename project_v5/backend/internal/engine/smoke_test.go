package engine

import "testing"

// TestSmokeButtonComponent is the verification scenario from the V5 Phase-1
// plan: build a reusable button component, instance it via RefNode with
// descendant overrides, expand, then mutate the resolved tree and undo.
func TestSmokeButtonComponent(t *testing.T) {
	doc := NewDocument()
	hist := NewCommandHistory()

	// 1. Build a reusable Button component (Frame containing Rect + Text).
	if err := hist.Execute(NewInsertCommand("", Node{
		"type":     "frame",
		"id":       "btn",
		"name":     "Button",
		"reusable": true,
		"layout":   "horizontal",
		"padding":  []any{8, 16},
		"children": []Node{
			{"type": "rectangle", "id": "bg", "fill": "#5BA4D9", "cornerRadius": 8},
			{"type": "text", "id": "label", "content": "Click me", "fontSize": 14, "fill": "#FFFFFF"},
		},
	}), doc); err != nil {
		t.Fatal(err)
	}

	// 2. Insert a RefNode pointing to the button, override the label.
	if err := hist.Execute(NewInsertCommand("", Node{
		"type": "ref",
		"id":   "btn_submit",
		"ref":  "btn",
		"descendants": map[string]any{
			"label": map[string]any{"content": "Submit"},
		},
	}), doc); err != nil {
		t.Fatal(err)
	}

	// 3. Expand the RefNode and assert the override is visible.
	cr := NewComponentResolver()
	res := cr.Expand(FindNodeByID(doc, "btn_submit"), doc)
	if !res.Ok {
		t.Fatalf("expand failed: %s", res.Error)
	}
	expanded := res.Root
	if NodeID(expanded) != "btn_submit" {
		t.Fatalf("expanded root id = %q, want btn_submit", NodeID(expanded))
	}
	cs := Children(expanded)
	if len(cs) != 2 {
		t.Fatalf("expanded children count = %d, want 2", len(cs))
	}
	if cs[1]["content"] != "Submit" {
		t.Fatalf("descendant override missed: label content = %v, want Submit", cs[1]["content"])
	}

	// 4. Mutate the resolved tree's text via UpdateCommand.
	//    The expanded tree IS NOT in doc (it's a clone), so to test the
	//    full Update→Undo flow we register the expanded root in a side doc.
	side := NewDocument()
	side.Children = []Node{expanded}
	hist2 := NewCommandHistory()
	if err := hist2.Execute(NewUpdateCommand("label", map[string]any{"content": "Save"}), side); err != nil {
		t.Fatal(err)
	}
	lbl := FindNodeByID(side, "label")
	if lbl["content"] != "Save" {
		t.Fatalf("after update: %v", lbl["content"])
	}

	// 5. Undo and assert revert.
	ok, err := hist2.Undo(side)
	if err != nil || !ok {
		t.Fatalf("undo failed: ok=%v err=%v", ok, err)
	}
	lbl = FindNodeByID(side, "label")
	if lbl["content"] != "Submit" {
		t.Fatalf("after undo: %v, want Submit", lbl["content"])
	}
}
