package engine

import (
	"testing"
)

// docWithFrame is a small fixture: one frame "card" containing one text
// "title". Used by every op test below so the assertions stay focused on
// the op behaviour, not setup.
func docWithFrame() *Document {
	doc := NewDocument()
	doc.Children = []Node{
		{"type": "frame", "id": "card",
			"children": []Node{
				{"type": "text", "id": "title", "content": "before"},
			},
		},
	}
	return doc
}

func TestApplyOpsInsert(t *testing.T) {
	doc := docWithFrame()
	err := ApplyOps(doc, []map[string]any{
		{
			"op":     "insert",
			"parent": "card",
			"props":  map[string]any{"type": "text", "id": "subtitle", "content": "added"},
		},
	})
	if err != nil {
		t.Fatalf("ApplyOps: %v", err)
	}
	card := FindNodeByID(doc, "card")
	if len(Children(card)) != 2 {
		t.Errorf("expected 2 children of card, got %d", len(Children(card)))
	}
	sub := FindNodeByID(doc, "subtitle")
	if sub == nil {
		t.Errorf("subtitle not found after insert")
	} else if sub["content"] != "added" {
		t.Errorf("subtitle content = %v, want 'added'", sub["content"])
	}
}

func TestApplyOpsUpdate(t *testing.T) {
	doc := docWithFrame()
	err := ApplyOps(doc, []map[string]any{
		{
			"op":     "update",
			"target": "title",
			"props":  map[string]any{"content": "after", "format": "stars"},
		},
	})
	if err != nil {
		t.Fatalf("ApplyOps: %v", err)
	}
	title := FindNodeByID(doc, "title")
	if title["content"] != "after" {
		t.Errorf("content = %v, want 'after'", title["content"])
	}
	if title["format"] != "stars" {
		t.Errorf("format = %v, want 'stars'", title["format"])
	}
}

func TestApplyOpsDelete(t *testing.T) {
	doc := docWithFrame()
	err := ApplyOps(doc, []map[string]any{
		{"op": "delete", "target": "title"},
	})
	if err != nil {
		t.Fatalf("ApplyOps: %v", err)
	}
	if FindNodeByID(doc, "title") != nil {
		t.Errorf("title should be gone after delete")
	}
	card := FindNodeByID(doc, "card")
	if len(Children(card)) != 0 {
		t.Errorf("card should be empty, has %d children", len(Children(card)))
	}
}

func TestApplyOpsMoveRoot(t *testing.T) {
	doc := docWithFrame()
	// Move "title" out of the card to document root.
	err := ApplyOps(doc, []map[string]any{
		{"op": "move", "target": "title", "parent": ""},
	})
	if err != nil {
		t.Fatalf("ApplyOps: %v", err)
	}
	if len(doc.Children) != 2 {
		t.Errorf("doc.Children len = %d, want 2 (card + title)", len(doc.Children))
	}
	card := FindNodeByID(doc, "card")
	if len(Children(card)) != 0 {
		t.Errorf("card should be empty after move, has %d", len(Children(card)))
	}
}

func TestApplyOpsRefChaining(t *testing.T) {
	// First op inserts a frame with ref "actions"; second op inserts a
	// child under "$actions". Applier must rewrite "$actions" to the
	// frame's auto-generated id.
	doc := NewDocument()
	doc.Children = []Node{{"type": "frame", "id": "card", "children": []Node{}}}
	err := ApplyOps(doc, []map[string]any{
		{
			"op":     "insert",
			"parent": "card",
			"ref":    "actions",
			"props":  map[string]any{"type": "frame"},
		},
		{
			"op":     "insert",
			"parent": "$actions",
			"props":  map[string]any{"type": "text", "content": "Buy now"},
		},
	})
	if err != nil {
		t.Fatalf("ApplyOps: %v", err)
	}
	// Find the actions frame inside card.
	card := FindNodeByID(doc, "card")
	cardKids := Children(card)
	if len(cardKids) != 1 {
		t.Fatalf("card.children len = %d, want 1 (actions frame)", len(cardKids))
	}
	actionsFrame := cardKids[0]
	if NodeType(actionsFrame) != "frame" {
		t.Errorf("expected frame, got %q", NodeType(actionsFrame))
	}
	actionsKids := Children(actionsFrame)
	if len(actionsKids) != 1 {
		t.Fatalf("actions.children len = %d, want 1 (text)", len(actionsKids))
	}
	if actionsKids[0]["content"] != "Buy now" {
		t.Errorf("inner content = %v, want 'Buy now'", actionsKids[0]["content"])
	}
}

