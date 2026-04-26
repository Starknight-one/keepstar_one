package units

import (
	"math"
	"strconv"
	"strings"
)

// ParseStatus reports the outcome of a parse attempt for an individual field.
// Stored alongside the typed value in master_variants.parse_status JSONB.
type ParseStatus string

const (
	ParseStatusOK            ParseStatus = "ok"
	ParseStatusAmbiguous     ParseStatus = "ambiguous"     // bare number, no unit, no default
	ParseStatusFailed        ParseStatus = "failed"        // non-numeric / unknown unit
	ParseStatusDualMismatch  ParseStatus = "dual_mismatch" // "30ml/200ml" — two values disagree
)

// Result is what the parser returns. Caller writes Value to typed column,
// Raw to *_raw column, Status into parse_status JSONB. UnitCount only set
// for multi-pack (>= 2).
//
// Dimension is what the parser CLASSIFIED the input as. Caller compares this
// to the dimension declared in the mapping artifact (e.g. field "Volume"
// dimension=volume). Mismatch (e.g. parser produces a mass result for a
// volume field) is treated as failed.
type Result struct {
	Value     int          // canonical unit value, e.g. 236 (mL) or 100 (g)
	Canonical Canonical    // mL / g / mm / pcs
	Dimension Dimension    // volume / mass / length / count
	UnitCount int          // 1 unless multi-pack ("2x30ml" → unit_count=2, value=30)
	Raw       string       // exact source string, untouched
	Status    ParseStatus  // ok / ambiguous / failed / dual_mismatch
	Note      string       // optional human-readable explanation when not ok
}

// ParseOpts tunes the parser. DefaultUnit lets the agent normalizer say
// "this field is bare numbers but they always mean ml" via the mapping
// artifact's `default_unit` field — without it, bare numbers fail.
type ParseOpts struct {
	DefaultUnit Canonical // optional; falls back if no unit token in input
	Resolver    AliasResolver // optional; if nil, only the in-code map is consulted
}

// Parse classifies the input string and returns a Result. Empty input → failed.
//
// Patterns recognized (in order of attempt):
//  1. <num><unit>          "30 ml", "236ml", "1L"
//  2. <num>x<num><unit>    "2x30ml", "2 × 500 ml" — multi-pack
//  3. <num><unit>/<num><unit>  "30ml/1 fl oz" — dual label, primary first
//  4. <num> alone          ambiguous unless DefaultUnit set
//  5. anything else        failed
func Parse(input string, opts ParseOpts) Result {
	r := Result{Raw: input}
	s := strings.TrimSpace(input)
	if s == "" {
		r.Status = ParseStatusFailed
		r.Note = "empty"
		return r
	}

	// Normalize fancy multiplication signs and spaces around them.
	s = strings.ReplaceAll(s, "×", "x")
	s = strings.ReplaceAll(s, "✕", "x")

	// Pattern 3: dual label "<num><unit>/<num><unit>". Keep the FIRST as primary,
	// validate the SECOND is within 2% tolerance.
	if i := strings.Index(s, "/"); i > 0 {
		left := strings.TrimSpace(s[:i])
		right := strings.TrimSpace(s[i+1:])
		// Both halves must look like <num><unit> (allow space).
		lr := parseSingle(left, opts)
		rr := parseSingle(right, opts)
		if lr.Status == ParseStatusOK && rr.Status == ParseStatusOK &&
			lr.Canonical == rr.Canonical {
			// Validate tolerance.
			if !withinTolerance(lr.Value, rr.Value, 0.02) {
				lr.Status = ParseStatusDualMismatch
				lr.Note = "dual label mismatch >2%"
			}
			lr.Raw = input
			return lr
		}
		// Fall through — try as a single token.
	}

	// Pattern 2: multi-pack "<num>x<num><unit>" (also "<num> x <num> <unit>")
	if i := indexMultiPackX(s); i > 0 {
		left := strings.TrimSpace(s[:i])
		right := strings.TrimSpace(s[i+1:])
		count, err := strconv.Atoi(left)
		if err == nil && count > 0 {
			inner := parseSingle(right, opts)
			if inner.Status == ParseStatusOK {
				inner.UnitCount = count
				inner.Raw = input
				return inner
			}
		}
	}

	// Pattern 1 & 4 & 5: single token.
	r = parseSingle(s, opts)
	r.Raw = input
	return r
}

