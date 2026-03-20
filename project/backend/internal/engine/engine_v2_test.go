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

func TestEngineV2_WithPreset(t *testing.T) {
	e := NewEngineV2()
	preset := &domain.PresetV2{
		Name:        "test_preset",
		EntityType:  domain.EntityTypeProduct,
		Template:    "GenericCard",
		DefaultMode: domain.FormationTypeGrid,
		DefaultSize: domain.WidgetSizeMedium,
		Fields: []domain.PresetV2Field{
			{FieldName: "name", TextStyle: &domain.TextStyle{FontSize: "xl", FontWeight: "bold", LetterSpacing: "tight"}, Slot: domain.AtomSlotTitle, Priority: 1},
			{FieldName: "price", Format: domain.FormatCurrency, TextStyle: &domain.TextStyle{FontSize: "lg", FontWeight: "bold"}, Slot: domain.AtomSlotPrice, Priority: 2},
		},
	}

	out := e.Execute(EngineV2Input{
		EntityType: domain.EntityTypeProduct,
		Preset:     preset,
		Products: []domain.Product{
			{ID: "p1", Name: "Test", Price: 2999, Images: []string{"https://example.com/img.jpg"}},
		},
	})

	w := out.Formation.Widgets[0]
	// Find name atom — should have preset's fontSize "xl"
	for _, a := range w.AtomsV2 {
		if a.FieldName == "name" {
			if a.TextStyle == nil {
				t.Fatal("name atom should have textStyle from preset")
			}
			if a.TextStyle.FontSize != "xl" {
				t.Errorf("expected name fontSize=xl from preset, got %q", a.TextStyle.FontSize)
			}
			if a.TextStyle.LetterSpacing != "tight" {
				t.Errorf("expected name letterSpacing=tight from preset, got %q", a.TextStyle.LetterSpacing)
			}
		}
		if a.FieldName == "price" {
			if a.Format != domain.FormatCurrency {
				t.Errorf("expected price format=currency from preset, got %q", a.Format)
			}
		}
	}
}

func TestEngineV2_AtomOverrides(t *testing.T) {
	e := NewEngineV2()
	out := e.Execute(EngineV2Input{
		EntityType: domain.EntityTypeProduct,
		Instructions: &AgentInstructions{
			Atoms: map[string]AtomOverride{
				"price": {Color: "red", Format: "currency"},
				"name":  {TextStyle: &domain.TextStyle{FontSize: "3xl"}},
			},
		},
		Products: []domain.Product{
			{ID: "p1", Name: "Test", Price: 100, Images: []string{"https://example.com/img.jpg"}},
		},
	})

	w := out.Formation.Widgets[0]
	for _, a := range w.AtomsV2 {
		if a.FieldName == "price" {
			if a.TextStyle == nil || a.TextStyle.Color != "red" {
				t.Errorf("expected price color=red from override, got %v", a.TextStyle)
			}
			if a.Format != "currency" {
				t.Errorf("expected price format=currency from override, got %q", a.Format)
			}
		}
		if a.FieldName == "name" {
			if a.TextStyle == nil || a.TextStyle.FontSize != "3xl" {
				t.Errorf("expected name fontSize=3xl from override, got %v", a.TextStyle)
			}
		}
	}
}

func TestEngineV2_MediaStyleDefaults(t *testing.T) {
	e := NewEngineV2()
	out := e.Execute(EngineV2Input{
		EntityType: domain.EntityTypeProduct,
		Products: []domain.Product{
			{ID: "p1", Name: "Test", Price: 100, Images: []string{"https://example.com/img.jpg"}},
		},
	})

	w := out.Formation.Widgets[0]
	for _, a := range w.AtomsV2 {
		if a.Type == domain.AtomTypeImage {
			if a.MediaStyle == nil {
				t.Fatal("image atom should have default mediaStyle")
			}
			if a.MediaStyle.ObjectFit != "cover" {
				t.Errorf("expected objectFit=cover, got %q", a.MediaStyle.ObjectFit)
			}
			if a.MediaStyle.AspectRatio == "" {
				t.Error("expected non-empty aspectRatio")
			}
		}
	}
}

