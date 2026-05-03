package presets

import (
	"encoding/json"
	"testing"
)

// TestSystemRegistryListContainsAdvertisedPresets — every name listed in
// agent2_prompt.go PRESETS section must resolve via the registry.
// Keep the expected list aligned with that prompt; if the prompt changes
// names this test catches the drift.
func TestSystemRegistryListContainsAdvertisedPresets(t *testing.T) {
	r := NewSystemPresetRegistry()
	have := map[string]bool{}
	for _, n := range r.List() {
		have[n] = true
	}
	required := []string{
		"product_card",
		"product_card_compact",
		"product_card_horizontal",
		"product_card_list_row",
		"product_detail",
		"product_detail_horizontal",
		"text_explainer",
		"empty_not_found",
		"error_generic",
	}
	for _, name := range required {
		if !have[name] {
			t.Errorf("registry missing required preset %q", name)
		}
	}
}

// TestSystemRegistryGetParsesValidJSON — every entry must round-trip
// through json.Unmarshal into a v9-shaped Document. Catches
// hand-edited JSON that breaks the schema.
func TestSystemRegistryGetParsesValidJSON(t *testing.T) {
	r := NewSystemPresetRegistry()
	for _, name := range r.List() {
		t.Run(name, func(t *testing.T) {
			body, ok := r.Get(name)
			if !ok {
				t.Fatalf("Get(%q) = false, expected hit", name)
			}
			var doc map[string]any
			if err := json.Unmarshal(body, &doc); err != nil {
				t.Fatalf("preset %q is not valid JSON: %v", name, err)
			}
			if _, ok := doc["children"]; !ok {
				t.Errorf("preset %q missing top-level children", name)
			}
		})
	}
}

// TestSystemRegistryCaches — second Get for the same name returns
// byte-identical cached payload (sync.Map hit, no re-parse).
func TestSystemRegistryCaches(t *testing.T) {
	r := NewSystemPresetRegistry()
	first, ok := r.Get("product_card")
	if !ok {
		t.Fatalf("Get first call missed")
	}
	second, ok := r.Get("product_card")
	if !ok {
		t.Fatalf("Get second call missed")
	}
	if &first[0] != &second[0] {
		t.Errorf("cache miss — second call returned a different backing array")
	}
}

// TestSystemRegistryGetMissReturnsFalse — unknown preset returns
// (nil, false) rather than panicking.
func TestSystemRegistryGetMissReturnsFalse(t *testing.T) {
	r := NewSystemPresetRegistry()
	body, ok := r.Get("definitely_not_a_real_preset")
	if ok || body != nil {
		t.Errorf("Get unknown = (%v, %v); want (nil, false)", body, ok)
	}
}

// TestSystemRegistryDefaultReplicate — replicate flags follow seed map.
func TestSystemRegistryDefaultReplicate(t *testing.T) {
	r := NewSystemPresetRegistry()
	if !r.DefaultReplicate("product_card") {
		t.Errorf("product_card default replicate = false, want true")
	}
	if r.DefaultReplicate("product_detail") {
		t.Errorf("product_detail default replicate = true, want false")
	}
	if r.DefaultReplicate("empty_not_found") {
		t.Errorf("empty_not_found default replicate = true, want false")
	}
}
