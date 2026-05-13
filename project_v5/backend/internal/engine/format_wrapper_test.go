package engine

import "testing"

// TestBindDataPreservesFormatProperty — `format` is a pass-through property
// the frontend consumes; binding must not strip, rewrite, or coerce it.
func TestBindDataPreservesFormatProperty(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{
			"type":         "text",
			"id":           "price",
			"fieldBinding": "priceFormatted",
			"format":       "currency",
		},
	}
	res := BindData(doc, []map[string]any{{"priceFormatted": "$29.99"}})
	if len(res.Bound) != 1 {
		t.Fatalf("expected 1 bound: %+v", res)
	}
	got := FindNodeByID(doc, "price")
	if got["format"] != "currency" {
		t.Errorf("format property mutated: %+v", got)
	}
	if got["content"] != "$29.99" {
		t.Errorf("content not bound: %+v", got)
	}
}

// TestBindDataPreservesFormatOnRawNumeric — confirms the architectural
// contract: when fieldBinding resolves to a non-string (e.g. rating float),
// `content` carries the raw value and `format` survives untouched. The
// frontend is responsible for the float → "★ 4.5" step.
func TestBindDataPreservesFormatOnRawNumeric(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{
			"type":         "text",
			"id":           "rating",
			"fieldBinding": "rating",
			"format":       "stars-compact",
		},
	}
	res := BindData(doc, []map[string]any{{"rating": 4.5}})
	if len(res.Bound) != 1 {
		t.Fatalf("expected 1 bound: %+v", res)
	}
	got := FindNodeByID(doc, "rating")
	if got["format"] != "stars-compact" {
		t.Errorf("format property mutated: %+v", got)
	}
	if got["content"] != 4.5 {
		t.Errorf("content should carry raw float, got %v (%T)", got["content"], got["content"])
	}
}

// TestBindDataPreservesWrapperProperty — same guarantee for `wrapper`.
func TestBindDataPreservesWrapperProperty(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{
			"type":         "text",
			"id":           "brand",
			"fieldBinding": "brand",
			"wrapper":      "badge",
		},
	}
	BindData(doc, []map[string]any{{"brand": "Acme"}})
	got := FindNodeByID(doc, "brand")
	if got["wrapper"] != "badge" {
		t.Errorf("wrapper property mutated: %+v", got)
	}
	if got["content"] != "Acme" {
		t.Errorf("content not bound: %+v", got)
	}
}

// TestBindDataLeavesFormatOnUnboundNode — when binding fails (no field on
// the data record), the node is recorded as Missing but its format and
// wrapper properties stay intact for the LLM's next turn.
func TestBindDataLeavesFormatOnUnboundNode(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{
			"type":         "text",
			"id":           "price",
			"fieldBinding": "priceFormatted",
			"format":       "currency",
			"wrapper":      "tag",
		},
	}
	res := BindData(doc, []map[string]any{{"name": "no price field"}})
	if len(res.Missing) != 1 || res.Missing[0] != "price" {
		t.Fatalf("expected price in Missing: %+v", res)
	}
	got := FindNodeByID(doc, "price")
	if got["format"] != "currency" || got["wrapper"] != "tag" {
		t.Errorf("format/wrapper lost on unbound node: %+v", got)
	}
	if _, has := got["content"]; has {
		t.Errorf("unbound node should have no content: %+v", got)
	}
	if _, has := got["__bound"]; has {
		t.Errorf("unbound node should not be marked __bound: %+v", got)
	}
}
