package usecases

import (
	"strings"
	"testing"

	"keepstar_v4/internal/domain"
	engine_v4 "keepstar_v4/internal/engine_v4"
	"keepstar_v4/internal/ports"
)

func TestFormatFieldsBlock_BasicShape(t *testing.T) {
	fields := []ports.FieldDefinition{
		{FieldName: "name", AtomType: domain.AtomTypeText, AtomSubtype: domain.SubtypeString, Label: "Название", DefaultSlot: domain.AtomSlotTitle, Priority: 1},
		{FieldName: "price", AtomType: domain.AtomTypeNumber, AtomSubtype: domain.SubtypeCurrency, Unit: "RUB", Label: "Цена", DefaultSlot: domain.AtomSlotPrice, Priority: 2},
		{FieldName: "images", AtomType: domain.AtomTypeImage, AtomSubtype: domain.SubtypeImageURL, Label: "Images", DefaultSlot: domain.AtomSlotHero, Priority: 0},
	}
	samples := map[string][]interface{}{
		"name":  {"COSRX BHA", "Laneige"},
		"price": {2490, 3990},
	}

	out := formatFieldsBlock(fields, samples)
	if out == "" {
		t.Fatal("expected non-empty output")
	}

	// Must be wrapped in the expected XML-ish tag
	if !strings.HasPrefix(out, `<fields entity="product">`) {
		t.Errorf("expected prefix <fields entity=\"product\">, got: %q", out[:min(40, len(out))])
	}
	if !strings.HasSuffix(out, `</fields>`) {
		t.Errorf("expected suffix </fields>, got: %q", out[max(0, len(out)-40):])
	}

	// Must contain all field names
	for _, f := range []string{"name", "price", "images"} {
		if !strings.Contains(out, f) {
			t.Errorf("expected field %q in output", f)
		}
	}

	// Priority ordering — images (priority 0) must appear before name (priority 1) which must appear before price (priority 2)
	imgIdx := strings.Index(out, "images")
	nameIdx := strings.Index(out, "name")
	priceIdx := strings.Index(out, "price")
	if !(imgIdx < nameIdx && nameIdx < priceIdx) {
		t.Errorf("expected priority order images < name < price, got indices %d/%d/%d", imgIdx, nameIdx, priceIdx)
	}

	// Samples must appear as JSON
	if !strings.Contains(out, `samples=["COSRX BHA","Laneige"]`) {
		t.Errorf("expected name samples as JSON array, got: %s", out)
	}
	if !strings.Contains(out, `samples=[2490,3990]`) {
		t.Errorf("expected price samples as JSON array, got: %s", out)
	}

	// Type descriptor must include subtype
	if !strings.Contains(out, "number/currency") {
		t.Errorf("expected compound type number/currency, got: %s", out)
	}
	if !strings.Contains(out, "text/string") {
		t.Errorf("expected compound type text/string, got: %s", out)
	}

	// Unit must be present for price
	if !strings.Contains(out, `unit="RUB"`) {
		t.Errorf("expected unit=\"RUB\" for price, got: %s", out)
	}

	// Slot hints must be present
	if !strings.Contains(out, "slot=title") {
		t.Errorf("expected slot=title for name, got: %s", out)
	}
	if !strings.Contains(out, "slot=hero") {
		t.Errorf("expected slot=hero for images, got: %s", out)
	}
}

func TestFormatFieldsBlock_EmptyInput(t *testing.T) {
	out := formatFieldsBlock(nil, nil)
	if out != "" {
		t.Errorf("expected empty output for nil fields, got: %q", out)
	}
}

func TestFormatFieldsBlock_NoSamples(t *testing.T) {
	fields := []ports.FieldDefinition{
		{FieldName: "model", AtomType: domain.AtomTypeText, AtomSubtype: domain.SubtypeString, Label: "Model", DefaultSlot: domain.AtomSlotTitle, Priority: 0},
	}
	out := formatFieldsBlock(fields, nil)
	if out == "" {
		t.Fatal("expected non-empty output even without samples")
	}
	if strings.Contains(out, "samples=") {
		t.Errorf("expected no samples= marker when no samples provided, got: %s", out)
	}
	if !strings.Contains(out, "model") {
		t.Errorf("expected field name in output, got: %s", out)
	}
}

