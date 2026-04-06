package engine_v4

import (
	"testing"

	"keepstar_v4/internal/domain"
)

// makeLiteralWidget builds a hero/explainer-style widget with no FieldName atoms.
func makeLiteralWidget(text string) domain.Widget {
	return domain.Widget{
		Atoms: []domain.Atom{
			{Type: domain.AtomTypeText, Value: text},
		},
	}
}

// makeEntityWidget builds a single entity-bound widget (no ReplicateConfig).
func makeEntityWidget() domain.Widget {
	return domain.Widget{
		Atoms: []domain.Atom{
			{Type: domain.AtomTypeText, FieldName: "name"},
			{Type: domain.AtomTypeNumber, FieldName: "price"},
		},
	}
}

// makeReplicateTemplate builds a widget marked for replication.
func makeReplicateTemplate() domain.Widget {
	return domain.Widget{
		Atoms: []domain.Atom{
			{Type: domain.AtomTypeImage, FieldName: "image"},
			{Type: domain.AtomTypeText, FieldName: "name"},
			{Type: domain.AtomTypeNumber, FieldName: "price"},
		},
		ReplicateConfig: &domain.ReplicateConfig{Enabled: true},
	}
}

func sampleProductData(n int) []map[string]interface{} {
	data := make([]map[string]interface{}, n)
	for i := 0; i < n; i++ {
		data[i] = map[string]interface{}{
			"id":    i + 1,
			"name":  []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta"}[i%6],
			"price": float64((i + 1) * 100),
			"image": "https://example.com/img.jpg",
		}
	}
	return data
}

// TestExpandInPlace_LiteralsPreserved verifies that expandReplicatedWidgets
// only touches widgets with ReplicateConfig.Enabled and leaves literals intact.
// Layout: [hero, gallery_template, cta] with 3 data items → [hero, c1, c2, c3, cta] = 5 widgets.
func TestExpandInPlace_LiteralsPreserved(t *testing.T) {
	formation := &domain.FormationWithData{
		Widgets: []domain.Widget{
			makeLiteralWidget("Welcome"),
			makeReplicateTemplate(),
			makeLiteralWidget("Buy now"),
		},
	}
	data := sampleProductData(3)

	expandReplicatedWidgets(formation, data, "product")

	if len(formation.Widgets) != 5 {
		t.Fatalf("expected 5 widgets after expansion (1 hero + 3 clones + 1 cta), got %d", len(formation.Widgets))
	}

	// Hero literal preserved at index 0.
	if formation.Widgets[0].EntityRef != nil {
		t.Errorf("literal hero should have no EntityRef, got %+v", formation.Widgets[0].EntityRef)
	}
	if formation.Widgets[0].ReplicateConfig != nil {
		t.Errorf("literal hero should have no ReplicateConfig, got %+v", formation.Widgets[0].ReplicateConfig)
	}

	// Clones at 1, 2, 3 each have EntityRef + GroupID.
	for i := 1; i <= 3; i++ {
		w := formation.Widgets[i]
		if w.EntityRef == nil {
			t.Errorf("clone %d missing EntityRef", i)
			continue
		}
		if w.ReplicateConfig == nil || w.ReplicateConfig.GroupID == "" {
			t.Errorf("clone %d missing ReplicateConfig.GroupID", i)
		}
	}
	// All clones in the same group.
	g1 := formation.Widgets[1].ReplicateConfig.GroupID
	if formation.Widgets[2].ReplicateConfig.GroupID != g1 ||
		formation.Widgets[3].ReplicateConfig.GroupID != g1 {
		t.Error("clones from the same template should share GroupID")
	}

	// CTA literal preserved at index 4.
	if formation.Widgets[4].EntityRef != nil {
		t.Errorf("literal cta should have no EntityRef, got %+v", formation.Widgets[4].EntityRef)
	}
}

// TestBindData_NoPositionalShift verifies that with a literal widget before the
// replicate template, gallery clones still receive correct data — they get
// data[0..N], not data[1..N+1] which would happen with naive positional bind.
func TestBindData_NoPositionalShift(t *testing.T) {
	formation := &domain.FormationWithData{
		Widgets: []domain.Widget{
			makeLiteralWidget("Header"),
			makeReplicateTemplate(),
		},
	}
	data := sampleProductData(2)

	expandReplicatedWidgets(formation, data, "product")
	// Top-level BindData would shift values if expand didn't inline-bind.
	BindData(formation, data)

	if len(formation.Widgets) != 3 {
		t.Fatalf("expected 3 widgets, got %d", len(formation.Widgets))
	}

	// Clone at index 1 should be bound to data[0] (name=alpha, price=100).
	c1 := formation.Widgets[1]
	if got := atomValue(c1, "name"); got != "alpha" {
		t.Errorf("clone[0] name: expected alpha, got %v", got)
	}
	if got := atomValue(c1, "price"); got != float64(100) {
		t.Errorf("clone[0] price: expected 100, got %v", got)
	}

	// Clone at index 2 should be bound to data[1] (name=beta, price=200).
	c2 := formation.Widgets[2]
	if got := atomValue(c2, "name"); got != "beta" {
		t.Errorf("clone[1] name: expected beta, got %v", got)
	}
	if got := atomValue(c2, "price"); got != float64(200) {
		t.Errorf("clone[1] price: expected 200, got %v", got)
	}
}

