package searchtext

import "testing"

func TestFlattenJSONBStableOrderAndArrays(t *testing.T) {
	// Why: the tsvector is computed from this text; if key order were
	// nondeterministic, every rebuild would churn the tsv and HNSW would see
	// spurious diffs. Order must be stable and arrays must be fully expanded.
	m := map[string]any{
		"product_form": "serum",
		"skin_type":    []any{"dry", "oily"},
		"spf":          float64(30),
	}
	got := flattenJSONB(m)
	want := "product_form serum skin_type dry oily spf 30"
	if got != want {
		t.Fatalf("flattenJSONB = %q, want %q", got, want)
	}
}

func TestBuildProjectionTextsVerticalAgnostic(t *testing.T) {
	// Why: typed attributes for ANY vertical must reach the search text via
	// tier2 — nothing here may assume cosmetics. An electronics attr (ram_gb)
	// must be present in the combined search text just like a cosmetics one.
	vol := 473
	out := BuildProjectionTexts(ProjectionSource{
		Name:  "CeraVe Cleanser",
		Brand: "CeraVe",
		Tier2: map[string]any{"ram_gb": float64(16)},
		Size:  "L",
		VolumeML: &vol,
	})
	if out.Tier1Text != "CeraVe Cleanser CeraVe" {
		t.Errorf("tier1 = %q", out.Tier1Text)
	}
	if out.VariantText != "L 473ml" {
		t.Errorf("variant = %q", out.VariantText)
	}
	for _, sub := range []string{"CeraVe Cleanser", "ram_gb 16", "473ml"} {
		if !contains(out.SearchText, sub) {
			t.Errorf("search text %q missing %q", out.SearchText, sub)
		}
	}
}

func TestBuildProjectionTextsEmpty(t *testing.T) {
	out := BuildProjectionTexts(ProjectionSource{Name: "X"})
	if out.Tier1Text != "X" || out.Tier2Text != "" || out.VariantText != "" || out.SearchText != "X" {
		t.Fatalf("unexpected empty-source output: %+v", out)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