func TestApplyOpsFailFast(t *testing.T) {
	// First op succeeds, second fails. Subsequent ops should NOT run.
	// In V4 fail-fast pattern we accept partial state — caller's
	// responsibility to handle / re-emit.
	doc := docWithFrame()
	err := ApplyOps(doc, []map[string]any{
		{"op": "update", "target": "title", "props": map[string]any{"content": "first"}},
		{"op": "delete", "target": "no-such-id"}, // fails
		{"op": "update", "target": "title", "props": map[string]any{"content": "would-not-run"}},
	})
	if err == nil {
		t.Fatalf("expected error on op 2, got nil")
	}
	title := FindNodeByID(doc, "title")
	if title["content"] != "first" {
		t.Errorf("title content after fail-fast = %v, want 'first'", title["content"])
	}
}

func TestApplyOpsOverrideRoot(t *testing.T) {
	// Override on a RefNode at root level (single-level props).
	doc := NewDocument()
	doc.Children = []Node{
		{"type": "frame", "id": "comp", "reusable": true, "children": []Node{}},
		{"type": "ref", "id": "use1", "ref": "comp"},
	}
	err := ApplyOps(doc, []map[string]any{
		{
			"op":     "override",
			"target": "use1",
			"props":  map[string]any{"opacity": 0.5},
		},
	})
	if err != nil {
		t.Fatalf("ApplyOps: %v", err)
	}
	use1 := FindNodeByID(doc, "use1")
	if use1["opacity"] != 0.5 {
		t.Errorf("root override not applied: %+v", use1)
	}
}

func TestApplyOpsOverrideDescendant(t *testing.T) {
	// Override on a RefNode with descendant-shape props.
	doc := NewDocument()
	doc.Children = []Node{
		{"type": "frame", "id": "comp", "reusable": true,
			"children": []Node{
				{"type": "text", "id": "label"},
			},
		},
		{"type": "ref", "id": "use1", "ref": "comp"},
	}
	err := ApplyOps(doc, []map[string]any{
		{
			"op":     "override",
			"target": "use1",
			"props":  map[string]any{"label": map[string]any{"content": "Hi"}},
		},
	})
	if err != nil {
		t.Fatalf("ApplyOps: %v", err)
	}
	use1 := FindNodeByID(doc, "use1")
	desc, _ := use1["descendants"].(map[string]any)
	if desc == nil {
		t.Fatalf("expected descendants map on use1: %+v", use1)
	}
	label, _ := desc["label"].(map[string]any)
	if label["content"] != "Hi" {
		t.Errorf("descendant override = %+v, want {content: Hi}", label)
	}
}

func TestApplyOpsMissingOp(t *testing.T) {
	doc := docWithFrame()
	err := ApplyOps(doc, []map[string]any{{"target": "title"}})
	if err == nil {
		t.Fatalf("expected error for missing op key")
	}
}

func TestApplyOpsEmptyList(t *testing.T) {
	doc := docWithFrame()
	if err := ApplyOps(doc, nil); err != nil {
		t.Errorf("nil ops should be a no-op, got: %v", err)
	}
	if err := ApplyOps(doc, []map[string]any{}); err != nil {
		t.Errorf("empty ops should be a no-op, got: %v", err)
	}
}

// TestApplyOpsParentAliases — V4-style parent values "root" and
// "formation" must normalise to "" (document root) at insert time.
// V5's op_insert.go treats empty parentID as root; without aliasing,
// the LLM emitting V4-style "formation" would error with «parent
// not found».
func TestApplyOpsParentAliases(t *testing.T) {
	cases := []struct {
		name   string
		parent string
	}{
		{"empty", ""},
		{"root", "root"},
		{"formation", "formation"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := NewDocument()
			err := ApplyOps(doc, []map[string]any{
				{
					"op":     "insert",
					"parent": tc.parent,
					"props":  map[string]any{"type": "frame", "id": "card-" + tc.name},
				},
			})
			if err != nil {
				t.Fatalf("ApplyOps with parent=%q: %v", tc.parent, err)
			}
			if len(doc.Children) != 1 {
				t.Fatalf("expected 1 root child, got %d (parent=%q)", len(doc.Children), tc.parent)
			}
			if NodeID(doc.Children[0]) != "card-"+tc.name {
				t.Errorf("inserted node id = %q, want card-%s", NodeID(doc.Children[0]), tc.name)
			}
		})
	}
}
