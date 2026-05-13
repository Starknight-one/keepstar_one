package engine

import "testing"

func TestDeleteFromFrame(t *testing.T) {
	doc := NewDocument()
	NewInsertCommand("", Node{"type": "frame", "id": "f1", "children": []Node{
		{"type": "text", "id": "t1"},
		{"type": "text", "id": "t2"},
	}}).Execute(doc)

	cmd := NewDeleteCommand("t1")
	if err := cmd.Execute(doc); err != nil {
		t.Fatal(err)
	}
	f1 := FindNodeByID(doc, "f1")
	cs := Children(f1)
	if len(cs) != 1 || NodeID(cs[0]) != "t2" {
		t.Fatalf("after delete: %+v", cs)
	}

	cmd.Undo(doc)
	f1 = FindNodeByID(doc, "f1")
	cs = Children(f1)
	if len(cs) != 2 || NodeID(cs[0]) != "t1" {
		t.Fatalf("after undo: %+v", cs)
	}
}

func TestDeleteFromRoot(t *testing.T) {
	doc := NewDocument()
	NewInsertCommand("", Node{"type": "frame", "id": "f1"}).Execute(doc)
	NewInsertCommand("", Node{"type": "frame", "id": "f2"}).Execute(doc)

	cmd := NewDeleteCommand("f1")
	if err := cmd.Execute(doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Children) != 1 || NodeID(doc.Children[0]) != "f2" {
		t.Fatalf("after delete: %+v", doc.Children)
	}

	cmd.Undo(doc)
	if len(doc.Children) != 2 || NodeID(doc.Children[0]) != "f1" {
		t.Fatalf("after undo: %+v", doc.Children)
	}
}
