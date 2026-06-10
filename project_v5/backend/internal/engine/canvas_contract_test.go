package engine

// Cross-repo contract regression: the admin repo's canvas translator
// (keepstar-admin/backend/internal/canvas/translate.go) emits preset
// doc_json that MUST keep rendering through the full V5 zero-LLM pipeline:
//
//	ExpandReplicates → ResolveAndInline → BindData → InjectDefaultActions
//
// The fixtures under testdata/ are real translate outputs (registered in
// prod 2026-06-10). If an engine change breaks these assertions, it breaks
// every preset the admin has published from the canvas — fix the engine or
// version the contract, never silently regress it.
//
// canvas_translate_card_v1.json        — minimal card: image + 2 bound texts.
// canvas_translate_card_emptyframe.json — same card + the empty
//	{"id":"actions"} frame (the bridge InjectDefaultActions fills) + an
//	empty plain frame (must survive untouched).

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"keepstar_v5/internal/domain"
)

// canvasContractData is the fake product slice every contract run binds
// against. "id" is required by InjectDefaultActions (entity resolution);
// name/priceFormatted/heroImage are the keys translate.go binds to.
func canvasContractData() []map[string]any {
	return []map[string]any{
		{"id": "p-0", "name": "Oak Coffee Table", "priceFormatted": "$199.00", "heroImage": "https://img.test/p0.jpg"},
		{"id": "p-1", "name": "Walnut Bookshelf", "priceFormatted": "$349.50", "heroImage": "https://img.test/p1.jpg"},
		{"id": "p-2", "name": "Birch Side Chair", "priceFormatted": "$89.99", "heroImage": "https://img.test/p2.jpg"},
	}
}

// loadCanvasFixture unmarshals one testdata fixture into a Document.
func loadCanvasFixture(t *testing.T, name string) *Document {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	var doc Document
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal fixture %s: %v", name, err)
	}
	return &doc
}

// runCanvasContractPipeline runs the exact zero-LLM render chain the
// production paths use (visual_assembly / navigation / preset preview),
// with an empty component set, and asserts the no-error half of the
// contract: no failed ref resolutions, no missing bindings.
func runCanvasContractPipeline(t *testing.T, doc *Document, data []map[string]any) {
	t.Helper()
	ExpandReplicates(doc, len(data))
	resolveStats := ResolveAndInline(doc)
	if len(resolveStats.Failed) > 0 {
		t.Fatalf("ResolveAndInline failed refs: %v", resolveStats.Failed)
	}
	bindRes := BindData(doc, data)
	if len(bindRes.Missing) > 0 {
		t.Fatalf("BindData missing bindings (every fixture key must resolve): %v", bindRes.Missing)
	}
	InjectDefaultActions(doc, domain.EntityTypeProduct, data)
}

// canvasClones returns the replicate clones of the translate-emitted card
// (children of the top-level grid carrying __templateOrigin), asserting
// there are exactly len(data) of them with sequential dataIndex.
func canvasClones(t *testing.T, doc *Document, wantCount int) []Node {
	t.Helper()
	if len(doc.Children) != 1 {
		t.Fatalf("expected 1 top-level grid, got %d children", len(doc.Children))
	}
	grid := doc.Children[0]
	if NodeID(grid) != "grid" || NodeType(grid) != NodeTypeFrame {
		t.Fatalf("top-level node is not the grid frame: id=%q type=%q", NodeID(grid), NodeType(grid))
	}
	clones := Children(grid)
	if len(clones) != wantCount {
		t.Fatalf("expected %d replicated cards, got %d", wantCount, len(clones))
	}
	for i, clone := range clones {
		if origin, _ := clone["__templateOrigin"].(string); origin != "card" {
			t.Errorf("clone[%d] __templateOrigin = %v, want \"card\"", i, clone["__templateOrigin"])
		}
		idx, ok := readDataIndex(clone)
		if !ok || idx != i {
			t.Errorf("clone[%d] dataIndex = %v, want %d", i, clone[attrDataIndex], i)
		}
	}
	return clones
}

