package engine_v4

import "keepstar_v4/internal/domain"

// ProductCardGridOps returns ops that build a product card template.
// Engine replicates this template for N data items.
func ProductCardGridOps() []Op {
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

// ProductDetailOps returns ops that build a single detailed product widget.
func ProductDetailOps() []Op {
	return []Op{
		{Type: OpInsert, Ref: "w", Parent: "formation", Props: map[string]interface{}{"type": "widget", "size": "large"}},
		{Type: OpInsert, Ref: "root", Parent: "$w", Props: map[string]interface{}{"type": "column", "gap": "lg"}},
		{Type: OpInsert, Parent: "$root", Props: map[string]interface{}{"type": "image", "fieldName": "images", "slot": "hero", "mediaStyle": map[string]interface{}{"aspectRatio": "16:9", "objectFit": "cover"}}},
		{Type: OpInsert, Ref: "content", Parent: "$root", Props: map[string]interface{}{"type": "column", "gap": "md"}},
		{Type: OpInsert, Parent: "$content", Props: map[string]interface{}{"type": "text", "fieldName": "name", "slot": "title", "textStyle": map[string]interface{}{"fontSize": "2xl", "fontWeight": "bold"}}},
		{Type: OpInsert, Ref: "price-row", Parent: "$content", Props: map[string]interface{}{"type": "row", "gap": "md", "align": "center"}},
		{Type: OpInsert, Parent: "$price-row", Props: map[string]interface{}{"type": "number", "fieldName": "price", "format": "currency", "slot": "price", "textStyle": map[string]interface{}{"fontSize": "xl"}}},
		{Type: OpInsert, Parent: "$price-row", Props: map[string]interface{}{"type": "number", "fieldName": "rating", "format": "stars"}},
		{Type: OpInsert, Parent: "$price-row", Props: map[string]interface{}{"type": "text", "fieldName": "brand", "wrapper": map[string]interface{}{"type": "badge"}, "slot": "badge"}},
		{Type: OpInsert, Parent: "$content", Props: map[string]interface{}{"type": "text", "fieldName": "description", "slot": "description", "textStyle": map[string]interface{}{"lineClamp": 6}}},
		{Type: OpInsert, Ref: "tags", Parent: "$content", Props: map[string]interface{}{"type": "flow", "gap": "sm"}},
		{Type: OpInsert, Parent: "$tags", Props: map[string]interface{}{"type": "text", "fieldName": "category", "wrapper": map[string]interface{}{"type": "tag", "variant": "outline"}, "slot": "tags"}},
		{Type: OpInsert, Parent: "$tags", Props: map[string]interface{}{"type": "text", "fieldName": "tags", "wrapper": map[string]interface{}{"type": "tag", "variant": "subtle"}, "slot": "tags"}},
	}
}

// DefaultWidgetActions returns the default set of actions for entity-bound widgets.
// Injected by engine after replication; agent can override by setting Actions via ops.
func DefaultWidgetActions() []domain.ActionDef {
	return []domain.ActionDef{
		{Type: domain.WidgetActionLike},
		{Type: domain.WidgetActionAddToCart},
	}
}

// GridColumnsForCount returns the optimal column count for a given item count.
func GridColumnsForCount(count int) int {
	if count <= 1 {
		return 1
	}
	if count <= 4 {
		return 2
	}
	return 3
}
