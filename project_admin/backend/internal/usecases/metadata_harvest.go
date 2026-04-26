// Package usecases — Metadata harvester (M4 spec §4.1 step 4).
//
// Pure-Go analysis of the staged Shopify catalog. No LLM, no DB writes apart
// from reads through ShopifyStagingPort. Output is a bounded MetaReport that
// the discovery agent reads as initial context (system prompt) and that
// auto_map_tier1 also consumes for obvious-mapping inference.
package usecases

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

// metaReportFieldCap caps the per-field analysis to keep MetaReport size
// inside the agent's input budget. Spec target ~1-2KB; with ~50 fields
// this leaves ~30 bytes per field on average. Top-10 values per field plus
// stats stays well under that.
const metaReportFieldCap = 80

// metaReportTopValues is the per-field "top N most frequent values"
// rendered into FieldStats.TopValues. Picked for human eyeballing; not a
// statistical sample.
const metaReportTopValues = 10

// metaReportStringTrunc is the max characters we keep per sample value.
// Long descriptions or HTML payloads would otherwise blow the report size.
const metaReportStringTrunc = 80

// MetadataHarvest computes the bounded MetaReport for one tenant by
// scanning the staging table. Safe to call repeatedly — read-only,
// idempotent, no side effects.
type MetadataHarvest struct {
	staging ports.ShopifyStagingPort
	log     *logger.Logger
}

func NewMetadataHarvest(staging ports.ShopifyStagingPort, log *logger.Logger) *MetadataHarvest {
	return &MetadataHarvest{staging: staging, log: log}
}

