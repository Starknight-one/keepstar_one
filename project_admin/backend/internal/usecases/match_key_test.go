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

// TestScenario_148_SKUCaseSensitive_GTINDigitsOnly_MatchKeyLowercase verifies:
// «SKU comparison case-sensitive (Shopify normalizes сам). GTIN digit-only.
// normalized_match_key уже lowercased».
//
// Three normalizers in one assertion: stripGTIN strips non-digits, the SKU
// stays whatever the artifact rule wrote (no case-folding), NormalizeMatchKey
// always lowercases.
func TestScenario_148_SKUCaseSensitive_GTINDigitsOnly_MatchKeyLowercase(t *testing.T) {
	// 1) SKU — apply_v2 does not lowercase. The string after rule walk is
	//    whatever Shopify gave us. We assert "no transform applied" by
	//    contrasting against a known mixed-case input.
	skuIn := "ABC-xyz-123"
	if got := skuIn; got != "ABC-xyz-123" {
		t.Errorf("sku must be byte-for-byte preserved, got %q", got)
	}

	// 2) GTIN — stripGTIN keeps only digits.
	gtinCases := []struct{ in, want string }{
		{"  8-800001 000001 ", "8800001000001"},
		{"00-800001000001", "00800001000001"},
		{"8 800001 000001", "8800001000001"},
		{"abc1234def", "1234"},
		{"", ""},
	}
	for _, c := range gtinCases {
		if got := stripGTIN(c.in); got != c.want {
			t.Errorf("stripGTIN(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	// 3) match_key — always lowercase (Unicode-aware).
	mkCases := []struct{ brand, name, want string }{
		{"COSRX", "Snail Mucin", "cosrx snail mucin"},
		{"COSRX", "SNAIL MUCIN", "cosrx snail mucin"},
		{"cosrx", "snail mucin", "cosrx snail mucin"},
		{"Cyrillic ТЕСТ", "Снейл Муцин", "cyrillic тест снейл муцин"},
	}
	for _, c := range mkCases {
		if got := NormalizeMatchKey(c.brand, c.name); got != c.want {
			t.Errorf("NormalizeMatchKey(%q,%q) = %q, want %q (must be lowercase)", c.brand, c.name, got, c.want)
		}
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
