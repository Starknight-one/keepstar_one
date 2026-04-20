package tools

import (
	"testing"

	engine_v4 "keepstar_v4/internal/engine_v4"
)

// TestValidatePresetWithUserOps_PresetPlusMultiInsert_Error — top-level preset
// combined with 2+ user widget inserts is invalid: the user is composing
// multiple widgets but using the legacy single-template preset slot.
func TestValidatePresetWithUserOps_PresetPlusMultiInsert_Error(t *testing.T) {
	ops := []engine_v4.Op{
		{Type: engine_v4.OpInsert, Props: map[string]interface{}{"type": "widget"}},
		{Type: engine_v4.OpInsert, Props: map[string]interface{}{"type": "widget"}},
	}
	if msg := validatePresetWithUserOps("product_card", ops); msg == "" {
		t.Error("expected error for top-level preset + 2 widget inserts, got none")
	}
}

// TestValidatePresetWithUserOps_PresetPlusInsertWithPreset_Error — top-level
// preset + a widget insert that itself carries props.preset is ambiguous.
func TestValidatePresetWithUserOps_PresetPlusInsertWithPreset_Error(t *testing.T) {
	ops := []engine_v4.Op{
		{Type: engine_v4.OpInsert, Props: map[string]interface{}{"type": "widget", "preset": "product_card_compact"}},
	}
	if msg := validatePresetWithUserOps("product_card", ops); msg == "" {
		t.Error("expected error for top-level preset + per-widget preset in props, got none")
	}
}

// TestValidatePresetWithUserOps_PresetWithUpdateOps_OK — preset + override
// ops (update/delete on preset refs) is the canonical preset workflow.
func TestValidatePresetWithUserOps_PresetWithUpdateOps_OK(t *testing.T) {
	ops := []engine_v4.Op{
		{Type: engine_v4.OpUpdate, Target: "price", Props: map[string]interface{}{
			"textStyle": map[string]interface{}{"color": "red"},
		}},
		{Type: engine_v4.OpDelete, Target: "rating"},
	}
	if msg := validatePresetWithUserOps("product_card", ops); msg != "" {
		t.Errorf("expected ok for preset + update/delete ops, got %q", msg)
	}
}

// TestValidatePresetWithUserOps_PresetWithSingleInsert_OK — preset + 1 user
// widget insert is unusual but allowed (per plan): user adds an extra widget
// on top of the preset's template.
func TestValidatePresetWithUserOps_PresetWithSingleInsert_OK(t *testing.T) {
	ops := []engine_v4.Op{
		{Type: engine_v4.OpInsert, Props: map[string]interface{}{"type": "widget"}},
	}
	if msg := validatePresetWithUserOps("product_card", ops); msg != "" {
		t.Errorf("expected ok for preset + 1 widget insert, got %q", msg)
	}
}

// TestValidatePresetWithUserOps_NoPreset_OK — multi-widget composition without
// top-level preset is the new COMPOSING path, all widget inserts allowed.
func TestValidatePresetWithUserOps_NoPreset_OK(t *testing.T) {
	ops := []engine_v4.Op{
		{Type: engine_v4.OpInsert, Props: map[string]interface{}{"type": "widget"}},
		{Type: engine_v4.OpInsert, Props: map[string]interface{}{"type": "widget", "preset": "product_card"}},
		{Type: engine_v4.OpInsert, Props: map[string]interface{}{"type": "widget"}},
	}
	if msg := validatePresetWithUserOps("", ops); msg != "" {
		t.Errorf("expected ok with no top-level preset, got %q", msg)
	}
}

// TestValidatePresetWithUserOps_PresetWithAtomInserts_OK — preset + atom
// inserts (no widget inserts) is the standard "add a NEW! badge" pattern.
func TestValidatePresetWithUserOps_PresetWithAtomInserts_OK(t *testing.T) {
	ops := []engine_v4.Op{
		{Type: engine_v4.OpInsert, Parent: "$info", Props: map[string]interface{}{"type": "text", "value": "NEW!"}},
		{Type: engine_v4.OpInsert, Parent: "$root", Props: map[string]interface{}{"type": "row", "gap": "md"}},
	}
	if msg := validatePresetWithUserOps("product_card", ops); msg != "" {
		t.Errorf("expected ok for preset + atom/node inserts, got %q", msg)
	}
}
