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
