package engine

import (
	"testing"
)

// TestExpandReplicatesNoMarker — no replicate:true anywhere; doc is
// untouched. Sanity guard so plain (non-replicated) presets pass through.
func TestExpandReplicatesNoMarker(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{
			"type": "frame", "id": "card",
			"children": []Node{
				{"type": "text", "id": "name", "fieldBinding": "name"},
			},
		},
	}
	before := doc.Children[0]["id"]
	ExpandReplicates(doc, 5)
	if len(doc.Children) != 1 {
		t.Fatalf("doc grew: %+v", doc.Children)
	}
	if doc.Children[0]["id"] != before {
		t.Errorf("id should not change without marker: %v -> %v", before, doc.Children[0]["id"])
	}
}

// TestExpandReplicatesProducesNClones — the main success case.
// 1 marker, count=3 → 3 clones with dataIndex 0/1/2 and unique IDs throughout.
func TestExpandReplicatesProducesNClones(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{
			"type": "frame", "id": "card", "replicate": true,
			"children": []Node{
				{
					"type": "frame", "id": "card-info",
					"children": []Node{
						{"type": "text", "id": "card-name", "fieldBinding": "name"},
						{"type": "text", "id": "card-brand", "fieldBinding": "brand"},
					},
				},
			},
		},
	}

	ExpandReplicates(doc, 3)

	if len(doc.Children) != 3 {
		t.Fatalf("want 3 clones, got %d: %+v", len(doc.Children), doc.Children)
	}
	seenIDs := map[string]bool{}
	for i, clone := range doc.Children {
		// dataIndex stamped on root.
		di, ok := clone["dataIndex"].(int)
		if !ok || di != i {
			t.Errorf("clone[%d] dataIndex = %v (type %T), want %d", i, clone["dataIndex"], clone["dataIndex"], i)
		}
		// Replicate marker stripped.
		if _, has := clone["replicate"]; has {
			t.Errorf("clone[%d] still carries replicate marker", i)
		}
		// Root id is fresh and unique.
		rootID := NodeID(clone)
		if rootID == "card" {
			t.Errorf("clone[%d] root kept original id %q", i, rootID)
		}
		if seenIDs[rootID] {
			t.Errorf("clone[%d] root id %q collides", i, rootID)
		}
		seenIDs[rootID] = true
		// Descendant ids are fresh + unique too.
		WalkNodes(clone, func(n Node, _ int) {
			id := NodeID(n)
			if id == "card-info" || id == "card-name" || id == "card-brand" {
				t.Errorf("clone[%d] kept descendant id %q", i, id)
			}
			if seenIDs[id] {
				t.Errorf("clone[%d] descendant id %q collides", i, id)
			}
			seenIDs[id] = true
		})
	}

	// Source structure preserved across clones (each clone has the same shape).
	for i, clone := range doc.Children {
		info := Children(clone)
		if len(info) != 1 {
			t.Errorf("clone[%d] expected 1 info child: %+v", i, info)
			continue
		}
		atoms := Children(info[0])
		if len(atoms) != 2 {
			t.Errorf("clone[%d] expected 2 atoms in info: %+v", i, atoms)
		}
	}
}

// TestExpandReplicatesNestedOuterWins — replicate inside replicate.
// The outer marker fans out; the inner one is stripped (not re-fanned-out
// recursively). Combinatorial blow-up is avoided.
func TestExpandReplicatesNestedOuterWins(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{
			"type": "frame", "id": "outer", "replicate": true,
			"children": []Node{
				{
					"type": "frame", "id": "inner", "replicate": true,
					"children": []Node{
						{"type": "text", "id": "leaf", "fieldBinding": "name"},
					},
				},
			},
		},
	}

	ExpandReplicates(doc, 2)

	if len(doc.Children) != 2 {
		t.Fatalf("outer fan-out should produce 2 clones, got %d", len(doc.Children))
	}
	for i, outerClone := range doc.Children {
		// Outer marker stripped.
		if _, has := outerClone["replicate"]; has {
			t.Errorf("outerClone[%d] still has replicate", i)
		}
		// Inner clone exists exactly once (NOT 2× — that would be combinatorial).
		innerKids := Children(outerClone)
		if len(innerKids) != 1 {
			t.Errorf("outerClone[%d] expected exactly 1 inner child, got %d", i, len(innerKids))
			continue
		}
		// Inner marker also stripped.
		if _, has := innerKids[0]["replicate"]; has {
			t.Errorf("inner clone in outerClone[%d] still carries replicate", i)
		}
	}
}

