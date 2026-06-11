package presets

import "testing"

// TestSystemPresetTaxonomyComplete — the internal preset-library listing
// (GET /api/v1/internal/presets/system) builds every entry from the
// taxonomy maps; a seed without a category/description would ship a
// blank field to the admin canvas. Guards both directions so seeds and
// taxonomy can never drift apart silently.
func TestSystemPresetTaxonomyComplete(t *testing.T) {
	canonical := map[string]bool{
		"cards":      true,
		"details":    true,
		"components": true,
		"states":     true,
		"narrative":  true,
	}

	for name := range SystemPresetSeeds {
		cat, ok := SystemPresetCategory[name]
		if !ok {
			t.Errorf("SystemPresetSeeds[%q] has no SystemPresetCategory entry", name)
			continue
		}
		if !canonical[cat] {
			t.Errorf("SystemPresetCategory[%q] = %q, not a canonical family", name, cat)
		}
	}
	for name := range SystemPresetCategory {
		if _, ok := SystemPresetSeeds[name]; !ok {
			t.Errorf("SystemPresetCategory[%q] has no matching seed", name)
		}
	}

	// Descriptions drift both ways too: a seed without a description ships
	// a blank field to the admin canvas AND a generic fallback line to
	// Agent2; an orphan description entry hides a rename.
	for name := range SystemPresetSeeds {
		if SystemPresetDescriptions[name] == "" {
			t.Errorf("SystemPresetSeeds[%q] has no SystemPresetDescriptions entry", name)
		}
	}
	for name := range SystemPresetDescriptions {
		if _, ok := SystemPresetSeeds[name]; !ok {
			t.Errorf("SystemPresetDescriptions[%q] has no matching seed", name)
		}
	}

	for key := range SystemComponentSeeds {
		if SystemComponentDescriptions[key] == "" {
			t.Errorf("SystemComponentSeeds[%q] has no SystemComponentDescriptions entry", key)
		}
		if SystemComponentPublicNames[key] == "" {
			t.Errorf("SystemComponentSeeds[%q] has no SystemComponentPublicNames entry", key)
		}
	}
	for key := range SystemComponentDescriptions {
		if _, ok := SystemComponentSeeds[key]; !ok {
			t.Errorf("SystemComponentDescriptions[%q] has no matching seed", key)
		}
	}
	for key := range SystemComponentPublicNames {
		if _, ok := SystemComponentSeeds[key]; !ok {
			t.Errorf("SystemComponentPublicNames[%q] has no matching seed", key)
		}
	}
}
