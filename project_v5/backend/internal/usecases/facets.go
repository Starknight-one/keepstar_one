package usecases

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/engine"
)

// Data-derived filter facets. Deterministic, no LLM. The builder
// introspects each product's flattened attribute map (typed + tier2 +
// extra, via engine.ProductToMap) and infers a filter dimension per
// attribute — vertical-agnostic: furniture surfaces material/dimensions,
// cosmetics surfaces skin_type/concern, all from the same code. Filters
// are a deterministic layer that sits BESIDE the generative layout; they
// never go through Agent1/Agent2.
//
// Two facet types are produced:
//   - "range" — all values numeric → {min, max}
//   - "enum"  — low-cardinality string/array → {values:[{value,count}]}
//
// Facets are GUIDED: each facet's value set is computed over the products
// matching every OTHER active filter (not its own), so the user can still
// see and change a facet they've already filtered by.

const (
	facetMaxEnumCardinality = 12 // string facets with more distinct values than this are dropped
	facetMinCoveragePct     = 0.5 // an attribute must be present on at least this fraction of items
	facetMaxValuesReturned  = 20  // cap enum values returned to the UI
	facetMaxEnumValueLen    = 40  // values longer than this read as free text, not a filter
)

// facetSkipKeys are attribute keys that are never useful as filters
// (identifiers, free text, media, formatting helpers).
var facetSkipKeys = map[string]bool{
	"id": true, "tenantId": true, "masterProductId": true, "masterVariantId": true,
	"name": true, "displayName": true, "originalName": true, "description": true,
	"sku": true, "gtins": true, "images": true, "heroImage": true,
	"priceFormatted": true, "currency": true, "marketingClaim": true, "tags": true,
	"stockQuantity": true,
}

// facetPriority pins common dimensions to the top of the panel; everything
// else sorts after, alphabetically by label.
var facetPriority = map[string]int{"price": 0, "brand": 1, "category": 2, "rating": 3}

// Facet is one filter dimension shipped to the widget.
type Facet struct {
	Key    string       `json:"key"`
	Label  string       `json:"label"`
	Type   string       `json:"type"`           // "enum" | "range"
	Unit   string       `json:"unit,omitempty"` // e.g. "currency" (value stored in minor units)
	Values []FacetValue `json:"values,omitempty"`
	Min    float64      `json:"min,omitempty"`
	Max    float64      `json:"max,omitempty"`
}

// FacetValue is one selectable value of an enum facet, with the count of
// matching products.
type FacetValue struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// AppliedFilter is one active filter the widget sends back. Enum filters
// carry the selected values (OR within the facet); range filters carry an
// optional min and/or max. The full active set is sent on every change.
type AppliedFilter struct {
	Key    string   `json:"key"`
	Type   string   `json:"type"`
	Values []string `json:"values,omitempty"`
	Min    *float64 `json:"min,omitempty"`
	Max    *float64 `json:"max,omitempty"`
}

// BuildGuidedFacets derives the filter facets from the products, with each
// facet's value set computed over the subset matching every OTHER active
// filter. Pass active=nil for the plain (no-filter) facet set.
func BuildGuidedFacets(products []domain.Product, active []AppliedFilter) []Facet {
	maps := productAttrMaps(products)
	metas := discoverFacets(maps)
	out := make([]Facet, 0, len(metas))
	for _, m := range metas {
		subset := subsetExcept(maps, active, m.key)
		out = append(out, computeFacet(m, subset))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if ri, rj := facetRank(out[i].Key), facetRank(out[j].Key); ri != rj {
			return ri < rj
		}
		return out[i].Label < out[j].Label
	})
	return out
}

// FilterProducts returns the products matching every active filter (AND
// across facets, OR within an enum facet). Empty active = unchanged.
func FilterProducts(products []domain.Product, active []AppliedFilter) []domain.Product {
	if len(active) == 0 {
		return products
	}
	out := make([]domain.Product, 0, len(products))
	for _, p := range products {
		if matchAttrs(engine.ProductToMap(p), active, "") {
			out = append(out, p)
		}
	}
	return out
}

type facetMeta struct {
	key     string
	isRange bool
}

