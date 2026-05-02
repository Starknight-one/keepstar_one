package engine

import "testing"

// helper: build a doc:
// document
//  ├─ frame "f1"
//  │   ├─ text "t1"
//  │   └─ frame "f2"
//  │       └─ rect "r1" (reusable)
//  ├─ group "g1"
//  └─ ref "x1" (ref → r1)
func buildDoc() *Document {
	doc := NewDocument()
	rect := Node{"type": "rectangle", "id": "r1", "reusable": true}
	innerFrame := Node{"type": "frame", "id": "f2", "children": []Node{rect}}
	text := Node{"type": "text", "id": "t1", "content": "hi"}
	frame := Node{"type": "frame", "id": "f1", "children": []Node{text, innerFrame}}
	group := Node{"type": "group", "id": "g1"}
	ref := Node{"type": "ref", "id": "x1", "ref": "r1"}
	doc.Children = []Node{frame, group, ref}
	return doc
}

func TestFindNodeByID(t *testing.T) {
	doc := buildDoc()
	cases := []struct {
		id   string
		want bool
	}{
		{"f1", true},
		{"t1", true},
		{"f2", true},
		{"r1", true},
		{"g1", true},
		{"x1", true},
		{"nope", false},
	}
	for _, c := range cases {
		got := FindNodeByID(doc, c.id)
		if (got != nil) != c.want {
			t.Errorf("FindNodeByID(%q) = %v, want present=%v", c.id, got, c.want)
		}
	}
}

func TestFindParent(t *testing.T) {
	doc := buildDoc()
	// t1 → parent f1, index 0
	ref := FindParent(doc, "t1")
	if ref == nil {
		t.Fatal("FindParent(t1) returned nil")
	}
	if NodeID(ref.Parent.(Node)) != "f1" || ref.Index != 0 {
		t.Errorf("FindParent(t1): got parent=%v idx=%d, want f1/0", NodeID(ref.Parent.(Node)), ref.Index)
	}
	// f1 → parent doc, index 0
	ref = FindParent(doc, "f1")
	if ref == nil {
		t.Fatal("FindParent(f1) returned nil")
	}
	if _, isDoc := ref.Parent.(*Document); !isDoc {
		t.Errorf("FindParent(f1) parent should be *Document, got %T", ref.Parent)
	}
	if ref.Index != 0 {
		t.Errorf("FindParent(f1) index = %d, want 0", ref.Index)
	}
	// missing
	if FindParent(doc, "nope") != nil {
		t.Error("FindParent(nope) should be nil")
	}
}

func TestFindHelpers(t *testing.T) {
	doc := buildDoc()
	reusables := FindReusableNodes(doc)
	if len(reusables) != 1 || NodeID(reusables[0]) != "r1" {
		t.Errorf("FindReusableNodes = %v, want [r1]", reusables)
	}
	frames := FindNodesByType(doc, NodeTypeFrame)
	if len(frames) != 2 {
		t.Errorf("FindNodesByType(frame) count = %d, want 2", len(frames))
	}
	refs := FindRefsToNode(doc, "r1")
	if len(refs) != 1 || NodeID(refs[0]) != "x1" {
		t.Errorf("FindRefsToNode(r1) = %v, want [x1]", refs)
	}
}

func TestFindSlots(t *testing.T) {
	root := Node{
		"type": "frame",
		"id":   "root",
		"children": []Node{
			{"type": "frame", "id": "slot1", "slot": []any{"hero", "title"}},
			{"type": "frame", "id": "noslot"},
			{"type": "frame", "id": "slot-false", "slot": false},
			{"type": "frame", "id": "slot2", "slot": []string{"badge"}},
		},
	}
	slots := FindSlots(root)
	if len(slots) != 2 {
		t.Fatalf("expected 2 slots, got %d: %+v", len(slots), slots)
	}
	if NodeID(slots[0].Node) != "slot1" || len(slots[0].SlotNames) != 2 {
		t.Errorf("slot1 mismatch: %+v", slots[0])
	}
	if NodeID(slots[1].Node) != "slot2" || len(slots[1].SlotNames) != 1 {
		t.Errorf("slot2 mismatch: %+v", slots[1])
	}
}
