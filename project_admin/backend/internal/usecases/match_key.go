// Package usecases — match key normalization shared by apply_v2 (Go) and the
// catalog_migrations backfill (SQL). The cascade in MatchOrCreateMaster has
// three deterministic stages:
//
//   1. SKU (lowercased) exact match.
//   2. GTIN (digits-only) exact match.
//   3. normalized_match_key (this function) exact match.
//
// NormalizeMatchKey MUST produce identical output to the Postgres backfill in
// catalog_migrations.go — see that file for the SQL recipe. The character
// classes are unicode-aware (PostgreSQL's `[:alnum:]` matches Cyrillic in the
// Neon collation, so we use \p{L}\p{N} in Go too).
package usecases

import (
	"regexp"
	"strings"
)

var (
	matchKeyStrip    = regexp.MustCompile(`[^\p{L}\p{N}\s]+`)
	matchKeyCollapse = regexp.MustCompile(`\s+`)
)

// NormalizeMatchKey collapses brand + name into a strict-exact lookup key.
//
// Empty result means "no signal" — apply_v2 skips stage 3 of the cascade and
// proceeds to create a new master. A key produced from "" + "" is "".
func NormalizeMatchKey(brand, name string) string {
	combined := brand + " " + name
	combined = matchKeyStrip.ReplaceAllString(combined, "")
	combined = matchKeyCollapse.ReplaceAllString(combined, " ")
	combined = strings.ToLower(combined)
	return strings.TrimSpace(combined)
}