// assertCanvasCloneBound checks the per-clone binding half of the contract:
// every text atom's content equals the fake product value for its field
// (exact match, per dataIndex), and the image atom got fills[0].url from
// heroImage.
func assertCanvasCloneBound(t *testing.T, clone Node, i int, record map[string]any) {
	t.Helper()
	var nameNode, priceNode, heroNode Node
	WalkNodes(clone, func(n Node, _ int) {
		switch fb, _ := n[attrFieldBinding].(string); fb {
		case "name":
			nameNode = n
		case "priceFormatted":
			priceNode = n
		case "heroImage":
			heroNode = n
		}
	})

	if nameNode == nil {
		t.Errorf("clone[%d]: name text atom missing", i)
	} else if got := nameNode[attrContent]; got != record["name"] {
		t.Errorf("clone[%d] name content = %v, want %v", i, got, record["name"])
	}
	if priceNode == nil {
		t.Errorf("clone[%d]: priceFormatted text atom missing", i)
	} else if got := priceNode[attrContent]; got != record["priceFormatted"] {
		t.Errorf("clone[%d] priceFormatted content = %v, want %v", i, got, record["priceFormatted"])
	}

	if heroNode == nil {
		t.Errorf("clone[%d]: heroImage image atom missing", i)
		return
	}
	if NodeType(heroNode) != NodeTypeImage {
		t.Errorf("clone[%d] hero atom type = %q, want image", i, NodeType(heroNode))
	}
	fills, _ := heroNode[attrFills].([]any)
	if len(fills) == 0 {
		t.Errorf("clone[%d] hero atom has no fills after binding", i)
		return
	}
	first, _ := fills[0].(map[string]any)
	if first == nil {
		t.Errorf("clone[%d] hero fills[0] is not a map: %T", i, fills[0])
		return
	}
	if first[attrFillType] != FillTypeImage {
		t.Errorf("clone[%d] hero fills[0].type = %v, want %q", i, first[attrFillType], FillTypeImage)
	}
	if got := first[attrFillImageURL]; got != record["heroImage"] {
		t.Errorf("clone[%d] hero fills[0].url = %v, want %v", i, got, record["heroImage"])
	}
}

// TestCanvasTranslateContract_CardV1 — the minimal registered card must
// fan out to 3 cards, each bound to its own product record.
func TestCanvasTranslateContract_CardV1(t *testing.T) {
	doc := loadCanvasFixture(t, "canvas_translate_card_v1.json")
	data := canvasContractData()

	runCanvasContractPipeline(t, doc, data)

	clones := canvasClones(t, doc, len(data))
	for i, clone := range clones {
		assertCanvasCloneBound(t, clone, i, data[i])
	}
}

// TestCanvasTranslateContract_EmptyFrames — the card variant carrying the
// empty {"id":"actions"} bridge frame and an empty plain frame. The actions
// frame must be populated by InjectDefaultActions with buttons wired to the
// clone's own entity; the plain empty frame must survive empty, no error.
func TestCanvasTranslateContract_EmptyFrames(t *testing.T) {
	doc := loadCanvasFixture(t, "canvas_translate_card_emptyframe.json")
	data := canvasContractData()

	runCanvasContractPipeline(t, doc, data)

	clones := canvasClones(t, doc, len(data))
	for i, clone := range clones {
		assertCanvasCloneBound(t, clone, i, data[i])

		// Fixture card children order: hero, t1, t2, actions, spacer.
		kids := Children(clone)
		if len(kids) != 5 {
			t.Fatalf("clone[%d] expected 5 children (hero,t1,t2,actions,spacer), got %d", i, len(kids))
		}

		// (e1) the empty actions frame was populated by InjectDefaultActions.
		actions := kids[3]
		if NodeID(actions) != "actions" || NodeType(actions) != NodeTypeFrame {
			t.Fatalf("clone[%d] child[3] is not the actions frame: id=%q type=%q", i, NodeID(actions), NodeType(actions))
		}
		buttons := Children(actions)
		if len(buttons) == 0 {
			t.Fatalf("clone[%d] actions frame NOT populated by InjectDefaultActions", i)
		}
		if len(buttons) != 2 {
			t.Errorf("clone[%d] actions frame expected 2 default buttons (like+cart), got %d", i, len(buttons))
		}
		for _, b := range buttons {
			act, _ := b[attrAction].(map[string]any)
			if act == nil {
				t.Errorf("clone[%d] injected button missing action payload: %v", i, b)
				continue
			}
			entity, _ := act["entity"].(map[string]any)
			if entity == nil || entity["id"] != data[i]["id"] {
				t.Errorf("clone[%d] injected action entity = %v, want id %v", i, act["entity"], data[i]["id"])
			}
		}

		// (e2) the empty plain frame survived as an empty frame.
		spacer := kids[4]
		if NodeType(spacer) != NodeTypeFrame {
			t.Errorf("clone[%d] child[4] (spacer) type = %q, want frame", i, NodeType(spacer))
		}
		if NodeID(spacer) == "actions" {
			t.Errorf("clone[%d] spacer frame stole the actions id", i)
		}
		if got := Children(spacer); len(got) != 0 {
			t.Errorf("clone[%d] empty plain frame gained children: %v", i, got)
		}
	}
}
