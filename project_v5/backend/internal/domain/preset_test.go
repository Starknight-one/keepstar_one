package domain

import (
	"encoding/json"
	"testing"
)

// TestPresetJSONRoundTrip — preset envelope marshalling preserves both the
// metadata fields and the raw doc body. Engine-side document round-trip is
// covered by chunk-1 engine tests.
func TestPresetJSONRoundTrip(t *testing.T) {
	docBody := `{"version":1,"children":[{"type":"frame","id":"card","replicate":true}]}`
	src := Preset{
		ID:               "preset-uuid",
		TenantID:         "tenant-uuid",
		Name:             "product_card",
		Category:         "product",
		EntityType:       "product",
		DefaultReplicate: true,
		Version:          1,
		Status:           PresetStatusPublished,
		DocumentJSON:     json.RawMessage(docBody),
	}
	raw, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var dst Preset
	if err := json.Unmarshal(raw, &dst); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if dst.Name != "product_card" || dst.Status != PresetStatusPublished {
		t.Errorf("metadata lost: %+v", dst)
	}
	// json.RawMessage round-trips byte-for-byte (modulo whitespace
	// normalisation done by the encoder); just confirm the doc body is
	// non-empty and parses as valid JSON.
	var parsed map[string]any
	if err := json.Unmarshal(dst.DocumentJSON, &parsed); err != nil {
		t.Fatalf("doc body not valid JSON: %v (%s)", err, string(dst.DocumentJSON))
	}
	if _, has := parsed["children"]; !has {
		t.Errorf("doc body lost children: %v", parsed)
	}
}
