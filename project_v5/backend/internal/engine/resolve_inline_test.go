package engine

import "testing"

func TestResolveAndInlineNoRefs(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{"type": "frame", "id": "card", "children": []Node{
			{"type": "text", "id": "title", "fieldBinding": "name"},
		}},
	}
	stats := ResolveAndInline(doc)
	if stats.Resolved != 0 || len(stats.Failed) != 0 {
		t.Errorf("no refs → no work: %+v", stats)
	}
	if NodeID(doc.Children[0]) != "card" {
		t.Errorf("doc structure changed: %+v", doc)
	}
}

func TestResolveAndInlineTopLevelRef(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{"type": "frame", "id": "comp-root", "reusable": true,
			"children": []Node{
				{"type": "text", "id": "leaf", "fieldBinding": "name"},
			},
		},
		{"type": "ref", "id": "use-1", "ref": "comp-root"},
	}
	stats := ResolveAndInline(doc)
	if stats.Resolved != 1 {
		t.Errorf("expected 1 resolved, got %+v", stats)
	}
	// The ref slot now holds the resolved subtree under id "use-1"
	// (expandRef forces clone.id = refNode.id).
	resolved := doc.Children[1]
	if NodeID(resolved) != "use-1" {
		t.Errorf("ref slot id = %q, want use-1", NodeID(resolved))
	}
	if NodeType(resolved) != "frame" {
		t.Errorf("ref slot should now be a frame, got %q", NodeType(resolved))
	}
	// Reusable definition still present.
	if NodeID(doc.Children[0]) != "comp-root" {
		t.Errorf("reusable definition lost: %+v", doc.Children[0])
	}
}

func TestResolveAndInlineFailedRef(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{"type": "ref", "id": "broken", "ref": "no-such-component"},
	}
	stats := ResolveAndInline(doc)
	if stats.Resolved != 0 {
		t.Errorf("expected 0 resolved, got %d", stats.Resolved)
	}
	if len(stats.Failed) != 1 || stats.Failed[0] != "broken" {
		t.Errorf("expected ['broken'] in Failed, got %v", stats.Failed)
	}
	// Original ref should still be there for debugging.
	if NodeType(doc.Children[0]) != NodeTypeRef {
		t.Errorf("failed ref should remain in place, got: %+v", doc.Children[0])
	}
}

func TestResolveAndInlineNestedRefs(t *testing.T) {
	// Two components: "leaf-comp" used by "wrapper-comp" used by the preset.
	doc := NewDocument()
	doc.Children = []Node{
		{"type": "frame", "id": "leaf-comp", "reusable": true,
			"children": []Node{
				{"type": "text", "id": "leaf-text", "fieldBinding": "name"},
			},
		},
		{"type": "frame", "id": "wrapper-comp", "reusable": true,
			"children": []Node{
				{"type": "ref", "id": "wrapper-uses-leaf", "ref": "leaf-comp"},
			},
		},
		{"type": "ref", "id": "consumer", "ref": "wrapper-comp"},
	}
	ResolveAndInline(doc)
	// expandRef recurses into nested refs (component_resolver.go:108-120),
	// so the consumer should end up containing a fully-expanded subtree
	// with no surviving NodeTypeRef descendants.
	consumer := doc.Children[2]
	if NodeType(consumer) != "frame" {
		t.Fatalf("consumer should be a resolved frame, got %q", NodeType(consumer))
	}
	WalkNodes(consumer, func(n Node, _ int) {
		if NodeType(n) == NodeTypeRef {
			t.Errorf("found unresolved ref inside consumer: %+v", n)
		}
	})
}

// TestResolveAndInlineStripsReusableFromInstances — regression guard for
// the bug surfaced by chunk-5 e2e: cloneNode(source) propagates the
// component's `reusable:true` marker into the resolved subtree; without
// stripping it, BindData skips instances entirely. ResolveAndInline must
// remove the marker on the resolved root.
func TestResolveAndInlineStripsReusableFromInstances(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{"type": "frame", "id": "comp-root", "reusable": true,
			"children": []Node{
				{"type": "text", "id": "leaf", "fieldBinding": "name"},
			},
		},
		{"type": "ref", "id": "use-1", "ref": "comp-root"},
	}
	ResolveAndInline(doc)
	resolved := doc.Children[1]
	if r, _ := resolved["reusable"].(bool); r {
		t.Errorf("resolved instance still carries reusable:true: %+v", resolved)
	}
	// The original definition is unchanged.
	if r, _ := doc.Children[0]["reusable"].(bool); !r {
		t.Errorf("source definition lost reusable marker: %+v", doc.Children[0])
	}
}

func TestResolveAndInlineRefInsideFrame(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{"type": "frame", "id": "comp-root", "reusable": true,
			"children": []Node{
				{"type": "text", "id": "leaf", "fieldBinding": "name"},
			},
		},
		{"type": "frame", "id": "card",
			"children": []Node{
				{"type": "text", "id": "title", "content": "literal"},
				{"type": "ref", "id": "embedded", "ref": "comp-root"},
			},
		},
	}
	stats := ResolveAndInline(doc)
	if stats.Resolved != 1 {
		t.Errorf("expected 1 nested resolution, got %+v", stats)
	}
	card := FindNodeByID(doc, "card")
	kids := Children(card)
	if len(kids) != 2 {
		t.Fatalf("card should still have 2 children, got %d", len(kids))
	}
	if NodeType(kids[1]) == NodeTypeRef {
		t.Errorf("nested ref still unresolved: %+v", kids[1])
	}
	if NodeID(kids[1]) != "embedded" {
		t.Errorf("resolved id = %q, want embedded", NodeID(kids[1]))
	}
}