// parseSingle handles "<num><unit>" / "<num> <unit>" / bare "<num>".
func parseSingle(s string, opts ParseOpts) Result {
	r := Result{}
	s = strings.TrimSpace(s)
	if s == "" {
		r.Status = ParseStatusFailed
		r.Note = "empty"
		return r
	}
	// Find the boundary between the numeric prefix and the unit suffix.
	numEnd := 0
	for numEnd < len(s) {
		c := s[numEnd]
		if (c >= '0' && c <= '9') || c == '.' || c == ',' {
			numEnd++
			continue
		}
		break
	}
	if numEnd == 0 {
		r.Status = ParseStatusFailed
		r.Note = "no numeric prefix"
		return r
	}

	numStr := s[:numEnd]
	// European comma decimal: "1,5" → "1.5".
	numStr = strings.Replace(numStr, ",", ".", 1)
	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil || num < 0 {
		r.Status = ParseStatusFailed
		r.Note = "bad number"
		return r
	}

	unitStr := strings.TrimSpace(s[numEnd:])
	if unitStr == "" {
		// Bare number — use default_unit if provided.
		if opts.DefaultUnit == "" {
			r.Status = ParseStatusAmbiguous
			r.Note = "bare number, no default unit"
			return r
		}
		r.Value = roundToInt(num)
		r.Canonical = opts.DefaultUnit
		r.Dimension = CanonicalDimension(opts.DefaultUnit)
		r.UnitCount = 1
		r.Status = ParseStatusOK
		return r
	}

	// Resolve unit token. Prefer the resolver (DB) if provided so per-tenant
	// aliases override the in-code map.
	canonical, factor, ok := resolveAlias(unitStr, opts.Resolver)
	if !ok {
		r.Status = ParseStatusFailed
		r.Note = "unknown unit: " + unitStr
		return r
	}

	r.Value = roundToInt(num * factor)
	r.Canonical = canonical
	r.Dimension = CanonicalDimension(canonical)
	r.UnitCount = 1
	r.Status = ParseStatusOK
	return r
}

// resolveAlias consults the optional tenant-aware resolver first, falls back
// to the in-code global map. We need both the canonical unit AND the factor;
// the DB resolver returns the canonical unit, and the factor comes from the
// in-code table (since the table is authoritative for arithmetic).
//
// This means a tenant alias like "tube" → mL only works if "tube" is also in
// rawTokenFactors with factor=1.0. Tenant aliases for novel UNITS (not just
// novel SPELLINGS) require a code change — by design (we don't trust the DB
// to do unit math).
func resolveAlias(token string, resolver AliasResolver) (Canonical, float64, bool) {
	if resolver != nil {
		if c, ok := resolver.Resolve(token); ok {
			// Look up factor from in-code table by canonical unit's own name.
			if a, ok := rawTokenFactors[string(c)]; ok {
				return a.canonical, a.factor, true
			}
			// If the resolver returned a canonical we don't know how to scale by token
			// (rare — e.g. tenant aliases "litre" → mL but "litre" not in our table),
			// look up by the resolved canonical's *own raw_token equivalent*. For mL
			// this is a 1:1 factor; for L (which is also canonical-mapped to mL) the
			// factor is 1000. So we look it up by `string(c)`:
			return c, 1.0, true
		}
	}
	return LookupAlias(token)
}

func roundToInt(f float64) int {
	return int(math.Round(f))
}

func withinTolerance(a, b int, frac float64) bool {
	if a == 0 && b == 0 {
		return true
	}
	if a == 0 || b == 0 {
		return false
	}
	larger := a
	if b > larger {
		larger = b
	}
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return float64(diff)/float64(larger) <= frac
}

// indexMultiPackX returns the index of an "x" that's the multi-pack separator:
// digits + optional space on the LEFT and optional space + digit on the RIGHT.
// This avoids mis-tokenizing words like "extra" while accepting "2x30", "2 x 30",
// and "2 × 30" (× has been pre-replaced with x by Parse).
func indexMultiPackX(s string) int {
	for i := 1; i < len(s)-1; i++ {
		if s[i] != 'x' && s[i] != 'X' {
			continue
		}
		// Walk back over optional whitespace, expect digit.
		l := i - 1
		for l >= 0 && (s[l] == ' ' || s[l] == '\t') {
			l--
		}
		if l < 0 || s[l] < '0' || s[l] > '9' {
			continue
		}
		// Walk forward over optional whitespace, expect digit.
		r := i + 1
		for r < len(s) && (s[r] == ' ' || s[r] == '\t') {
			r++
		}
		if r >= len(s) || s[r] < '0' || s[r] > '9' {
			continue
		}
		return i
	}
	return -1
}
