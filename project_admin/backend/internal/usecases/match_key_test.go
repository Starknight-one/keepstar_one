package usecases

import "testing"

func TestNormalizeMatchKey(t *testing.T) {
	cases := []struct {
		name  string
		brand string
		title string
		want  string
	}{
		{"both empty", "", "", ""},
		{"brand only", "The Ordinary", "", "the ordinary"},
		{"name only", "", "Niacinamide 10%", "niacinamide 10"},
		{"basic concat", "Frudia", "Avocado Gel Peel", "frudia avocado gel peel"},
		{"strips punctuation", "Esthetic House (СP-1)", "Bamboo Refresh Face Mist",
			"esthetic house сp1 bamboo refresh face mist"},
		{"collapses multi space", "  The   Ordinary  ", "  Niacinamide  ",
			"the ordinary niacinamide"},
		{"keeps cyrillic letters", "COSRX",
			"Увлажняющий тонер с гиалуроновой кислотой COSRX Hydrium Watery Toner",
			"cosrx увлажняющий тонер с гиалуроновой кислотой cosrx hydrium watery toner"},
		{"hyphenated word becomes joined", "Frudia", "Blueberry Gel-to-Foam",
			"frudia blueberry geltofoam"},
		{"keeps numbers", "Apple", "MacBook Pro 14 M3", "apple macbook pro 14 m3"},
		{"strips emoji", "Brand", "Cute 🌸 Product", "brand cute product"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeMatchKey(tc.brand, tc.title)
			if got != tc.want {
				t.Errorf("NormalizeMatchKey(%q,%q) = %q, want %q",
					tc.brand, tc.title, got, tc.want)
			}
		})
	}
}

func TestNormalizeMatchKey_MatchesSQLBackfill(t *testing.T) {
	// These three are real rows pulled from catalog.master_products after the
	// SQL backfill ran in migration #94. Keeping them as a regression suite
	// guards against drift between the SQL and Go normalizers.
	cases := []struct {
		brand, name, want string
	}{
		{"SALTRAIN", "Squeeze Tube Clamp", "saltrain squeeze tube clamp"},
		{"Purito", "Defence Barrier pH Cleanser", "purito defence barrier ph cleanser"},
		{"The Saem", "Natural Condition Sebum Control Foam",
			"the saem natural condition sebum control foam"},
	}
	for _, tc := range cases {
		got := NormalizeMatchKey(tc.brand, tc.name)
		if got != tc.want {
			t.Errorf("Go normalizer drifted from SQL backfill: brand=%q name=%q got=%q want=%q",
				tc.brand, tc.name, got, tc.want)
		}
	}
}
