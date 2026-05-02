package engine

import (
	"encoding/json"
	"testing"
)

// TestBindDataProductCard covers the main success+miss+literal mix:
// 3 bindable text nodes (name, price, brand), 1 literal text (no
// fieldBinding), 1 unbound text (fieldBinding present but field missing).
func TestBindDataProductCard(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{
			"type": "frame", "id": "card", "layout": "vertical",
			"children": []Node{
				{"type": "text", "id": "title", "fieldBinding": "name"},
				{"type": "text", "id": "price-text", "fieldBinding": "price"},
				{"type": "text", "id": "brand", "fieldBinding": "brand"},
				{"type": "text", "id": "tagline", "content": "Hand-picked for you"}, // literal
				{"type": "text", "id": "rating", "fieldBinding": "rating"},          // missing in data
			},
		},
	}

	data := []map[string]any{
		{"name": "Cleanser", "price": 1290, "brand": "COSRX"},
	}

	res := BindData(doc, data)

	if len(res.Bound) != 3 {
		t.Errorf("bound: %v, want 3", res.Bound)
	}
	if len(res.Missing) != 1 || res.Missing[0] != "rating" {
		t.Errorf("missing: %v", res.Missing)
	}
	// Skipped includes literal `tagline`, plus the parent frame (which has
	// no fieldBinding either). Both are correct — frames aren't bindable.
	if len(res.Skipped) < 2 {
		t.Errorf("skipped should contain literal tagline + frame: %v", res.Skipped)
	}

	// Find the title node and verify the value landed in `content`.
	title := FindNodeByID(doc, "title")
	if title == nil || title["content"] != "Cleanser" {
		t.Errorf("title content: %+v", title)
	}
	if title["__bound"] != true {
		t.Errorf("title should be marked bound: %+v", title)
	}

	// Literal tagline must be untouched.
	tag := FindNodeByID(doc, "tagline")
	if tag["content"] != "Hand-picked for you" || tag["__bound"] == true {
		t.Errorf("literal tagline mutated: %+v", tag)
	}

	// Honest __bound: rating asked for a field not present → __bound NOT set.
	rating := FindNodeByID(doc, "rating")
	if _, has := rating["__bound"]; has {
		t.Errorf("rating must not be marked bound (Грабля #1): %+v", rating)
	}
	if _, has := rating["content"]; has {
		t.Errorf("rating must not have content set: %+v", rating)
	}
}

// TestBindDataLiteralLLMText covers the text_explainer scenario: a Document
// whose only node is a TextNode with `content` set directly (LLM wrote a
// string into it), no `fieldBinding`. BindData must leave it untouched.
func TestBindDataLiteralLLMText(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{"type": "text", "id": "explainer", "content": "Why we recommend this product..."},
	}
	res := BindData(doc, nil)
	if len(res.Bound) != 0 || len(res.Missing) != 0 {
		t.Errorf("literal-only doc should produce nothing: %+v", res)
	}
	n := doc.Children[0]
	if n["content"] != "Why we recommend this product..." {
		t.Errorf("literal content mutated: %+v", n)
	}
}

// TestBindDataReplicateViaDataIndex covers the per-instance scope marker:
// 3 sibling text nodes share the same fieldBinding but have dataIndex 0/1/2.
// Each gets the matching record's name.
func TestBindDataReplicateViaDataIndex(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{
			"type": "frame", "id": "row",
			"children": []Node{
				{"type": "text", "id": "n0", "fieldBinding": "name", "dataIndex": 0},
				{"type": "text", "id": "n1", "fieldBinding": "name", "dataIndex": 1},
				{"type": "text", "id": "n2", "fieldBinding": "name", "dataIndex": 2},
			},
		},
	}
	data := []map[string]any{
		{"name": "Alpha"},
		{"name": "Bravo"},
		{"name": "Charlie"},
	}
	res := BindData(doc, data)
	if len(res.Bound) != 3 {
		t.Errorf("expected 3 bound, got %v", res.Bound)
	}
	for i, id := range []string{"n0", "n1", "n2"} {
		n := FindNodeByID(doc, id)
		want := data[i]["name"]
		if n["content"] != want {
			t.Errorf("%s content = %v, want %v", id, n["content"], want)
		}
	}
}

