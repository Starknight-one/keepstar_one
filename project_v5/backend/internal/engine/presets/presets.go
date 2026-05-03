// Package presets ships the V5 seed presets as embedded JSON. These are
// the bootstrap values for tenants who haven't authored anything in the
// (future) v9-canvas microservice yet — they're seeded into v5_presets via
// the preset_seed_test fixture or, eventually, a setup tool.
//
// The JSON shape MUST round-trip through engine.Document — verified by
// TestProductCardSeedRoundTrip.
package presets

import _ "embed"

// ProductCardJSON is the v9-style scene-graph preset for a product card.
// Chunk 5 refactored it to consume two reusable components via Refs
// (price-rating-root, brand-badge-root) — the "card" Frame keeps
// replicate:true and inline atoms for the hero image and title; the
// remaining meta and brand surfaces are RefNodes pointing into components.
//
//go:embed seed/product_card.json
var ProductCardJSON []byte

// ProductCardListRowJSON is a horizontal list-row variant used to validate
// that v9 RefNode reuse actually works across structurally distinct
// presets. Same component refs as ProductCardJSON, plus a description
// atom unique to this layout.
//
//go:embed seed/product_card_list_row.json
var ProductCardListRowJSON []byte

// ComponentPriceRatingJSON is a multi-atom v9 component (price + rating
// in a row) that both product card variants reference via their meta slot.
//
//go:embed seed/component_price_rating.json
var ComponentPriceRatingJSON []byte

// ComponentBrandBadgeJSON is a single-atom + wrapper component that both
// product card variants reference for the brand surface.
//
//go:embed seed/component_brand_badge.json
var ComponentBrandBadgeJSON []byte

// Chunk 9 — additional system presets seeded into the in-process
// SystemPresetRegistry (DB miss → registry fallback). The names match
// the catalog the Agent2 system prompt advertises so any LLM ask for
// product_detail / empty_not_found / etc. resolves without a tenant
// having to author them in the (future) v9-canvas microservice.

//go:embed seed/product_card_compact.json
var ProductCardCompactJSON []byte

//go:embed seed/product_card_horizontal.json
var ProductCardHorizontalJSON []byte

//go:embed seed/product_detail.json
var ProductDetailJSON []byte

//go:embed seed/product_detail_horizontal.json
var ProductDetailHorizontalJSON []byte

//go:embed seed/text_explainer.json
var TextExplainerJSON []byte

//go:embed seed/empty_not_found.json
var EmptyNotFoundJSON []byte

//go:embed seed/error_generic.json
var ErrorGenericJSON []byte

// SystemPresetSeeds maps the public preset name to its embedded JSON
// body. All entries here are served by SystemPresetRegistry as a
// DB-fallback for any tenant. ProductCard variants share the two
// existing components (price-rating-root, brand-badge-root) so
// Materialise + ResolveAndInline pull them in via the tenant's
// component table the same way as the chunk-5 micropresets.
//
// Keys must match the names listed in agent2_prompt.go PRESETS section,
// otherwise the LLM picks names the registry cannot serve.
var SystemPresetSeeds = map[string][]byte{
	"product_card":              ProductCardJSON,
	"product_card_compact":      ProductCardCompactJSON,
	"product_card_horizontal":   ProductCardHorizontalJSON,
	"product_card_list_row":     ProductCardListRowJSON,
	"product_detail":            ProductDetailJSON,
	"product_detail_horizontal": ProductDetailHorizontalJSON,
	"text_explainer":            TextExplainerJSON,
	"empty_not_found":           EmptyNotFoundJSON,
	"error_generic":             ErrorGenericJSON,
}

// SystemPresetDefaultReplicate captures the default-replicate behaviour
// the registry advertises in domain.Preset.DefaultReplicate. Detail
// presets and system one-offs default to false; product cards default
// to true.
var SystemPresetDefaultReplicate = map[string]bool{
	"product_card":              true,
	"product_card_compact":      true,
	"product_card_horizontal":   true,
	"product_card_list_row":     true,
	"product_detail":            false,
	"product_detail_horizontal": false,
	"text_explainer":            false,
	"empty_not_found":           false,
	"error_generic":             false,
}
