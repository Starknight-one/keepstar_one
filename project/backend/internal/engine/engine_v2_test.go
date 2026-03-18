package engine

import (
	"testing"

	"keepstar/internal/domain"
)

func TestEngineV2_EmptyInput(t *testing.T) {
	e := NewEngineV2()
	out := e.Execute(EngineV2Input{
		EntityType: domain.EntityTypeProduct,
	})
	if out.Formation == nil {
		t.Fatal("expected non-nil formation")
	}
	if len(out.Formation.Widgets) != 0 {
		t.Errorf("expected 0 widgets, got %d", len(out.Formation.Widgets))
	}
}

func TestEngineV2_SingleProduct(t *testing.T) {
	e := NewEngineV2()
	out := e.Execute(EngineV2Input{
		EntityType: domain.EntityTypeProduct,
		Products: []domain.Product{
			{
				ID:    "p1",
				Name:  "Test Product",
				Price: 2999,
				Brand: "TestBrand",
				Images: []string{"https://example.com/img.jpg"},
				Rating: 4.5,
			},
		},
	})

	if out.Formation == nil {
		t.Fatal("expected non-nil formation")
	}
	if len(out.Formation.Widgets) != 1 {
		t.Fatalf("expected 1 widget, got %d", len(out.Formation.Widgets))
	}

	w := out.Formation.Widgets[0]

	// Should have v2 atoms
	if len(w.AtomsV2) == 0 {
		t.Error("expected v2 atoms to be populated")
	}

	// Should have v1 compat atoms
	if len(w.Atoms) == 0 {
		t.Error("expected v1 compat atoms to be populated")
	}

	// Should have layout tree
	if w.Layout == nil {
		t.Error("expected layout tree to be populated")
	}

	// Should have v1 zones
	if len(w.Zones) == 0 {
		t.Error("expected v1 compat zones to be populated")
	}

	// Entity ref
	if w.EntityRef == nil || w.EntityRef.ID != "p1" {
		t.Error("expected entity ref with ID p1")
	}

	// Single product → single layout
	if out.Formation.Mode != domain.FormationTypeSingle {
		t.Errorf("expected single mode for 1 product, got %s", out.Formation.Mode)
	}
}

func TestEngineV2_MultipleProducts_GridLayout(t *testing.T) {
	e := NewEngineV2()
	products := make([]domain.Product, 4)
	for i := range products {
		products[i] = domain.Product{
			ID:    "p" + string(rune('1'+i)),
			Name:  "Product " + string(rune('A'+i)),
			Price: 1000 * (i + 1),
			Images: []string{"https://example.com/img.jpg"},
		}
	}

	out := e.Execute(EngineV2Input{
		EntityType: domain.EntityTypeProduct,
		Products:   products,
	})

	if len(out.Formation.Widgets) != 4 {
		t.Fatalf("expected 4 widgets, got %d", len(out.Formation.Widgets))
	}

	if out.Formation.Mode != domain.FormationTypeGrid {
		t.Errorf("expected grid mode for 4 products, got %s", out.Formation.Mode)
	}

	if out.Formation.Grid == nil {
		t.Error("expected grid config to be set")
	}
}

func TestEngineV2_WithFieldDefinitions(t *testing.T) {
	e := NewEngineV2()
	defs := []FieldDefinitionEntry{
		{FieldName: "images", AtomType: domain.AtomTypeImage, AtomSubtype: domain.SubtypeImageURL, DefaultDisplay: "image-cover", DefaultSlot: domain.AtomSlotHero, Priority: 0},
		{FieldName: "name", AtomType: domain.AtomTypeText, AtomSubtype: domain.SubtypeString, DefaultDisplay: "h2", DefaultSlot: domain.AtomSlotTitle, Label: "Name", Priority: 1},
		{FieldName: "price", AtomType: domain.AtomTypeNumber, AtomSubtype: domain.SubtypeCurrency, DefaultDisplay: "price", DefaultSlot: domain.AtomSlotPrice, Label: "Price", Priority: 2},
	}

	out := e.Execute(EngineV2Input{
		EntityType: domain.EntityTypeProduct,
		FieldDefs:  defs,
		Products: []domain.Product{
			{
				ID:       "p1",
				Name:     "Widget Pro",
				Price:    4999,
				Currency: "USD",
				Images:   []string{"https://example.com/pro.jpg"},
			},
		},
	})

	if len(out.Formation.Widgets) != 1 {
		t.Fatalf("expected 1 widget, got %d", len(out.Formation.Widgets))
	}

	w := out.Formation.Widgets[0]
	// Should have exactly 3 atoms (images, name, price) from field definitions
	if len(w.AtomsV2) != 3 {
		t.Errorf("expected 3 v2 atoms, got %d", len(w.AtomsV2))
	}

	// Check label is populated
	for _, a := range w.AtomsV2 {
		if a.FieldName == "name" && a.Label != "Name" {
			t.Errorf("expected label 'Name', got %q", a.Label)
		}
	}
}