// TestBindDataDataIndexOutOfRange covers safety: dataIndex past data length
// should not panic; the node should land in Missing, not Bound.
func TestBindDataDataIndexOutOfRange(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{"type": "text", "id": "x", "fieldBinding": "name", "dataIndex": 5},
	}
	data := []map[string]any{{"name": "Alpha"}}
	res := BindData(doc, data)
	if len(res.Missing) != 1 || res.Missing[0] != "x" {
		t.Errorf("expected x in Missing: %+v", res)
	}
	if len(res.Bound) != 0 {
		t.Errorf("nothing should be bound: %+v", res)
	}
}

// TestBindDataAfterJSONRoundTrip — the key check that BindData copes with
// `[]any` children (the post-Unmarshal shape) just as well as `[]Node`.
func TestBindDataAfterJSONRoundTrip(t *testing.T) {
	src := NewDocument()
	src.Children = []Node{
		{
			"type": "frame", "id": "card",
			"children": []Node{
				{"type": "text", "id": "title", "fieldBinding": "name"},
			},
		},
	}
	raw, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var dst Document
	if err := json.Unmarshal(raw, &dst); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	res := BindData(&dst, []map[string]any{{"name": "Roundtripped"}})
	if len(res.Bound) != 1 {
		t.Errorf("after roundtrip expected 1 bound, got %v", res.Bound)
	}
	title := FindNodeByID(&dst, "title")
	if title == nil || title["content"] != "Roundtripped" {
		t.Errorf("post-roundtrip binding lost: %+v", title)
	}
}

// TestBindDataImageFreshFills covers chunk-4 image binding: an image atom
// with no existing fills should get a freshly-installed fill array of
// length 1 with the resolved URL.
func TestBindDataImageFreshFills(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{"type": NodeTypeImage, "id": "hero", "fieldBinding": "heroImage"},
	}
	data := []map[string]any{{"heroImage": "https://cdn.example/x.jpg"}}
	res := BindData(doc, data)

	if len(res.Bound) != 1 || res.Bound[0] != "hero" {
		t.Fatalf("expected hero bound: %+v", res)
	}
	hero := FindNodeByID(doc, "hero")
	fills, _ := hero["fills"].([]any)
	if len(fills) != 1 {
		t.Fatalf("expected single fill, got: %+v", hero["fills"])
	}
	first, _ := fills[0].(map[string]any)
	if first["type"] != FillTypeImage {
		t.Errorf("fill type = %v, want %s", first["type"], FillTypeImage)
	}
	if first["image"] != "https://cdn.example/x.jpg" {
		t.Errorf("fill image = %v", first["image"])
	}
	if first["mode"] != "fill" {
		t.Errorf("fill mode = %v, want fill", first["mode"])
	}
	// Image atom must NOT have a `value` or `content` set as a side effect.
	if _, has := hero["value"]; has {
		t.Errorf("image atom should not have value: %+v", hero)
	}
	if _, has := hero["content"]; has {
		t.Errorf("image atom should not have content: %+v", hero)
	}
}

// TestBindDataImageUpdatesExistingFills makes sure we mutate the existing
// fill[0] in place rather than appending. Preserves any extra metadata
// callers (e.g. canvas) put on the fill object.
func TestBindDataImageUpdatesExistingFills(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{
			"type": NodeTypeImage, "id": "hero", "fieldBinding": "heroImage",
			"fills": []any{
				map[string]any{
					"type":    FillTypeImage,
					"image":   "https://placeholder.example/p.jpg",
					"mode":    "fit",
					"opacity": 0.5,
				},
			},
		},
	}
	data := []map[string]any{{"heroImage": "https://cdn.example/real.jpg"}}
	BindData(doc, data)

	hero := FindNodeByID(doc, "hero")
	fills, _ := hero["fills"].([]any)
	if len(fills) != 1 {
		t.Fatalf("fills length changed: %+v", fills)
	}
	first, _ := fills[0].(map[string]any)
	if first["image"] != "https://cdn.example/real.jpg" {
		t.Errorf("image not updated: %+v", first)
	}
	if first["mode"] != "fit" {
		t.Errorf("existing mode lost: %+v", first)
	}
	if first["opacity"] != 0.5 {
		t.Errorf("existing opacity lost: %+v", first)
	}
}

