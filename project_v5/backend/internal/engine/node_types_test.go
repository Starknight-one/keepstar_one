package engine

import "testing"

func TestPredicates(t *testing.T) {
	frame := Node{"type": "frame", "id": "f"}
	group := Node{"type": "group", "id": "g"}
	rect := Node{"type": "rectangle", "id": "r", "fill": "#FFF"}
	text := Node{"type": "text", "id": "t", "content": "hi"}
	ref := Node{"type": "ref", "id": "x", "ref": "f"}

	if !HasChildren(frame) || !HasChildren(group) {
		t.Errorf("HasChildren should be true for frame/group")
	}
	if HasChildren(rect) || HasChildren(text) || HasChildren(ref) {
		t.Errorf("HasChildren should be false for rect/text/ref")
	}
	if !HasGraphics(rect) {
		t.Errorf("HasGraphics(rect) should be true")
	}
	if HasGraphics(text) {
		t.Errorf("HasGraphics(text) should be false")
	}
	if !IsRef(ref) || IsRef(frame) {
		t.Errorf("IsRef wrong")
	}
}

func TestChildrenNormalization(t *testing.T) {
	// []Node case
	frame := Node{
		"type":     "frame",
		"id":       "f",
		"children": []Node{{"type": "text", "id": "a"}},
	}
	cs := Children(frame)
	if len(cs) != 1 || NodeID(cs[0]) != "a" {
		t.Fatalf("[]Node children malformed: %+v", cs)
	}

	// []any case (simulates JSON unmarshal output)
	frame2 := Node{
		"type": "frame",
		"id":   "f2",
		"children": []any{
			map[string]any{"type": "text", "id": "b"},
		},
	}
	cs2 := Children(frame2)
	if len(cs2) != 1 || NodeID(cs2[0]) != "b" {
		t.Fatalf("[]any children malformed: %+v", cs2)
	}

	// SetChildren writes []Node back
	SetChildren(frame, append(cs, Node{"type": "text", "id": "c"}))
	cs3 := Children(frame)
	if len(cs3) != 2 {
		t.Fatalf("SetChildren did not append, got %d", len(cs3))
	}

	// Setting nil deletes the key
	SetChildren(frame, nil)
	if _, has := frame["children"]; has {
		t.Errorf("SetChildren(nil) should remove key")
	}
}