// TestExpandReplicatesCountZero — count=0 means "no data" (e.g., empty
// search). The marker is removed and the parent ends up with no children
// from this branch.
func TestExpandReplicatesCountZero(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{"type": "frame", "id": "before", "children": []Node{}},
		{"type": "frame", "id": "card", "replicate": true, "children": []Node{}},
		{"type": "frame", "id": "after", "children": []Node{}},
	}
	ExpandReplicates(doc, 0)
	if len(doc.Children) != 2 {
		t.Fatalf("expected only before+after, got: %+v", doc.Children)
	}
	if NodeID(doc.Children[0]) != "before" || NodeID(doc.Children[1]) != "after" {
		t.Errorf("siblings around marker reordered: %+v", doc.Children)
	}
}

// TestExpandReplicatesMarkerDeepInTree — non-replicated wrapper Frame that
// CONTAINS a replicated card. Recursion into non-marker containers must
// find the inner marker and fan it out without touching the wrapper.
func TestExpandReplicatesMarkerDeepInTree(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{
			"type": "frame", "id": "section",
			"children": []Node{
				{"type": "text", "id": "header", "content": "Top picks"},
				{"type": "frame", "id": "card", "replicate": true,
					"children": []Node{
						{"type": "text", "id": "name", "fieldBinding": "name"},
					},
				},
			},
		},
	}
	ExpandReplicates(doc, 2)

	if len(doc.Children) != 1 || NodeID(doc.Children[0]) != "section" {
		t.Fatalf("wrapper section was disturbed: %+v", doc.Children)
	}
	kids := Children(doc.Children[0])
	if len(kids) != 3 {
		t.Fatalf("section should have header + 2 clones, got %d: %+v", len(kids), kids)
	}
	if NodeID(kids[0]) != "header" {
		t.Errorf("header swallowed: %+v", kids[0])
	}
	for i, clone := range kids[1:] {
		di, _ := clone["dataIndex"].(int)
		if di != i {
			t.Errorf("clone[%d] dataIndex = %v, want %d", i, clone["dataIndex"], i)
		}
	}
}

// TestExpandReplicatesIntegratesWithBindData — the end-to-end sanity check
// for chunk 4: a single-marker template + 3 records → 3 cloned cards with
// per-clone bindings, including dataIndex inheritance into descendants.
func TestExpandReplicatesIntegratesWithBindData(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{
			"type": "frame", "id": "card", "replicate": true,
			"children": []Node{
				{"type": "text", "id": "name-atom", "fieldBinding": "name"},
				{"type": "text", "id": "brand-atom", "fieldBinding": "brand"},
			},
		},
	}
	data := []map[string]any{
		{"name": "Alpha", "brand": "AB"},
		{"name": "Bravo", "brand": "BC"},
		{"name": "Charlie", "brand": "CD"},
	}

	ExpandReplicates(doc, len(data))
	res := BindData(doc, data)

	if len(res.Bound) != 6 { // 3 clones × 2 atoms each
		t.Errorf("bound count = %d, want 6: %+v", len(res.Bound), res)
	}

	// Every clone's atom bound to the matching record.
	for i, clone := range doc.Children {
		atoms := Children(clone)
		if len(atoms) != 2 {
			t.Fatalf("clone[%d] expected 2 atoms, got %d", i, len(atoms))
		}
		if atoms[0]["content"] != data[i]["name"] {
			t.Errorf("clone[%d] name = %v, want %v", i, atoms[0]["content"], data[i]["name"])
		}
		if atoms[1]["content"] != data[i]["brand"] {
			t.Errorf("clone[%d] brand = %v, want %v", i, atoms[1]["content"], data[i]["brand"])
		}
	}
}
