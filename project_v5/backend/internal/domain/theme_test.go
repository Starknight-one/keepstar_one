package domain

import (
	"reflect"
	"testing"
)

// TestDefaultThemeTokens is the byte-identical guard: the Go canonical default
// tokens MUST equal the Phase 1 contract values, which equal the hardcoded
// widget.css fallback (project_v5/frontend/src/widget.css:16-46). If someone
// edits widget.css token values without updating DefaultThemeTokens (or vice
// versa), this test fails — preventing isDefault:true tenants from drifting.
func TestDefaultThemeTokens(t *testing.T) {
	got := DefaultThemeTokens()
	want := ThemeTokens{
		Colors: map[string]string{
			"blue":       "#5BA4D9",
			"orange":     "#F0924A",
			"bg":         "#ffffff",
			"text":       "#1a1a1a",
			"text_muted": "#666666",
			"border":     "#e5e7eb",
			"line":       "#e5e7eb",
		},
		Radius:     map[string]int{"base": 8},
		Gap:        map[string]int{"xs": 4, "sm": 8, "md": 16, "lg": 24, "xl": 32},
		FontSize:   map[string]int{"xs": 11, "sm": 13, "base": 14, "md": 15, "lg": 18, "xl": 22, "2xl": 28, "3xl": 36},
		FontWeight: map[string]int{"light": 300, "normal": 400, "medium": 500, "semibold": 600, "bold": 700},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DefaultThemeTokens drifted from contract/widget.css\n got=%#v\nwant=%#v", got, want)
	}
}

// TestDefaultThemeIsDefault asserts DefaultTheme carries IsDefault=true and the
// canonical token set — the explicit attach payload for the no-row case.
func TestDefaultThemeIsDefault(t *testing.T) {
	d := DefaultTheme()
	if !d.IsDefault {
		t.Fatalf("DefaultTheme().IsDefault = false, want true")
	}
	if !reflect.DeepEqual(d.Tokens, DefaultThemeTokens()) {
		t.Fatalf("DefaultTheme().Tokens != DefaultThemeTokens()")
	}
}

// TestDefaultThemeTokensFreshMap asserts each call returns an independent map,
// so a caller mutating the result (merge) never corrupts the shared default.
func TestDefaultThemeTokensFreshMap(t *testing.T) {
	a := DefaultThemeTokens()
	a.Colors["blue"] = "#000000"
	b := DefaultThemeTokens()
	if b.Colors["blue"] != "#5BA4D9" {
		t.Fatalf("DefaultThemeTokens shares mutable state: b.Colors[blue]=%q", b.Colors["blue"])
	}
}

// TestMergeThemeTokensOverDefaults asserts a partial override overlays the
// defaults key-by-key and the result is COMPLETE (every default key survives).
func TestMergeThemeTokensOverDefaults(t *testing.T) {
	override := ThemeTokens{
		Colors:   map[string]string{"blue": "#112233"},
		FontSize: map[string]int{"xl": 99},
	}
	got := MergeThemeTokensOverDefaults(override)

	if got.Colors["blue"] != "#112233" {
		t.Errorf("override color not applied: got %q", got.Colors["blue"])
	}
	if got.Colors["orange"] != "#F0924A" {
		t.Errorf("non-overridden color lost: got %q", got.Colors["orange"])
	}
	if got.FontSize["xl"] != 99 {
		t.Errorf("override fontSize not applied: got %d", got.FontSize["xl"])
	}
	if got.FontSize["base"] != 14 {
		t.Errorf("non-overridden fontSize lost: got %d", got.FontSize["base"])
	}
	// Completeness: same key cardinality as defaults across every group.
	def := DefaultThemeTokens()
	if len(got.Colors) != len(def.Colors) ||
		len(got.Gap) != len(def.Gap) ||
		len(got.FontSize) != len(def.FontSize) ||
		len(got.FontWeight) != len(def.FontWeight) ||
		len(got.Radius) != len(def.Radius) {
		t.Errorf("merge result not complete:\n got=%#v\n def=%#v", got, def)
	}
}

// TestMergeNilOverrideReturnsDefaults asserts a nil/empty override yields the
// pure defaults (the value-equality of the no-row case).
func TestMergeNilOverrideReturnsDefaults(t *testing.T) {
	got := MergeThemeTokensOverDefaults(ThemeTokens{})
	if !reflect.DeepEqual(got, DefaultThemeTokens()) {
		t.Fatalf("nil override did not return defaults\n got=%#v\nwant=%#v", got, DefaultThemeTokens())
	}
}
