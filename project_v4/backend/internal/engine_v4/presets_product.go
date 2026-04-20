package engine_v4

// Product card presets (6 variants) — registered in init().
//
// All product presets expose the same set of refs so override ops are
// predictable regardless of which card variant Agent2 picked:
//
//   $w    — the widget container
//   $root — top-level layout node inside the widget
//   $info — vertical stack with title/price/etc
//   $meta — horizontal row inside $info with small meta atoms (price/rating)
//
// Hardcoded fieldName set: images, name, price, rating, brand, description,
// category, tags. Adapts only to what data actually binds — missing fields
// leave atoms empty (B7 will fix this via field_definition roles).

var productCardRefs = []string{"w", "root", "info", "meta"}

// productCardOps — standard card for grids of 2–4 items (medium size).
// Matches the legacy ProductCardGridOps output byte-for-byte.
func productCardOps() []Op {
	return []Op{
		{Type: OpInsert, Ref: "w", Parent: "formation", Props: map[string]interface{}{"type": "widget", "size": "medium"}},
		{Type: OpInsert, Ref: "root", Parent: "$w", Props: map[string]interface{}{"type": "column", "gap": "sm"}},
		{Type: OpInsert, Parent: "$root", Props: map[string]interface{}{"type": "image", "fieldName": "images", "slot": "hero", "mediaStyle": map[string]interface{}{"aspectRatio": "4:3", "objectFit": "cover"}}},
		{Type: OpInsert, Ref: "info", Parent: "$root", Props: map[string]interface{}{"type": "column", "gap": "xs"}},
		{Type: OpInsert, Parent: "$info", Props: map[string]interface{}{"type": "text", "fieldName": "name", "slot": "title", "textStyle": map[string]interface{}{"fontSize": "xl", "fontWeight": "bold"}}},
		{Type: OpInsert, Ref: "meta", Parent: "$info", Props: map[string]interface{}{"type": "row", "gap": "md"}},
		{Type: OpInsert, Parent: "$meta", Props: map[string]interface{}{"type": "number", "fieldName": "price", "format": "currency", "slot": "price"}},
		{Type: OpInsert, Parent: "$meta", Props: map[string]interface{}{"type": "number", "fieldName": "rating", "format": "stars-compact"}},
		{Type: OpInsert, Parent: "$info", Props: map[string]interface{}{"type": "text", "fieldName": "brand", "wrapper": map[string]interface{}{"type": "badge", "variant": "subtle"}, "slot": "badge"}},
	}
}

// productCardCompactOps — dense grid (5+ items). Fewer atoms, size=small.
func productCardCompactOps() []Op {
	return []Op{
		{Type: OpInsert, Ref: "w", Parent: "formation", Props: map[string]interface{}{"type": "widget", "size": "small"}},
		{Type: OpInsert, Ref: "root", Parent: "$w", Props: map[string]interface{}{"type": "column", "gap": "xs"}},
		{Type: OpInsert, Parent: "$root", Props: map[string]interface{}{"type": "image", "fieldName": "images", "slot": "hero", "mediaStyle": map[string]interface{}{"aspectRatio": "1:1", "objectFit": "cover"}}},
		{Type: OpInsert, Ref: "info", Parent: "$root", Props: map[string]interface{}{"type": "column", "gap": "xs"}},
		{Type: OpInsert, Parent: "$info", Props: map[string]interface{}{"type": "text", "fieldName": "name", "slot": "title", "textStyle": map[string]interface{}{"fontSize": "sm", "fontWeight": "medium", "lineClamp": 2}}},
		{Type: OpInsert, Ref: "meta", Parent: "$info", Props: map[string]interface{}{"type": "row", "gap": "sm"}},
		{Type: OpInsert, Parent: "$meta", Props: map[string]interface{}{"type": "number", "fieldName": "price", "format": "currency", "slot": "price", "textStyle": map[string]interface{}{"fontSize": "sm", "fontWeight": "semibold"}}},
	}
}

// productCardHorizontalOps — image on the left, info on the right. Good for
// carousels or standalone horizontal cards.
func productCardHorizontalOps() []Op {
	return []Op{
		{Type: OpInsert, Ref: "w", Parent: "formation", Props: map[string]interface{}{"type": "widget", "size": "medium"}},
		{Type: OpInsert, Ref: "root", Parent: "$w", Props: map[string]interface{}{"type": "row", "gap": "md", "align": "center"}},
		{Type: OpInsert, Parent: "$root", Props: map[string]interface{}{"type": "image", "fieldName": "images", "slot": "hero", "mediaStyle": map[string]interface{}{"aspectRatio": "1:1", "objectFit": "cover"}}},
		{Type: OpInsert, Ref: "info", Parent: "$root", Props: map[string]interface{}{"type": "column", "gap": "xs"}},
		{Type: OpInsert, Parent: "$info", Props: map[string]interface{}{"type": "text", "fieldName": "name", "slot": "title", "textStyle": map[string]interface{}{"fontSize": "lg", "fontWeight": "bold"}}},
		{Type: OpInsert, Ref: "meta", Parent: "$info", Props: map[string]interface{}{"type": "row", "gap": "md", "align": "center"}},
		{Type: OpInsert, Parent: "$meta", Props: map[string]interface{}{"type": "number", "fieldName": "price", "format": "currency", "slot": "price"}},
		{Type: OpInsert, Parent: "$meta", Props: map[string]interface{}{"type": "number", "fieldName": "rating", "format": "stars-compact"}},
		{Type: OpInsert, Parent: "$info", Props: map[string]interface{}{"type": "text", "fieldName": "brand", "wrapper": map[string]interface{}{"type": "badge", "variant": "subtle"}, "slot": "badge"}},
	}
}