func productAttrMaps(products []domain.Product) []map[string]any {
	out := make([]map[string]any, len(products))
	for i, p := range products {
		out[i] = engine.ProductToMap(p)
	}
	return out
}

// discoverFacets finds which attribute keys are usable filters and whether
// each is a numeric range or a categorical enum, scanning the full set so
// the panel structure stays stable as the user filters.
func discoverFacets(maps []map[string]any) []facetMeta {
	type agg struct {
		present   int
		distinct  map[string]bool
		valCounts map[string]int // per-value product count (for the grouping gate)
		hasStr    bool
		hasNum    bool
		maxLen    int
	}
	stats := map[string]*agg{}
	for _, m := range maps {
		for k, v := range m {
			if facetSkipKeys[k] {
				continue
			}
			ss, nn, ok := tokensForKey(v)
			if !ok {
				continue
			}
			a := stats[k]
			if a == nil {
				a = &agg{distinct: map[string]bool{}, valCounts: map[string]int{}}
				stats[k] = a
			}
			a.present++
			seen := map[string]bool{} // one product counts once per value
			for _, s := range ss {
				a.hasStr = true
				ls := strings.ToLower(s)
				a.distinct[ls] = true
				if len(s) > a.maxLen {
					a.maxLen = len(s)
				}
				if !seen[ls] {
					a.valCounts[ls]++
					seen[ls] = true
				}
			}
			for _, n := range nn {
				a.hasNum = true
				ks := strconv.FormatFloat(n, 'f', -1, 64)
				a.distinct[ks] = true
				if !seen[ks] {
					a.valCounts[ks]++
					seen[ks] = true
				}
			}
		}
	}
	n := len(maps)
	if n == 0 {
		return nil
	}
	out := make([]facetMeta, 0, len(stats))
	for k, a := range stats {
		if float64(a.present)/float64(n) < facetMinCoveragePct {
			continue
		}
		if len(a.distinct) < 2 {
			continue
		}
		// Pure-numeric attribute → range. No cardinality/length/grouping gates.
		if a.hasNum && !a.hasStr {
			out = append(out, facetMeta{key: k, isRange: true})
			continue
		}
		// Enum quality gates: bounded cardinality, not free text, and at
		// least one value actually groups ≥2 items (otherwise it's an
		// identifier-like column, not a useful filter).
		if len(a.distinct) > facetMaxEnumCardinality {
			continue
		}
		if a.maxLen > facetMaxEnumValueLen {
			continue
		}
		if maxCount(a.valCounts) < 2 {
			continue
		}
		out = append(out, facetMeta{key: k, isRange: false})
	}
	return out
}

func maxCount(m map[string]int) int {
	mx := 0
	for _, c := range m {
		if c > mx {
			mx = c
		}
	}
	return mx
}

// computeFacet builds the value domain for one facet over the given subset.
func computeFacet(meta facetMeta, maps []map[string]any) Facet {
	f := Facet{Key: meta.key, Label: humanize(meta.key)}
	if meta.key == "price" {
		f.Unit = "currency"
	}
	if meta.isRange {
		f.Type = "range"
		min, max := math.Inf(1), math.Inf(-1)
		has := false
		for _, m := range maps {
			_, nn, ok := tokensForKey(m[meta.key])
			if !ok {
				continue
			}
			for _, n := range nn {
				has = true
				if n < min {
					min = n
				}
				if n > max {
					max = n
				}
			}
		}
		if has {
			f.Min, f.Max = min, max
		}
		return f
	}

	f.Type = "enum"
	counts := map[string]int{}
	for _, m := range maps {
		ss, nn, ok := tokensForKey(m[meta.key])
		if !ok {
			continue
		}
		seen := map[string]bool{} // count each product once per distinct value
		for _, s := range ss {
			if !seen[s] {
				counts[s]++
				seen[s] = true
			}
		}
		for _, n := range nn {
			k := strconv.FormatFloat(n, 'f', -1, 64)
			if !seen[k] {
				counts[k]++
				seen[k] = true
			}
		}
	}
	vals := make([]FacetValue, 0, len(counts))
	for v, c := range counts {
		vals = append(vals, FacetValue{Value: v, Count: c})
	}
	sort.SliceStable(vals, func(i, j int) bool {
		if vals[i].Count != vals[j].Count {
			return vals[i].Count > vals[j].Count
		}
		return vals[i].Value < vals[j].Value
	})
	if len(vals) > facetMaxValuesReturned {
		vals = vals[:facetMaxValuesReturned]
	}
	f.Values = vals
	return f
}

