package ports

import "context"

// CanvasDesignTokenView is one design token row from the admin storage layer.
// Tokens are grouped by Category (e.g. "color", "radius", "spacing") and may
// carry a ThemeAxis/ThemeValue pair for theming overrides (e.g. theme
// "season":"halloween" overrides the base accent color).
type CanvasDesignTokenView struct {
	Category   string `json:"category"`
	Name       string `json:"name"`
	Value      string `json:"value"`
	ThemeAxis  string `json:"themeAxis,omitempty"`
	ThemeValue string `json:"themeValue,omitempty"`
}

// CanvasDesignContextPort reads the tenant's design context metadata from the
// admin storage layer. Two concerns:
//  1. Version counter — used by the TenantPresetLoader to decide when to
//     invalidate the cached design context snapshot. Bumped by admin on every
//     publish/delete/token edit via BumpDesignContextVersion.
//  2. Design tokens — key-value pairs grouped by category, optionally scoped
//     to a theme axis.
//
// Callers pass either a tenant UUID or slug.
type CanvasDesignContextPort interface {
	// GetVersion returns admin.tenant_design_context_version.version for the
	// tenant, or 0 when the row is missing (first-publish case).
	GetVersion(ctx context.Context, tenantIDOrSlug string) (int64, error)

	// ListDesignTokens returns every token row for the tenant, ordered by
	// category, name, theme_axis, theme_value.
	ListDesignTokens(ctx context.Context, tenantIDOrSlug string) ([]CanvasDesignTokenView, error)
}
