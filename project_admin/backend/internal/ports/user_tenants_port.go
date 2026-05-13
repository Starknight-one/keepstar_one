package ports

import (
	"context"
	"time"
)

// UserTenantMembership is one row from admin.user_tenants joined with the
// tenant's display name. `Role` is the role the user holds in this tenant.
type UserTenantMembership struct {
	TenantID   string    `json:"tenant_id"`
	TenantSlug string    `json:"tenant_slug"`
	TenantName string    `json:"tenant_name"`
	Role       string    `json:"role"`
	CreatedAt  time.Time `json:"created_at"`
}

type UserTenantsPort interface {
	ListForUser(ctx context.Context, userID string) ([]UserTenantMembership, error)
	HasMembership(ctx context.Context, userID, tenantID string) (role string, ok bool, err error)
	Add(ctx context.Context, userID, tenantID, role, invitedBy string) error
	Remove(ctx context.Context, userID, tenantID string) error

	// SoftDeleteEmptyOrphanTenants soft-deletes tenants where the user is
	// 'owner', the tenant has no integrations / products / canvas content,
	// and the tenant id is NOT preserveTenantID. Returns the list of
	// deleted tenant IDs (for logging). Called from ProvisionShopOwner
	// after a Shopify install email-merge so a Google-auto-provisioned
	// blank workspace stops cluttering the picker.
	SoftDeleteEmptyOrphanTenants(ctx context.Context, userID, preserveTenantID string) ([]string, error)
}
