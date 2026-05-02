package engine

import "testing"

func TestMoveAcrossParents(t *testing.T) {
	doc := NewDocument()
	NewInsertCommand("", Node{"type": "frame", "id": "f1", "children": []Node{
		{"type": "text", "id": "t1", "content": "hi"},
	}}).Execute(doc)
	NewInsertCommand("", Node{"type": "frame", "id": "f2"}).Execute(doc)

	cmd := NewMoveCommand("t1", "f2", 0, true)
	if err := cmd.Execute(doc); err != nil {
		t.Fatal(err)
	}
	if FindParent(doc, "t1").Parent.(Node)["id"] != "f2" {
		t.Fatal("t1 should be under f2")
	}

	cmd.Undo(doc)
	if FindParent(doc, "t1").Parent.(Node)["id"] != "f1" {
		t.Fatal("t1 should be under f1 after undo")
	}
}

func TestMoveToRoot(t *testing.T) {
	doc := NewDocument()
	NewInsertCommand("", Node{"type": "frame", "id": "f1", "children": []Node{
		{"type": "rectangle", "id": "r1"},
	}}).Execute(doc)

	cmd := NewMoveCommand("r1", "", 0, false)
	if err := cmd.Execute(doc); err != nil {
		t.Fatal(err)
	}
	ref := FindParent(doc, "r1")
	if _, isDoc := ref.Parent.(*Document); !isDoc {
		t.Fatal("r1 should be at document root")
	}
}

func TestMoveIndexClamped(t *testing.T) {
	doc := NewDocument()
	NewInsertCommand("", Node{"type": "frame", "id": "f1"}).Execute(doc)
	NewInsertCommand("f1", Node{"type": "text", "id": "t1"}).Execute(doc)
	NewInsertCommand("f1", Node{"type": "text", "id": "t2"}).Execute(doc)
	NewInsertCommand("", Node{"type": "frame", "id": "f2"}).Execute(doc)

	// Move t1 into f2 with crazy big index
	cmd := NewMoveCommand("t1", "f2", 999, true)
	if err := cmd.Execute(doc); err != nil {
		t.Fatal(err)
	}
	f2 := FindNodeByID(doc, "f2")
	cs := Children(f2)
	if len(cs) != 1 || NodeID(cs[0]) != "t1" {
		t.Fatalf("f2 children after move: %+v", cs)
	}
}