// subsetExcept returns the attribute maps matching every active filter
// except the one named exceptKey (for guided faceting).
func subsetExcept(maps []map[string]any, active []AppliedFilter, exceptKey string) []map[string]any {
	if len(active) == 0 {
		return maps
	}
	out := make([]map[string]any, 0, len(maps))
	for _, m := range maps {
		if matchAttrs(m, active, exceptKey) {
			out = append(out, m)
		}
	}
	return out
}

// matchAttrs reports whether an attribute map satisfies every active filter
// except the one named exceptKey (pass "" to apply all).
func matchAttrs(attrs map[string]any, active []AppliedFilter, exceptKey string) bool {
	for _, f := range active {
		if f.Key == exceptKey {
			continue
		}
		if !matchOne(attrs, f) {
			return false
		}
	}
	return true
}

func matchOne(attrs map[string]any, f AppliedFilter) bool {
	ss, nn, ok := tokensForKey(attrs[f.Key])
	if !ok {
		return false
	}
	if f.Type == "range" {
		for _, n := range nn {
			if f.Min != nil && n < *f.Min {
				continue
			}
			if f.Max != nil && n > *f.Max {
				continue
			}
			return true
		}
		return false
	}
	// enum — OR across selected values, matched against string or numeric tokens.
	if len(f.Values) == 0 {
		return true
	}
	sel := make(map[string]bool, len(f.Values))
	for _, v := range f.Values {
		sel[strings.ToLower(v)] = true
	}
	for _, s := range ss {
		if sel[strings.ToLower(s)] {
			return true
		}
	}
	for _, n := range nn {
		if sel[strconv.FormatFloat(n, 'f', -1, 64)] {
			return true
		}
	}
	return false
}

// tokensForKey normalises an attribute value into string tokens and/or
// numeric tokens. Numeric-looking strings become numbers. Maps / unknown
// shapes return ok=false (not filterable).
func tokensForKey(v any) (strs []string, nums []float64, ok bool) {
	switch t := v.(type) {
	case nil:
		return nil, nil, false
	case string:
		if t == "" {
			return nil, nil, false
		}
		if n, err := strconv.ParseFloat(t, 64); err == nil {
			return nil, []float64{n}, true
		}
		return []string{t}, nil, true
	case float64:
		return nil, []float64{t}, true
	case int:
		return nil, []float64{float64(t)}, true
	case int64:
		return nil, []float64{float64(t)}, true
	case bool:
		return []string{strconv.FormatBool(t)}, nil, true
	case []string:
		s := make([]string, 0, len(t))
		for _, e := range t {
			if e != "" {
				s = append(s, e)
			}
		}
		if len(s) == 0 {
			return nil, nil, false
		}
		return s, nil, true
	case []interface{}:
		var ss []string
		var nn []float64
		for _, e := range t {
			es, en, eok := tokensForKey(e)
			if eok {
				ss = append(ss, es...)
				nn = append(nn, en...)
			}
		}
		if len(ss) == 0 && len(nn) == 0 {
			return nil, nil, false
		}
		return ss, nn, true
	}
	return nil, nil, false
}

// humanize turns an attribute key (camelCase or snake_case) into a label.
func humanize(key string) string {
	var b strings.Builder
	prevLower := false
	for i, r := range key {
		if r == '_' {
			b.WriteRune(' ')
			prevLower = false
			continue
		}
		if unicode.IsUpper(r) && prevLower {
			b.WriteRune(' ')
		}
		if i == 0 {
			b.WriteRune(unicode.ToUpper(r))
		} else {
			b.WriteRune(r)
		}
		prevLower = unicode.IsLower(r)
	}
	return b.String()
}

func facetRank(key string) int {
	if r, ok := facetPriority[key]; ok {
		return r
	}
	return 100
}
