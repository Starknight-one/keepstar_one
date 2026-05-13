package engine

import (
	"reflect"
	"testing"
)

func TestRenderedDataIndices_NilOrEmpty(t *testing.T) {
	if got := RenderedDataIndices(nil); got != nil {
		t.Errorf("nil doc → got %v, want nil", got)
	}
	if got := RenderedDataIndices(&Document{}); got != nil {
		t.Errorf("empty doc → got %v, want nil", got)
	}
}

func TestRenderedDataIndices_SkipsLiteralsAndReusables(t *testing.T) {
	// Hand-crafted post-replicate document: 3 clones with dataIndex 0/1/2,
	// one literal frame (no templateOrigin), one reusable component
	// definition (skipped by isReusable). Walker should return [0,1,2] only.
	doc := &Document{
		Children: []Node{
			{"id": "comp-product-card", "type": "frame", "reusable": true}, // skipped
			{"id": "literal-hero", "type": "frame"},                          // skipped
			{"id": "card-0", "type": "frame", attrTemplateOrigin: "tpl-card", attrDataIndex: 0},
			{"id": "card-1", "type": "frame", attrTemplateOrigin: "tpl-card", attrDataIndex: 1},
			{"id": "card-2", "type": "frame", attrTemplateOrigin: "tpl-card", attrDataIndex: 2},
		},
	}
	got := RenderedDataIndices(doc)
	want := []int{0, 1, 2}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestRenderedDataIndices_DedupAndJSONFloats(t *testing.T) {
	// JSON unmarshal turns ints into float64 — readDataIndex must handle
	// that. Test with mixed int + float64 values, plus a duplicate index
	// (defensive: ensure we dedupe rather than emit twice).
	doc := &Document{
		Children: []Node{
			{"id": "a", "type": "frame", attrTemplateOrigin: "t", attrDataIndex: float64(0)},
			{"id": "b", "type": "frame", attrTemplateOrigin: "t", attrDataIndex: 1},
			{"id": "c", "type": "frame", attrTemplateOrigin: "t", attrDataIndex: float64(0)}, // dup
		},
	}
	got := RenderedDataIndices(doc)
	if !reflect.DeepEqual(got, []int{0, 1}) {
		t.Errorf("got %v, want [0,1] (dedup + float coercion)", got)
	}
}

func TestRenderedDataIndices_NoReplicateMarkers(t *testing.T) {
	// text_explainer / error preset documents have no replicate clones —
	// only literal frames at root. Should return nil so caller omits the
	// rendered block entirely.
	doc := &Document{
		Children: []Node{
			{"id": "text-frame", "type": "frame"},
			{"id": "error-frame", "type": "frame"},
		},
	}
	if got := RenderedDataIndices(doc); got != nil {
		t.Errorf("got %v, want nil for literal-only doc", got)
	}
}
