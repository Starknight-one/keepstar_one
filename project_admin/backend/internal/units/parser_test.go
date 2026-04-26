package units

import (
	"strings"
	"testing"
)

// Each row is one parser test case. The dimension is verified, and value is
// compared to the canonical-unit value (e.g. mL, g, mm, pcs).
type tc struct {
	in        string
	defaultU  Canonical
	wantStat  ParseStatus
	wantVal   int
	wantUnit  Canonical
	wantCount int    // 1 unless multi-pack
	noteHint  string // substring expected in Note when not OK; "" to skip check
}

func TestParse_Volume(t *testing.T) {
	cases := []tc{
		{"30 ml", "", ParseStatusOK, 30, CanonicalML, 1, ""},
		{"30ml", "", ParseStatusOK, 30, CanonicalML, 1, ""},
		{"236ml", "", ParseStatusOK, 236, CanonicalML, 1, ""},
		{"100mL", "", ParseStatusOK, 100, CanonicalML, 1, ""},
		{"30 ml.", "", ParseStatusOK, 30, CanonicalML, 1, ""},
		{"1L", "", ParseStatusOK, 1000, CanonicalML, 1, ""},
		{"1.5L", "", ParseStatusOK, 1500, CanonicalML, 1, ""},
		{"1,5L", "", ParseStatusOK, 1500, CanonicalML, 1, ""},
		{"500 milliliters", "", ParseStatusOK, 500, CanonicalML, 1, ""},
		{"16 fl oz", "", ParseStatusOK, 473, CanonicalML, 1, ""},
		{"8 oz", "", ParseStatusOK, 237, CanonicalML, 1, ""},
		{"1 liter", "", ParseStatusOK, 1000, CanonicalML, 1, ""},
	}
	runCases(t, cases)
}

func TestParse_Mass(t *testing.T) {
	cases := []tc{
		{"100 g", "", ParseStatusOK, 100, CanonicalG, 1, ""},
		{"100g", "", ParseStatusOK, 100, CanonicalG, 1, ""},
		{"100 grams", "", ParseStatusOK, 100, CanonicalG, 1, ""},
		{"50 gram", "", ParseStatusOK, 50, CanonicalG, 1, ""},
		{"30g", "", ParseStatusOK, 30, CanonicalG, 1, ""},
		{"30 g", "", ParseStatusOK, 30, CanonicalG, 1, ""},
		{"1kg", "", ParseStatusOK, 1000, CanonicalG, 1, ""},
		{"2.5 kg", "", ParseStatusOK, 2500, CanonicalG, 1, ""},
		{"5lb", "", ParseStatusOK, 2268, CanonicalG, 1, ""},
		{"1 lbs", "", ParseStatusOK, 454, CanonicalG, 1, ""},
	}
	runCases(t, cases)
}

func TestParse_Length(t *testing.T) {
	cases := []tc{
		{"100mm", "", ParseStatusOK, 100, CanonicalMM, 1, ""},
		{"10 cm", "", ParseStatusOK, 100, CanonicalMM, 1, ""},
		{"1 m", "", ParseStatusOK, 1000, CanonicalMM, 1, ""},
		{"14 inch", "", ParseStatusOK, 356, CanonicalMM, 1, ""},
		{"14 in", "", ParseStatusOK, 356, CanonicalMM, 1, ""},
		{"14\"", "", ParseStatusOK, 356, CanonicalMM, 1, ""},
	}
	runCases(t, cases)
}

func TestParse_Count(t *testing.T) {
	cases := []tc{
		{"10 pcs", "", ParseStatusOK, 10, CanonicalPcs, 1, ""},
		{"3 pieces", "", ParseStatusOK, 3, CanonicalPcs, 1, ""},
	}
	runCases(t, cases)
}

func TestParse_MultiPack(t *testing.T) {
	cases := []tc{
		{"2x30ml", "", ParseStatusOK, 30, CanonicalML, 2, ""},
		{"2 x 30 ml", "", ParseStatusOK, 30, CanonicalML, 2, ""},
		{"2 × 500 ml", "", ParseStatusOK, 500, CanonicalML, 2, ""},
		{"3x100g", "", ParseStatusOK, 100, CanonicalG, 3, ""},
		{"6x355ml", "", ParseStatusOK, 355, CanonicalML, 6, ""}, // sixpack
	}
	runCases(t, cases)
}

