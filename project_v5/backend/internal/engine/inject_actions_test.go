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

// Helpers.

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
