package engine

import (
	"testing"
)

// TestBuildTreeMapEmptyDoc — nil + empty docs return nil so callers can
// skip the <formation_tree> block on first turn.
func TestBuildTreeMapEmptyDoc(t *testing.T) {
	if BuildTreeMap(nil) != nil {
		t.Errorf("nil doc should produce nil tree_map")
	}
	if BuildTreeMap(NewDocument()) != nil {
		t.Errorf("empty doc should produce nil tree_map")
	}
}

// TestBuildTreeMapReplicatedCards — fan-out of 3 cards collapses into
// one TreeMapInstances with count=3 + a single Shape. Atoms include the
// title (bound to "name") and the ref slot.
func TestBuildTreeMapReplicatedCards(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{
			"type": "frame", "id": "card", "replicate": true,
			"children": []Node{
				{"type": "text", "id": "title", "fieldBinding": "name", "slot": "title"},
				{"type": "ref", "id": "card-meta", "ref": "price-rating-root", "slot": "price"},
			},
		},
	}
	ExpandReplicates(doc, 3)

	tm := BuildTreeMap(doc)
	if tm == nil {
		t.Fatalf("BuildTreeMap returned nil")
	}
	if tm.Instances == nil {
		t.Fatalf("Instances missing")
	}
	if tm.Instances.Count != 3 {
		t.Errorf("Count = %d, want 3", tm.Instances.Count)
	}
	if len(tm.Instances.IDs) != 3 {
		t.Errorf("len(IDs) = %d, want 3", len(tm.Instances.IDs))
	}
	if tm.Instances.GroupedBy != "card" {
		t.Errorf("GroupedBy = %q, want card", tm.Instances.GroupedBy)
	}
	if len(tm.Instances.Shape.Atoms) != 2 {
		t.Errorf("Shape.Atoms count = %d, want 2", len(tm.Instances.Shape.Atoms))
	}
	if tm.Instances.Shape.Atoms[0].FieldBinding != "name" {
		t.Errorf("first atom field = %q, want name", tm.Instances.Shape.Atoms[0].FieldBinding)
	}
	if tm.Instances.Shape.Atoms[1].Ref != "price-rating-root" {
		t.Errorf("second atom ref = %q, want price-rating-root", tm.Instances.Shape.Atoms[1].Ref)
	}
}

// TestBuildTreeMapMultiWidgetCompose — hero literal + replicated gallery
// + cta literal: largest group (gallery) becomes Instances; hero/cta
// land in Literals.
func TestBuildTreeMapMultiWidgetCompose(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{
			"type": "frame", "id": "hero",
			"children": []Node{
				{"type": "text", "id": "hero-text", "content": "New collection"},
			},
		},
		{
			"type": "frame", "id": "gallery", "replicate": true,
			"children": []Node{
				{"type": "text", "id": "g-title", "fieldBinding": "name"},
			},
		},
		{
			"type": "frame", "id": "cta",
			"children": []Node{
				{"type": "text", "id": "cta-text", "content": "Shop now", "wrapper": "button"},
			},
		},
	}
	ExpandReplicates(doc, 2)

	tm := BuildTreeMap(doc)
	if tm == nil {
		t.Fatalf("BuildTreeMap returned nil")
	}
	if tm.Instances == nil || tm.Instances.Count != 2 {
		t.Fatalf("Instances Count = %v, want 2", tm.Instances)
	}
	// Literals: hero + cta. Order matches doc order.
	if len(tm.Literals) != 2 {
		t.Fatalf("len(Literals) = %d, want 2 (hero + cta)", len(tm.Literals))
	}
	if tm.Literals[0].ID != "hero" {
		t.Errorf("Literals[0].ID = %q, want hero", tm.Literals[0].ID)
	}
	if tm.Literals[1].ID != "cta" {
		t.Errorf("Literals[1].ID = %q, want cta", tm.Literals[1].ID)
	}
	// CTA atom should carry wrapper hint.
	if tm.Literals[1].Atoms[0].Wrapper != "button" {
		t.Errorf("CTA wrapper = %q, want button", tm.Literals[1].Atoms[0].Wrapper)
	}
}

// TestBuildTreeMapSingleLiteral — non-replicated detail view: no
// Instances, one Literal entry.
func TestBuildTreeMapSingleLiteral(t *testing.T) {
	doc := NewDocument()
	doc.Children = []Node{
		{
			"type": "frame", "id": "detail",
			"children": []Node{
				{"type": "text", "id": "title", "fieldBinding": "name", "slot": "title"},
				{"type": "text", "id": "desc", "fieldBinding": "description"},
			},
		},
	}

	tm := BuildTreeMap(doc)
	if tm == nil {
		t.Fatalf("BuildTreeMap returned nil")
	}
	if tm.Instances != nil {
		t.Errorf("non-replicated doc should have nil Instances, got %+v", tm.Instances)
	}
	if len(tm.Literals) != 1 || tm.Literals[0].ID != "detail" {
		t.Fatalf("expected 1 literal 'detail', got %+v", tm.Literals)
	}
	if len(tm.Literals[0].Atoms) != 2 {
		t.Errorf("detail atoms = %d, want 2", len(tm.Literals[0].Atoms))
	}
}

// TestBuildTreeMapResolvedComponents — once a ref node is resolved, the
// component's atoms appear in Components (deduped by ref name) but NOT
// in the ref node's children listing in Shape.
func TestBuildTreeMapResolvedComponents(t *testing.T) {
	doc := NewDocument()
	// One reusable component definition at root + one card using it.
	doc.Children = []Node{
		{
			"type": "frame", "id": "price-rating-root", "reusable": true,
			"children": []Node{
				{"type": "text", "fieldBinding": "priceFormatted", "format": "currency"},
				{"type": "text", "fieldBinding": "rating", "format": "stars-compact"},
			},
		},
		{
			"type": "frame", "id": "card", "replicate": true,
			"children": []Node{
				{"type": "text", "id": "title", "fieldBinding": "name"},
				{"type": "ref", "id": "card-meta", "ref": "price-rating-root", "slot": "price"},
			},
		},
	}
	ExpandReplicates(doc, 2)
	ResolveAndInline(doc)

	tm := BuildTreeMap(doc)
	if tm == nil {
		t.Fatalf("BuildTreeMap returned nil")
	}
	if tm.Instances == nil || tm.Instances.Count != 2 {
		t.Fatalf("Instances missing or wrong count: %+v", tm.Instances)
	}
	// Shape atoms must NOT include the resolved children of the ref
	// (those are componentInternal — only ref slot itself shows up).
	for _, a := range tm.Instances.Shape.Atoms {
		if a.FieldBinding == "priceFormatted" || a.FieldBinding == "rating" {
			t.Errorf("Shape leaked a component-internal atom: %+v", a)
		}
	}
	// Components must list the price-rating-root component once with both
	// inner field bindings.
	if len(tm.Components) != 1 {
		t.Fatalf("len(Components) = %d, want 1 (deduped)", len(tm.Components))
	}
	comp := tm.Components[0]
	if comp.ID != "price-rating-root" {
		t.Errorf("Components[0].ID = %q, want price-rating-root", comp.ID)
	}
	if len(comp.Atoms) != 2 {
		t.Errorf("component atoms = %d, want 2", len(comp.Atoms))
	}
}