// --- formatDesignContextBlock tests ---

func TestFormatDesignContextBlock_BasicShape(t *testing.T) {
	snap := &engine_v4.DesignContextSnapshot{
		TenantIDOrSlug: "heybabe",
		Version:        7,
		Presets: []*engine_v4.Preset{
			{
				Name:             "banana_card",
				Category:         "product",
				Description:      "Yellow promo card",
				DefaultReplicate: true,
				Build: func() []engine_v4.Op {
					return []engine_v4.Op{
						{Type: "insert", Ref: "w", Parent: "formation", Props: map[string]interface{}{"type": "widget"}},
						{Type: "insert", Ref: "root", Parent: "$w", Props: map[string]interface{}{"type": "column"}},
						{Type: "insert", Parent: "$root", Props: map[string]interface{}{"type": "image", "fieldName": "images"}},
						{Type: "insert", Parent: "$root", Props: map[string]interface{}{"type": "text", "fieldName": "name"}},
					}
				},
			},
			{
				Name:        "product_card",
				Category:    "product",
				Description: "Tenant override",
				Build: func() []engine_v4.Op {
					return []engine_v4.Op{
						{Type: "insert", Ref: "w", Parent: "formation", Props: map[string]interface{}{"type": "widget"}},
						{Type: "insert", Parent: "$w", Props: map[string]interface{}{"type": "text", "fieldName": "brand"}},
					}
				},
			},
		},
		Components: []ports.CanvasComponentView{
			{Name: "price_pill", Category: "atom", Description: "Rounded currency pill"},
		},
		Tokens: []ports.CanvasDesignTokenView{
			{Category: "color", Name: "accent", Value: "#FFD600"},
			{Category: "color", Name: "text_primary", Value: "#111111"},
			{Category: "radius", Name: "card", Value: "12px"},
		},
		OverridesGlobal: map[string]bool{"product_card": true},
	}

	out := formatDesignContextBlock(snap)
	if out == "" {
		t.Fatal("expected non-empty output")
	}

	// Wrapper tag with version
	if !strings.HasPrefix(out, `<tenant_design_context version="7">`) {
		t.Errorf("expected version wrapper, got prefix: %q", out[:60])
	}
	if !strings.HasSuffix(out, `</tenant_design_context>`) {
		t.Errorf("expected closing tag, got suffix: %q", out[len(out)-40:])
	}

	// Preset ordering — alphabetical: banana_card < product_card
	bIdx := strings.Index(out, `name="banana_card"`)
	pIdx := strings.Index(out, `name="product_card"`)
	if bIdx < 0 || pIdx < 0 || bIdx >= pIdx {
		t.Error("expected banana_card before product_card (alphabetical)")
	}

	// Overrides attr on product_card only
	if !strings.Contains(out, `overrides="global"`) {
		t.Error("expected overrides=\"global\" on product_card")
	}

	// Replicate attr on banana_card only
	if !strings.Contains(out, `replicate="true"`) {
		t.Error("expected replicate=\"true\" on banana_card")
	}

	// Binds for banana_card
	if !strings.Contains(out, "<binds>images, name</binds>") {
		t.Errorf("expected banana_card binds=images, name, got: %s", out)
	}

	// Component
	if !strings.Contains(out, `name="price_pill"`) {
		t.Error("expected price_pill component")
	}

	// Tokens grouped by category
	if !strings.Contains(out, `<category name="color">`) {
		t.Error("expected color category")
	}
	if !strings.Contains(out, `<category name="radius">`) {
		t.Error("expected radius category")
	}
	if !strings.Contains(out, `accent="#FFD600"`) {
		t.Error("expected accent token")
	}
}

