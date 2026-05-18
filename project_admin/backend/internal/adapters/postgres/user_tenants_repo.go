package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"keepstar-admin/internal/ports"
)

type UserTenantsRepo struct{ client *Client }

func NewUserTenantsRepo(c *Client) *UserTenantsRepo { return &UserTenantsRepo{client: c} }

func (r *UserTenantsRepo) ListForUser(ctx context.Context, userID string) ([]ports.UserTenantMembership, error) {
	// Picker filter (scenario 177): hide tenants whose every integration is
	// disconnected. Tenants with no integrations at all (fresh manual signups)
	// still surface so the user can configure their first source.
	rows, err := r.client.pool.Query(ctx,
		`SELECT ut.tenant_id::text, COALESCE(t.slug, ''), COALESCE(t.name, ''), ut.role, ut.created_at
		 FROM admin.user_tenants ut
		 JOIN catalog.tenants t ON t.id = ut.tenant_id
		 WHERE ut.user_id = $1
		   AND t.deleted_at IS NULL
		   AND (
		     NOT EXISTS (SELECT 1 FROM admin.tenant_integrations WHERE tenant_id = t.id)
		     OR EXISTS (SELECT 1 FROM admin.tenant_integrations WHERE tenant_id = t.id AND status != 'disconnected')
		   )
		 ORDER BY ut.created_at ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list user tenants: %w", err)
	}
	defer rows.Close()
	var out []ports.UserTenantMembership
	for rows.Next() {
		var m ports.UserTenantMembership
		if err := rows.Scan(&m.TenantID, &m.TenantSlug, &m.TenantName, &m.Role, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func (r *UserTenantsRepo) HasMembership(ctx context.Context, userID, tenantID string) (string, bool, error) {
	var role string
	err := r.client.pool.QueryRow(ctx,
		`SELECT role FROM admin.user_tenants WHERE user_id = $1 AND tenant_id = $2`,
		userID, tenantID,
	).Scan(&role)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("membership lookup: %w", err)
	}
	return role, true, nil
}

func (r *UserTenantsRepo) Add(ctx context.Context, userID, tenantID, role, invitedBy string) error {
	_, err := r.client.pool.Exec(ctx,
		`INSERT INTO admin.user_tenants (user_id, tenant_id, role, invited_by)
		 VALUES ($1, $2, $3, NULLIF($4,'')::uuid)
		 ON CONFLICT (user_id, tenant_id) DO UPDATE SET role = EXCLUDED.role`,
		userID, tenantID, role, invitedBy)
	if err != nil {
		return fmt.Errorf("add user tenant: %w", err)
	}
	return nil
}

func (r *UserTenantsRepo) Remove(ctx context.Context, userID, tenantID string) error {
	_, err := r.client.pool.Exec(ctx,
		`DELETE FROM admin.user_tenants WHERE user_id = $1 AND tenant_id = $2`,
		userID, tenantID)
	if err != nil {
		return fmt.Errorf("remove user tenant: %w", err)
	}
	return nil
}

// SoftDeleteEmptyOrphanTenants finds tenants the user owns that have no
// content (no integrations, no products, no canvas presets/components/tokens)
// EXCEPT preserveTenantID, soft-deletes them, and removes their memberships.
// Returns the IDs that were deleted so the caller can log them.
//
// Empty-detection is conservative: we only delete tenants where every
// content-table for the tenant returns zero rows. If the user actually used
// a "blank" tenant (opened canvas, ran a webhook), at least one EXISTS check
// will be true and the tenant survives.
//
// Runs the find + delete in a single transaction so a concurrent write that
// adds an integration row between the find and the delete doesn't get
// stomped (the SELECT sees the same snapshot as the UPDATE).
func (r *UserTenantsRepo) SoftDeleteEmptyOrphanTenants(ctx context.Context, userID, preserveTenantID string) ([]string, error) {
	tx, err := r.client.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx,
		`SELECT t.id::text
		 FROM admin.user_tenants ut
		 JOIN catalog.tenants t ON t.id = ut.tenant_id
		 WHERE ut.user_id = $1
		   AND ut.role = 'owner'
		   AND t.id::text != $2
		   AND t.deleted_at IS NULL
		   AND NOT EXISTS (SELECT 1 FROM admin.tenant_integrations WHERE tenant_id = t.id)
		   AND NOT EXISTS (SELECT 1 FROM catalog.products WHERE tenant_id = t.id)
		   AND NOT EXISTS (SELECT 1 FROM admin.tenant_presets WHERE tenant_id = t.id)
		   AND NOT EXISTS (SELECT 1 FROM admin.tenant_components WHERE tenant_id = t.id)
		   AND NOT EXISTS (SELECT 1 FROM admin.tenant_design_tokens WHERE tenant_id = t.id)`,
		userID, preserveTenantID,
	)
	if err != nil {
		return nil, fmt.Errorf("list empty orphan tenants: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, id := range ids {
		if _, err := tx.Exec(ctx,
			`UPDATE catalog.tenants SET deleted_at = NOW(), updated_at = NOW() WHERE id = $1`,
			id,
		); err != nil {
			return nil, fmt.Errorf("soft-delete tenant %s: %w", id, err)
		}
		if _, err := tx.Exec(ctx,
			`DELETE FROM admin.user_tenants WHERE tenant_id = $1`,
			id,
		); err != nil {
			return nil, fmt.Errorf("remove memberships for %s: %w", id, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return ids, nil
}
