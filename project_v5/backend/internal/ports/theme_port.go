package ports

import (
	"context"

	"keepstar_v5/internal/domain"
)

// ThemePort is the design-system theme surface V5 uses to attach doc.theme at
// render time and to serve the admin GET/PUT theme endpoints.
//
// tenantSlugOrID accepts a tenant slug OR UUID — same convention as PresetPort;
// the chat/preview attach paths pass tenant.ID (UUID), the admin endpoints pass
// the resolved tenant too. GetTheme NEVER errors on a missing row (that is the
// default-token case); it returns domain.DefaultTheme() with IsDefault=true.
type ThemePort interface {
	// GetTheme returns the tenant's COMPLETE token set (stored override merged
	// over canonical defaults), or the pure defaults (IsDefault=true) when the
	// tenant has no row. Unknown tenant slug → domain.ErrTenantNotFound.
	GetTheme(ctx context.Context, tenantSlugOrID string) (domain.Theme, error)

	// UpsertTheme persists the tenant's override token set (later phases).
	UpsertTheme(ctx context.Context, tenantSlugOrID string, tokens domain.ThemeTokens) error
}
