package presets

import (
	"encoding/json"
	"testing"

	"keepstar_v5/internal/engine"
)

// TestProductCardSeedRoundTrip — guards the seed file at compile/test time:
// it must parse as engine.Document, expose the expected outer "card"
// frame with replicate:true, and the binding atoms with correct field
// names. Catches drift between the seed JSON and the engine schema before
// it reaches the DB.
func TestProductCardSeedRoundTrip(t *testing.T) {
	if len(ProductCardJSON) == 0 {
		t.Fatal("ProductCardJSON is empty — embed broken")
	}
	var doc engine.Document
	if err := json.Unmarshal(ProductCardJSON, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(doc.Children) != 1 {
		t.Fatalf("expected 1 root child, got %d", len(doc.Children))
	}
	card := doc.Children[0]
	if engine.NodeID(card) != "card" {
		t.Errorf("root id = %q, want card", engine.NodeID(card))
	}
	if rep, _ := card["replicate"].(bool); !rep {
		t.Errorf("card frame must carry replicate:true")
	}
	// Walk the seed and assert the expected fieldBindings exist.
	bindings := map[string]string{}
	engine.WalkNodes(card, func(n engine.Node, _ int) {
		if fb, ok := n["fieldBinding"].(string); ok && fb != "" {
			bindings[engine.NodeID(n)] = fb
		}
	})
	expect := map[string]string{
		"hero-img": "heroImage",
		"title":    "name",
		"price":    "priceFormatted",
		"rating":   "rating",
		"brand":    "brand",
	}
	for id, want := range expect {
		if bindings[id] != want {
			t.Errorf("binding for %q = %q, want %q (full: %v)", id, bindings[id], want, bindings)
		}
	}
}
