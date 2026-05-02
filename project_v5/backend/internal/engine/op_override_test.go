package engine

import "testing"

func TestSetOverride(t *testing.T) {
	doc := NewDocument()
	// reusable button
	NewInsertCommand("", Node{
		"type":     "frame",
		"id":       "btn",
		"reusable": true,
		"children": []Node{
			{"type": "text", "id": "label", "content": "Click"},
		},
	}).Execute(doc)
	NewInsertCommand("", Node{"type": "ref", "id": "x1", "ref": "btn"}).Execute(doc)

	cmd := NewSetOverrideCommand("x1", "label", map[string]any{"content": "Submit"})
	if err := cmd.Execute(doc); err != nil {
		t.Fatal(err)
	}
	x := FindNodeByID(doc, "x1")
	desc := x["descendants"].(map[string]any)
	lbl := desc["label"].(map[string]any)
	if lbl["content"] != "Submit" {
		t.Errorf("override content = %v, want Submit", lbl["content"])
	}

	// merge another property
	cmd2 := NewSetOverrideCommand("x1", "label", map[string]any{"fontSize": 14})
	cmd2.Execute(doc)
	desc = x["descendants"].(map[string]any)
	lbl = desc["label"].(map[string]any)
	if lbl["content"] != "Submit" || lbl["fontSize"] != 14 {
		t.Errorf("after merge: %+v", lbl)
	}

	// undo cmd2
	cmd2.Undo(doc)
	desc = x["descendants"].(map[string]any)
	lbl = desc["label"].(map[string]any)
	if _, has := lbl["fontSize"]; has {
		t.Errorf("undo cmd2: fontSize should be gone")
	}
	if lbl["content"] != "Submit" {
		t.Errorf("undo cmd2: content lost")
	}

	// undo cmd1 → label entry should disappear, descendants should be cleaned up
	cmd.Undo(doc)
	if _, has := x["descendants"]; has {
		t.Errorf("undo cmd1: descendants should be cleaned up, still present: %+v", x["descendants"])
	}
}

func TestSetOverrideOnNonRef(t *testing.T) {
	doc := NewDocument()
	NewInsertCommand("", Node{"type": "frame", "id": "f1"}).Execute(doc)
	cmd := NewSetOverrideCommand("f1", "anything", map[string]any{"x": 1})
	if err := cmd.Execute(doc); err == nil {
		t.Fatal("expected error setting override on non-ref node")
	}
}

func TestSetRootOverride(t *testing.T) {
	doc := NewDocument()
	NewInsertCommand("", Node{"type": "ref", "id": "x1", "ref": "src"}).Execute(doc)

	cmd := NewSetRootOverrideCommand("x1", map[string]any{"opacity": 0.5})
	if err := cmd.Execute(doc); err != nil {
		t.Fatal(err)
	}
	x := FindNodeByID(doc, "x1")
	if x["opacity"] != 0.5 {
		t.Errorf("opacity = %v, want 0.5", x["opacity"])
	}

	cmd.Undo(doc)
	x = FindNodeByID(doc, "x1")
	if _, has := x["opacity"]; has {
		t.Errorf("after undo, opacity should be deleted")
	}
}
