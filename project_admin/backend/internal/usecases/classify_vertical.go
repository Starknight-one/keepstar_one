// Package usecases — per-row vertical classification.
//
// Apply_v2 calls ClassifyVertical for every inbox item to pick which artifact
// branch to apply. Source of truth is Shopify's product_type field (merchant-
// labeled). When it's empty or unknown we fall back to the artifact's
// classify_rules — string-match heuristics the discovery agent built from
// brand/name patterns (e.g. "brand=Apple → electronics"). If both miss,
// vertical is 'unknown' and apply_v2 routes all attributes to tier3.
//
// Decision cascade (in order):
//   1. Alias lookup on product_type via VerticalAliasLookup port.
//   2. ClassifyRule evaluation in the order the agent committed them.
//   3. "unknown".
//
// Source field on the result lets apply_v2 emit action_log warnings only when
// classification fell through to rules or fallback — alias hits are silent.
package usecases

import (
	"context"
	"strings"

	"keepstar-admin/internal/domain"
)

// ClassifyVerticalSource tells the caller which stage produced the answer.
type ClassifyVerticalSource string

const (
	ClassifySourceAlias   ClassifyVerticalSource = "alias"
	ClassifySourceRule    ClassifyVerticalSource = "rule"
	ClassifySourceUnknown ClassifyVerticalSource = "unknown"
)

// VerticalAliasLookup is the minimal port classify uses to translate
// shopify product_type into a vertical. The postgres adapter implements
// this against catalog.vertical_aliases.
type VerticalAliasLookup interface {
	LookupVertical(ctx context.Context, productType string) (vertical string, found bool, err error)
}

// ClassifyRule re-exports domain.ClassifyRule so this file's docs sit next
// to the evaluator. See domain/mapping_artifact_v3.go for the canonical
// struct.
//
// When syntax (case-insensitive):
//
//	"product_type contains '<substring>'"
//	"brand = '<exact>'"
//	"name contains '<substring>'"
//	"tag = '<exact>'"
//
// Unknown syntax → rule treated as no-match.
type ClassifyRule = domain.ClassifyRule

// ClassifyContext holds the row-level inputs ClassifyVertical consults when
// alias lookup misses. ProductType is consumed by the alias stage already;
// it's repeated here so rules can ALSO match on it (e.g. when the alias
// table doesn't cover a niche product_type but the agent has a contains-rule
// for it).
type ClassifyContext struct {
	ProductType string
	Brand       string
	Name        string
	Tags        []string
}

// ClassifyVertical runs the cascade. Returns (vertical, source) where
// vertical is "" only when source is "unknown" AND apply_v2 wants to log
// the fallthrough; callers should never pass "" to MatchOrCreateMaster
// without first defaulting it to "unknown".
func ClassifyVertical(ctx context.Context, lookup VerticalAliasLookup, rules []ClassifyRule, c ClassifyContext) (string, ClassifyVerticalSource) {
	pt := strings.TrimSpace(c.ProductType)

	// Stage 1: alias on product_type.
	if pt != "" && lookup != nil {
		if vertical, found, err := lookup.LookupVertical(ctx, pt); err == nil && found {
			return vertical, ClassifySourceAlias
		}
	}

	// Stage 2: classify_rules in artifact order.
	for _, rule := range rules {
		if matchesRule(rule.When, c) && rule.ThenVertical != "" {
			return rule.ThenVertical, ClassifySourceRule
		}
	}

	// Stage 3: unknown.
	return "unknown", ClassifySourceUnknown
}

// matchesRule evaluates the simple When DSL against the row context.
// Returns false on parse failure — surfacing parse errors at row-time
// would spam the log; instead the artifact build pass validates rule
// syntax once.
func matchesRule(when string, c ClassifyContext) bool {
	w := strings.TrimSpace(strings.ToLower(when))
	if w == "" {
		return false
	}

	// "product_type contains 'X'"
	if rest, ok := stripPrefix(w, "product_type contains "); ok {
		needle, ok := unquote(rest)
		if !ok || needle == "" {
			return false
		}
		return strings.Contains(strings.ToLower(c.ProductType), needle)
	}

	// "brand = 'X'"
	if rest, ok := stripPrefix(w, "brand = "); ok {
		want, ok := unquote(rest)
		if !ok || want == "" {
			return false
		}
		return strings.EqualFold(strings.TrimSpace(c.Brand), want)
	}

	// "name contains 'X'"
	if rest, ok := stripPrefix(w, "name contains "); ok {
		needle, ok := unquote(rest)
		if !ok || needle == "" {
			return false
		}
		return strings.Contains(strings.ToLower(c.Name), needle)
	}

	// "tag = 'X'"
	if rest, ok := stripPrefix(w, "tag = "); ok {
		want, ok := unquote(rest)
		if !ok || want == "" {
			return false
		}
		for _, tag := range c.Tags {
			if strings.EqualFold(strings.TrimSpace(tag), want) {
				return true
			}
		}
		return false
	}

	return false
}

func stripPrefix(s, prefix string) (string, bool) {
	if strings.HasPrefix(s, prefix) {
		return strings.TrimSpace(s[len(prefix):]), true
	}
	return "", false
}

// unquote pulls the inner string out of a single- or double-quoted literal.
// Returns false when the input is not a balanced quote.
func unquote(s string) (string, bool) {
	if len(s) < 2 {
		return "", false
	}
	q := s[0]
	if (q != '\'' && q != '"') || s[len(s)-1] != q {
		return "", false
	}
	return s[1 : len(s)-1], true
}
