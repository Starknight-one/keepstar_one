package engine_v4

import (
	"testing"

	"keepstar_v4/internal/domain"
)

// TestRefChainingLayoutNesting verifies that $ref chaining works for nested layout nodes.
// This was broken: insertLayoutNode returned a pending ID but didn't register it in idx.nodes,
// so subsequent ops couldn't find the parent and atoms fell to root level.
func TestRefChainingLayoutNesting(t *testing.T) {
	formation := &domain.FormationWithData{}

	ops := ProductCardGridOps() // uses $w, $root, $info, $meta refs
	warnings := ApplyOps(formation, ops)

	// Should produce no warnings about missing parents
	for _, w := range warnings {
		t.Errorf("unexpected warning: %s", w)
	}

	if len(formation.Widgets) != 1 {
		t.Fatalf("expected 1 widget, got %d", len(formation.Widgets))
	}

	widget := formation.Widgets[0]

	// Should have 6 atoms: image, name, price, rating, brand (+ column/row nodes don't count as atoms)
	// Actually: image, text(name), number(price), number(rating), text(brand) = 5
	if len(widget.Atoms) < 5 {
		t.Errorf("expected at least 5 atoms, got %d", len(widget.Atoms))
	}

	// Critical: layout should NOT be flat. Root should have nested children.
	if widget.Layout == nil {
		t.Fatal("widget.Layout is nil")
	}

	root := widget.Layout
	if len(root.Children) == 0 {
		t.Fatal("root layout has no children")
	}

	// Root should have a column child ($root), NOT all atoms flat
	hasNestedNode := false
	for _, ch := range root.Children {
		if ch.Node != nil {
			hasNestedNode = true
			// This nested node ($root column) should itself have children
			if len(ch.Node.Children) == 0 {
				t.Error("nested $root node has no children — nesting is broken")
			}
			break
		}
	}
	if !hasNestedNode {
		t.Error("root layout has no nested nodes — all atoms are flat (layout nesting bug)")
	}
}

// TestRefChainingDeepNesting verifies 3-level nesting: widget → root → info → meta.
func TestRefChainingDeepNesting(t *testing.T) {
	formation := &domain.FormationWithData{}

	ops := ProductCardGridOps()
	ApplyOps(formation, ops)

	widget := formation.Widgets[0]
	root := widget.Layout

	// Walk: root → $root (column) → $info (column) → $meta (row)
	// root is the auto-created widget root layout
	// First child should be $root column
	if len(root.Children) == 0 || root.Children[0].Node == nil {
		t.Fatal("no $root column node")
	}
	rootCol := root.Children[0].Node

	// $root column should have: image atom + $info column
	infoNode := findNodeByType(rootCol, domain.LayoutNodeColumn)
	if infoNode == nil {
		t.Fatal("$info column not found inside $root")
	}

	// $info should have: name atom + $meta row + brand atom
	metaNode := findNodeByType(infoNode, domain.LayoutNodeRow)
	if metaNode == nil {
		t.Fatal("$meta row not found inside $info — deep nesting is broken")
	}

	// $meta should have price + rating atoms
	atomCount := 0
	for _, ch := range metaNode.Children {
		if ch.AtomIndex != nil {
			atomCount++
		}
	}
	if atomCount < 2 {
		t.Errorf("$meta row should have at least 2 atoms (price + rating), got %d", atomCount)
	}
}

func findNodeByType(parent *domain.LayoutNode, nodeType domain.LayoutNodeType) *domain.LayoutNode {
	for _, ch := range parent.Children {
		if ch.Node != nil && ch.Node.Type == nodeType {
			return ch.Node
		}
	}
	return nil
}

// TestReplicationPreservesLayout verifies that deep copy via replication keeps layout structure.
func TestReplicationPreservesLayout(t *testing.T) {
	data := []map[string]interface{}{
		{"id": "1", "name": "Product A", "price": 10.0, "images": "img1.jpg", "rating": 4.5, "brand": "Brand A"},
		{"id": "2", "name": "Product B", "price": 20.0, "images": "img2.jpg", "rating": 3.5, "brand": "Brand B"},
		{"id": "3", "name": "Product C", "price": 30.0, "images": "img3.jpg", "rating": 5.0, "brand": "Brand C"},
	}

	input := ExecuteInput{
		Ops:        ProductCardGridOps(),
		Data:       data,
		EntityType: "product",
		Layout:     "grid",
		Columns:    3,
		Replicate:  true, // B3: replication is now explicit
	}

	output := NewEngine().Execute(input)

	if len(output.Formation.Widgets) != 3 {
		t.Fatalf("expected 3 widgets after replication, got %d", len(output.Formation.Widgets))
	}

	// Each widget should have proper layout nesting, not flat
	for i, w := range output.Formation.Widgets {
		if w.Layout == nil {
			t.Errorf("widget[%d] has nil layout", i)
			continue
		}
		hasNestedNode := false
		for _, ch := range w.Layout.Children {
			if ch.Node != nil {
				hasNestedNode = true
				break
			}
		}
		if !hasNestedNode {
			t.Errorf("widget[%d] layout is flat — replication lost nesting", i)
		}

		// Check data binding
		if w.EntityRef == nil || w.EntityRef.ID != data[i]["id"] {
			t.Errorf("widget[%d] EntityRef mismatch", i)
		}
	}

	// Check that data was bound
	for i, w := range output.Formation.Widgets {
		for _, a := range w.Atoms {
			if a.FieldName == "name" && a.Value != data[i]["name"] {
				t.Errorf("widget[%d] name atom: expected %v, got %v", i, data[i]["name"], a.Value)
			}
		}
	}
}
