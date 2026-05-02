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