// TestBindDataImageMissingField — image atom asks for a field absent from
// data; must NOT set fills, must NOT mark __bound, must land in Missing.
// Mirrors Грабля #1 fix for image atoms specifically.
func TestBindDataImageMissingField(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{"type": NodeTypeImage, "id": "hero", "fieldBinding": "heroImage"},
	}
	data := []map[string]any{{"name": "Cleanser"}}
	res := BindData(doc, data)

	if len(res.Missing) != 1 || res.Missing[0] != "hero" {
		t.Errorf("expected hero in Missing: %+v", res)
	}
	hero := FindNodeByID(doc, "hero")
	if _, has := hero["fills"]; has {
		t.Errorf("missing image must not write fills: %+v", hero)
	}
	if _, has := hero["__bound"]; has {
		t.Errorf("missing image must not mark __bound: %+v", hero)
	}
}

// TestBindDataImageNonStringValue — if data carries a non-string value at
// the bound key (defensive guard against mis-shaped catalog rows), treat
// it as a miss rather than coercing junk into the fill URL.
func TestBindDataImageNonStringValue(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{"type": NodeTypeImage, "id": "hero", "fieldBinding": "heroImage"},
	}
	data := []map[string]any{{"heroImage": []string{"a", "b"}}}
	res := BindData(doc, data)

	if len(res.Missing) != 1 || res.Missing[0] != "hero" {
		t.Errorf("expected hero in Missing for non-string image: %+v", res)
	}
	hero := FindNodeByID(doc, "hero")
	if _, has := hero["fills"]; has {
		t.Errorf("non-string image must not write fills: %+v", hero)
	}
}

// TestBindDataInheritsDataIndexFromAncestor — the chunk-4 mechanism that
// lets a replicate fan-out stamp dataIndex once on the clone root and have
// every descendant atom bind against that record. Closes plan-agent
// finding #2.
func TestBindDataInheritsDataIndexFromAncestor(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		// Card-shaped subtree with dataIndex on the OUTER frame only.
		{
			"type": "frame", "id": "card-1", "dataIndex": 1,
			"children": []Node{
				{
					"type": "frame", "id": "card-1-info",
					"children": []Node{
						{"type": "text", "id": "card-1-name", "fieldBinding": "name"},
						{"type": "text", "id": "card-1-brand", "fieldBinding": "brand"},
					},
				},
			},
		},
		{
			"type": "frame", "id": "card-2", "dataIndex": 2,
			"children": []Node{
				{"type": "text", "id": "card-2-name", "fieldBinding": "name"},
			},
		},
	}
	data := []map[string]any{
		{"name": "Alpha", "brand": "A-Brand"},
		{"name": "Bravo", "brand": "B-Brand"},
		{"name": "Charlie", "brand": "C-Brand"},
	}
	BindData(doc, data)

	// card-1's descendants should have inherited dataIndex=1 → "Bravo"/"B-Brand".
	if n := FindNodeByID(doc, "card-1-name"); n["content"] != "Bravo" {
		t.Errorf("card-1-name = %v, want Bravo", n["content"])
	}
	if n := FindNodeByID(doc, "card-1-brand"); n["content"] != "B-Brand" {
		t.Errorf("card-1-brand = %v, want B-Brand", n["content"])
	}
	// card-2's descendant should have inherited dataIndex=2 → "Charlie".
	if n := FindNodeByID(doc, "card-2-name"); n["content"] != "Charlie" {
		t.Errorf("card-2-name = %v, want Charlie", n["content"])
	}
}

