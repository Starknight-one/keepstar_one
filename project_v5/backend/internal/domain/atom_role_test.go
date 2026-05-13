package domain

import "testing"

// TestAtomEnumsAreStringTyped guards against accidental type breakage —
// FieldDefinition stores AtomType/Subtype/Slot/Format and they need to
// round-trip through DB strings without coercion.
func TestAtomEnumsAreStringTyped(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"AtomTypeText", string(AtomTypeText), "text"},
		{"AtomTypeNumber", string(AtomTypeNumber), "number"},
		{"AtomTypeImage", string(AtomTypeImage), "image"},
		{"SubtypeCurrency", string(SubtypeCurrency), "currency"},
		{"SubtypeRating", string(SubtypeRating), "rating"},
		{"AtomSlotHero", string(AtomSlotHero), "hero"},
		{"AtomSlotPrice", string(AtomSlotPrice), "price"},
		{"FormatStars", string(FormatStars), "stars"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
}
