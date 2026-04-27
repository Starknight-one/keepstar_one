// Package usecases — deterministic classifiers for collection.kind and product
// vertical (catalog completion 2026-04-28, Phase A5).
//
// Two pure functions, no IO:
//
//   ClassifyKind     — given a collection name, decide whether it's a real
//                      taxonomy node ("Cleansers"), a marketing showcase
//                      ("Best Sellers"), or a promo ("Sale 30%"). The kind
//                      determines whether we map it to master_categories.
//
//   ClassifyVertical — given productType / vendor / tags, infer the product
//                      vertical (cosmetics / furniture / footwear / unknown).
//                      Used by harvester-lite when stamping new attribute /
//                      category candidates so curator UI can group them.
//
// Both classifiers are intentionally simple keyword matchers. They produce
// the OBVIOUS answer; for grey-zone cases we fall through to "category" /
// "unknown" and let the curator (or, later, the merge agent) decide.
//
// Why deterministic and not LLM: speed + cost + audit. Running an LLM at
// import time on every collection name multiplies cost per tenant by 100x
// for trivial classifications, and the LLM doesn't help on the easy cases
// (a string with "%" in the title is promo, full stop). The grey zone is
// what the merge agent is for.
package usecases

import (
	"strings"

	"keepstar-admin/internal/domain"
)

// promoSignals matches collection names that announce a sale or discount.
// Mixed English / Russian since heybabes/dev-store have both.
var promoSignals = []string{
	"sale", "discount", "deal", "off ", "% off", "%off",
	"clearance", "outlet", "promo", "promotion",
	"акция", "скидк", "распродаж", "ликвидац",
}

// showcaseSignals matches collection names that highlight a curated subset
// (best sellers, new arrivals, editor's picks). These don't map to master
// taxonomy — they're marketing.
var showcaseSignals = []string{
	"best ", "best-", "bestseller", "bestsellers",
	"top ", "top-",
	"featured", "popular", "trending",
	"new arriv", "new-arriv", "new in", "just in",
	"editor", "staff pick", "must have", "must-have",
	"хит", "новинк", "топ ",
}

// ClassifyKind returns the kind for a collection name. Default is Category.
func ClassifyKind(name string) domain.TenantCategoryKind {
	if name == "" {
		return domain.TenantCategoryKindCategory
	}
	lc := strings.ToLower(name)

	// % anywhere is a strong promo signal ("Sale 30%", "-50% off").
	if strings.Contains(lc, "%") {
		return domain.TenantCategoryKindPromo
	}
	for _, p := range promoSignals {
		if strings.Contains(lc, p) {
			return domain.TenantCategoryKindPromo
		}
	}
	for _, s := range showcaseSignals {
		if strings.Contains(lc, s) {
			return domain.TenantCategoryKindShowcase
		}
	}
	return domain.TenantCategoryKindCategory
}

// Vertical names. We keep the set small — adding a vertical means adding a
// keyword list here AND deciding what Tier-2 fields it carries. Both are
// curator concerns, so the source of truth lives in catalog.field_definitions
// (post-promotion); this file only seeds the at-import classification.
const (
	VerticalCosmetics = "cosmetics"
	VerticalFurniture = "furniture"
	VerticalFootwear  = "footwear"
	VerticalUnknown   = "unknown"
)

var verticalKeywords = []struct {
	vertical string
	terms    []string
}{
	{VerticalCosmetics, []string{
		"cream", "lotion", "serum", "cleanser", "toner", "moisturiz",
		"sunscreen", "spf", "mask", "balm", "scrub", "essence",
		"skincare", "skin care", "makeup", "make-up", "cosmet",
		"perfume", "fragrance", "shampoo", "conditioner",
		"hair care", "haircare", "lipstick", "mascara", "eyeshadow",
	}},
	{VerticalFurniture, []string{
		"chair", "sofa", "couch", "table", "desk", "bed",
		"dresser", "wardrobe", "bookshelf", "shelving", "shelf",
		"lamp", "lighting", "ottoman", "armchair", "stool", "bench",
		"furniture",
	}},
	{VerticalFootwear, []string{
		"shoe", "sneaker", "trainer", "boot", "sandal", "heel",
		"loafer", "slipper", "footwear", "trail runner", "running shoe",
	}},
}

// ClassifyVertical inspects productType + vendor + tags and returns the
// best-guess vertical. Returns VerticalUnknown if nothing matches — caller
// stores candidates under "unknown" and curator re-classifies them later.
func ClassifyVertical(productType, vendor string, tags []string) string {
	hay := strings.ToLower(strings.Join(append([]string{productType, vendor}, tags...), " | "))
	if hay == " | " {
		return VerticalUnknown
	}
	for _, group := range verticalKeywords {
		for _, term := range group.terms {
			if strings.Contains(hay, term) {
				return group.vertical
			}
		}
	}
	return VerticalUnknown
}
