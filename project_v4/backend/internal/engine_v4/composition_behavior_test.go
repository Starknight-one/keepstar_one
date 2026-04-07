package engine_v4

import (
	"fmt"
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

// ─── Phase 2 — per-widget preset + replicate via insertWidget props ───

// TestOpsInlinePreset_TwoWidgetsNoCollision verifies that two `insert widget`
// ops with `props.preset = "product_card"` in the same batch produce two
// independent product_card widgets — no ref collisions on $w/$root/$info/$meta.
func TestOpsInlinePreset_TwoWidgetsNoCollision(t *testing.T) {
	formation := &domain.FormationWithData{}
	ops := []Op{
		{Type: OpInsert, Ref: "card1", Parent: "formation", Props: map[string]interface{}{"type": "widget", "preset": "product_card"}},
		{Type: OpInsert, Ref: "card2", Parent: "formation", Props: map[string]interface{}{"type": "widget", "preset": "product_card"}},
	}
	warnings := ApplyOps(formation, ops)

	for _, w := range warnings {
		t.Errorf("unexpected warning: %s", w)
	}
	if len(formation.Widgets) != 2 {
		t.Fatalf("expected 2 widgets after inline preset expansion, got %d", len(formation.Widgets))
	}

	// Both widgets should have the full product_card layout: 5 atoms,
	// nested $root → image + $info → name + $meta → price + rating, brand badge.
	for i, w := range formation.Widgets {
		if len(w.Atoms) != 5 {
			t.Errorf("widget[%d]: expected 5 atoms, got %d", i, len(w.Atoms))
		}
		if w.Layout == nil || len(w.Layout.Children) == 0 {
			t.Errorf("widget[%d]: layout is empty", i)
			continue
		}
		// The deep nesting check from TestRefChainingLayoutNesting:
		// root has at least one nested node child (the $root column).
		nested := false
		for _, ch := range w.Layout.Children {
			if ch.Node != nil && len(ch.Node.Children) > 0 {
				nested = true
				break
			}
		}
		if !nested {
			t.Errorf("widget[%d]: layout is flat — refs collided or expansion broke nesting", i)
		}
	}
}

// TestOpsInlinePreset_UserRefSubstituted verifies that the user-provided
// `op.Ref` substitutes the preset's first ref (`w`), so subsequent override
// ops can target the inserted widget via $<userRef>.
func TestOpsInlinePreset_UserRefSubstituted(t *testing.T) {
	formation := &domain.FormationWithData{}
	ops := []Op{
		{Type: OpInsert, Ref: "myCard", Parent: "formation", Props: map[string]interface{}{"type": "widget", "preset": "product_card"}},
		// Override op uses $myCard to add a literal text into the widget root.
		{Type: OpInsert, Parent: "$myCard", Props: map[string]interface{}{"type": "text", "value": "BADGE"}},
	}
	warnings := ApplyOps(formation, ops)

	for _, w := range warnings {
		// Note: insertAtom on the widget root works only if root has space; we
		// expect either success or "parent not found" — we accept success here.
		if w == "" {
			continue
		}
	}
	if len(formation.Widgets) != 1 {
		t.Fatalf("expected 1 widget, got %d", len(formation.Widgets))
	}
	w := formation.Widgets[0]
	// Original 5 product_card atoms + 1 user-injected literal "BADGE".
	if len(w.Atoms) != 6 {
		t.Errorf("expected 6 atoms (5 preset + 1 override), got %d", len(w.Atoms))
	}
	foundBadge := false
	for _, a := range w.Atoms {
		if a.Value == "BADGE" {
			foundBadge = true
			break
		}
	}
	if !foundBadge {
		t.Error("user override op did not land on the inserted widget — $myCard substitution broken")
	}
}

// TestOpsPerWidgetReplicate verifies that `props.replicate = true` on a widget
// insert sets ReplicateConfig.Enabled, and a subsequent expandReplicatedWidgets
// produces N clones bound to data items.
func TestOpsPerWidgetReplicate(t *testing.T) {
	formation := &domain.FormationWithData{}
	ops := []Op{
		{
			Type:   OpInsert,
			Parent: "formation",
			Props: map[string]interface{}{
				"type":           "widget",
				"preset":         "product_card",
				"replicate":      true,
				"replicateLimit": float64(3), // JSON unmarshal would give float64
			},
		},
	}
	warnings := ApplyOps(formation, ops)
	for _, w := range warnings {
		t.Errorf("unexpected warning: %s", w)
	}
	if len(formation.Widgets) != 1 {
		t.Fatalf("expected 1 template widget after ops, got %d", len(formation.Widgets))
	}
	tmpl := formation.Widgets[0]
	if tmpl.ReplicateConfig == nil || !tmpl.ReplicateConfig.Enabled {
		t.Fatal("ReplicateConfig.Enabled should be true after props.replicate")
	}
	if tmpl.ReplicateConfig.Limit != 3 {
		t.Errorf("ReplicateConfig.Limit: expected 3, got %d", tmpl.ReplicateConfig.Limit)
	}

	// Now run the engine post-process steps.
	data := sampleProductData(5) // 5 items but limit=3
	expandReplicatedWidgets(formation, data, "product")

	if len(formation.Widgets) != 3 {
		t.Fatalf("expected 3 clones (limit=3 from props.replicateLimit), got %d", len(formation.Widgets))
	}
	for i, w := range formation.Widgets {
		if w.EntityRef == nil {
			t.Errorf("clone[%d] missing EntityRef", i)
		}
		if got := atomValue(w, "name"); got == nil {
			t.Errorf("clone[%d] name not bound", i)
		}
	}
}

// TestOpsPerWidgetDataIndex verifies that `props.dataIndex` on a widget insert
// sets ReplicateConfig.DataIndex which autoDetectEntityRefs honours.
func TestOpsPerWidgetDataIndex(t *testing.T) {
	formation := &domain.FormationWithData{}
	ops := []Op{
		{
			Type:   OpInsert,
			Parent: "formation",
			Props: map[string]interface{}{
				"type":      "widget",
				"preset":    "product_card",
				"dataIndex": float64(2),
			},
		},
	}
	warnings := ApplyOps(formation, ops)
	for _, w := range warnings {
		t.Errorf("unexpected warning: %s", w)
	}

	if len(formation.Widgets) != 1 {
		t.Fatalf("expected 1 widget, got %d", len(formation.Widgets))
	}
	w := formation.Widgets[0]
	if w.ReplicateConfig == nil {
		t.Fatal("ReplicateConfig should be set from props.dataIndex")
	}
	if w.ReplicateConfig.Enabled {
		t.Error("ReplicateConfig.Enabled should be false (only dataIndex set)")
	}
	if w.ReplicateConfig.DataIndex != 2 {
		t.Errorf("DataIndex: expected 2, got %d", w.ReplicateConfig.DataIndex)
	}

	data := sampleProductData(5)
	expandReplicatedWidgets(formation, data, "product")
	autoDetectEntityRefs(formation, data, "product")

	if len(formation.Widgets) != 1 {
		t.Fatalf("expected 1 widget after expand (no replicate), got %d", len(formation.Widgets))
	}
	got := formation.Widgets[0]
	if got.EntityRef == nil {
		t.Fatal("EntityRef should be auto-set from dataIndex=2")
	}
	if got.EntityRef.ID != "3" {
		t.Errorf("EntityRef.ID: expected '3' (data[2]), got %q", got.EntityRef.ID)
	}
	if name := atomValue(got, "name"); name != "gamma" {
		t.Errorf("name: expected gamma (data[2]), got %v", name)
	}
}

// TestOpsInlinePreset_UnknownPresetWarns verifies that an unknown preset name
// produces a warning + falls through to insertWidget without crashing.
func TestOpsInlinePreset_UnknownPresetWarns(t *testing.T) {
	formation := &domain.FormationWithData{}
	ops := []Op{
		{Type: OpInsert, Parent: "formation", Props: map[string]interface{}{"type": "widget", "preset": "no_such_preset"}},
	}
	warnings := ApplyOps(formation, ops)

	if len(warnings) == 0 {
		t.Error("expected warning for unknown preset")
	}
	foundUnknown := false
	for _, w := range warnings {
		if containsString(w, "unknown preset") {
			foundUnknown = true
			break
		}
	}
	if !foundUnknown {
		t.Errorf("warnings did not mention 'unknown preset': %v", warnings)
	}

	// Empty widget should still be created via fall-through.
	if len(formation.Widgets) != 1 {
		t.Errorf("expected 1 fall-through widget, got %d", len(formation.Widgets))
	}
}

// TestOpsInlinePreset_PresetMixedWithLiterals verifies a presentation-style
// composition: literal hero + preset card replicate + literal cta. After
// expansion + post-process the formation should have 1 hero + N clones + 1 cta.
func TestOpsInlinePreset_PresetMixedWithLiterals(t *testing.T) {
	formation := &domain.FormationWithData{}
	ops := []Op{
		// Hero literal
		{Type: OpInsert, Ref: "hero", Parent: "formation", Props: map[string]interface{}{"type": "widget", "size": "large"}},
		{Type: OpInsert, Parent: "$hero", Props: map[string]interface{}{"type": "text", "value": "New collection"}},
		// Gallery via preset + replicate
		{Type: OpInsert, Parent: "formation", Props: map[string]interface{}{"type": "widget", "preset": "product_card", "replicate": true}},
		// CTA literal
		{Type: OpInsert, Ref: "cta", Parent: "formation", Props: map[string]interface{}{"type": "widget", "size": "small"}},
		{Type: OpInsert, Parent: "$cta", Props: map[string]interface{}{"type": "text", "value": "Buy now"}},
	}

	warnings := ApplyOps(formation, ops)
	for _, w := range warnings {
		t.Errorf("unexpected warning: %s", w)
	}

	// Before expand: 3 widgets (hero, gallery template, cta).
	if len(formation.Widgets) != 3 {
		t.Fatalf("expected 3 widgets after ApplyOps, got %d", len(formation.Widgets))
	}

	data := sampleProductData(4)
	expandReplicatedWidgets(formation, data, "product")
	autoDetectEntityRefs(formation, data, "product")
	BindData(formation, data)
	ApplyConstraints(formation)

	// After expand: hero + 4 clones + cta = 6 widgets.
	if len(formation.Widgets) != 6 {
		t.Fatalf("expected 6 widgets (hero + 4 clones + cta), got %d", len(formation.Widgets))
	}

	// Hero literal at index 0.
	if formation.Widgets[0].EntityRef != nil {
		t.Error("hero literal should not have EntityRef")
	}
	heroHasText := false
	for _, a := range formation.Widgets[0].Atoms {
		if a.Value == "New collection" {
			heroHasText = true
			break
		}
	}
	if !heroHasText {
		t.Error("hero literal text was not preserved")
	}

	// Clones at 1..4 with EntityRef + bound name.
	for i := 1; i <= 4; i++ {
		w := formation.Widgets[i]
		if w.EntityRef == nil {
			t.Errorf("clone[%d] missing EntityRef", i)
		}
		if name := atomValue(w, "name"); name == nil {
			t.Errorf("clone[%d] name not bound", i)
		}
	}

	// CTA literal at index 5.
	cta := formation.Widgets[5]
	if cta.EntityRef != nil {
		t.Error("cta literal should not have EntityRef")
	}
	ctaHasText := false
	for _, a := range cta.Atoms {
		if a.Value == "Buy now" {
			ctaHasText = true
			break
		}
	}
	if !ctaHasText {
		t.Error("cta literal text was not preserved")
	}
}

func containsString(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// ─── Phase 3 — auto-sections post-process ───

// TestGroupIntoSections_SingleGroup_Flat — 6 cards from one replicate group
// produce a single grid section, which then rolls back to flat formation for
// backwards compat with legacy single-widget flows ("show me creams").
func TestGroupIntoSections_SingleGroup_Flat(t *testing.T) {
	formation := &domain.FormationWithData{
		Widgets: []domain.Widget{makeReplicateTemplate()},
	}
	expandReplicatedWidgets(formation, sampleProductData(6), "product")
	if got := len(formation.Widgets); got != 6 {
		t.Fatalf("setup: expected 6 widgets after expand, got %d", got)
	}

	groupIntoSections(formation)

	if len(formation.Sections) != 0 {
		t.Errorf("expected sections cleared (single-section rollback), got %d", len(formation.Sections))
	}
	if len(formation.Widgets) != 6 {
		t.Errorf("expected 6 flat widgets after rollback, got %d", len(formation.Widgets))
	}
	if formation.Mode != domain.FormationTypeGrid {
		t.Errorf("expected mode=grid, got %s", formation.Mode)
	}
	if formation.Grid == nil || formation.Grid.Cols != 3 {
		t.Errorf("expected grid cols=3 (GridColumnsForCount(6)), got %+v", formation.Grid)
	}
}

// TestGroupIntoSections_LiteralsOnly_Composed — 3 literal widgets each become
// their own single-mode section. Result is composed (not flat) because there
// are multiple sections.
func TestGroupIntoSections_LiteralsOnly_Composed(t *testing.T) {
	formation := &domain.FormationWithData{
		Widgets: []domain.Widget{
			makeLiteralWidget("Hero"),
			makeLiteralWidget("Explainer"),
			makeLiteralWidget("CTA"),
		},
	}

	groupIntoSections(formation)

	if formation.Mode != domain.FormationTypeComposed {
		t.Errorf("expected mode=composed, got %s", formation.Mode)
	}
	if len(formation.Widgets) != 0 {
		t.Errorf("expected formation.Widgets cleared, got %d", len(formation.Widgets))
	}
	if len(formation.Sections) != 3 {
		t.Fatalf("expected 3 single-mode sections, got %d", len(formation.Sections))
	}
	for i, sec := range formation.Sections {
		if sec.Mode != domain.FormationTypeSingle {
			t.Errorf("section[%d] mode=%s, expected single", i, sec.Mode)
		}
		if len(sec.Widgets) != 1 {
			t.Errorf("section[%d] widget count=%d, expected 1", i, len(sec.Widgets))
		}
	}
}

// TestGroupIntoSections_MixedComposition — hero + gallery (3 replicated cards)
// + cta produces 3 sections in order: single/grid/single. This is the canonical
// "presentation" use case.
func TestGroupIntoSections_MixedComposition(t *testing.T) {
	formation := &domain.FormationWithData{
		Widgets: []domain.Widget{
			makeLiteralWidget("Hero"),
			makeReplicateTemplate(),
			makeLiteralWidget("Buy now"),
		},
	}
	expandReplicatedWidgets(formation, sampleProductData(3), "product")
	if got := len(formation.Widgets); got != 5 {
		t.Fatalf("setup: expected 5 widgets after expand (1 hero + 3 clones + 1 cta), got %d", got)
	}

	groupIntoSections(formation)

	if formation.Mode != domain.FormationTypeComposed {
		t.Errorf("expected mode=composed, got %s", formation.Mode)
	}
	if len(formation.Widgets) != 0 {
		t.Errorf("expected formation.Widgets cleared, got %d", len(formation.Widgets))
	}
	if len(formation.Sections) != 3 {
		t.Fatalf("expected 3 sections (hero/gallery/cta), got %d", len(formation.Sections))
	}

	// Section 0: hero literal
	if formation.Sections[0].Mode != domain.FormationTypeSingle {
		t.Errorf("section[0] mode=%s, expected single (hero)", formation.Sections[0].Mode)
	}
	if got := len(formation.Sections[0].Widgets); got != 1 {
		t.Errorf("section[0] widget count=%d, expected 1 (hero)", got)
	}

	// Section 1: gallery — 3 clones, grid, cols=2 (GridColumnsForCount(3))
	if formation.Sections[1].Mode != domain.FormationTypeGrid {
		t.Errorf("section[1] mode=%s, expected grid (gallery)", formation.Sections[1].Mode)
	}
	if got := len(formation.Sections[1].Widgets); got != 3 {
		t.Errorf("section[1] widget count=%d, expected 3 (gallery)", got)
	}
	if formation.Sections[1].Grid == nil || formation.Sections[1].Grid.Cols != 2 {
		t.Errorf("section[1] expected grid cols=2 (GridColumnsForCount(3)), got %+v", formation.Sections[1].Grid)
	}

	// Section 2: cta literal
	if formation.Sections[2].Mode != domain.FormationTypeSingle {
		t.Errorf("section[2] mode=%s, expected single (cta)", formation.Sections[2].Mode)
	}
	if got := len(formation.Sections[2].Widgets); got != 1 {
		t.Errorf("section[2] widget count=%d, expected 1 (cta)", got)
	}
}

// TestGroupIntoSections_OrderPreserved — section order matches widget order;
// gallery sandwiched between two literals stays in the middle.
func TestGroupIntoSections_OrderPreserved(t *testing.T) {
	formation := &domain.FormationWithData{
		Widgets: []domain.Widget{
			makeLiteralWidget("first"),
			makeReplicateTemplate(),
			makeLiteralWidget("middle"),
		},
	}
	expandReplicatedWidgets(formation, sampleProductData(2), "product")
	groupIntoSections(formation)

	if len(formation.Sections) != 3 {
		t.Fatalf("expected 3 sections, got %d", len(formation.Sections))
	}

	first := formation.Sections[0].Widgets[0]
	if len(first.Atoms) == 0 || first.Atoms[0].Value != "first" {
		t.Errorf("section[0] should hold 'first' literal, got value=%v", literalValue(first))
	}

	if formation.Sections[1].Mode != domain.FormationTypeGrid {
		t.Errorf("section[1] should be grid (gallery), got %s", formation.Sections[1].Mode)
	}

	middle := formation.Sections[2].Widgets[0]
	if len(middle.Atoms) == 0 || middle.Atoms[0].Value != "middle" {
		t.Errorf("section[2] should hold 'middle' literal, got value=%v", literalValue(middle))
	}
}

// TestGroupIntoSections_SingleEntityDetail_Flat — 1 entity-bound widget that
// went through autoDetectEntityRefs (no GroupID) is treated as a literal,
// becomes one single-section, then rolls back to flat with mode=single.
// This is the legacy "show me detail" flow.
func TestGroupIntoSections_SingleEntityDetail_Flat(t *testing.T) {
	formation := &domain.FormationWithData{
		Widgets: []domain.Widget{makeEntityWidget()},
	}
	autoDetectEntityRefs(formation, sampleProductData(2), "product")

	groupIntoSections(formation)

	if len(formation.Sections) != 0 {
		t.Errorf("expected sections cleared (single-section rollback), got %d", len(formation.Sections))
	}
	if len(formation.Widgets) != 1 {
		t.Errorf("expected 1 flat widget after rollback, got %d", len(formation.Widgets))
	}
	if formation.Mode != domain.FormationTypeSingle {
		t.Errorf("expected mode=single, got %s", formation.Mode)
	}
	if formation.Widgets[0].EntityRef == nil {
		t.Error("expected EntityRef preserved across grouping")
	}
}

// TestStampTreeIDs_Composed — after groupIntoSections produces a composed
// formation, StampTreeIDs walks formation.Sections and assigns stable IDs
// in the w-s{N}-w{M} format. Verifies the existing stampWidgetIDs handles
// the composed path correctly.
func TestStampTreeIDs_Composed(t *testing.T) {
	formation := &domain.FormationWithData{
		Widgets: []domain.Widget{
			makeLiteralWidget("Hero"),
			makeReplicateTemplate(),
			makeLiteralWidget("CTA"),
		},
	}
	expandReplicatedWidgets(formation, sampleProductData(3), "product")
	groupIntoSections(formation)
	StampTreeIDs(formation)

	if formation.Mode != domain.FormationTypeComposed {
		t.Fatalf("setup: expected composed, got %s", formation.Mode)
	}
	if len(formation.Sections) != 3 {
		t.Fatalf("setup: expected 3 sections, got %d", len(formation.Sections))
	}

	// Section 0: hero literal — w-s0-w0
	if id := formation.Sections[0].Widgets[0].ID; id != "w-s0-w0" {
		t.Errorf("section[0] widget[0] id=%q, expected w-s0-w0", id)
	}

	// Section 1: 3 gallery clones — w-s1-w0, w-s1-w1, w-s1-w2
	for i := 0; i < 3; i++ {
		expected := fmt.Sprintf("w-s1-w%d", i)
		if id := formation.Sections[1].Widgets[i].ID; id != expected {
			t.Errorf("section[1] widget[%d] id=%q, expected %s", i, id, expected)
		}
	}

	// Section 2: cta literal — w-s2-w0
	if id := formation.Sections[2].Widgets[0].ID; id != "w-s2-w0" {
		t.Errorf("section[2] widget[0] id=%q, expected w-s2-w0", id)
	}
}

// TestEngineExecute_LegacyReplicateGridStaysFlat — sanity that the full
// engine pipeline preserves backwards compat for the legacy "show me creams"
// case. After Phase 3 grouping the result must still look like flat
// formation.Widgets with mode=grid.
func TestEngineExecute_LegacyReplicateGridStaysFlat(t *testing.T) {
	out := NewEngine().Execute(ExecuteInput{
		Ops:        ProductCardGridOps(),
		Data:       sampleProducts(5),
		EntityType: "product",
		Layout:     "grid",
		Columns:    3,
		Replicate:  true,
	})

	f := out.Formation
	if len(f.Sections) != 0 {
		t.Errorf("expected no sections (rollback), got %d", len(f.Sections))
	}
	if len(f.Widgets) != 5 {
		t.Errorf("expected 5 flat widgets, got %d", len(f.Widgets))
	}
	if f.Mode != domain.FormationTypeGrid {
		t.Errorf("expected mode=grid, got %s", f.Mode)
	}
	if f.Grid == nil || f.Grid.Cols != 3 {
		t.Errorf("expected grid cols=3 (preserved from input.Columns), got %+v", f.Grid)
	}
}

// literalValue returns the value of the first non-FieldName atom for a widget.
// Used in tests where we need to inspect literal text content.
func literalValue(w domain.Widget) interface{} {
	for _, a := range w.Atoms {
		if a.FieldName == "" {
			return a.Value
		}
	}
	return nil
}

// ─── Phase 4 — BuildTreeMap multi-widget schema ───

// TestBuildTreeMap_SingleGrid_Replicated — legacy "show me creams" path:
// 6 cards from one preset → tree map has one replicated entry with count=6.
func TestBuildTreeMap_SingleGrid_Replicated(t *testing.T) {
	out := NewEngine().Execute(ExecuteInput{
		Ops:        ProductCardGridOps(),
		Data:       sampleProducts(6),
		EntityType: "product",
		Layout:     "grid",
		Columns:    3,
		Replicate:  true,
	})

	tree := out.TreeMap
	if tree == nil {
		t.Fatal("expected tree map, got nil")
	}
	widgets, ok := tree["widgets"].([]map[string]interface{})
	if !ok {
		t.Fatalf("expected widgets array, got %T", tree["widgets"])
	}
	if len(widgets) != 1 {
		t.Fatalf("expected 1 entry (single replicate group), got %d", len(widgets))
	}
	if kind, _ := widgets[0]["kind"].(string); kind != "replicated" {
		t.Errorf("entry kind=%v, expected replicated", widgets[0]["kind"])
	}
	if count, _ := widgets[0]["count"].(int); count != 6 {
		t.Errorf("entry count=%v, expected 6", widgets[0]["count"])
	}
	ids, _ := widgets[0]["ids"].([]string)
	if len(ids) != 6 {
		t.Errorf("entry ids length=%d, expected 6", len(ids))
	}
	if widgets[0]["template"] == nil {
		t.Error("entry missing template")
	}
	if total, _ := tree["widget_count"].(int); total != 6 {
		t.Errorf("widget_count=%v, expected 6", tree["widget_count"])
	}
	if mode, _ := tree["mode"].(string); mode != "grid" {
		t.Errorf("mode=%v, expected grid", tree["mode"])
	}
}

// TestBuildTreeMap_SingleDetail_Literal — legacy "show me detail" path:
// 1 entity widget without replication → one literal entry.
func TestBuildTreeMap_SingleDetail_Literal(t *testing.T) {
	out := NewEngine().Execute(ExecuteInput{
		Ops:        ProductDetailOps(),
		Data:       sampleProducts(1),
		EntityType: "product",
	})

	tree := out.TreeMap
	if tree == nil {
		t.Fatal("expected tree map, got nil")
	}
	widgets, ok := tree["widgets"].([]map[string]interface{})
	if !ok {
		t.Fatalf("expected widgets array, got %T", tree["widgets"])
	}
	if len(widgets) != 1 {
		t.Fatalf("expected 1 literal entry, got %d", len(widgets))
	}
	if kind, _ := widgets[0]["kind"].(string); kind != "literal" {
		t.Errorf("entry kind=%v, expected literal", widgets[0]["kind"])
	}
	if widgets[0]["id"] == nil {
		t.Error("literal entry missing id")
	}
	if widgets[0]["atoms"] == nil {
		t.Error("literal entry missing atoms")
	}
	if total, _ := tree["widget_count"].(int); total != 1 {
		t.Errorf("widget_count=%v, expected 1", tree["widget_count"])
	}
}

// TestBuildTreeMap_MultiWidgetComposition — full composition: hero + gallery×3
// + cta produces 3 entries (literal/replicated/literal) in order.
func TestBuildTreeMap_MultiWidgetComposition(t *testing.T) {
	formation := &domain.FormationWithData{
		Widgets: []domain.Widget{
			makeLiteralWidget("Hero"),
			makeReplicateTemplate(),
			makeLiteralWidget("Buy now"),
		},
	}
	expandReplicatedWidgets(formation, sampleProductData(3), "product")
	groupIntoSections(formation)
	StampTreeIDs(formation)
	tree := BuildTreeMap(formation)

	if tree == nil {
		t.Fatal("expected tree map, got nil")
	}
	widgets, ok := tree["widgets"].([]map[string]interface{})
	if !ok {
		t.Fatalf("expected widgets array, got %T", tree["widgets"])
	}
	if len(widgets) != 3 {
		t.Fatalf("expected 3 entries (hero/gallery/cta), got %d", len(widgets))
	}

	// Entry 0: hero literal
	if kind, _ := widgets[0]["kind"].(string); kind != "literal" {
		t.Errorf("entry[0] kind=%v, expected literal (hero)", widgets[0]["kind"])
	}
	if id, _ := widgets[0]["id"].(string); id != "w-s0-w0" {
		t.Errorf("entry[0] id=%v, expected w-s0-w0", widgets[0]["id"])
	}

	// Entry 1: gallery replicated
	if kind, _ := widgets[1]["kind"].(string); kind != "replicated" {
		t.Errorf("entry[1] kind=%v, expected replicated (gallery)", widgets[1]["kind"])
	}
	if count, _ := widgets[1]["count"].(int); count != 3 {
		t.Errorf("entry[1] count=%v, expected 3", widgets[1]["count"])
	}
	ids, _ := widgets[1]["ids"].([]string)
	if len(ids) != 3 || ids[0] != "w-s1-w0" || ids[1] != "w-s1-w1" || ids[2] != "w-s1-w2" {
		t.Errorf("entry[1] ids=%v, expected [w-s1-w0, w-s1-w1, w-s1-w2]", ids)
	}

	// Entry 2: cta literal
	if kind, _ := widgets[2]["kind"].(string); kind != "literal" {
		t.Errorf("entry[2] kind=%v, expected literal (cta)", widgets[2]["kind"])
	}
	if id, _ := widgets[2]["id"].(string); id != "w-s2-w0" {
		t.Errorf("entry[2] id=%v, expected w-s2-w0", widgets[2]["id"])
	}

	if total, _ := tree["widget_count"].(int); total != 5 {
		t.Errorf("widget_count=%v, expected 5 (1 hero + 3 clones + 1 cta)", tree["widget_count"])
	}
	if mode, _ := tree["mode"].(string); mode != "composed" {
		t.Errorf("mode=%v, expected composed", tree["mode"])
	}
}

// TestBuildTreeMap_EmptyFormation — empty formation returns nil.
func TestBuildTreeMap_EmptyFormation(t *testing.T) {
	if tree := BuildTreeMap(nil); tree != nil {
		t.Errorf("expected nil for nil formation, got %v", tree)
	}
	if tree := BuildTreeMap(&domain.FormationWithData{}); tree != nil {
		t.Errorf("expected nil for empty formation, got %v", tree)
	}
}
