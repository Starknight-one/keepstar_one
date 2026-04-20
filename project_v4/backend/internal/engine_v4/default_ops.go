package engine_v4

import "keepstar_v4/internal/domain"

// ProductCardGridOps is a thin wrapper around the "product_card" preset for
// backwards compatibility with usecases and handlers that still call it
// directly (navigation_back, action_view, pipeline_execute fallback,
// navigation_expand, handler_debug, handler_testbench).
//
// New code should go through the preset registry via GetPreset("product_card").
func ProductCardGridOps() []Op {
	if p, ok := GetPreset("product_card"); ok {
		return p.Build()
	}
	return nil
}

// ProductDetailOps is a thin wrapper around the "product_detail" preset —
// same backwards-compat rationale as ProductCardGridOps.
func ProductDetailOps() []Op {
	if p, ok := GetPreset("product_detail"); ok {
		return p.Build()
	}
	return nil
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
