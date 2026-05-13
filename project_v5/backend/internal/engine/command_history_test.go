package engine

import "testing"

func TestCommandHistory(t *testing.T) {
	doc := NewDocument()
	hist := NewCommandHistory()

	c1 := NewInsertCommand("", Node{"type": "frame", "id": "f1"})
	c2 := NewInsertCommand("", Node{"type": "frame", "id": "f2"})

	if hist.CanUndo() {
		t.Errorf("empty history should not be undoable")
	}

	hist.Execute(c1, doc)
	hist.Execute(c2, doc)
	if len(doc.Children) != 2 {
		t.Fatalf("after 2 inserts: %d children", len(doc.Children))
	}
	if !hist.CanUndo() {
		t.Errorf("should be undoable")
	}

	ok, err := hist.Undo(doc)
	if err != nil || !ok {
		t.Fatalf("undo: ok=%v err=%v", ok, err)
	}
	if len(doc.Children) != 1 || NodeID(doc.Children[0]) != "f1" {
		t.Fatalf("after undo: %+v", doc.Children)
	}
	if !hist.CanRedo() {
		t.Errorf("should be redoable")
	}

	hist.Redo(doc)
	if len(doc.Children) != 2 || NodeID(doc.Children[1]) != "f2" {
		t.Fatalf("after redo: %+v", doc.Children)
	}

	// New execute clears redo
	hist.Undo(doc)
	c3 := NewInsertCommand("", Node{"type": "frame", "id": "f3"})
	hist.Execute(c3, doc)
	if hist.CanRedo() {
		t.Errorf("redo should be cleared after new execute")
	}

	hist.Clear()
	if hist.CanUndo() || hist.CanRedo() {
		t.Errorf("after clear, neither should be enabled")
	}
}