// productCardListRowOps — wide row for list layouts. Narrow image, wide info.
// Uses layout: "list" on the formation. Spans full row width by default.
func productCardListRowOps() []Op {
	return []Op{
		{Type: OpInsert, Ref: "w", Parent: "formation", Props: map[string]interface{}{"type": "widget", "size": "small"}},
		{Type: OpInsert, Ref: "root", Parent: "$w", Props: map[string]interface{}{"type": "row", "gap": "md", "align": "center"}},
		{Type: OpInsert, Parent: "$root", Props: map[string]interface{}{"type": "image", "fieldName": "images", "slot": "hero", "mediaStyle": map[string]interface{}{"aspectRatio": "1:1", "objectFit": "cover"}}},
		{Type: OpInsert, Ref: "info", Parent: "$root", Props: map[string]interface{}{"type": "column", "gap": "xs"}},
		{Type: OpInsert, Parent: "$info", Props: map[string]interface{}{"type": "text", "fieldName": "name", "slot": "title", "textStyle": map[string]interface{}{"fontSize": "md", "fontWeight": "semibold"}}},
		{Type: OpInsert, Parent: "$info", Props: map[string]interface{}{"type": "text", "fieldName": "description", "slot": "description", "textStyle": map[string]interface{}{"fontSize": "sm", "lineClamp": 2, "color": "muted"}}},
		{Type: OpInsert, Ref: "meta", Parent: "$info", Props: map[string]interface{}{"type": "row", "gap": "md", "align": "center"}},
		{Type: OpInsert, Parent: "$meta", Props: map[string]interface{}{"type": "number", "fieldName": "price", "format": "currency", "slot": "price", "textStyle": map[string]interface{}{"fontWeight": "semibold"}}},
		{Type: OpInsert, Parent: "$meta", Props: map[string]interface{}{"type": "number", "fieldName": "rating", "format": "stars-compact"}},
		{Type: OpInsert, Parent: "$meta", Props: map[string]interface{}{"type": "text", "fieldName": "brand", "wrapper": map[string]interface{}{"type": "badge", "variant": "subtle"}, "slot": "badge"}},
	}
}

// productDetailOps — detailed single-product widget (vertical layout, 16:9 hero).
// Matches legacy ProductDetailOps.
func productDetailOps() []Op {
	return []Op{
		{Type: OpInsert, Ref: "w", Parent: "formation", Props: map[string]interface{}{"type": "widget", "size": "large"}},
		{Type: OpInsert, Ref: "root", Parent: "$w", Props: map[string]interface{}{"type": "column", "gap": "lg"}},
		{Type: OpInsert, Parent: "$root", Props: map[string]interface{}{"type": "image", "fieldName": "images", "slot": "hero", "mediaStyle": map[string]interface{}{"aspectRatio": "16:9", "objectFit": "cover"}}},
		{Type: OpInsert, Ref: "info", Parent: "$root", Props: map[string]interface{}{"type": "column", "gap": "md"}},
		{Type: OpInsert, Parent: "$info", Props: map[string]interface{}{"type": "text", "fieldName": "name", "slot": "title", "textStyle": map[string]interface{}{"fontSize": "2xl", "fontWeight": "bold"}}},
		{Type: OpInsert, Ref: "meta", Parent: "$info", Props: map[string]interface{}{"type": "row", "gap": "md", "align": "center"}},
		{Type: OpInsert, Parent: "$meta", Props: map[string]interface{}{"type": "number", "fieldName": "price", "format": "currency", "slot": "price", "textStyle": map[string]interface{}{"fontSize": "xl"}}},
		{Type: OpInsert, Parent: "$meta", Props: map[string]interface{}{"type": "number", "fieldName": "rating", "format": "stars"}},
		{Type: OpInsert, Parent: "$meta", Props: map[string]interface{}{"type": "text", "fieldName": "brand", "wrapper": map[string]interface{}{"type": "badge"}, "slot": "badge"}},
		{Type: OpInsert, Parent: "$info", Props: map[string]interface{}{"type": "text", "fieldName": "description", "slot": "description", "textStyle": map[string]interface{}{"lineClamp": 6}}},
		{Type: OpInsert, Ref: "tags", Parent: "$info", Props: map[string]interface{}{"type": "flow", "gap": "sm"}},
		{Type: OpInsert, Parent: "$tags", Props: map[string]interface{}{"type": "text", "fieldName": "category", "wrapper": map[string]interface{}{"type": "tag", "variant": "outline"}, "slot": "tags"}},
		{Type: OpInsert, Parent: "$tags", Props: map[string]interface{}{"type": "text", "fieldName": "tags", "wrapper": map[string]interface{}{"type": "tag", "variant": "subtle"}, "slot": "tags"}},
	}
}

