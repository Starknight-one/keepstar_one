package engine

import (
	"testing"

	"keepstar_v5/internal/domain"
)

// Replicate-clone case: each clone's empty "actions" frame should get
// like + cart_add buttons bound to the right entity.
func TestInjectDefaultActions_ReplicateClones(t *testing.T) {
	doc := &Document{
		Version: DocumentVersion,
		Children: []Node{
			cloneFrame("clone-0", 0, "tpl-card", []Node{
				actionsFrame(),
			}),
			cloneFrame("clone-1", 1, "tpl-card", []Node{
				actionsFrame(),
			}),
		},
	}
	data := []map[string]any{
		{"id": "p-1", "name": "A"},
		{"id": "p-2", "name": "B"},
	}

	count := InjectDefaultActions(doc, domain.EntityTypeProduct, data)
	if count != 2 {
		t.Fatalf("expected 2 actions frames populated, got %d", count)
	}

	for i, root := range doc.Children {
		actions := findChildByID(root, "actions")
		if actions == nil {
			t.Fatalf("clone %d: actions frame missing", i)
		}
		children := Children(actions)
		if len(children) != 2 {
			t.Fatalf("clone %d: expected 2 buttons, got %d", i, len(children))
		}
		assertButton(t, children[0], domain.UserActionLike, data[i]["id"].(string))
		assertButton(t, children[1], domain.UserActionCartAdd, data[i]["id"].(string))
	}
}

// Idempotency: re-running the pass on a doc that already has buttons
// must not duplicate them.
func TestInjectDefaultActions_Idempotent(t *testing.T) {
	doc := &Document{
		Version: DocumentVersion,
		Children: []Node{
			cloneFrame("clone-0", 0, "tpl-card", []Node{
				actionsFrame(),
			}),
		},
	}
	data := []map[string]any{{"id": "p-1"}}

	if got := InjectDefaultActions(doc, domain.EntityTypeProduct, data); got != 1 {
		t.Fatalf("first pass: expected 1, got %d", got)
	}
	// Re-run.
	if got := InjectDefaultActions(doc, domain.EntityTypeProduct, data); got != 0 {
		t.Fatalf("second pass: expected 0 (idempotent), got %d", got)
	}
	actions := findChildByID(doc.Children[0], "actions")
	if got := len(Children(actions)); got != 2 {
		t.Fatalf("expected 2 buttons after idempotent re-run, got %d", got)
	}
}

// Single-entity / no-replicate case (product_detail style): single
// entity, dataIndex inherited as 0.
func TestInjectDefaultActions_SingleEntityDetail(t *testing.T) {
	doc := &Document{
		Version: DocumentVersion,
		Children: []Node{
			{
				"type": NodeTypeFrame,
				"id":   "detail",
				"children": []Node{
					actionsFrame(),
				},
			},
		},
	}
	data := []map[string]any{{"id": "p-only", "name": "Solo"}}

	count := InjectDefaultActions(doc, domain.EntityTypeProduct, data)
	if count != 1 {
		t.Fatalf("expected 1 actions frame populated, got %d", count)
	}
	actions := findChildByID(doc.Children[0], "actions")
	children := Children(actions)
	if len(children) != 2 {
		t.Fatalf("expected 2 buttons, got %d", len(children))
	}
	assertButton(t, children[0], domain.UserActionLike, "p-only")
}

// Empty data slice → no injection (no entity to bind to).
func TestInjectDefaultActions_NoData(t *testing.T) {
	doc := &Document{
		Version: DocumentVersion,
		Children: []Node{
			cloneFrame("clone-0", 0, "tpl-card", []Node{
				actionsFrame(),
			}),
		},
	}
	if got := InjectDefaultActions(doc, domain.EntityTypeProduct, nil); got != 0 {
		t.Fatalf("expected 0 with empty data, got %d", got)
	}
}

// LLM-populated actions frame must be left alone (idempotency guard
// also covers the "explicit override" case).
func TestInjectDefaultActions_RespectsExplicitChildren(t *testing.T) {
	llmButton := Node{
		"type":    NodeTypeText,
		"content": "buy",
		"wrapper": "button",
		"action": map[string]any{
			"kind": string(domain.UserActionExternalLink),
			"params": map[string]any{
				"url": "https://example.com",
			},
		},
	}
	doc := &Document{
		Version: DocumentVersion,
		Children: []Node{
			cloneFrame("clone-0", 0, "tpl-card", []Node{
				{
					"type":     NodeTypeFrame,
					"id":       "actions",
					"children": []Node{llmButton},
				},
			}),
		},
	}
	data := []map[string]any{{"id": "p-1"}}
	if got := InjectDefaultActions(doc, domain.EntityTypeProduct, data); got != 0 {
		t.Fatalf("expected 0 when LLM populated actions, got %d", got)
	}
	actions := findChildByID(doc.Children[0], "actions")
	if got := len(Children(actions)); got != 1 {
		t.Fatalf("LLM button must survive, got %d children", got)
	}
}