func TestEngineV2_PresetThenOverride(t *testing.T) {
	e := NewEngineV2()
	preset := &domain.PresetV2{
		Name:       "test",
		EntityType: domain.EntityTypeProduct,
		Fields: []domain.PresetV2Field{
			{FieldName: "name", TextStyle: &domain.TextStyle{FontSize: "xl", FontWeight: "semibold"}},
			{FieldName: "price", TextStyle: &domain.TextStyle{FontSize: "lg", FontWeight: "bold"}},
		},
	}

	out := e.Execute(EngineV2Input{
		EntityType: domain.EntityTypeProduct,
		Preset:     preset,
		Instructions: &AgentInstructions{
			Atoms: map[string]AtomOverride{
				"name": {TextStyle: &domain.TextStyle{FontSize: "3xl"}}, // Override preset's xl → 3xl
			},
		},
		Products: []domain.Product{
			{ID: "p1", Name: "Test", Price: 100},
		},
	})

	w := out.Formation.Widgets[0]
	for _, a := range w.AtomsV2 {
		if a.FieldName == "name" {
			if a.TextStyle == nil {
				t.Fatal("expected textStyle on name")
			}
			// fontSize should be overridden to 3xl (agent > preset)
			if a.TextStyle.FontSize != "3xl" {
				t.Errorf("expected fontSize=3xl (override wins), got %q", a.TextStyle.FontSize)
			}
			// fontWeight should remain semibold from preset (not wiped by override)
			if a.TextStyle.FontWeight != "semibold" {
				t.Errorf("expected fontWeight=semibold (preset preserved), got %q", a.TextStyle.FontWeight)
			}
		}
	}
}

func TestAutoSelectPreset(t *testing.T) {
	tests := []struct {
		entityType domain.EntityType
		layout     string
		size       domain.WidgetSize
		expected   string
	}{
		{domain.EntityTypeProduct, "grid", domain.WidgetSizeMedium, "product_card_grid"},
		{domain.EntityTypeProduct, "single", domain.WidgetSizeLarge, "product_card_detail"},
		{domain.EntityTypeProduct, "list", domain.WidgetSizeSmall, "product_row"},
		{domain.EntityTypeService, "grid", domain.WidgetSizeMedium, "service_card"},
		{domain.EntityTypeService, "single", domain.WidgetSizeLarge, "service_detail"},
	}

	for _, tt := range tests {
		got := AutoSelectPreset(tt.entityType, tt.layout, tt.size)
		if got != tt.expected {
			t.Errorf("AutoSelectPreset(%s, %s, %s) = %q, want %q", tt.entityType, tt.layout, tt.size, got, tt.expected)
		}
	}
}

// ============================================================================
// Phase 6: New tests for V2 engine completion
// ============================================================================

func TestEngineV2_WidgetContainerOverrides(t *testing.T) {
	e := NewEngineV2()
	out := e.Execute(EngineV2Input{
		EntityType: domain.EntityTypeProduct,
		Instructions: &AgentInstructions{
			WidgetBackground:   "#F9FAFB",
			WidgetBorderRadius: "lg",
			WidgetShadow:       "md",
			WidgetPadding:      "lg",
			WidgetBorder:       "1px solid #E5E7EB",
			Gap:                "lg",
		},
		Products: []domain.Product{
			{ID: "p1", Name: "Test", Price: 100, Images: []string{"https://example.com/img.jpg"}},
		},
	})

	w := out.Formation.Widgets[0]
	if w.Layout == nil {
		t.Fatal("expected layout")
	}
	if w.Layout.Background != "#F9FAFB" {
		t.Errorf("expected background=#F9FAFB, got %q", w.Layout.Background)
	}
	if w.Layout.BorderRadius != "lg" {
		t.Errorf("expected borderRadius=lg, got %q", w.Layout.BorderRadius)
	}
	if w.Layout.Shadow != "md" {
		t.Errorf("expected shadow=md, got %q", w.Layout.Shadow)
	}
	if w.Layout.Padding != "lg" {
		t.Errorf("expected padding=lg, got %q", w.Layout.Padding)
	}
	if w.Layout.Border != "1px solid #E5E7EB" {
		t.Errorf("expected border='1px solid #E5E7EB', got %q", w.Layout.Border)
	}
	if w.Layout.Gap != "lg" {
		t.Errorf("expected gap=lg, got %q", w.Layout.Gap)
	}
}

func TestEngineV2_ColumnsOverride(t *testing.T) {
	e := NewEngineV2()
	products := make([]domain.Product, 6)
	for i := range products {
		products[i] = domain.Product{
			ID:     "p" + string(rune('1'+i)),
			Name:   "Product",
			Price:  100 * (i + 1),
			Images: []string{"https://example.com/img.jpg"},
		}
	}

	out := e.Execute(EngineV2Input{
		EntityType:   domain.EntityTypeProduct,
		Products:     products,
		Instructions: &AgentInstructions{Columns: 3},
	})

	if out.Formation.Grid == nil {
		t.Fatal("expected grid config")
	}
	if out.Formation.Grid.Cols != 3 {
		t.Errorf("expected 3 columns, got %d", out.Formation.Grid.Cols)
	}
}

