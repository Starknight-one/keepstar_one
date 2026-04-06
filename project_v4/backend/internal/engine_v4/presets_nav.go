package engine_v4

// Navigation presets — used for pre-built adjacent views (liked, cart) and
// catalog category cards. liked_grid and cart_grid are aliases on top of
// product-card variants so that E2 (pre-built navigation) can reference
// them by a stable nav name without knowing about product_card internals.
//
// cart_grid is intentionally a single-widget preset (card grid only). The
// totals widget is out of scope until B4 (multi-widget) lands.

// catalogCategoryCardOps — a card for a catalog group/category:
// image + title + item count. Binds to fields name/images/count. For now
// `count` is expected from Product data maps as a literal-filled field;
// when a proper Catalog entity appears this preset will bind to that.
func catalogCategoryCardOps() []Op {
	return []Op{
		{Type: OpInsert, Ref: "w", Parent: "formation", Props: map[string]interface{}{"type": "widget", "size": "medium"}},
		{Type: OpInsert, Ref: "root", Parent: "$w", Props: map[string]interface{}{"type": "column", "gap": "sm"}},
		{Type: OpInsert, Parent: "$root", Props: map[string]interface{}{"type": "image", "fieldName": "images", "slot": "hero", "mediaStyle": map[string]interface{}{"aspectRatio": "4:3", "objectFit": "cover"}}},
		{Type: OpInsert, Ref: "info", Parent: "$root", Props: map[string]interface{}{"type": "column", "gap": "xs"}},
		{Type: OpInsert, Parent: "$info", Props: map[string]interface{}{"type": "text", "fieldName": "name", "slot": "title", "textStyle": map[string]interface{}{"fontSize": "lg", "fontWeight": "semibold"}}},
		{Type: OpInsert, Parent: "$info", Props: map[string]interface{}{"type": "number", "fieldName": "count", "format": "number", "textStyle": map[string]interface{}{"fontSize": "sm", "color": "muted"}}},
	}
}

func init() {
	RegisterPreset(&Preset{
		Name:             "catalog_category_card",
		Description:      "Catalog group/category card: image + title + item count. Binds name/images/count.",
		Category:         "nav",
		Refs:             []string{"w", "root", "info"},
		DefaultReplicate: true,
		Build:            catalogCategoryCardOps,
	})
	// liked_grid — alias to product_card for the liked-items view. Separate
	// entry so that E2 pre-built nav can request it by a nav-specific name.
	RegisterPreset(&Preset{
		Name:             "liked_grid",
		Description:      "Grid of liked products. Alias on product_card with a nav-specific name.",
		Category:         "nav",
		Refs:             productCardRefs,
		DefaultReplicate: true,
		Build:            productCardOps,
	})
	// cart_grid — alias to product_card_horizontal for the cart view. Totals
	// widget will be added as a composition when B4 (multi-widget) lands.
	RegisterPreset(&Preset{
		Name:             "cart_grid",
		Description:      "Grid of cart items. Alias on product_card_horizontal. Totals widget pending B4 (multi-widget).",
		Category:         "nav",
		Refs:             productCardRefs,
		DefaultReplicate: true,
		Build:            productCardHorizontalOps,
	})
}