func TestParse_DualLabel(t *testing.T) {
	// Within tolerance — first wins, status OK.
	r := Parse("236ml/8oz", ParseOpts{})
	if r.Status != ParseStatusOK {
		t.Fatalf("236ml/8oz: want OK, got %s (%s)", r.Status, r.Note)
	}
	if r.Value != 236 || r.Canonical != CanonicalML {
		t.Errorf("236ml/8oz: want value=236 mL, got %d %s", r.Value, r.Canonical)
	}

	// Out of tolerance — dual_mismatch.
	r = Parse("30ml/200ml", ParseOpts{})
	if r.Status != ParseStatusDualMismatch {
		t.Errorf("30ml/200ml: want dual_mismatch, got %s (%s)", r.Status, r.Note)
	}

	// Different units → mismatch (parser falls through to single-token attempt and fails).
	r = Parse("30ml/30g", ParseOpts{})
	if r.Status == ParseStatusOK {
		t.Errorf("30ml/30g: should not be OK, got OK")
	}
}

func TestParse_BareNumber(t *testing.T) {
	// Without default → ambiguous.
	r := Parse("60", ParseOpts{})
	if r.Status != ParseStatusAmbiguous {
		t.Errorf("60: want ambiguous, got %s", r.Status)
	}
	if r.Value != 0 {
		t.Errorf("60 ambiguous: value should be 0, got %d", r.Value)
	}

	// With default → ok.
	r = Parse("60", ParseOpts{DefaultUnit: CanonicalML})
	if r.Status != ParseStatusOK {
		t.Errorf("60 default=mL: want ok, got %s (%s)", r.Status, r.Note)
	}
	if r.Value != 60 || r.Canonical != CanonicalML {
		t.Errorf("60 default=mL: want 60 mL, got %d %s", r.Value, r.Canonical)
	}
}

func TestParse_Junk(t *testing.T) {
	cases := []tc{
		{"", "", ParseStatusFailed, 0, "", 0, "empty"},
		{"large", "", ParseStatusFailed, 0, "", 0, "no numeric"},
		{"abc 30", "", ParseStatusFailed, 0, "", 0, ""},
		{"S/M/L size", "", ParseStatusFailed, 0, "", 0, ""},
		{"30 furlongs", "", ParseStatusFailed, 0, "", 0, "unknown unit"},
	}
	runCases(t, cases)
}

func TestParse_RawPreserved(t *testing.T) {
	r := Parse("  236 mL  ", ParseOpts{})
	if r.Raw != "  236 mL  " {
		t.Errorf("Raw not preserved: got %q", r.Raw)
	}
}

func TestParse_TenantResolver_Override(t *testing.T) {
	// Tenant alias "tube" → mL (factor 1.0 implied).
	resolver := NewInMemoryResolver(map[string]Canonical{
		"tube": CanonicalML,
	})
	r := Parse("30 tube", ParseOpts{Resolver: resolver})
	// Tenant resolver returns mL canonical, parser uses factor lookup;
	// "tube" not in rawTokenFactors → falls to factor 1.0 path in resolveAlias.
	if r.Status != ParseStatusOK {
		t.Errorf("30 tube with resolver: want ok, got %s (%s)", r.Status, r.Note)
	}
	if r.Value != 30 || r.Canonical != CanonicalML {
		t.Errorf("30 tube with resolver: want 30 mL, got %d %s", r.Value, r.Canonical)
	}
}

func TestNormalizeToken(t *testing.T) {
	cases := map[string]string{
		"ml":     "ml",
		"ML":     "ml",
		"  ml  ": "ml",
		"Fl Oz.": "fl oz",
	}
	for in, want := range cases {
		got := normalizeToken(in)
		if got != want {
			t.Errorf("normalizeToken(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCanonicalDimension(t *testing.T) {
	cases := map[Canonical]Dimension{
		CanonicalML:  DimensionVolume,
		CanonicalG:   DimensionMass,
		CanonicalMM:  DimensionLength,
		CanonicalPcs: DimensionCount,
	}
	for c, want := range cases {
		if got := CanonicalDimension(c); got != want {
			t.Errorf("CanonicalDimension(%s) = %s, want %s", c, got, want)
		}
	}
}

// --- runner ---

func runCases(t *testing.T, cases []tc) {
	t.Helper()
	for _, c := range cases {
		c := c
		t.Run(c.in, func(t *testing.T) {
			opts := ParseOpts{DefaultUnit: c.defaultU}
			got := Parse(c.in, opts)
			if got.Status != c.wantStat {
				t.Fatalf("status: want %s, got %s (note=%q)", c.wantStat, got.Status, got.Note)
			}
			if c.wantStat != ParseStatusOK {
				if c.noteHint != "" && !strings.Contains(got.Note, c.noteHint) {
					t.Errorf("note: want substring %q, got %q", c.noteHint, got.Note)
				}
				return
			}
			if got.Value != c.wantVal {
				t.Errorf("value: want %d, got %d", c.wantVal, got.Value)
			}
			if got.Canonical != c.wantUnit {
				t.Errorf("canonical: want %s, got %s", c.wantUnit, got.Canonical)
			}
			if got.UnitCount != c.wantCount {
				t.Errorf("count: want %d, got %d", c.wantCount, got.UnitCount)
			}
		})
	}
}
