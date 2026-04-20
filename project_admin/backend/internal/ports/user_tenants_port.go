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
}
