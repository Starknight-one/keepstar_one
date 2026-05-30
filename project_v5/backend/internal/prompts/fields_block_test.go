package prompts

import (
	"context"
	"errors"
	"strings"
	"testing"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// stubFieldDefPort lets a test drive ListFieldDefinitions / SampleFieldValues
// independently — including the "table is gone" error path.
type stubFieldDefPort struct {
	defs    []ports.FieldDefinition
	defsErr error
	samples map[string][]interface{}
}

func (s stubFieldDefPort) ListFieldDefinitions(_ context.Context, _ string, _ domain.EntityType) ([]ports.FieldDefinition, error) {
	return s.defs, s.defsErr
}
func (s stubFieldDefPort) GetFieldDefinition(_ context.Context, _ string, _ domain.EntityType, _ string) (*ports.FieldDefinition, error) {
	return nil, nil
}
func (s stubFieldDefPort) SampleFieldValues(_ context.Context, _ string, _ domain.EntityType, _ int) (map[string][]interface{}, error) {
	return s.samples, nil
}

// lineFor returns the <fields> row whose field name is `field`, or "".
func lineFor(block, field string) string {
	for _, ln := range strings.Split(block, "\n") {
		if strings.HasPrefix(strings.TrimSpace(ln), field+" ") {
			return ln
		}
	}
	return ""
}

// When catalog.field_definitions is missing, ListFieldDefinitions errors. The
// block must NOT fail — it must degrade to a vocabulary derived from the real
// sample data, with atom types inferred from field name + value. This is the
// fix that keeps Agent2 alive after the table was dropped.
func TestFormatFieldsBlock_FailsOpenAndDerivesFromData(t *testing.T) {
	port := stubFieldDefPort{
		defsErr: errors.New(`ERROR: relation "catalog.field_definitions" does not exist (SQLSTATE 42P01)`),
		samples: map[string][]interface{}{
			"name":     {"GAOPAN Velvet Sectional Sofa"},
			"price":    {51765},
			"rating":   {4.5},
			"brand":    {"GAOPAN"},
			"category": {"Sofas"},
			"images":   {"https://m.media-amazon.com/images/I/abc.jpg"},
			"material": {"velvet"},
		},
	}

	block, err := FormatFieldsBlock(context.Background(), port, "pim-furniture-demo", domain.EntityTypeProduct, 3)
	if err != nil {
		t.Fatalf("expected fail-open (nil error), got %v", err)
	}
	if !strings.Contains(block, `<fields entity="product">`) {
		t.Fatalf("missing block header:\n%s", block)
	}

	// Inferred atom types must reflect name + value heuristics.
	checks := map[string]string{
		"price":    "number/currency",
		"rating":   "number/rating",
		"images":   "image/url",
		"name":     "text/string",
		"material": "text/string",
	}
	for field, want := range checks {
		ln := lineFor(block, field)
		if ln == "" {
			t.Errorf("field %q absent from data-derived block:\n%s", field, block)
			continue
		}
		if !strings.Contains(ln, want) {
			t.Errorf("field %q: want type %q, got line: %s", field, want, ln)
		}
	}

	// Real sample values must ride along (the playbook disambiguates by sample).
	if !strings.Contains(block, "GAOPAN Velvet Sectional Sofa") {
		t.Errorf("expected samples in block:\n%s", block)
	}
	// A humanized label is synthesised for data-only fields.
	if ln := lineFor(block, "material"); !strings.Contains(ln, `label="Material"`) {
		t.Errorf("expected humanized label for material, got: %s", ln)
	}
}

// Published definitions stay authoritative and come first (by priority); fields
// seen only in the data are appended. field_definitions is enrichment, not a
// gate — but when present it wins on metadata.
func TestFormatFieldsBlock_UnionPublishedAndDataOnly(t *testing.T) {
	port := stubFieldDefPort{
		defs: []ports.FieldDefinition{
			{FieldName: "name", EntityType: domain.EntityTypeProduct, AtomType: domain.AtomTypeText, AtomSubtype: domain.SubtypeString, Label: "Product Name", DefaultSlot: domain.AtomSlotTitle, Priority: 1},
		},
		samples: map[string][]interface{}{
			"name":     {"Boyd Sleep Bed Frame"},
			"material": {"oak"},
		},
	}

	block, err := FormatFieldsBlock(context.Background(), port, "t", domain.EntityTypeProduct, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Published metadata (custom label + slot) must be preserved, not re-derived.
	nameLine := lineFor(block, "name")
	if !strings.Contains(nameLine, `label="Product Name"`) || !strings.Contains(nameLine, "slot=title") {
		t.Errorf("published def should win on metadata, got: %s", nameLine)
	}
	// "name" must appear exactly once (no duplicate from the data-only pass).
	if got := strings.Count(block, `label="Product Name"`); got != 1 {
		t.Errorf("expected name row once, got %d:\n%s", got, block)
	}
	// data-only field still surfaces.
	if lineFor(block, "material") == "" {
		t.Errorf("data-only field 'material' should appear:\n%s", block)
	}
	// Published field comes before the appended data-only field.
	if strings.Index(block, "name") > strings.Index(block, "material") {
		t.Errorf("published 'name' should precede data-only 'material':\n%s", block)
	}
}
