package engine

import (
	"keepstar_v5/internal/domain"
)

// ProductToMap flattens a Product into a single map[string]any for binding.
// Three layers, each only fills missing keys (so earlier layers win):
//
//  1. Typed fields (canonical PIM columns: name/price/brand/...)
//  2. Tier2 (master-level curated JSONB — set by curator/discovery)
//  3. Extra (per-listing JSONB — vendor-supplied / raw import)
//
// Precedence: Typed > Tier2 > Extra. This is reversed from V4 (which had
// Typed > Extra > Tier2). Reason: curator ought to win over raw vendor
// values — and the DB-side merge_apply already takes that side
// (`tier2 = master_products.tier2 || EXCLUDED.tier2` in the upsert
// concatenates with EXCLUDED winning on key collision). V5 keeps Go-side
// and DB-side aligned.
//
// Empty/zero-valued typed fields are skipped so they don't shadow Tier2 or
// Extra values for the same key. `StockQuantity` is written only when > 0:
// a 0 means stock is absent or suppressed by the tenant's stock-visibility
// flag (see tool_catalog_search), so it must not bind.
func ProductToMap(p domain.Product) map[string]any {
	m := make(map[string]any)

	// 1. Typed fields — canonical, win on collision.
	// "id" is what InjectDefaultActions resolves the action entity from;
	// without it like/cart buttons silently skip injection (resolveEntityID
	// finds no key) for every catalog whose Extra lacks a vendor "id".
	if p.ID != "" {
		m["id"] = p.ID
	}
	if p.Name != "" {
		m["name"] = p.Name
	}
	if p.OriginalName != "" {
		m["originalName"] = p.OriginalName
	}
	if p.DisplayName != "" {
		m["displayName"] = p.DisplayName
	}
	if p.Description != "" {
		m["description"] = p.Description
	}
	if p.Price > 0 {
		m["price"] = p.Price
	}
	if p.PriceFormatted != "" {
		m["priceFormatted"] = p.PriceFormatted
	}
	if p.Currency != "" {
		m["currency"] = p.Currency
	}
	if p.Rating > 0 {
		m["rating"] = p.Rating
	}
	if p.StockQuantity > 0 {
		m["stockQuantity"] = p.StockQuantity
	}
	if p.Brand != "" {
		m["brand"] = p.Brand
	}
	if p.Category != "" {
		m["category"] = p.Category
	}
	if len(p.Images) > 0 {
		m["images"] = p.Images
		m["heroImage"] = p.Images[0]
	}
	if len(p.Tags) > 0 {
		m["tags"] = p.Tags
	}
	if p.SKU != "" {
		m["sku"] = p.SKU
	}
	if len(p.GTINs) > 0 {
		m["gtins"] = p.GTINs
	}
	if p.Size != "" {
		m["size"] = p.Size
	}
	if p.Color != "" {
		m["color"] = p.Color
	}
	if p.WeightG != nil {
		m["weightG"] = *p.WeightG
	}
	if p.VolumeML != nil {
		m["volumeMl"] = *p.VolumeML
	}
	if p.ProductForm != "" {
		m["productForm"] = p.ProductForm
	}
	if p.Texture != "" {
		m["texture"] = p.Texture
	}
	if p.RoutineStep != "" {
		m["routineStep"] = p.RoutineStep
	}
	if len(p.SkinType) > 0 {
		m["skinType"] = p.SkinType
	}
	if len(p.Concern) > 0 {
		m["concern"] = p.Concern
	}
	if len(p.KeyIngredients) > 0 {
		m["keyIngredients"] = p.KeyIngredients
	}
	if len(p.TargetArea) > 0 {
		m["targetArea"] = p.TargetArea
	}
	if p.MarketingClaim != "" {
		m["marketingClaim"] = p.MarketingClaim
	}
	if len(p.Benefits) > 0 {
		m["benefits"] = p.Benefits
	}

	// 2. Tier2 — master-curated. Wins over Extra.
	for k, v := range p.Tier2 {
		if _, exists := m[k]; !exists {
			m[k] = v
		}
	}

	// 3. Extra — raw vendor JSONB. Loses to typed and Tier2.
	for k, v := range p.Extra {
		if _, exists := m[k]; !exists {
			m[k] = v
		}
	}

	return m
}

// ServiceToMap flattens a Service the same way. Services have fewer typed
// fields and only one extension layer (Attributes + Extra), so precedence
// is just Typed > Attributes > Extra.
func ServiceToMap(s domain.Service) map[string]any {
	m := make(map[string]any)
	// Same contract as ProductToMap: "id" feeds InjectDefaultActions.
	if s.ID != "" {
		m["id"] = s.ID
	}
	if s.Name != "" {
		m["name"] = s.Name
	}
	if s.Description != "" {
		m["description"] = s.Description
	}
	if s.Price > 0 {
		m["price"] = s.Price
	}
	if s.PriceFormatted != "" {
		m["priceFormatted"] = s.PriceFormatted
	}
	if s.Currency != "" {
		m["currency"] = s.Currency
	}
	if s.Duration != "" {
		m["duration"] = s.Duration
	}
	if len(s.Images) > 0 {
		m["images"] = s.Images
		m["heroImage"] = s.Images[0]
	}
	if s.Rating > 0 {
		m["rating"] = s.Rating
	}
	if s.Category != "" {
		m["category"] = s.Category
	}
	if s.Provider != "" {
		m["provider"] = s.Provider
	}
	if s.Availability != "" {
		m["availability"] = s.Availability
	}
	if len(s.Tags) > 0 {
		m["tags"] = s.Tags
	}

	for k, v := range s.Attributes {
		if _, exists := m[k]; !exists {
			m[k] = v
		}
	}
	for k, v := range s.Extra {
		if _, exists := m[k]; !exists {
			m[k] = v
		}
	}
	return m
}
