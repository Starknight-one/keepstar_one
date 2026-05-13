package presets

import (
	"encoding/json"
	"testing"

	"keepstar_v5/internal/engine"
)

// TestProductCardSeedRoundTrip — chunk 5 refactor moved price/rating/brand
// out of inline atoms and into RefNodes. The card now carries hero+title
// directly and points at two components for meta + brand.
//
// Chunk 12 added a top-level grid-wrapper frame so flex-wrap renders the
// replicate clones as a sensible row-of-cards instead of stacking them.
// The card frame is now nested inside the wrapper at children[0].
func TestProductCardSeedRoundTrip(t *testing.T) {
	if len(ProductCardJSON) == 0 {
		t.Fatal("ProductCardJSON is empty — embed broken")
	}
	var doc engine.Document
	if err := json.Unmarshal(ProductCardJSON, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(doc.Children) != 1 {
		t.Fatalf("expected 1 root child (grid wrapper), got %d", len(doc.Children))
	}
	grid := doc.Children[0]
	if engine.NodeID(grid) != "grid" {
		t.Errorf("root id = %q, want grid", engine.NodeID(grid))
	}
	gridChildren := engine.Children(grid)
	if len(gridChildren) != 1 {
		t.Fatalf("grid wrapper should have exactly 1 card child, got %d", len(gridChildren))
	}
	card := gridChildren[0]
	if engine.NodeID(card) != "card" {
		t.Errorf("card id = %q, want card", engine.NodeID(card))
	}
	if rep, _ := card["replicate"].(bool); !rep {
		t.Errorf("card frame must carry replicate:true")
	}

	// Two refs expected, pointing at the chunk-5 components.
	refs := map[string]string{}
	bindings := map[string]string{}
	engine.WalkNodes(card, func(n engine.Node, _ int) {
		if engine.NodeType(n) == engine.NodeTypeRef {
			refs[engine.NodeID(n)], _ = n["ref"].(string)
		}
		if fb, ok := n["fieldBinding"].(string); ok && fb != "" {
			bindings[engine.NodeID(n)] = fb
		}
	})
	wantRefs := map[string]string{
		"card-meta":  "price-rating-root",
		"card-brand": "brand-badge-root",
	}
	for id, want := range wantRefs {
		if refs[id] != want {
			t.Errorf("ref %q points at %q, want %q (full: %v)", id, refs[id], want, refs)
		}
	}
	// Inline bindings that survived the refactor.
	wantInline := map[string]string{
		"hero-img": "heroImage",
		"title":    "name",
	}
	for id, want := range wantInline {
		if bindings[id] != want {
			t.Errorf("inline binding %q = %q, want %q (full: %v)", id, bindings[id], want, bindings)
		}
	}
}

// TestProductCardListRowSeedRoundTrip — second preset, structurally
// different (row vs column, +description atom), reuses the same
// components. Guards both the field bindings unique to this preset and
// the shared refs.
func TestProductCardListRowSeedRoundTrip(t *testing.T) {
	if len(ProductCardListRowJSON) == 0 {
		t.Fatal("ProductCardListRowJSON is empty — embed broken")
	}
	var doc engine.Document
	if err := json.Unmarshal(ProductCardListRowJSON, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Chunk 12 added a top-level list wrapper (column-stack) so the row
	// cards live one level down. Walk into it.
	if len(doc.Children) != 1 || engine.NodeID(doc.Children[0]) != "list" {
		t.Fatalf("root not list wrapper: %+v", doc.Children)
	}
	listChildren := engine.Children(doc.Children[0])
	if len(listChildren) != 1 || engine.NodeID(listChildren[0]) != "row-card" {
		t.Fatalf("list wrapper child not row-card: %+v", listChildren)
	}
	root := listChildren[0]
	if rep, _ := root["replicate"].(bool); !rep {
		t.Errorf("row-card must carry replicate:true")
	}
	bindings := map[string]string{}
	refs := map[string]string{}
	engine.WalkNodes(root, func(n engine.Node, _ int) {
		if fb, ok := n["fieldBinding"].(string); ok && fb != "" {
			bindings[engine.NodeID(n)] = fb
		}
		if engine.NodeType(n) == engine.NodeTypeRef {
			refs[engine.NodeID(n)], _ = n["ref"].(string)
		}
	})
	if bindings["row-desc"] != "description" {
		t.Errorf("row-desc fieldBinding = %q, want description", bindings["row-desc"])
	}
	if refs["row-meta"] != "price-rating-root" {
		t.Errorf("row-meta ref = %q", refs["row-meta"])
	}
	if refs["row-brand"] != "brand-badge-root" {
		t.Errorf("row-brand ref = %q", refs["row-brand"])
	}
}

// TestComponentSeedsRoundTrip — both reusable components must parse, and
// their root IDs must match what the presets reference.
func TestComponentSeedsRoundTrip(t *testing.T) {
	cases := []struct {
		name      string
		raw       []byte
		wantRoot  string
		wantField string
	}{
		{"price_rating", ComponentPriceRatingJSON, "price-rating-root", "priceFormatted"},
		{"brand_badge", ComponentBrandBadgeJSON, "brand-badge-root", "brand"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var doc engine.Document
			if err := json.Unmarshal(tc.raw, &doc); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if len(doc.Children) != 1 {
				t.Fatalf("component should have exactly 1 root, got %d", len(doc.Children))
			}
			root := doc.Children[0]
			if engine.NodeID(root) != tc.wantRoot {
				t.Errorf("root id = %q, want %q", engine.NodeID(root), tc.wantRoot)
			}
			// Walk for at least one fieldBinding pointing at the expected key.
			found := false
			engine.WalkNodes(root, func(n engine.Node, _ int) {
				if fb, _ := n["fieldBinding"].(string); fb == tc.wantField {
					found = true
				}
			})
			// Single-atom component: WalkNodes only descends into frame/group
			// children, so a top-level Text atom won't be visited. Check root
			// itself too.
			if fb, _ := root["fieldBinding"].(string); fb == tc.wantField {
				found = true
			}
			if !found {
				t.Errorf("did not find fieldBinding=%q in component %q", tc.wantField, tc.name)
			}
		})
	}
}
