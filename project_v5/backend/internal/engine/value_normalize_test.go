package engine

import "testing"

func TestNormalizeFills(t *testing.T) {
	// nil → empty
	if got := NormalizeFills(nil); len(got) != 0 {
		t.Errorf("nil → %v, want []", got)
	}
	// scalar → single-element slice
	if got := NormalizeFills("#FFFFFF"); len(got) != 1 || got[0] != "#FFFFFF" {
		t.Errorf("scalar → %v, want [#FFFFFF]", got)
	}
	// object → single-element slice
	fill := map[string]any{"type": "color", "color": "#FF0000"}
	if got := NormalizeFills(fill); len(got) != 1 {
		t.Errorf("object → %v, want 1-element slice", got)
	}
	// array passthrough
	in := []any{"#FFF", "#000"}
	if got := NormalizeFills(in); len(got) != 2 {
		t.Errorf("array → %v, want 2-element slice", got)
	}
}

func TestNormalizeEffects(t *testing.T) {
	if got := NormalizeEffects(nil); len(got) != 0 {
		t.Errorf("nil → %v, want []", got)
	}
	eff := map[string]any{"type": "blur", "radius": 4}
	if got := NormalizeEffects(eff); len(got) != 1 {
		t.Errorf("object → %v", got)
	}
	in := []any{eff, eff}
	if got := NormalizeEffects(in); len(got) != 2 {
		t.Errorf("array → %v", got)
	}
}
