package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"keepstar-admin/internal/ports"
)

type InvitationsRepo struct{ client *Client }

func NewInvitationsRepo(c *Client) *InvitationsRepo { return &InvitationsRepo{client: c} }

func (r *InvitationsRepo) Create(ctx context.Context, inv *ports.Invitation) error {
	q := `INSERT INTO admin.invitations
			(tenant_id, email, role, token_hash, inviter_id, expires_at)
		  VALUES ($1, $2, $3, $4, NULLIF($5,'')::uuid, $6)
		  RETURNING id, created_at`
	return r.client.pool.QueryRow(ctx, q,
		inv.TenantID, inv.Email, inv.Role, inv.TokenHash, inv.InviterID, inv.ExpiresAt,
	).Scan(&inv.ID, &inv.CreatedAt)
}

func (r *InvitationsRepo) GetByID(ctx context.Context, id string) (*ports.Invitation, error) {
	q := `SELECT id, tenant_id::text, email::text, role, token_hash,
				 COALESCE(inviter_id::text, ''), expires_at, accepted_at, created_at
		  FROM admin.invitations
		  WHERE id = $1
		  LIMIT 1`
	var inv ports.Invitation
	err := r.client.pool.QueryRow(ctx, q, id).Scan(
		&inv.ID, &inv.TenantID, &inv.Email, &inv.Role, &inv.TokenHash,
		&inv.InviterID, &inv.ExpiresAt, &inv.AcceptedAt, &inv.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get invitation by id: %w", err)
	}
	return &inv, nil
}

func (r *InvitationsRepo) FindActive(ctx context.Context, tokenHash string) (*ports.Invitation, error) {
	q := `SELECT id, tenant_id::text, email::text, role, token_hash,
				 COALESCE(inviter_id::text, ''), expires_at, accepted_at, created_at
		  FROM admin.invitations
		  WHERE token_hash = $1
		  LIMIT 1`
	var inv ports.Invitation
	err := r.client.pool.QueryRow(ctx, q, tokenHash).Scan(
		&inv.ID, &inv.TenantID, &inv.Email, &inv.Role, &inv.TokenHash,
		&inv.InviterID, &inv.ExpiresAt, &inv.AcceptedAt, &inv.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find invitation: %w", err)
	}
	return &inv, nil
}

func (r *InvitationsRepo) MarkAccepted(ctx context.Context, id string) error {
	q := `UPDATE admin.invitations SET accepted_at = NOW()
		  WHERE id = $1 AND accepted_at IS NULL`
	tag, err := r.client.pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("mark invitation accepted: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("invitation already accepted or missing")
	}
	return nil
}

func (r *InvitationsRepo) ListForTenant(ctx context.Context, tenantID string) ([]*ports.Invitation, error) {
	rows, err := r.client.pool.Query(ctx,
		`SELECT id, tenant_id::text, email::text, role, token_hash,
				COALESCE(inviter_id::text, ''), expires_at, accepted_at, created_at
		 FROM admin.invitations
		 WHERE tenant_id = $1
		 ORDER BY created_at DESC
		 LIMIT 100`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list invitations: %w", err)
	}
	defer rows.Close()
	var out []*ports.Invitation
	for rows.Next() {
		var inv ports.Invitation
		if err := rows.Scan(&inv.ID, &inv.TenantID, &inv.Email, &inv.Role, &inv.TokenHash,
			&inv.InviterID, &inv.ExpiresAt, &inv.AcceptedAt, &inv.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &inv)
	}
	return out, nil
}

func (r *InvitationsRepo) Preview(ctx context.Context, tokenHash string) (*ports.InvitationPreview, error) {
	q := `SELECT i.email::text, i.role, i.tenant_id::text,
				 COALESCE(t.name, ''), COALESCE(t.slug, ''),
				 COALESCE(u.email::text, ''), i.expires_at
		  FROM admin.invitations i
		  JOIN catalog.tenants t ON t.id = i.tenant_id
		  LEFT JOIN admin.admin_users u ON u.id = i.inviter_id
		  WHERE i.token_hash = $1 AND i.accepted_at IS NULL
		  LIMIT 1`
	var p ports.InvitationPreview
	err := r.client.pool.QueryRow(ctx, q, tokenHash).Scan(
		&p.Email, &p.Role, &p.TenantID, &p.TenantName, &p.TenantSlug,
		&p.InviterName, &p.ExpiresAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("preview invitation: %w", err)
	}
	return &p, nil
}

func (r *InvitationsRepo) CountRecentByInviter(ctx context.Context, inviterID string, since time.Time) (int, error) {
	var n int
	err := r.client.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM admin.invitations WHERE inviter_id = $1 AND created_at >= $2`,
		inviterID, since,
	).Scan(&n)
	return n, err
}
