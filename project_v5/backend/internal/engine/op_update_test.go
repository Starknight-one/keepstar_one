package engine

import "testing"

func TestUpdate(t *testing.T) {
	doc := NewDocument()
	NewInsertCommand("", Node{"type": "text", "id": "t1", "content": "hi"}).Execute(doc)

	cmd := NewUpdateCommand("t1", map[string]any{
		"content":  "hello",
		"fontSize": 16,
	})
	if err := cmd.Execute(doc); err != nil {
		t.Fatal(err)
	}
	t1 := FindNodeByID(doc, "t1")
	if t1["content"] != "hello" || t1["fontSize"] != 16 {
		t.Errorf("after update: %+v", t1)
	}

	if err := cmd.Undo(doc); err != nil {
		t.Fatal(err)
	}
	t1 = FindNodeByID(doc, "t1")
	if t1["content"] != "hi" {
		t.Errorf("after undo, content = %v, want hi", t1["content"])
	}
	if _, has := t1["fontSize"]; has {
		t.Errorf("after undo, fontSize should be deleted (was not set originally)")
	}
}

func TestUpdateMissingNode(t *testing.T) {
	doc := NewDocument()
	cmd := NewUpdateCommand("nope", map[string]any{"x": 1})
	if err := cmd.Execute(doc); err == nil {
		t.Fatal("expected error on missing node")
	}
}
