package engine

import "testing"

func TestInsertAtRoot(t *testing.T) {
	doc := NewDocument()
	cmd := NewInsertCommand("", Node{"type": "frame", "id": "f1"})
	if err := cmd.Execute(doc); err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if len(doc.Children) != 1 || NodeID(doc.Children[0]) != "f1" {
		t.Fatalf("after insert: %+v", doc.Children)
	}
	if err := cmd.Undo(doc); err != nil {
		t.Fatalf("Undo err: %v", err)
	}
	if len(doc.Children) != 0 {
		t.Fatalf("after undo, should be empty: %+v", doc.Children)
	}
}

func TestInsertAutogenID(t *testing.T) {
	doc := NewDocument()
	cmd := NewInsertCommand("", Node{"type": "rectangle"})
	if err := cmd.Execute(doc); err != nil {
		t.Fatal(err)
	}
	id := cmd.CreatedID()
	if id == "" || len(id) != 5 {
		t.Errorf("CreatedID malformed: %q", id)
	}
	if NodeID(doc.Children[0]) != id {
		t.Errorf("node has different id than CreatedID: %q vs %q", NodeID(doc.Children[0]), id)
	}
}

func TestInsertIntoFrame(t *testing.T) {
	doc := NewDocument()
	frameCmd := NewInsertCommand("", Node{"type": "frame", "id": "f1"})
	frameCmd.Execute(doc)
	textCmd := NewInsertCommand("f1", Node{"type": "text", "id": "t1", "content": "hi"})
	if err := textCmd.Execute(doc); err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	frame := FindNodeByID(doc, "f1")
	cs := Children(frame)
	if len(cs) != 1 || NodeID(cs[0]) != "t1" {
		t.Fatalf("frame children: %+v", cs)
	}
	textCmd.Undo(doc)
	frame = FindNodeByID(doc, "f1")
	cs = Children(frame)
	if len(cs) != 0 {
		t.Fatalf("after undo, frame children should be empty: %+v", cs)
	}
}

func TestInsertMissingParent(t *testing.T) {
	doc := NewDocument()
	cmd := NewInsertCommand("ghost", Node{"type": "text"})
	if err := cmd.Execute(doc); err == nil {
		t.Fatal("expected error on missing parent")
	}
}

func TestInsertCannotHaveChildren(t *testing.T) {
	doc := NewDocument()
	NewInsertCommand("", Node{"type": "rectangle", "id": "r1"}).Execute(doc)
	cmd := NewInsertCommand("r1", Node{"type": "text"})
	if err := cmd.Execute(doc); err == nil {
		t.Fatal("expected error inserting into rectangle")
	}
}
