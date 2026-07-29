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
// The 2026-06-15 redesign rebuilt it on the renderer's capability layer: a
// decorated "card" Frame (fill/cornerRadius/stroke/effect/clip, replicate
// true), a hero frame holding the image plus absolutely-positioned overlays
// (optional badge pill, like-slot, add-slot), and an info frame with the
// title, an inline price+rating row, and plain muted brand text. It no
// longer references the price-rating-root / brand-badge-root components
// (those stay for the list-row preset); like→like-slot and cart_add→add-slot
// are wired via acceptsAction slots (see inject_actions.go).
//
//go:embed seed/product_card.json
var ProductCardJSON []byte

// ProductCardListRowJSON is a horizontal list-row variant used to validate
// that v9 RefNode reuse actually works across structurally distinct
// presets. Still references the shared components (price-rating-root,
// brand-badge-root), plus a description atom unique to this layout.
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

// Component-vocabulary PR1 presets — carousel and comparison exercise the
// widget's horizontal scroll-strip substrate (layout.scroll:"x") and the
// line divider leaf; the accordion detail exercises collapsible frames
// (collapsible:true + defaultOpen, first child renders as the header).

//go:embed seed/product_carousel.json
var ProductCarouselJSON []byte

//go:embed seed/product_comparison.json
var ProductComparisonJSON []byte

//go:embed seed/product_detail_accordion.json
var ProductDetailAccordionJSON []byte

// Interface-runtime v1 presets (RUNTIME_SPEC.md §5.3, R20) — the seeds the
// three forms (storefront / crm / onboarding) compose beyond the product
// catalog. Binding vocabularies are cross-lane contracts:
//
//   - operation_card replicates over the SYNTHETIC `opCard` EntitySet (R23)
//     built from search_library hits: EntityToMap camelCases the LibraryHit
//     fields + card content → title, kind, description, inputSummary, does,
//     outputSummary, why.
//   - lead_table / lead_detail bind the EntityToMap vocabulary of the demo
//     lead definition (exec_demo_bundle.go): name, phone, email, message,
//     preferredTimeFormatted, refTitle, statusLabel, createdAtFormatted, id.
//   - booking_form binds the ProductToMap vocabulary of the listing it is
//     drilled from (hidden listingId input ← fieldBinding "id", §5.2).
//   - success_plaque binds resultBindData (handler_operations.go): summary
//     (+ operation / outcome / recordId / entityKind available).
//   - design_system_preview renders the DefaultThemeTokens values as static
//     content — byte-equal to what the create_tenant applier writes (R22),
//     so the preview never lies about the applied tokens.
//   - uploader_card ships `disarmed:true` (R25); the armed beat-3 re-render
//     flips it and binds the minted ingest token via render-time ops.
//   - registration_form submits via form_submit (R6) — credentials never
//     enter the operation registry; the seed's endpoint assumes the
//     register_user step id, override via ops when step ids differ.
//
// Form presets (registration_form / booking_form / lead_detail) carry a
// `formId` frame + §5.2 form primitives and deliberately contain NO
// "actions"-id frames or acceptsAction slots, so InjectDefaultActions
// stays a no-op on them.

//go:embed seed/design_system_preview.json
var DesignSystemPreviewJSON []byte

//go:embed seed/uploader_card.json
var UploaderCardJSON []byte

//go:embed seed/operation_card.json
var OperationCardJSON []byte

//go:embed seed/registration_form.json
var RegistrationFormJSON []byte

//go:embed seed/booking_form.json
var BookingFormJSON []byte

//go:embed seed/lead_table.json
var LeadTableJSON []byte

//go:embed seed/lead_detail.json
var LeadDetailJSON []byte

//go:embed seed/success_plaque.json
var SuccessPlaqueJSON []byte

// surface_links replicates over the SYNTHETIC `surfaceLink` EntitySet
// (meta_apply_manifest.go, written once issue_surface_urls applies):
// {label, url, surface}. The URL atom is wrapper "link": BindData writes
// the address into `content`, and the deterministic link pass in
// compose_turn turns it into the external_link action the widget opens.
//
//go:embed seed/surface_links.json
var SurfaceLinksJSON []byte