func TestEngineV2_WithInstructions_ShowHide(t *testing.T) {
	e := NewEngineV2()
	out := e.Execute(EngineV2Input{
		EntityType: domain.EntityTypeProduct,
		Instructions: &AgentInstructions{
			Show: []string{"name", "price"},
			Hide: []string{"images", "rating", "brand", "category", "description", "tags", "stockQuantity", "productForm", "skinType", "concern", "keyIngredients"},
		},
		Products: []domain.Product{
			{ID: "p1", Name: "Test", Price: 100, Images: []string{"https://x.com/a.jpg"}},
		},
	})

	w := out.Formation.Widgets[0]
	// Only name and price should remain
	for _, a := range w.AtomsV2 {
		if a.FieldName != "name" && a.FieldName != "price" {
			t.Errorf("unexpected field %q in atoms", a.FieldName)
		}
	}
}

func TestEngineV2_V1Compat_AtomsAndZones(t *testing.T) {
	e := NewEngineV2()
	out := e.Execute(EngineV2Input{
		EntityType: domain.EntityTypeProduct,
		Products: []domain.Product{
			{
				ID:    "p1",
				Name:  "Test",
				Price: 100,
				Images: []string{"https://example.com/img.jpg"},
			},
		},
	})

	w := out.Formation.Widgets[0]

	// V1 atoms should be populated from AtomsV2
	if len(w.Atoms) == 0 {
		t.Fatal("v1 atoms should be populated for backward compat")
	}

	// Each v1 atom should have a non-empty display
	for _, a := range w.Atoms {
		if a.Display == "" {
			t.Errorf("v1 atom %q has empty display", a.FieldName)
		}
	}

	// V1 zones should be populated from Layout tree
	if len(w.Zones) == 0 {
		t.Fatal("v1 zones should be populated for backward compat")
	}
}

func TestAutoLayout_GroupsByType(t *testing.T) {
	atoms := []domain.AtomV2{
		{Type: domain.AtomTypeImage, FieldName: "images"},
		{Type: domain.AtomTypeText, FieldName: "name", TextStyle: &domain.TextStyle{FontSize: "2xl", FontWeight: "semibold"}},
		{Type: domain.AtomTypeNumber, Subtype: domain.SubtypeCurrency, FieldName: "price", Slot: domain.AtomSlotPrice},
		{Type: domain.AtomTypeNumber, Subtype: domain.SubtypeRating, FieldName: "rating"},
		{Type: domain.AtomTypeText, FieldName: "brand", Wrapper: &domain.WrapperConfig{Type: "tag"}},
		{Type: domain.AtomTypeText, FieldName: "description", TextStyle: &domain.TextStyle{FontSize: "sm"}},
	}

	layout := AutoLayout(atoms)
	if layout == nil {
		t.Fatal("expected non-nil layout")
	}
	if layout.Type != domain.LayoutNodeColumn {
		t.Errorf("expected root to be column, got %s", layout.Type)
	}

	// Should have groups: hero, headings, price-rating, body, tags
	if len(layout.Children) < 4 {
		t.Errorf("expected at least 4 groups, got %d", len(layout.Children))
	}
}

func TestAtomV2Constraints_BadgeOverflow(t *testing.T) {
	atoms := []domain.AtomV2{
		{
			Type:     domain.AtomTypeText,
			Value:    "This is a very long badge text that exceeds twenty characters",
			Wrapper:  &domain.WrapperConfig{Type: "badge"},
			Rigidity: domain.RigidityFlexible,
		},
	}

	result := applyAtomV2Constraints(atoms)
	// Should have been downgraded to tag
	if result[0].Wrapper != nil && result[0].Wrapper.Type == "badge" {
		t.Error("expected badge to be downgraded for long text")
	}
}

func TestAtomV2Constraints_LockedNotTouched(t *testing.T) {
	atoms := []domain.AtomV2{
		{
			Type:     domain.AtomTypeText,
			Value:    "This is a very long badge text that exceeds twenty characters",
			Wrapper:  &domain.WrapperConfig{Type: "badge"},
			Rigidity: domain.RigidityLocked,
		},
	}

	result := applyAtomV2Constraints(atoms)
	// Locked atom should not be modified
	if result[0].Wrapper == nil || result[0].Wrapper.Type != "badge" {
		t.Error("locked atom should not be modified")
	}
}

func TestProductToMap_BasicFields(t *testing.T) {
	p := domain.Product{
		ID:    "p1",
		Name:  "Test",
		Price: 2999,
		Brand: "TestBrand",
	}

	m := ProductToMap(p)
	if m["name"] != "Test" {
		t.Errorf("expected name=Test, got %v", m["name"])
	}
	if m["price"] != 2999 {
		t.Errorf("expected price=2999, got %v", m["price"])
	}
	if m["brand"] != "TestBrand" {
		t.Errorf("expected brand=TestBrand, got %v", m["brand"])
	}
}

func TestProductToMap_EmptyFieldsOmitted(t *testing.T) {
	p := domain.Product{
		ID:   "p1",
		Name: "Test",
	}

	m := ProductToMap(p)
	if _, exists := m["brand"]; exists {
		t.Error("empty brand should not be in map")
	}
	if _, exists := m["price"]; exists {
		t.Error("zero price should not be in map")
	}
}

func TestGenericFieldGetter(t *testing.T) {
	data := map[string]interface{}{
		"name":  "Test Product",
		"price": 2999,
	}

	getter := GenericFieldGetter(data)
	if getter("name") != "Test Product" {
		t.Error("expected name from getter")
	}
	if getter("nonexistent") != nil {
		t.Error("expected nil for unknown field")
	}
}