func TestEngineV2_IconStyleOverride(t *testing.T) {
	e := NewEngineV2()
	out := e.Execute(EngineV2Input{
		EntityType: domain.EntityTypeProduct,
		Instructions: &AgentInstructions{
			Show: []string{"name", "price"},
			Atoms: map[string]AtomOverride{
				"name": {IconStyle: &domain.IconStyle{Size: "xl", Color: "blue"}},
			},
		},
		Products: []domain.Product{
			{ID: "p1", Name: "Test", Price: 100},
		},
	})

	w := out.Formation.Widgets[0]
	for _, a := range w.AtomsV2 {
		if a.FieldName == "name" && a.IconStyle != nil {
			if a.IconStyle.Size != "xl" {
				t.Errorf("expected iconStyle.size=xl, got %q", a.IconStyle.Size)
			}
			if a.IconStyle.Color != "blue" {
				t.Errorf("expected iconStyle.color=blue, got %q", a.IconStyle.Color)
			}
		}
	}
}

func TestEngineV2_MediaStyleOverrides(t *testing.T) {
	e := NewEngineV2()
	out := e.Execute(EngineV2Input{
		EntityType: domain.EntityTypeProduct,
		Instructions: &AgentInstructions{
			Atoms: map[string]AtomOverride{
				"images": {MediaStyle: &domain.MediaStyle{AspectRatio: "16:9", ObjectFit: "contain"}},
			},
		},
		Products: []domain.Product{
			{ID: "p1", Name: "Test", Price: 100, Images: []string{"https://example.com/img.jpg"}},
		},
	})

	w := out.Formation.Widgets[0]
	for _, a := range w.AtomsV2 {
		if a.Type == domain.AtomTypeImage {
			if a.MediaStyle == nil {
				t.Fatal("expected mediaStyle on image atom")
			}
			if a.MediaStyle.AspectRatio != "16:9" {
				t.Errorf("expected aspectRatio=16:9, got %q", a.MediaStyle.AspectRatio)
			}
			if a.MediaStyle.ObjectFit != "contain" {
				t.Errorf("expected objectFit=contain, got %q", a.MediaStyle.ObjectFit)
			}
		}
	}
}

func TestShrinkToFit(t *testing.T) {
	node := &domain.LayoutNode{
		Type:    domain.LayoutNodeColumn,
		Gap:     "lg",
		Padding: "xl",
		Children: []domain.LayoutChild{
			{Node: &domain.LayoutNode{Type: domain.LayoutNodeRow, Gap: "md", Children: []domain.LayoutChild{{AtomIndex: intPtr(0)}}}},
		},
	}
	shrinkToFit(node, 200)
	if node.Gap != "md" {
		t.Errorf("expected gap=md after shrink, got %q", node.Gap)
	}
	if node.Padding != "lg" {
		t.Errorf("expected padding=lg after shrink, got %q", node.Padding)
	}
	if node.Children[0].Node.Gap != "sm" {
		t.Errorf("expected child gap=sm after shrink, got %q", node.Children[0].Node.Gap)
	}
}

func TestRemoveEmptyGroups(t *testing.T) {
	node := &domain.LayoutNode{
		Type: domain.LayoutNodeColumn,
		Children: []domain.LayoutChild{
			{Node: &domain.LayoutNode{Type: domain.LayoutNodeRow, Children: []domain.LayoutChild{}}}, // empty
			{AtomIndex: intPtr(0)},
			{Node: &domain.LayoutNode{Type: domain.LayoutNodeRow, Children: []domain.LayoutChild{{AtomIndex: intPtr(1)}}}}, // non-empty
		},
	}
	removeEmptyGroups(node)
	if len(node.Children) != 2 {
		t.Errorf("expected 2 children after removing empty, got %d", len(node.Children))
	}
}

func TestFlattenDeep(t *testing.T) {
	// 4 levels deep: root(0) > level1(1) > level2(2) > level3(3) > atom
	// With maxDepth=2, level2 at depth 2 should have its nested children flattened to atoms
	deepNode := &domain.LayoutNode{
		Type: domain.LayoutNodeColumn,
		Children: []domain.LayoutChild{
			{Node: &domain.LayoutNode{
				Type: domain.LayoutNodeColumn,
				Children: []domain.LayoutChild{
					{Node: &domain.LayoutNode{
						Type: domain.LayoutNodeColumn,
						Children: []domain.LayoutChild{
							{Node: &domain.LayoutNode{
								Type:     domain.LayoutNodeColumn,
								Children: []domain.LayoutChild{{AtomIndex: intPtr(0)}, {AtomIndex: intPtr(1)}},
							}},
						},
					}},
				},
			}},
		},
	}
	flattenDeep(deepNode, 2)
	// At depth 2, the node should have been flattened — its children should be atom leaves
	child := deepNode.Children[0].Node.Children[0].Node
	if len(child.Children) != 2 {
		t.Errorf("expected 2 atom children after flatten, got %d", len(child.Children))
	}
	for _, c := range child.Children {
		if c.AtomIndex == nil {
			t.Error("expected all children to be atom leaves after flatten")
		}
	}
}