// Slot routing: when a clone declares typed `acceptsAction` slots, the
// like button must land in the like-slot and the cart_add button in the
// cart-slot — different parents — and the legacy "actions" frame must be
// left untouched (slot routing wins).
func TestInjectDefaultActions_SlotRouting(t *testing.T) {
	likeSlot := func() Node {
		return Node{"type": NodeTypeFrame, "id": "like-slot", "acceptsAction": "like", "children": []Node{}}
	}
	cartSlot := func() Node {
		return Node{"type": NodeTypeFrame, "id": "cart-slot", "acceptsAction": "cart_add", "children": []Node{}}
	}
	doc := &Document{
		Version: DocumentVersion,
		Children: []Node{
			cloneFrame("clone-0", 0, "card", []Node{
				likeSlot(),
				cartSlot(),
				actionsFrame(), // legacy frame also present — must stay empty
			}),
			cloneFrame("clone-1", 1, "card", []Node{
				likeSlot(),
				cartSlot(),
			}),
		},
	}
	data := []map[string]any{
		{"id": "p-1", "name": "A"},
		{"id": "p-2", "name": "B"},
	}

	// clone-0: 2 slots; clone-1: 2 slots → 4 frames populated.
	if got := InjectDefaultActions(doc, domain.EntityTypeProduct, data); got != 4 {
		t.Fatalf("expected 4 slots populated, got %d", got)
	}

	for i, root := range doc.Children {
		id := data[i]["id"].(string)

		like := findChildByID(root, "like-slot")
		likeKids := Children(like)
		if len(likeKids) != 1 {
			t.Fatalf("clone %d: like-slot expected 1 button, got %d", i, len(likeKids))
		}
		assertButton(t, likeKids[0], domain.UserActionLike, id)
		assertActionKind(t, likeKids[0], domain.UserActionLike)

		cart := findChildByID(root, "cart-slot")
		cartKids := Children(cart)
		if len(cartKids) != 1 {
			t.Fatalf("clone %d: cart-slot expected 1 button, got %d", i, len(cartKids))
		}
		assertButton(t, cartKids[0], domain.UserActionCartAdd, id)
		assertActionKind(t, cartKids[0], domain.UserActionCartAdd)
	}

	// clone-0's legacy "actions" frame stayed empty (slot routing won).
	legacy := findChildByID(doc.Children[0], "actions")
	if got := len(Children(legacy)); got != 0 {
		t.Errorf("legacy actions frame must stay empty under slot routing, got %d children", got)
	}
}

// Slot routing idempotency: re-running must not duplicate slot buttons.
func TestInjectDefaultActions_SlotRoutingIdempotent(t *testing.T) {
	doc := &Document{
		Version: DocumentVersion,
		Children: []Node{
			cloneFrame("clone-0", 0, "card", []Node{
				{"type": NodeTypeFrame, "id": "like-slot", "acceptsAction": "like", "children": []Node{}},
			}),
		},
	}
	data := []map[string]any{{"id": "p-1"}}
	if got := InjectDefaultActions(doc, domain.EntityTypeProduct, data); got != 1 {
		t.Fatalf("first pass: expected 1, got %d", got)
	}
	if got := InjectDefaultActions(doc, domain.EntityTypeProduct, data); got != 0 {
		t.Fatalf("second pass: expected 0 (idempotent), got %d", got)
	}
	like := findChildByID(doc.Children[0], "like-slot")
	if got := len(Children(like)); got != 1 {
		t.Fatalf("expected 1 button after idempotent re-run, got %d", got)
	}
}

// Default (legacy) injection must still stamp actionKind so the frontend
// can style the buttons even outside slot routing.
func TestInjectDefaultActions_StampsActionKind(t *testing.T) {
	doc := &Document{
		Version: DocumentVersion,
		Children: []Node{
			cloneFrame("clone-0", 0, "card", []Node{actionsFrame()}),
		},
	}
	data := []map[string]any{{"id": "p-1"}}
	InjectDefaultActions(doc, domain.EntityTypeProduct, data)
	actions := findChildByID(doc.Children[0], "actions")
	kids := Children(actions)
	if len(kids) != 2 {
		t.Fatalf("expected 2 buttons, got %d", len(kids))
	}
	assertActionKind(t, kids[0], domain.UserActionLike)
	assertActionKind(t, kids[1], domain.UserActionCartAdd)
}

// Helpers.

func assertActionKind(t *testing.T, n Node, want domain.UserActionKind) {
	t.Helper()
	got, _ := n["actionKind"].(string)
	if got != string(want) {
		t.Fatalf("expected actionKind=%s, got %q", want, got)
	}
}

func cloneFrame(id string, dataIndex int, originID string, children []Node) Node {
	return Node{
		"type":             NodeTypeFrame,
		"id":               id,
		"dataIndex":        dataIndex,
		"__templateOrigin": originID,
		"children":         children,
	}
}

func actionsFrame() Node {
	return Node{
		"type":     NodeTypeFrame,
		"id":       "actions",
		"children": []Node{},
	}
}

func findChildByID(root Node, id string) Node {
	if NodeID(root) == id {
		return root
	}
	if !HasChildren(root) {
		return nil
	}
	for _, c := range Children(root) {
		if found := findChildByID(c, id); found != nil {
			return found
		}
	}
	return nil
}

func assertButton(t *testing.T, n Node, expectKind domain.UserActionKind, expectEntityID string) {
	t.Helper()
	if NodeType(n) != NodeTypeText {
		t.Fatalf("expected text node, got type=%v", n["type"])
	}
	if w, _ := n["wrapper"].(string); w != "button" {
		t.Fatalf("expected wrapper=button, got %q", w)
	}
	act, ok := n["action"].(map[string]any)
	if !ok {
		t.Fatalf("expected action map, got %T", n["action"])
	}
	if kind, _ := act["kind"].(string); kind != string(expectKind) {
		t.Fatalf("expected kind=%s, got %v", expectKind, act["kind"])
	}
	entity, ok := act["entity"].(map[string]any)
	if !ok {
		t.Fatalf("expected entity map, got %T", act["entity"])
	}
	if id, _ := entity["id"].(string); id != expectEntityID {
		t.Fatalf("expected entity.id=%s, got %v", expectEntityID, entity["id"])
	}
	if typ, _ := entity["type"].(string); typ != string(domain.EntityTypeProduct) {
		t.Fatalf("expected entity.type=product, got %v", entity["type"])
	}
}