// manifest_summary replicates over the SYNTHETIC `manifestStep` EntitySet
// (refreshed after every applier run): {op, title, status, statusLabel,
// detail} — the final "вот всё твоё ПО" build receipt.
//
//go:embed seed/manifest_summary.json
var ManifestSummaryJSON []byte

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
	"product_carousel":          ProductCarouselJSON,
	"product_comparison":        ProductComparisonJSON,
	"product_detail":            ProductDetailJSON,
	"product_detail_accordion":  ProductDetailAccordionJSON,
	"product_detail_horizontal": ProductDetailHorizontalJSON,
	"text_explainer":            TextExplainerJSON,
	"empty_not_found":           EmptyNotFoundJSON,
	"error_generic":             ErrorGenericJSON,
	"design_system_preview":     DesignSystemPreviewJSON,
	"uploader_card":             UploaderCardJSON,
	"operation_card":            OperationCardJSON,
	"registration_form":         RegistrationFormJSON,
	"booking_form":              BookingFormJSON,
	"lead_table":                LeadTableJSON,
	"lead_detail":               LeadDetailJSON,
	"success_plaque":            SuccessPlaqueJSON,
	"surface_links":             SurfaceLinksJSON,
	"manifest_summary":          ManifestSummaryJSON,
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
	"product_carousel":          true,
	"product_comparison":        true,
	"product_detail":            false,
	"product_detail_accordion":  false,
	"product_detail_horizontal": false,
	"text_explainer":            false,
	"empty_not_found":           false,
	"error_generic":             false,
	"design_system_preview":     false,
	"uploader_card":             false,
	"operation_card":            true,
	"registration_form":         false,
	"booking_form":              false,
	"lead_table":                true,
	"lead_detail":               false,
	"success_plaque":            false,
	"surface_links":             true,
	"manifest_summary":          true,
}

// SystemPresetReplicateSource names the data source a preset's replicate
// frame is AUTHORED against — for the onboarding presets, the synthetic
// EntitySet slug the runtime writes for them (R23). The renderer falls back
// to it whenever the model's `replicate` argument does not name a live
// source: omitted, mistyped, or given as a bare count, all of which
// otherwise collapse the row template and ship a headline over nothing
// (live tail #3, the surface-links handover). Same law as the rest of this
// path — the artifact must not depend on the model spelling an argument
// right. Catalog presets have no entry: their source is the query result,
// and their count semantics are unchanged.
var SystemPresetReplicateSource = map[string]string{
	"operation_card":   "opCard",
	"surface_links":    "surfaceLink",
	"manifest_summary": "manifestStep",
}

// SystemPresetCategory maps each system preset name to its family in
// the preset-library taxonomy advertised by the internal listing
// (GET /api/v1/internal/presets/system). Canonical values: cards |
// details | states | narrative | forms | onboarding (components carry
// their own fixed "components" category in the handler). Every
// SystemPresetSeeds key must have an entry here — guarded by
// TestSystemPresetTaxonomyComplete.
var SystemPresetCategory = map[string]string{
	"product_card":              "cards",
	"product_card_compact":      "cards",
	"product_card_horizontal":   "cards",
	"product_card_list_row":     "cards",
	"product_carousel":          "cards",
	"product_comparison":        "cards",
	"product_detail":            "details",
	"product_detail_accordion":  "details",
	"product_detail_horizontal": "details",
	"text_explainer":            "narrative",
	"empty_not_found":           "states",
	"error_generic":             "states",
	"design_system_preview":     "onboarding",
	"uploader_card":             "onboarding",
	"operation_card":            "onboarding",
	"registration_form":         "forms",
	"booking_form":              "forms",
	"lead_table":                "cards",
	"lead_detail":               "details",
	"success_plaque":            "states",
	"surface_links":             "onboarding",
	"manifest_summary":          "onboarding",
}

// SystemComponentSeeds maps component name → embedded JSON. Mirrors
// SystemPresetSeeds but for the reusable subtrees referenced by presets
// via RefNode (e.g. product_card → ref{ref:"price-rating-root"}).
//
// Without this registry, fresh tenants see refs unresolved (engine has
// nothing to inline) and the rendered card collapses to title+image
// without price/rating/brand surfaces. The component name is the
// `name` field consumed by ComponentPort + the matching top-level node
// id inside the JSON (FindNodeByID resolves `ref` against the
// document tree).
var SystemComponentSeeds = map[string][]byte{
	"price-rating-root": ComponentPriceRatingJSON,
	"brand-badge-root":  ComponentBrandBadgeJSON,
}

// SystemComponentDescriptions maps each SystemComponentSeeds key (the
// engine-internal RefNode target) to a short Agent2-style description
// for the internal preset-library listing. Every SystemComponentSeeds
// key must have an entry here — guarded by
// TestSystemPresetTaxonomyComplete.
var SystemComponentDescriptions = map[string]string{
	"price-rating-root": "price + rating row, reused inside card presets",
	"brand-badge-root":  "brand badge atom, reused inside card presets",
}

// SystemComponentPublicNames maps each SystemComponentSeeds key to the
// public name the internal listing advertises (matches the seed file
// names, e.g. seed/component_price_rating.json). The seed key itself is
// the ref target node id presets point at — an engine detail we keep
// off the wire.
var SystemComponentPublicNames = map[string]string{
	"price-rating-root": "component_price_rating",
	"brand-badge-root":  "component_brand_badge",
}