// productDetailHorizontalOps — detail with image on the left, info column on the right.
func productDetailHorizontalOps() []Op {
	return []Op{
		{Type: OpInsert, Ref: "w", Parent: "formation", Props: map[string]interface{}{"type": "widget", "size": "large"}},
		{Type: OpInsert, Ref: "root", Parent: "$w", Props: map[string]interface{}{"type": "row", "gap": "lg", "align": "start"}},
		{Type: OpInsert, Parent: "$root", Props: map[string]interface{}{"type": "image", "fieldName": "images", "slot": "hero", "mediaStyle": map[string]interface{}{"aspectRatio": "4:3", "objectFit": "cover"}}},
		{Type: OpInsert, Ref: "info", Parent: "$root", Props: map[string]interface{}{"type": "column", "gap": "md"}},
		{Type: OpInsert, Parent: "$info", Props: map[string]interface{}{"type": "text", "fieldName": "name", "slot": "title", "textStyle": map[string]interface{}{"fontSize": "2xl", "fontWeight": "bold"}}},
		{Type: OpInsert, Ref: "meta", Parent: "$info", Props: map[string]interface{}{"type": "row", "gap": "md", "align": "center"}},
		{Type: OpInsert, Parent: "$meta", Props: map[string]interface{}{"type": "number", "fieldName": "price", "format": "currency", "slot": "price", "textStyle": map[string]interface{}{"fontSize": "xl"}}},
		{Type: OpInsert, Parent: "$meta", Props: map[string]interface{}{"type": "number", "fieldName": "rating", "format": "stars"}},
		{Type: OpInsert, Parent: "$meta", Props: map[string]interface{}{"type": "text", "fieldName": "brand", "wrapper": map[string]interface{}{"type": "badge"}, "slot": "badge"}},
		{Type: OpInsert, Parent: "$info", Props: map[string]interface{}{"type": "text", "fieldName": "description", "slot": "description", "textStyle": map[string]interface{}{"lineClamp": 8}}},
		{Type: OpInsert, Ref: "tags", Parent: "$info", Props: map[string]interface{}{"type": "flow", "gap": "sm"}},
		{Type: OpInsert, Parent: "$tags", Props: map[string]interface{}{"type": "text", "fieldName": "category", "wrapper": map[string]interface{}{"type": "tag", "variant": "outline"}, "slot": "tags"}},
		{Type: OpInsert, Parent: "$tags", Props: map[string]interface{}{"type": "text", "fieldName": "tags", "wrapper": map[string]interface{}{"type": "tag", "variant": "subtle"}, "slot": "tags"}},
	}
}

func init() {
	RegisterPreset(&Preset{
		Name:             "product_card",
		Description:      "Standard product card for grids of 2–4 items (medium size).",
		Category:         "product",
		Refs:             productCardRefs,
		DefaultReplicate: true,
		Build:            productCardOps,
	})
	RegisterPreset(&Preset{
		Name:             "product_card_compact",
		Description:      "Compact product card for dense grids (5+ items). Fewer atoms, small size.",
		Category:         "product",
		Refs:             productCardRefs,
		DefaultReplicate: true,
		Build:            productCardCompactOps,
	})
	RegisterPreset(&Preset{
		Name:             "product_card_horizontal",
		Description:      "Horizontal product card — image left, info right. Good for carousels.",
		Category:         "product",
		Refs:             productCardRefs,
		DefaultReplicate: true,
		Build:            productCardHorizontalOps,
	})
	RegisterPreset(&Preset{
		Name:             "product_card_list_row",
		Description:      "Wide list row (narrow image + stretched info column). Use with layout: \"list\".",
		Category:         "product",
		Refs:             productCardRefs,
		DefaultReplicate: true,
		Build:            productCardListRowOps,
	})
	RegisterPreset(&Preset{
		Name:             "product_detail",
		Description:      "Full product detail (vertical, 16:9 hero, price row, description, tags).",
		Category:         "product",
		Refs:             productCardRefs,
		DefaultReplicate: false,
		Build:            productDetailOps,
	})
	RegisterPreset(&Preset{
		Name:             "product_detail_horizontal",
		Description:      "Product detail with image on the left and info column on the right.",
		Category:         "product",
		Refs:             productCardRefs,
		DefaultReplicate: false,
		Build:            productDetailHorizontalOps,
	})
}
