package postgres

// Unit tests for the system-registry fallback path of PresetAdapter —
// no DB needed: systemPresetFallback only consults the in-process
// registry, so we construct the adapter bare (nil client).

import (
	"testing"

	"keepstar_v5/internal/engine/presets"
)

// TestSystemPresetFallbackDescription — registry-fallback presets must
// carry the real Agent2-visible description (SystemPresetDescriptions),
// not the old hardcoded "system-default preset" placeholder. The admin
// canvas surfaces Description; a placeholder there hides what the LLM
// actually sees.
func TestSystemPresetFallbackDescription(t *testing.T) {
	a := &PresetAdapter{systemRegistry: presets.NewSystemPresetRegistry()}

	for _, name := range []string{"product_card", "empty_not_found"} {
		want := presets.SystemPresetDescriptions[name]
		if want == "" {
			t.Fatalf("SystemPresetDescriptions[%q] empty — test precondition broken", name)
		}
		p := a.systemPresetFallback(name)
		if p == nil {
			t.Fatalf("systemPresetFallback(%q) = nil, want preset", name)
		}
		if p.Description != want {
			t.Errorf("%s Description = %q, want %q", name, p.Description, want)
		}
		if !p.IsSystem {
			t.Errorf("%s IsSystem = false, want true", name)
		}
	}
}

// TestSystemPresetFallbackUnknownName — names outside the registry keep
// returning nil (GetPublishedPreset then maps that to ErrPresetNotFound).
func TestSystemPresetFallbackUnknownName(t *testing.T) {
	a := &PresetAdapter{systemRegistry: presets.NewSystemPresetRegistry()}
	if p := a.systemPresetFallback("not_a_system_preset_zzzz"); p != nil {
		t.Errorf("systemPresetFallback(unknown) = %+v, want nil", p)
	}
}
