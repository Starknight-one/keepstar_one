package engine

import (
	"testing"
)

func makePresetDoc() *Document {
	return &Document{
		Version: "2.10",
		Children: []Node{
			{
				"type": "frame", "id": "card",
				"children": []Node{
					{"type": "ref", "id": "card-meta", "ref": "price-rating-root"},
				},
			},
		},
	}
}

func makeComponentDoc(rootID string) *Document {
	return &Document{
		Version: "2.10",
		Children: []Node{
			{
				"type": "frame", "id": rootID,
				"children": []Node{
					{"type": "text", "id": rootID + "-leaf", "fieldBinding": "name"},
				},
			},
		},
	}
}

func TestMaterialiseNoComponents(t *testing.T) {
	preset := makePresetDoc()
	merged := Materialise(preset, nil)
	if merged == nil {
		t.Fatal("nil merged doc")
	}
	if len(merged.Children) != 1 {
		t.Errorf("expected 1 child (preset only), got %d", len(merged.Children))
	}
	// Returned doc is a clone — mutating merged must not affect preset.
	merged.Children[0]["touched"] = true
	if _, has := preset.Children[0]["touched"]; has {
		t.Errorf("preset was mutated through clone")
	}
}

func TestMaterialiseAppendsComponents(t *testing.T) {
	preset := makePresetDoc()
	comp := makeComponentDoc("price-rating-root")
	merged := Materialise(preset, []*Document{comp})

	if len(merged.Children) != 2 {
		t.Fatalf("expected preset+component = 2 children, got %d", len(merged.Children))
	}
	appended := merged.Children[1]
	if NodeID(appended) != "price-rating-root" {
		t.Errorf("appended root id = %q, want price-rating-root", NodeID(appended))
	}
	if r, _ := appended["reusable"].(bool); !r {
		t.Errorf("appended root must carry reusable:true; got %+v", appended)
	}
	// Component definition must be a clone — mutating must not affect source.
	appended["touched"] = true
	if _, has := comp.Children[0]["touched"]; has {
		t.Errorf("source component mutated through Materialise")
	}
}

func TestMaterialiseSkipsEmptyComponent(t *testing.T) {
	preset := makePresetDoc()
	empty := &Document{Version: "2.10", Children: nil}
	merged := Materialise(preset, []*Document{empty, nil})
	if len(merged.Children) != 1 {
		t.Errorf("empty/nil components must be skipped silently; got %d children", len(merged.Children))
	}
}

func TestMaterialiseMultiRootTakesFirstOnly(t *testing.T) {
	preset := makePresetDoc()
	multi := &Document{
		Version: "2.10",
		Children: []Node{
			{"type": "frame", "id": "first-root", "children": []Node{}},
			{"type": "frame", "id": "second-root", "children": []Node{}},
		},
	}
	merged := Materialise(preset, []*Document{multi})
	if len(merged.Children) != 2 {
		t.Fatalf("multi-root component should contribute one child, got %d", len(merged.Children))
	}
	if NodeID(merged.Children[1]) != "first-root" {
		t.Errorf("expected first-root to be appended, got %q", NodeID(merged.Children[1]))
	}
}

func TestMaterialiseRefThenResolveEndToEnd(t *testing.T) {
	preset := makePresetDoc()
	comp := makeComponentDoc("price-rating-root")
	merged := Materialise(preset, []*Document{comp})

	stats := ResolveAndInline(merged)
	if stats.Resolved != 1 {
		t.Errorf("expected 1 resolution, got %+v", stats)
	}
	// Ref should have been replaced by the cloned component subtree
	// (now under id "card-meta" — expandRef forces clone.id = refNode.id).
	card := FindNodeByID(merged, "card")
	kids := Children(card)
	if len(kids) != 1 {
		t.Fatalf("card should still have 1 child after inline, got %d", len(kids))
	}
	resolved := kids[0]
	if NodeID(resolved) != "card-meta" {
		t.Errorf("resolved child id = %q, want card-meta", NodeID(resolved))
	}
	// And the reusable definition still sits at the top alongside.
	if NodeID(merged.Children[1]) != "price-rating-root" {
		t.Errorf("reusable definition lost")
	}
}
