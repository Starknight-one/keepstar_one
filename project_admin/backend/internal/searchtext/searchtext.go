// Package searchtext assembles the denormalized, vertical-agnostic text used
// for catalog search (embeddings + full-text). It is pure (no DB, no domain
// imports) so it can be reused by the projection builder and CLIs and unit
// tested in isolation.
package searchtext

import (
	"sort"
	"strconv"
	"strings"
)

// maxSearchText caps the combined embedding/tsv input so embedding token cost
// stays bounded on long descriptions/tier3 blobs.
const maxSearchText = 2000

// ProjectionSource is the raw input for one (listing, variant, master) row.
// jsonb columns arrive as decoded map[string]any (nil when empty).
type ProjectionSource struct {
	// master (tier 1 identity)
	Name        string
	Brand       string
	Description string
	Vertical    string
	// master typed/overflow attributes (post-Group-D these hold all vertical attrs)
	Tier2 map[string]any
	Tier3 map[string]any
	// variant axes
	Color    string
	Size     string
	Material string
	VolumeML *int
	WeightG  *int
	Axes     map[string]any
	// listing
	Overrides map[string]any
}

// ProjectionTexts is the per-column output plus the combined search text.
type ProjectionTexts struct {
	Tier1Text    string
	Tier2Text    string
	Tier3Text    string
	VariantText  string
	OverrideText string
	SearchText   string
}

// BuildProjectionTexts produces the five *_text columns and a combined,
// length-capped search text. Vertical-agnostic: nothing here is cosmetics
// specific — typed attributes are read generically from tier2/tier3/axes.
func BuildProjectionTexts(src ProjectionSource) ProjectionTexts {
	t := ProjectionTexts{
		Tier1Text:    joinNonEmpty(" ", src.Name, src.Brand, src.Description),
		Tier2Text:    flattenJSONB(src.Tier2),
		Tier3Text:    flattenJSONB(src.Tier3),
		VariantText:  buildVariantText(src),
		OverrideText: flattenJSONB(src.Overrides),
	}
	combined := joinNonEmpty(" ", t.Tier1Text, t.Tier2Text, t.VariantText, t.Tier3Text, t.OverrideText)
	t.SearchText = capRunes(strings.Join(strings.Fields(combined), " "), maxSearchText)
	return t
}

func buildVariantText(src ProjectionSource) string {
	parts := []string{src.Color, src.Size, src.Material}
	if src.VolumeML != nil {
		parts = append(parts, strconv.Itoa(*src.VolumeML)+"ml")
	}
	if src.WeightG != nil {
		parts = append(parts, strconv.Itoa(*src.WeightG)+"g")
	}
	parts = append(parts, flattenJSONB(src.Axes))
	return joinNonEmpty(" ", parts...)
}

// flattenJSONB walks a decoded jsonb map and emits "key value" tokens in
// stable key order (deterministic so the tsvector is stable across rebuilds).
// Nested maps recurse; arrays are flattened; scalars are stringified.
func flattenJSONB(m map[string]any) string {
	if len(m) == 0 {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		v := flattenValue(m[k])
		if v == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(k)
		b.WriteByte(' ')
		b.WriteString(v)
	}
	return b.String()
}

func flattenValue(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case map[string]any:
		return flattenJSONB(x)
	case []any:
		parts := make([]string, 0, len(x))
		for _, e := range x {
			if s := flattenValue(e); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " ")
	default:
		return ""
	}
}

func joinNonEmpty(sep string, parts ...string) string {
	out := parts[:0:0]
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return strings.Join(out, sep)
}

func capRunes(s string, max int) string {
	if max <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
