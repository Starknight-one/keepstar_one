// Package units provides a deterministic state-machine parser for product
// dimension fields (volume, mass, length, count) and a small registry of
// canonical units with conversion factors.
//
// Design rationale (spec §1.12, §5):
//   - Conversion constants live in code, NOT in the DB. A typo in a DB row
//     would silently corrupt every catalog. Code is versioned with the binary
//     and unit-tested.
//   - Aliases (тext-token → canonical-unit) live in the DB so tenants can
//     override per-tenant. See aliases.go.
//   - The parser is stateful and never calls an LLM. The agent normalizer
//     (M4) classifies the dimension of a field ONCE (volume vs mass) and
//     that knowledge is encoded in the mapping artifact's transform field.
package units

// Canonical is the set of canonical SI-style units we store typed values in.
// Every parser result either returns one of these or fails.
type Canonical string

const (
	CanonicalML  Canonical = "mL" // volume — millilitres
	CanonicalG   Canonical = "g"  // mass — grams
	CanonicalMM  Canonical = "mm" // length — millimetres
	CanonicalPcs Canonical = "pcs" // count — pieces
)

// Dimension groups canonical units that are interchangeable for parser
// classification (e.g. "should this field hold a volume or a mass?").
type Dimension string

const (
	DimensionVolume Dimension = "volume"
	DimensionMass   Dimension = "mass"
	DimensionLength Dimension = "length"
	DimensionCount  Dimension = "count"
)

// CanonicalDimension returns which dimension a canonical unit belongs to.
func CanonicalDimension(c Canonical) Dimension {
	switch c {
	case CanonicalML:
		return DimensionVolume
	case CanonicalG:
		return DimensionMass
	case CanonicalMM:
		return DimensionLength
	case CanonicalPcs:
		return DimensionCount
	}
	return ""
}

// conversionFactor returns the multiplier to go from `from` to its canonical
// unit. Returned as float64 for the multiplication step; result is rounded
// to int by callers (since typed columns are INTEGER).
//
// We store conversions only between aliases of the same dimension. The
// alias table maps "L" → CanonicalML with factor 1000, "fl oz" → CanonicalML
// with factor 29.5735, etc. Aliases that are already canonical have factor 1.0.
//
// This factor table is INTERNAL and consumed only by the parser through
// Convert(). The DB alias table just stores `canonical_unit` (the target);
// the factor is determined here based on the raw_token.
//
// We intentionally do NOT support cross-dimension conversion (you can't
// convert "30 g" to mL without density). Such cases are parser failures.
type alias struct {
	canonical Canonical
	factor    float64
}

// rawTokenFactors lookup is keyed by lowercased token. The DB unit_aliases
// table seeds the same set so tenant overrides + global seed agree.
//
// Seed comment in catalog_migrations.go must stay in sync with this map for
// any token where factor != 1.0 (otherwise DB says "fl oz → mL" but parser
// stores 16 instead of 473 for "16 fl oz").
var rawTokenFactors = map[string]alias{
	// Volume → mL (English only — MVP per spec §1.14)
	"ml":          {CanonicalML, 1.0},
	"milliliter":  {CanonicalML, 1.0},
	"milliliters": {CanonicalML, 1.0},
	"l":           {CanonicalML, 1000.0},
	"liter":       {CanonicalML, 1000.0},
	"liters":      {CanonicalML, 1000.0},
	"fl oz":       {CanonicalML, 29.5735},
	"oz":          {CanonicalML, 29.5735}, // ambiguous — could be fluid or weight; we treat bare "oz" as fluid since that's the common cosmetics case
	// Mass → g
	"g":        {CanonicalG, 1.0},
	"gr":       {CanonicalG, 1.0},
	"gram":     {CanonicalG, 1.0},
	"grams":    {CanonicalG, 1.0},
	"kg":       {CanonicalG, 1000.0},
	"kilogram": {CanonicalG, 1000.0},
	"lb":       {CanonicalG, 453.592},
	"lbs":      {CanonicalG, 453.592},
	// Length → mm
	"mm":   {CanonicalMM, 1.0},
	"cm":   {CanonicalMM, 10.0},
	"m":    {CanonicalMM, 1000.0},
	"inch": {CanonicalMM, 25.4},
	"in":   {CanonicalMM, 25.4},
	"\"":   {CanonicalMM, 25.4},
	// Count → pcs
	"pcs":    {CanonicalPcs, 1.0},
	"pieces": {CanonicalPcs, 1.0},
}

// LookupAlias returns the canonical unit + factor for a raw token. Returns
// false if the token is not recognized at all.
//
// This is the IN-CODE path. The DB alias table is consulted by the parser
// FIRST through aliases.go for tenant-specific overrides; this map is the
// global fallback that always works (no DB roundtrip needed for tests).
func LookupAlias(rawToken string) (Canonical, float64, bool) {
	a, ok := rawTokenFactors[normalizeToken(rawToken)]
	if !ok {
		return "", 0, false
	}
	return a.canonical, a.factor, true
}

// normalizeToken trims, lowercases, and squashes inner whitespace runs to a
// single space. "Fl Oz." → "fl oz".
func normalizeToken(s string) string {
	out := make([]byte, 0, len(s))
	prevSpace := false
	for _, r := range s {
		// drop trailing punct that doesn't matter
		if r == '.' || r == ',' {
			continue
		}
		if r == ' ' || r == '\t' {
			if prevSpace || len(out) == 0 {
				continue
			}
			out = append(out, ' ')
			prevSpace = true
			continue
		}
		// lowercase ASCII A-Z. Non-ASCII falls through unchanged: spec is
		// English-only for MVP (§1.14) so unknown unicode tokens are intended
		// to fail lookup rather than silently match.
		if r >= 'A' && r <= 'Z' {
			out = append(out, byte(r+32))
		} else if r < 128 {
			out = append(out, byte(r))
		} else {
			out = appendRune(out, r)
		}
		prevSpace = false
	}
	// rtrim
	for len(out) > 0 && out[len(out)-1] == ' ' {
		out = out[:len(out)-1]
	}
	return string(out)
}

// appendRune is utf8.AppendRune; reproduced minimally here to avoid pulling
// "unicode/utf8" for one call.
func appendRune(b []byte, r rune) []byte {
	if r < 0x80 {
		return append(b, byte(r))
	}
	if r < 0x800 {
		return append(b, byte(0xC0|r>>6), byte(0x80|r&0x3F))
	}
	if r < 0x10000 {
		return append(b, byte(0xE0|r>>12), byte(0x80|(r>>6)&0x3F), byte(0x80|r&0x3F))
	}
	return append(b, byte(0xF0|r>>18), byte(0x80|(r>>12)&0x3F), byte(0x80|(r>>6)&0x3F), byte(0x80|r&0x3F))
}
