// Package usecases — Junk variant detector (M4 spec §4.7).
//
// Heuristic that flags variants likely to be addons rather than real products
// (gift wrap, engraving, warranty, protection plan, etc). The harvester (4d)
// runs this on every Shopify variant; ≥2 signals push the variant into
// catalog.tenant_variant_candidates_junk for triage in the Detected Add-ons
// page (M9 / curator UI).
//
// Pure function — no port, no I/O. Deterministic for the same input. Useful
// to test in isolation with table-driven cases.
package usecases

import (
	"regexp"
	"strings"

	"keepstar-admin/internal/domain"
)

// junkAxisRE catches the common addon names that show up as Shopify variant
// option values. Word-boundaries keep "warranty bag" (a real product) from
// matching while "warranty service" still does. Add patterns sparingly —
// false positives push real products into the junk queue and annoy curators.
var junkAxisRE = regexp.MustCompile(`(?i)\b(gift\s*wrap|engraving|engrave|warranty|insurance|add[\s\-]?on|protection\s*plan|service\s*plan|extended\s*warranty)\b`)

// JunkDetectorInput is the variant context the detector needs. Kept narrow:
// we don't need the whole variant struct, just the four signal sources.
//
// ParentMinPrice is the minimum price among the variant's siblings on the
// same product (for the "small price delta" heuristic). When the variant is
// the only one or sibling prices are unknown, pass 0 — that signal then
// won't fire and the heuristic falls back on the other three.
type JunkDetectorInput struct {
	AxisName       string  // option-value like "Gift wrap" / "Yes" / "Engraving: Custom text"
	HasGTIN        bool
	HasSKU         bool
	HasWeight      bool
	HasVolume      bool
	PriceCents     int
	ParentMinPrice int // cheapest variant on the same product, in cents
}

// DetectJunk returns the list of signals that fired. Caller decides what to
// do with the result; the spec rule of thumb is ≥2 signals = pending junk.
//
// Returning the signals (not just a bool) lets the persistence layer record
// detected_reason JSONB for curator UI to show "why we flagged this".
func DetectJunk(in JunkDetectorInput) []domain.JunkSignal {
	signals := make([]domain.JunkSignal, 0, 4)

	if in.AxisName != "" && junkAxisRE.MatchString(in.AxisName) {
		signals = append(signals, domain.JunkSignalAxisNamePattern)
	}

	if !in.HasGTIN && !in.HasSKU {
		signals = append(signals, domain.JunkSignalNoIdentifiers)
	}

	if !in.HasWeight && !in.HasVolume {
		signals = append(signals, domain.JunkSignalNoDimensions)
	}

	// Small absolute price: addon variants (gift wrap, engraving, sample
	// sizes, $1 upgrades) are typically very cheap regardless of where they
	// sit relative to siblings. Spec §4.7 names this "small price delta" but
	// the literal pseudocode (delta<$10 AND price<$50) misses gift wrap
	// priced as a standalone $5 variant of a $30 product (delta=$25). We
	// widen to: variant price < $10 absolute. The real signal is that the
	// variant is too cheap to be the actual product.
	//
	// PriceCents == 0 is treated as "unknown" and the signal doesn't fire —
	// some Shopify variants legitimately have $0 listed (e.g. when sold at
	// the parent product's price level).
	if in.PriceCents > 0 && in.PriceCents < 1000 {
		signals = append(signals, domain.JunkSignalSmallPriceDelta)
	}

	return signals
}

// IsJunk is a convenience wrapper around DetectJunk that applies the spec's
// "≥2 signals" rule. Use DetectJunk directly when you also need the signal
// list for storage (which the harvester does).
func IsJunk(in JunkDetectorInput) bool {
	return len(DetectJunk(in)) >= 2
}

// SignalReasonMap converts a signal slice to the JSONB-friendly map shape
// stored in tenant_variant_candidates_junk.detected_reason. Keys are the
// signal names; values are short human descriptors that can be rendered
// directly as badges in the curator UI.
func SignalReasonMap(signals []domain.JunkSignal, axisName string) map[string]string {
	out := make(map[string]string, len(signals))
	for _, s := range signals {
		switch s {
		case domain.JunkSignalAxisNamePattern:
			out[string(s)] = strings.TrimSpace(axisName)
		case domain.JunkSignalNoIdentifiers:
			out[string(s)] = "no SKU and no barcode"
		case domain.JunkSignalNoDimensions:
			out[string(s)] = "no weight and no volume"
		case domain.JunkSignalSmallPriceDelta:
			out[string(s)] = "small price delta from cheapest sibling"
		}
	}
	return out
}