// TestConstraints_GroupAware_LiteralsSkipped verifies that the C1 field-presence
// threshold scopes to replicate groups: 3 literals + 6 product clones should
// NOT cause `name`/`price`/`image` to be removed from clones because of the
// 3 literals dragging the count below 70%.
func TestConstraints_GroupAware_LiteralsSkipped(t *testing.T) {
	formation := &domain.FormationWithData{
		Widgets: []domain.Widget{
			makeLiteralWidget("Hero"),
			makeReplicateTemplate(),
			makeLiteralWidget("Mid"),
			makeLiteralWidget("Footer"),
		},
	}
	data := sampleProductData(6)

	expandReplicatedWidgets(formation, data, "product")
	BindData(formation, data)
	ApplyConstraints(formation)

	// 1 hero + 6 clones + 2 mid/footer = 9 widgets.
	if len(formation.Widgets) != 9 {
		t.Fatalf("expected 9 widgets, got %d", len(formation.Widgets))
	}

	// Each clone (indices 1..6) must still have its three fields after constraints.
	for i := 1; i <= 6; i++ {
		w := formation.Widgets[i]
		if !widgetHasFieldNameAtoms(&w) {
			t.Errorf("clone %d has no FieldName atoms after constraints — C1 dragged them out", i)
			continue
		}
		seen := map[string]bool{}
		for _, a := range w.Atoms {
			if a.FieldName != "" {
				seen[a.FieldName] = true
			}
		}
		for _, f := range []string{"image", "name", "price"} {
			if !seen[f] {
				t.Errorf("clone %d missing field %q after constraints", i, f)
			}
		}
	}
}

// TestEntityRef_AutoDetect verifies that a single entity-bound widget in a
// composition gets EntityRef + inline binding from data[0] (default).
func TestEntityRef_AutoDetect(t *testing.T) {
	formation := &domain.FormationWithData{
		Widgets: []domain.Widget{
			makeLiteralWidget("Welcome"),
			makeEntityWidget(),
		},
	}
	data := sampleProductData(2)

	expandReplicatedWidgets(formation, data, "product")
	autoDetectEntityRefs(formation, data, "product")
	BindData(formation, data)

	if len(formation.Widgets) != 2 {
		t.Fatalf("expected 2 widgets (no expansion), got %d", len(formation.Widgets))
	}

	w := formation.Widgets[1]
	if w.EntityRef == nil {
		t.Fatal("entity widget should have EntityRef auto-detected")
	}
	if w.EntityRef.ID != "1" {
		t.Errorf("EntityRef.ID: expected '1' (data[0]), got %q", w.EntityRef.ID)
	}
	// Bound to data[0] (alpha/100), not data[1] (beta/200) — positional shift would have caused beta.
	if got := atomValue(w, "name"); got != "alpha" {
		t.Errorf("auto-bound entity name: expected alpha, got %v", got)
	}
	if got := atomValue(w, "price"); got != float64(100) {
		t.Errorf("auto-bound entity price: expected 100, got %v", got)
	}
}

// TestEntityRef_DataIndexOverride verifies that ReplicateConfig.DataIndex picks
// a specific data item for a single non-replicated entity widget.
func TestEntityRef_DataIndexOverride(t *testing.T) {
	w := makeEntityWidget()
	w.ReplicateConfig = &domain.ReplicateConfig{DataIndex: 2}

	formation := &domain.FormationWithData{
		Widgets: []domain.Widget{w},
	}
	data := sampleProductData(4)

	expandReplicatedWidgets(formation, data, "product")
	autoDetectEntityRefs(formation, data, "product")
	BindData(formation, data)

	if len(formation.Widgets) != 1 {
		t.Fatalf("expected 1 widget, got %d", len(formation.Widgets))
	}

	got := formation.Widgets[0]
	if got.EntityRef == nil {
		t.Fatal("widget should have EntityRef from DataIndex=2")
	}
	if got.EntityRef.ID != "3" {
		t.Errorf("EntityRef.ID: expected '3' (data[2]), got %q", got.EntityRef.ID)
	}
	if name := atomValue(got, "name"); name != "gamma" {
		t.Errorf("name: expected gamma (data[2]), got %v", name)
	}
	if price := atomValue(got, "price"); price != float64(300) {
		t.Errorf("price: expected 300 (data[2]), got %v", price)
	}
}

// TestExpandInPlace_EmptyDataDropsTemplate verifies that a replicate template
// with zero data items is dropped from the formation entirely.
func TestExpandInPlace_EmptyDataDropsTemplate(t *testing.T) {
	formation := &domain.FormationWithData{
		Widgets: []domain.Widget{
			makeLiteralWidget("hero"),
			makeReplicateTemplate(),
			makeLiteralWidget("footer"),
		},
	}

	expandReplicatedWidgets(formation, []map[string]interface{}{}, "product")

	if len(formation.Widgets) != 2 {
		t.Fatalf("expected 2 widgets (template dropped), got %d", len(formation.Widgets))
	}
}

// TestExpandInPlace_LimitRespected verifies that ReplicateConfig.Limit caps clone count.
func TestExpandInPlace_LimitRespected(t *testing.T) {
	tmpl := makeReplicateTemplate()
	tmpl.ReplicateConfig.Limit = 2

	formation := &domain.FormationWithData{Widgets: []domain.Widget{tmpl}}
	data := sampleProductData(5)

	expandReplicatedWidgets(formation, data, "product")

	if len(formation.Widgets) != 2 {
		t.Errorf("expected 2 clones (limit=2), got %d", len(formation.Widgets))
	}
}

// atomValue returns the value of the first atom with the given fieldName.
func atomValue(w domain.Widget, fieldName string) interface{} {
	for _, a := range w.Atoms {
		if a.FieldName == fieldName {
			return a.Value
		}
	}
	return nil
}