func TestFormatDesignContextBlock_EmptyInput(t *testing.T) {
	// nil snapshot
	if out := formatDesignContextBlock(nil); out != "" {
		t.Errorf("nil snapshot should return empty, got: %q", out)
	}

	// empty snapshot (no presets, components, or tokens)
	snap := &engine_v4.DesignContextSnapshot{
		Version:         1,
		OverridesGlobal: map[string]bool{},
	}
	if out := formatDesignContextBlock(snap); out != "" {
		t.Errorf("empty snapshot should return empty, got: %q", out)
	}
}

func TestFormatDesignContextBlock_Truncation(t *testing.T) {
	// 60 presets — should cap at 50 with <more count="10"/> footer.
	presets := make([]*engine_v4.Preset, 60)
	for i := range presets {
		name := "preset_" + strings.Repeat("0", 3-len(strings.TrimLeft(string(rune('0'+i/100)), "0")))
		presets[i] = &engine_v4.Preset{
			Name:     name + string(rune('a'+i%26)) + string(rune('a'+i/26)),
			Category: "product",
			Build:    func() []engine_v4.Op { return nil },
		}
	}
	snap := &engine_v4.DesignContextSnapshot{
		Version:         1,
		Presets:         presets,
		OverridesGlobal: map[string]bool{},
	}

	out := formatDesignContextBlock(snap)
	if !strings.Contains(out, `<more count="10"/>`) {
		t.Errorf("expected <more count=\"10\"/> for 60 presets, got: %s", out)
	}
	// Count <preset name= occurrences
	count := strings.Count(out, "<preset name=")
	if count != 50 {
		t.Errorf("expected 50 preset entries, got %d", count)
	}
}

func TestCompactPresetBinds_Ordering(t *testing.T) {
	p := &engine_v4.Preset{
		Name: "test",
		Build: func() []engine_v4.Op {
			return []engine_v4.Op{
				// widget insert — no fieldName
				{Type: "insert", Ref: "w", Parent: "formation", Props: map[string]interface{}{"type": "widget"}},
				// column — no fieldName
				{Type: "insert", Ref: "root", Parent: "$w", Props: map[string]interface{}{"type": "column"}},
				// atom with fieldName
				{Type: "insert", Parent: "$root", Props: map[string]interface{}{"type": "image", "fieldName": "images"}},
				{Type: "insert", Parent: "$root", Props: map[string]interface{}{"type": "text", "fieldName": "name"}},
				{Type: "insert", Parent: "$root", Props: map[string]interface{}{"type": "number", "fieldName": "price"}},
				// duplicate fieldName — should not appear twice
				{Type: "insert", Parent: "$root", Props: map[string]interface{}{"type": "text", "fieldName": "name"}},
				// nil props — safe skip
				{Type: "insert", Parent: "$root"},
			}
		},
	}

	binds := compactPresetBinds(p)
	if binds != "images, name, price" {
		t.Errorf("expected 'images, name, price', got %q", binds)
	}
}

func TestCompactPresetBinds_NilPreset(t *testing.T) {
	if binds := compactPresetBinds(nil); binds != "" {
		t.Errorf("nil preset should return empty, got %q", binds)
	}
	p := &engine_v4.Preset{Name: "no-build"}
	if binds := compactPresetBinds(p); binds != "" {
		t.Errorf("nil Build should return empty, got %q", binds)
	}
}

func TestFormatDesignContextBlock_TokensWithThemeAxis(t *testing.T) {
	snap := &engine_v4.DesignContextSnapshot{
		Version: 3,
		Tokens: []ports.CanvasDesignTokenView{
			{Category: "color", Name: "bg_card", Value: "#FFFFFF"},
			{Category: "color", Name: "bg_card", Value: "#1A0A2E", ThemeAxis: "season", ThemeValue: "halloween"},
		},
		OverridesGlobal: map[string]bool{},
	}

	out := formatDesignContextBlock(snap)
	if !strings.Contains(out, `theme="season":"halloween"`) {
		t.Errorf("expected theme axis annotation, got: %s", out)
	}
	if !strings.Contains(out, `bg_card="#FFFFFF"`) {
		t.Error("expected base token value")
	}
}

