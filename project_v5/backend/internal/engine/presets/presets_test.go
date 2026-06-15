package presets

import (
	"encoding/json"
	"testing"

	"keepstar_v5/internal/engine"
)

// TestProductCardSeedRoundTrip — the product-card redesign (2026-06-15)
// rebuilt the card around the renderer's new capability layer: a decorated
// card frame (fill/cornerRadius/stroke/effect/clip), a hero frame holding
// the image plus absolutely-positioned overlays (badge / like-slot /
// add-slot), and an info frame whose price+rating are INLINE (the
// brand-badge ref was dropped — brand is now plain muted text).
//
// The price-rating-root / brand-badge-root components stay in the registry
// for the other presets, but product_card no longer references them.
//
// The top-level grid wrapper (flex-wrap row-of-cards) and the
// replicate:true card frame are preserved — the canvas/binding pipeline
// keys off them.
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

	bindings := map[string]string{}
	var refCount int
	engine.WalkNodes(card, func(n engine.Node, _ int) {
		if engine.NodeType(n) == engine.NodeTypeRef {
			refCount++
		}
		if fb, ok := n["fieldBinding"].(string); ok && fb != "" {
			bindings[engine.NodeID(n)] = fb
		}
	})

	// Redesign inlines everything — no component refs remain in the card.
	if refCount != 0 {
		t.Errorf("product_card must not reference components anymore, found %d refs", refCount)
	}

	// All four demo fields bind inline: hero image, title, price, rating,
	// brand (badge is optional — present below but allowed to be missing).
	wantInline := map[string]string{
		"hero-img":   "heroImage",
		"title":      "name",
		"price":      "priceFormatted",
		"rating":     "rating",
		"brand":      "brand",
		"badge-text": "badge",
	}
	for id, want := range wantInline {
		if bindings[id] != want {
			t.Errorf("inline binding %q = %q, want %q (full: %v)", id, bindings[id], want, bindings)
		}
	}

	// Typed action slots exist so InjectDefaultActions routes the heart over
	// the hero and the add over the body (different parents).
	likeSlot := engine.FindNodeByID(&doc, "like-slot")
	if likeSlot == nil || likeSlot["acceptsAction"] != "like" {
		t.Errorf("like-slot frame missing or not acceptsAction:like (%v)", likeSlot)
	}
	addSlot := engine.FindNodeByID(&doc, "add-slot")
	if addSlot == nil || addSlot["acceptsAction"] != "cart_add" {
		t.Errorf("add-slot frame missing or not acceptsAction:cart_add (%v)", addSlot)
	}

	// The card frame carries the new decoration capability props.
	if card["fill"] != "#FFFFFF" {
		t.Errorf("card fill = %v, want #FFFFFF", card["fill"])
	}
	if cr, _ := card["cornerRadius"].(float64); cr != 16 {
		t.Errorf("card cornerRadius = %v, want 16", card["cornerRadius"])
	}
	if clip, _ := card["clip"].(bool); !clip {
		t.Errorf("card clip = %v, want true", card["clip"])
	}
}

// TestProductCardSeedRendersThroughPipeline — the redesigned product_card
// must survive the full zero-LLM render chain (Materialise → ExpandReplicates
// → ResolveAndInline → BindData → InjectDefaultActions) with realistic data:
// fan out to N bound cards, route like→like-slot and cart_add→add-slot, and
// leave the legacy "actions" frame absent (slot routing). An optional badge
// field that's missing must NOT be reported as a fatal bind error here — it
// surfaces as a Missing diagnostic the renderer degrades on, so we assert
// the demo's required fields bound and the slots got their buttons.
func TestProductCardSeedRendersThroughPipeline(t *testing.T) {
	var preset engine.Document
	if err := json.Unmarshal(ProductCardJSON, &preset); err != nil {
		t.Fatalf("unmarshal preset: %v", err)
	}
	// No component refs anymore — materialise with an empty component set.
	doc := engine.Materialise(&preset, nil)

	data := []map[string]any{
		{"id": "p-0", "name": "Rice Cleansing Cream", "brand": "The Saem", "priceFormatted": "$119.00", "rating": 4.0, "heroImage": "https://img.test/rice.jpg", "badge": "Bestseller"},
		{"id": "p-1", "name": "Avocado Cleansing Cream", "brand": "The Saem", "priceFormatted": "$99.00", "rating": 4.3, "heroImage": "https://img.test/avocado.jpg"},
	}

	engine.ExpandReplicates(doc, len(data))
	if stats := engine.ResolveAndInline(doc); len(stats.Failed) > 0 {
		t.Fatalf("ResolveAndInline failed refs: %v", stats.Failed)
	}
	engine.BindData(doc, data)
	populated := engine.InjectDefaultActions(doc, "product", data)

	// Two clones × two typed slots = four frames populated.
	if populated != len(data)*2 {
		t.Fatalf("expected %d slots populated, got %d", len(data)*2, populated)
	}

	grid := engine.FindNodeByID(doc, "grid")
	if grid == nil {
		t.Fatal("grid frame missing after pipeline")
	}
	clones := engine.Children(grid)
	if len(clones) != len(data) {
		t.Fatalf("expected %d clones, got %d", len(data), len(clones))
	}

	for i, clone := range clones {
		got := map[string]any{}
		engine.WalkNodes(clone, func(n engine.Node, _ int) {
			if fb, _ := n["fieldBinding"].(string); fb != "" {
				got[fb] = n["content"]
			}
		})
		if got["name"] != data[i]["name"] {
			t.Errorf("clone[%d] name = %v, want %v", i, got["name"], data[i]["name"])
		}
		if got["brand"] != data[i]["brand"] {
			t.Errorf("clone[%d] brand = %v, want %v", i, got["brand"], data[i]["brand"])
		}
		if got["priceFormatted"] != data[i]["priceFormatted"] {
			t.Errorf("clone[%d] price = %v, want %v", i, got["priceFormatted"], data[i]["priceFormatted"])
		}
		if got["rating"] != data[i]["rating"] {
			t.Errorf("clone[%d] rating = %v, want %v", i, got["rating"], data[i]["rating"])
		}

		// ExpandReplicates re-mints descendant ids, so the slots are found
		// by their stable acceptsAction attribute, not by id. The like-slot
		// must hold the heart, the add-slot the plus — different parents.
		var likeBtn, addBtn engine.Node
		engine.WalkNodes(clone, func(n engine.Node, _ int) {
			switch n["acceptsAction"] {
			case "like":
				if kids := engine.Children(n); len(kids) == 1 {
					likeBtn = kids[0]
				} else {
					t.Errorf("clone[%d] like-slot expected 1 button, got %d", i, len(kids))
				}
			case "cart_add":
				if kids := engine.Children(n); len(kids) == 1 {
					addBtn = kids[0]
				} else {
					t.Errorf("clone[%d] add-slot expected 1 button, got %d", i, len(kids))
				}
			}
		})
		if likeBtn == nil || likeBtn["actionKind"] != "like" {
			t.Errorf("clone[%d] like-slot button missing/wrong kind: %v", i, likeBtn)
		}
		if addBtn == nil || addBtn["actionKind"] != "cart_add" {
			t.Errorf("clone[%d] add-slot button missing/wrong kind: %v", i, addBtn)
		}
		// No legacy "actions" frame in the redesign.
		if legacy := engine.FindNodeByID(clone, "actions"); legacy != nil {
			t.Errorf("clone[%d] unexpected legacy actions frame", i)
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