// Run produces a MetaReport from whatever's currently in staging for the
// tenant. If staging is empty, returns an empty (but non-nil) report; the
// caller decides whether that's an error condition.
func (h *MetadataHarvest) Run(ctx context.Context, tenantID string) (*domain.MetaReport, error) {
	report := &domain.MetaReport{TenantID: tenantID}
	stats := newFieldStatsAccumulator()

	// Pull metadata-kind staging rows first (vendors, types, tags, metafield defs).
	// These are small, single-row blobs.
	if err := h.loadMetadataRows(ctx, tenantID, report); err != nil {
		return nil, fmt.Errorf("load metadata: %w", err)
	}
	if err := h.loadMenuTree(ctx, tenantID, report); err != nil {
		return nil, fmt.Errorf("load menu: %w", err)
	}

	// Iterate products and accumulate per-field stats.
	err := h.staging.IterateProducts(ctx, tenantID, func(_ string, payload json.RawMessage, _ time.Time) error {
		report.TotalProducts++
		var product map[string]json.RawMessage
		if err := json.Unmarshal(payload, &product); err != nil {
			return nil // skip malformed; bulk-op should never produce these
		}
		walkFields("product", "", product, stats)

		// Variants live under _v2_variants (added by streamJSONLToStaging).
		if rawVariants, ok := product["_v2_variants"]; ok {
			var variants []json.RawMessage
			if err := json.Unmarshal(rawVariants, &variants); err == nil {
				for _, vRaw := range variants {
					report.TotalVariants++
					var variant map[string]json.RawMessage
					if err := json.Unmarshal(vRaw, &variant); err == nil {
						walkFields("variant", "", variant, stats)
					}
				}
			}
		}
		// Metafields too — discovery agent cares deeply about these.
		if rawMfs, ok := product["_v2_metafields"]; ok {
			var mfs []json.RawMessage
			if err := json.Unmarshal(rawMfs, &mfs); err == nil {
				for _, mfRaw := range mfs {
					var mf map[string]json.RawMessage
					if err := json.Unmarshal(mfRaw, &mf); err == nil {
						walkFields("metafield", "", mf, stats)
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("iterate products: %w", err)
	}

	report.Fields = stats.finalize(report.TotalProducts)
	return report, nil
}

// loadMetadataRows reads the small "metadata"-kind staging rows produced
// during DumpToStaging — shop_references and metafield_defs. We don't have
// a per-row getter on the port (kept the surface minimal for 4a), so we
// reach into staging via a helper that knows the source_kind+source_id.
//
// Implementation note: rather than add a GetByKey method to the port now
// (which would only be used here), we open a dedicated query through the
// staging port via a temporary IterateAll-style approach. To keep the port
// lean we instead pull the few metadata rows by reusing IterateProducts'
// generality via SQL. Since the port doesn't expose that today, we accept a
// minor inefficiency: we fan out three CountByKind/IterateProducts-style
// calls would be wrong (it iterates products only). Cleanest: add a
// GetMetadata helper to the port. We do that now.
func (h *MetadataHarvest) loadMetadataRows(ctx context.Context, tenantID string, report *domain.MetaReport) error {
	// shop_references row carries vendors / product types / tags as one blob.
	if mp, ok := h.staging.(metadataReader); ok {
		if raw, err := mp.GetMetadata(ctx, tenantID, "shop_references"); err == nil && raw != nil {
			var refs struct {
				ProductVendors []string `json:"productVendors"`
				ProductTypes   []string `json:"productTypes"`
				ProductTags    []string `json:"productTags"`
			}
			if err := json.Unmarshal(raw, &refs); err == nil {
				report.Vendors = refs.ProductVendors
				report.ProductTypes = refs.ProductTypes
				report.Tags = refs.ProductTags
			}
		}
		// metafield definitions, three owner types
		for _, owner := range []string{"product", "productvariant", "collection"} {
			raw, err := mp.GetMetadata(ctx, tenantID, "metafield_defs:"+owner)
			if err != nil || raw == nil {
				continue
			}
			var defs []struct {
				Namespace string `json:"namespace"`
				Key       string `json:"key"`
				Type      string `json:"type"`
				Name      string `json:"name"`
				OwnerType string `json:"ownerType"`
			}
			if err := json.Unmarshal(raw, &defs); err != nil {
				continue
			}
			for _, d := range defs {
				report.MetafieldDefs = append(report.MetafieldDefs, domain.MetafieldDefSummary{
					OwnerType: d.OwnerType,
					Namespace: d.Namespace,
					Key:       d.Key,
					Type:      d.Type,
					Name:      d.Name,
				})
			}
		}
	}
	return nil
}

// loadMenuTree reads the navigation menu staging row and converts it to
// CollectionTree. Nodes whose handle/title matches a showcase/promo pattern
// are tagged accordingly so the artifact mapping can route them correctly.
func (h *MetadataHarvest) loadMenuTree(ctx context.Context, tenantID string, report *domain.MetaReport) error {
	mp, ok := h.staging.(metadataReader)
	if !ok {
		return nil
	}
	raw, err := mp.GetMetadata(ctx, tenantID, "main-menu")
	if err != nil || raw == nil {
		return nil
	}
	var menu struct {
		Items []menuItem `json:"items"`
	}
	if err := json.Unmarshal(raw, &menu); err != nil {
		return nil
	}
	report.CollectionTree = buildCollectionNodes(menu.Items)
	return nil
}

// menuItem mirrors shopify.NavigationMenuItem locally to avoid importing
// the adapter package from a usecase. Field set kept minimal to what we
// actually use; recursion limit matches Shopify (4 levels).
type menuItem struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	URL        string     `json:"url"`
	Type       string     `json:"type"`
	ResourceID string     `json:"resourceId,omitempty"`
	Items      []menuItem `json:"items,omitempty"`
}

func buildCollectionNodes(items []menuItem) []domain.CollectionNode {
	out := make([]domain.CollectionNode, 0, len(items))
	for _, it := range items {
		// Only include menu entries that point at collections. Everything
		// else (pages, blog links, external URLs) isn't a category.
		if !strings.EqualFold(it.Type, "COLLECTION") && it.ResourceID == "" {
			continue
		}
		handle := handleFromURL(it.URL)
		node := domain.CollectionNode{
			ExternalID: it.ResourceID,
			Handle:     handle,
			Title:      it.Title,
			Kind:       classifyCollectionKind(handle, it.Title),
			Children:   buildCollectionNodes(it.Items),
		}
		out = append(out, node)
	}
	return out
}

// handleFromURL extracts "best-sellers" from "/collections/best-sellers".
func handleFromURL(u string) string {
	if u == "" {
		return ""
	}
	if i := strings.LastIndex(u, "/"); i >= 0 && i+1 < len(u) {
		return u[i+1:]
	}
	return u
}

// classifyCollectionKind tags a collection as showcase or promo when the
// handle/title hints at it. Conservative — only obvious patterns. Anything
// ambiguous defaults to "category" and the agent (or curator) can reclassify.
func classifyCollectionKind(handle, title string) string {
	all := strings.ToLower(handle + " " + title)
	switch {
	case strings.Contains(all, "best-sellers"), strings.Contains(all, "best sellers"),
		strings.Contains(all, "featured"), strings.Contains(all, "trending"),
		strings.Contains(all, "new arrivals"), strings.Contains(all, "new-arrivals"),
		strings.Contains(all, "popular"):
		return "showcase"
	case strings.Contains(all, "sale"), strings.Contains(all, "promo"),
		strings.Contains(all, "discount"), strings.Contains(all, "clearance"),
		strings.Contains(all, "outlet"):
		return "promo"
	default:
		return "category"
	}
}

// metadataReader is the optional capability we expect from the staging
// adapter for non-product rows. Decoupled from the main port so adapters
// can grow or shrink this surface without churning the port.
type metadataReader interface {
	GetMetadata(ctx context.Context, tenantID, sourceID string) (json.RawMessage, error)
}

// =============================================================================
// Field stats accumulation
// =============================================================================

type valueAccum struct {
	count   int
	samples map[string]int // value (truncated) → count
	asInt   bool
	asFloat bool
	asBool  bool
	asArray bool
	asJSON  bool
	minNum  float64
	maxNum  float64
	hasNum  bool
	nonEN   int
	totalEN int
}

type fieldStatsAccumulator struct {
	fields map[string]*valueAccum // key = "ownerType:path"
	owners map[string]string      // key → ownerType
	paths  map[string]string      // key → path
}

func newFieldStatsAccumulator() *fieldStatsAccumulator {
	return &fieldStatsAccumulator{
		fields: make(map[string]*valueAccum),
		owners: make(map[string]string),
		paths:  make(map[string]string),
	}
}

// walkFields traverses a JSON object recording every leaf value into the
// accumulator. Path uses dotted notation; arrays become "[]" markers so
// {variants:[{sku:"abc"}]} becomes path "variants.[].sku".
func walkFields(ownerType, prefix string, obj map[string]json.RawMessage, acc *fieldStatsAccumulator) {
	for key, raw := range obj {
		// Skip our own internal helpers.
		if strings.HasPrefix(key, "_v2_") || key == "__parentId" {
			continue
		}
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		walkValue(ownerType, path, raw, acc)
	}
}

func walkValue(ownerType, path string, raw json.RawMessage, acc *fieldStatsAccumulator) {
	r := strings.TrimSpace(string(raw))
	if r == "" || r == "null" {
		return
	}
	switch r[0] {
	case '{':
		var nested map[string]json.RawMessage
		if err := json.Unmarshal(raw, &nested); err == nil {
			walkFields(ownerType, path, nested, acc)
		}
		return
	case '[':
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err == nil {
			recordValue(ownerType, path, raw, true, acc)
			for _, e := range arr {
				walkValue(ownerType, path+".[]", e, acc)
			}
		}
		return
	}
	recordValue(ownerType, path, raw, false, acc)
}

func recordValue(ownerType, path string, raw json.RawMessage, isArrayMarker bool, acc *fieldStatsAccumulator) {
	key := ownerType + ":" + path
	v := acc.fields[key]
	if v == nil {
		v = &valueAccum{samples: make(map[string]int)}
		acc.fields[key] = v
		acc.owners[key] = ownerType
		acc.paths[key] = path
	}
	v.count++

	if isArrayMarker {
		v.asArray = true
		return
	}

	r := string(raw)
	switch {
	case r == "true" || r == "false":
		v.asBool = true
	case looksLikeInt(r):
		v.asInt = true
		if n, err := unmarshalFloat(raw); err == nil {
			v.recordNum(n)
		}
	case looksLikeFloat(r):
		v.asFloat = true
		if n, err := unmarshalFloat(raw); err == nil {
			v.recordNum(n)
		}
	case strings.HasPrefix(r, `"`):
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			s = strings.TrimSpace(s)
			if s == "" {
				return
			}
			truncated := truncateString(s, metaReportStringTrunc)
			v.samples[truncated]++
			v.totalEN++
			if !looksEnglish(s) {
				v.nonEN++
			}
		}
	default:
		v.asJSON = true
	}
}

func (v *valueAccum) recordNum(n float64) {
	if !v.hasNum {
		v.minNum = n
		v.maxNum = n
		v.hasNum = true
		return
	}
	if n < v.minNum {
		v.minNum = n
	}
	if n > v.maxNum {
		v.maxNum = n
	}
}

func (acc *fieldStatsAccumulator) finalize(totalProducts int) []domain.FieldStats {
	keys := make([]string, 0, len(acc.fields))
	for k := range acc.fields {
		keys = append(keys, k)
	}
	// Most-frequent first — gives the agent the most informative fields up top.
	sort.Slice(keys, func(i, j int) bool {
		return acc.fields[keys[i]].count > acc.fields[keys[j]].count
	})
	if len(keys) > metaReportFieldCap {
		keys = keys[:metaReportFieldCap]
	}
	out := make([]domain.FieldStats, 0, len(keys))
	for _, k := range keys {
		v := acc.fields[k]
		fs := domain.FieldStats{
			Name:          lastSegment(acc.paths[k]),
			Path:          acc.paths[k],
			OwnerType:     acc.owners[k],
			Frequency:     v.count,
			InferredType:  inferType(v),
			DistinctCount: len(v.samples),
		}
		if totalProducts > 0 {
			fs.EmptyRate = 1 - float64(v.count)/float64(totalProducts)
			if fs.EmptyRate < 0 {
				fs.EmptyRate = 0
			}
		}
		if v.hasNum {
			min := v.minNum
			max := v.maxNum
			fs.NumericMin = &min
			fs.NumericMax = &max
		}
		if v.totalEN > 0 {
			if float64(v.nonEN)/float64(v.totalEN) > 0.4 {
				fs.Language = "non-en"
			} else {
				fs.Language = "en"
			}
		}
		fs.TopValues = topValues(v.samples)
		out = append(out, fs)
	}
	return out
}

// inferType picks a single label from the accumulator's seen-shapes flags.
// Order matters — a field that ever held an int and ever held a float is
// "float" (numeric tower); int-only stays int. JSON beats array beats bool.
func inferType(v *valueAccum) string {
	switch {
	case v.asJSON:
		return "json"
	case v.asArray:
		return "array"
	case v.asFloat:
		return "float"
	case v.asInt:
		return "int"
	case v.asBool:
		return "bool"
	default:
		return "string"
	}
}

func topValues(m map[string]int) []domain.ValueCount {
	if len(m) == 0 {
		return nil
	}
	all := make([]domain.ValueCount, 0, len(m))
	for k, c := range m {
		all = append(all, domain.ValueCount{Value: k, Count: c})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Count != all[j].Count {
			return all[i].Count > all[j].Count
		}
		return all[i].Value < all[j].Value
	})
	if len(all) > metaReportTopValues {
		all = all[:metaReportTopValues]
	}
	return all
}

// =============================================================================
// Helpers
// =============================================================================

func looksLikeInt(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 && (r == '-' || r == '+') {
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func looksLikeFloat(s string) bool {
	hasDot := false
	for i, r := range s {
		if i == 0 && (r == '-' || r == '+') {
			continue
		}
		if r == '.' {
			if hasDot {
				return false
			}
			hasDot = true
			continue
		}
		if (r < '0' || r > '9') && r != 'e' && r != 'E' && r != '-' {
			return false
		}
	}
	return hasDot
}

func unmarshalFloat(raw json.RawMessage) (float64, error) {
	var n float64
	err := json.Unmarshal(raw, &n)
	return n, err
}

// looksEnglish is a quick-and-dirty ASCII-letter ratio check. Anything below
// 70% ASCII letters in the alphabetic chars is flagged non-English. Doesn't
// catch French/Spanish vs English nuances but reliably catches Cyrillic /
// CJK / Arabic / Hebrew etc., which is what we need for spec §1.14 enforcement.
func looksEnglish(s string) bool {
	letters := 0
	asciiLetters := 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			letters++
			if r < 128 {
				asciiLetters++
			}
		}
	}
	if letters == 0 {
		return true // numeric / punct — let it pass
	}
	return float64(asciiLetters)/float64(letters) >= 0.7
}

func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func lastSegment(path string) string {
	if i := strings.LastIndex(path, "."); i >= 0 {
		return path[i+1:]
	}
	return path
}
