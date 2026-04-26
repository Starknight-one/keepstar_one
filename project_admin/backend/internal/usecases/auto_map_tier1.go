// Package usecases — Tier 1 auto-mapping (M4 spec §4.1 step 5).
//
// Pure-Go heuristic that turns the obvious Shopify-field → master-field
// pairings into FieldMappingTarget entries before the discovery agent runs.
// The agent sees these as "already done" and concentrates its budget on the
// ambiguous fields. Anything we miss here lands in the agent's backlog.
//
// Heuristics are intentionally narrow:
//   - exact (case-insensitive) name match against a known synonym set
//   - numeric+unit detection routed through the units parser (M3) for
//     weight / volume
//   - a small set of barcode synonyms route to gtins
//   - collection.title → tenant_categories with handle-pattern kind classification
//
// Anything fuzzier (e.g. "How does an Asian-style Skincare brand map their
// custom 'Hydration Index' field?") is the agent's job, not ours.
package usecases

import (
	"strings"
	"time"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/units"
)

// brandSynonyms covers every Shopify field name we've seen merchants use for
// "the manufacturer / brand label". Comparison is case-insensitive after
// trim — keep entries lowercase.
var brandSynonyms = map[string]struct{}{
	"vendor": {}, "brand": {}, "manufacturer": {}, "maker": {},
	"producer": {}, "company": {}, "label": {},
}

// gtinSynonyms — barcode-like identifiers. EAN/UPC/ISBN are all GTIN
// variants that fit in the gtins[] slot (which is intentionally typed as
// TEXT[] so we can store all of them under one master variant).
var gtinSynonyms = map[string]struct{}{
	"barcode": {}, "gtin": {}, "upc": {}, "ean": {}, "ean13": {},
	"isbn": {}, "asin": {}, "mpn": {}, // mpn debatable but useful — agent can downgrade
}

// weightSynonyms covers the field names that deserve unit-parser routing.
// Numeric value alone isn't enough (Shopify stores weight as float + a
// separate weightUnit field) — we only emit the mapping; the harvester
// (4d) wires the actual parse step.
var weightSynonyms = map[string]struct{}{
	"weight": {}, "shipping_weight": {}, "shippingweight": {}, "net_weight": {},
}

var volumeSynonyms = map[string]struct{}{
	"volume": {}, "capacity": {}, "size": {}, "fluid_volume": {},
}

// AutoMapTier1Result captures what the heuristic decided and what stayed
// unmapped. The unmapped slice is what the agent gets explicitly told to
// look at — no point re-asking it about Vendor.
type AutoMapTier1Result struct {
	FieldMapping    map[string]domain.FieldMappingTarget    `json:"fieldMapping"`
	CategoryMapping map[string]domain.CategoryMappingTarget `json:"categoryMapping"`
	UnmappedFields  []string                                `json:"unmappedFields"`
	GeneratedAt     time.Time                               `json:"generatedAt"`
}

// AutoMapTier1 runs the deterministic heuristics over a MetaReport and
// returns a partial mapping artifact. Caller merges this with the agent's
// output before persisting (4c).
//
// Keeps `units` as a parameter so tests can inject a stub resolver, even
// though we don't actually call Parse here yet (parse happens at harvest
// time when we know the value). The check is: if a weight/volume field
// surfaces, mark its transform as the unit-parser hook so the harvester
// knows to call units.Parse(rawValue).
func AutoMapTier1(report *domain.MetaReport, _ units.AliasResolver) AutoMapTier1Result {
	res := AutoMapTier1Result{
		FieldMapping:    make(map[string]domain.FieldMappingTarget),
		CategoryMapping: make(map[string]domain.CategoryMappingTarget),
		GeneratedAt:     time.Now().UTC(),
	}
	if report == nil {
		return res
	}

	for _, fs := range report.Fields {
		name := strings.ToLower(fs.Name)
		path := fs.Path
		switch {
		case isBrand(name, fs.OwnerType):
			res.FieldMapping[fs.Path] = domain.FieldMappingTarget{
				Target: "master.brand",
			}
		case isGTIN(name):
			res.FieldMapping[fs.Path] = domain.FieldMappingTarget{
				Target: "master_variants.gtins[]",
			}
		case isWeight(name):
			res.FieldMapping[fs.Path] = domain.FieldMappingTarget{
				Target:    "master_variants.weight_g",
				Transform: "units.weight",
			}
		case isVolume(name) && fs.OwnerType != "product":
			// Skip product.size — it's apparel-style ("M" / "L"), not volume.
			// The agent handles ambiguous "size" cases by inspecting samples.
			res.FieldMapping[fs.Path] = domain.FieldMappingTarget{
				Target:    "master_variants.volume_ml",
				Transform: "units.volume",
			}
		default:
			// Skip non-English fields entirely (English-only MVP).
			if fs.Language == "non-en" {
				continue
			}
			res.UnmappedFields = append(res.UnmappedFields, path)
		}
	}

	// Categories — collection tree → tenant_categories with kind classification.
	for _, root := range report.CollectionTree {
		flattenCollections(root, "", res.CategoryMapping)
	}

	return res
}

func isBrand(name, ownerType string) bool {
	if ownerType == "metafield" {
		return false // metafields named "brand" are usually free-form custom data
	}
	_, ok := brandSynonyms[name]
	return ok
}

func isGTIN(name string) bool {
	_, ok := gtinSynonyms[name]
	return ok
}

func isWeight(name string) bool {
	_, ok := weightSynonyms[name]
	return ok
}

func isVolume(name string) bool {
	_, ok := volumeSynonyms[name]
	return ok
}

// flattenCollections walks the tree depth-first and emits one mapping entry
// per node. The mapping target stays empty for now — actual master_category
// linkage happens via the discovery agent (or curator review in M11). What
// we do nail down here is `kind` (category / showcase / promo) since that's
// pure pattern-matching with no judgment call.
func flattenCollections(node domain.CollectionNode, parentPath string, out map[string]domain.CategoryMappingTarget) {
	key := node.ExternalID
	if key == "" {
		key = node.Handle
	}
	if key == "" {
		key = node.Title
	}
	if key != "" {
		out[key] = domain.CategoryMappingTarget{
			TenantLabel: node.Title,
			Kind:        node.Kind,
			// Target stays empty — agent / curator decides master mapping.
		}
	}
	path := parentPath
	if node.Title != "" {
		if path != "" {
			path = path + " / " + node.Title
		} else {
			path = node.Title
		}
	}
	for _, child := range node.Children {
		flattenCollections(child, path, out)
	}
}
