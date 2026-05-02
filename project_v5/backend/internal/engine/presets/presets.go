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
// Five named groups (hero / info / meta / actions / specs) so the future
// canvas user can rearrange or hide entire sections without breaking
// fieldBindings on individual atoms. The outer "card" Frame carries
// replicate:true so ExpandReplicates fans it out per data record.
//
//go:embed seed/product_card.json
var ProductCardJSON []byte