func TestNormalizeSizeV2(t *testing.T) {
	widgets := []domain.Widget{
		{Size: domain.WidgetSizeSmall},
		{Size: domain.WidgetSizeMedium},
		{Size: domain.WidgetSizeSmall},
	}
	normalizeSizeV2(widgets)
	for i, w := range widgets {
		if w.Size != domain.WidgetSizeMedium {
			t.Errorf("widget %d: expected size=medium, got %s", i, w.Size)
		}
	}
}

func TestNormalizeStyleV2(t *testing.T) {
	widgets := []domain.Widget{
		{AtomsV2: []domain.AtomV2{
			{FieldName: "name", TextStyle: &domain.TextStyle{FontSize: "xl", FontWeight: "bold"}},
		}},
		{AtomsV2: []domain.AtomV2{
			{FieldName: "name", TextStyle: &domain.TextStyle{FontSize: "lg", FontWeight: "normal"}},
		}},
	}
	normalizeStyleV2(widgets)
	// Second widget's name should match first widget's fontSize
	if widgets[1].AtomsV2[0].TextStyle.FontSize != "xl" {
		t.Errorf("expected normalized fontSize=xl, got %q", widgets[1].AtomsV2[0].TextStyle.FontSize)
	}
	if widgets[1].AtomsV2[0].TextStyle.FontWeight != "bold" {
		t.Errorf("expected normalized fontWeight=bold, got %q", widgets[1].AtomsV2[0].TextStyle.FontWeight)
	}
}

func TestDefaultMediaStyle_Video(t *testing.T) {
	atoms := []domain.AtomV2{
		{Type: domain.AtomTypeVideo, FieldName: "video"},
		{Type: domain.AtomTypeAudio, FieldName: "audio"},
	}
	applyDefaultMediaStyle(atoms, domain.WidgetSizeMedium)

	if atoms[0].MediaStyle == nil {
		t.Fatal("expected mediaStyle on video atom")
	}
	if atoms[0].MediaStyle.AspectRatio != "16:9" {
		t.Errorf("expected video aspectRatio=16:9, got %q", atoms[0].MediaStyle.AspectRatio)
	}
	if !atoms[0].MediaStyle.Controls {
		t.Error("expected video controls=true by default")
	}
	if atoms[1].MediaStyle == nil {
		t.Fatal("expected mediaStyle on audio atom")
	}
	if !atoms[1].MediaStyle.Controls {
		t.Error("expected audio controls=true by default")
	}
}

func intPtr(v int) *int {
	return &v
}

func TestAutoLayout_ContainerDefaults(t *testing.T) {
	atoms := []domain.AtomV2{
		{Type: domain.AtomTypeImage, FieldName: "images"},
		{Type: domain.AtomTypeText, FieldName: "name", TextStyle: &domain.TextStyle{FontSize: "2xl"}},
		{Type: domain.AtomTypeNumber, Subtype: domain.SubtypeCurrency, FieldName: "price", Slot: domain.AtomSlotPrice},
		{Type: domain.AtomTypeNumber, Subtype: domain.SubtypeRating, FieldName: "rating"},
	}

	layout := AutoLayout(atoms)
	if layout == nil {
		t.Fatal("expected non-nil layout")
	}

	// Root should have padding
	if layout.Padding != "sm" {
		t.Errorf("expected root padding=sm, got %q", layout.Padding)
	}

	// Find hero node — should have borderRadius
	for _, child := range layout.Children {
		if child.Node != nil && child.Node.Name == "hero" {
			if child.Node.BorderRadius != "md" {
				t.Errorf("expected hero borderRadius=md, got %q", child.Node.BorderRadius)
			}
		}
		// Find price-rating node — should have distribution=between
		if child.Node != nil && child.Node.Name == "price-rating" {
			if child.Node.Distribution != "between" {
				t.Errorf("expected price-rating distribution=between, got %q", child.Node.Distribution)
			}
		}
	}
}