// TestBindDataChildOverridesAncestorDataIndex — explicit child dataIndex
// wins over inherited. Edge case for when a preset author wires a single
// "always-data[0]" header inside a replicated card.
func TestBindDataChildOverridesAncestorDataIndex(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{
			"type": "frame", "id": "card", "dataIndex": 2,
			"children": []Node{
				{"type": "text", "id": "inherited", "fieldBinding": "name"},
				{"type": "text", "id": "pinned", "fieldBinding": "name", "dataIndex": 0},
			},
		},
	}
	data := []map[string]any{
		{"name": "Alpha"}, {"name": "Bravo"}, {"name": "Charlie"},
	}
	BindData(doc, data)
	if n := FindNodeByID(doc, "inherited"); n["content"] != "Charlie" {
		t.Errorf("inherited = %v, want Charlie", n["content"])
	}
	if n := FindNodeByID(doc, "pinned"); n["content"] != "Alpha" {
		t.Errorf("pinned = %v, want Alpha", n["content"])
	}
}

// TestBindDataSkipsReusableSubtree — chunk-5 mechanism: top-level
// component definitions in a materialised doc are marked reusable:true.
// They must be skipped entirely so their template atoms don't get bound
// against data[0]. Atoms inside instances (ref-resolved into the consumer
// site) still bind because Materialise stamps reusable only on the
// definition copy, not on the resolved instance.
func TestBindDataSkipsReusableSubtree(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		// Reusable component definition — atoms inside must not bind.
		{
			"type": "frame", "id": "price-rating-root", "reusable": true,
			"children": []Node{
				{"type": "text", "id": "tpl-price", "fieldBinding": "name"},
			},
		},
		// Instance (e.g. a clone produced by ResolveAndInline) — atoms inside DO bind.
		{
			"type": "frame", "id": "instance",
			"children": []Node{
				{"type": "text", "id": "real-price", "fieldBinding": "name"},
			},
		},
	}
	res := BindData(doc, []map[string]any{{"name": "Cleanser"}})

	if len(res.Bound) != 1 || res.Bound[0] != "real-price" {
		t.Errorf("only real-price should bind, got: %+v", res)
	}
	tpl := FindNodeByID(doc, "tpl-price")
	if _, has := tpl["__bound"]; has {
		t.Errorf("template inside reusable subtree must not be bound: %+v", tpl)
	}
	if _, has := tpl["content"]; has {
		t.Errorf("template inside reusable subtree must not have content set: %+v", tpl)
	}
}

// TestBindDataReusableFalseStillBinds — defensive: reusable:false (or
// missing) leaves the subtree open to binding.
func TestBindDataReusableFalseStillBinds(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{
			"type": "frame", "id": "card", "reusable": false,
			"children": []Node{
				{"type": "text", "id": "name-atom", "fieldBinding": "name"},
			},
		},
	}
	res := BindData(doc, []map[string]any{{"name": "Cleanser"}})
	if len(res.Bound) != 1 || res.Bound[0] != "name-atom" {
		t.Errorf("reusable:false → atoms should bind: %+v", res)
	}
}

// TestBindDataDoesntDescendIntoUnexpandedRefs ensures binding doesn't try
// to mutate ref-node descendants directly. ComponentResolver expands refs
// into a separate tree; the source ref subtree has no fieldBinding and
// must be skipped.
func TestBindDataIgnoresRefNodes(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{"type": "ref", "id": "r1", "ref": "btn"},
	}
	data := []map[string]any{{"name": "anything"}}
	res := BindData(doc, data)
	// Ref node has no fieldBinding → Skipped, not Bound or Missing.
	if len(res.Bound) != 0 || len(res.Missing) != 0 {
		t.Errorf("ref node should be untouched: %+v", res)
	}
	if len(res.Skipped) != 1 {
		t.Errorf("ref node should be skipped exactly once: %+v", res)
	}
}
