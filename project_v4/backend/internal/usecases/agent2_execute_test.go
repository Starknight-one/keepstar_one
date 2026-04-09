package usecases

import (
	"strings"
	"testing"

	"keepstar_v4/internal/domain"
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

