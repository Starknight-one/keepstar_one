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
	rows, err := r.client.pool.Query(ctx,
		`SELECT ut.tenant_id::text, COALESCE(t.slug, ''), COALESCE(t.name, ''), ut.role, ut.created_at
		 FROM admin.user_tenants ut
		 JOIN catalog.tenants t ON t.id = ut.tenant_id
		 WHERE ut.user_id = $1
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
