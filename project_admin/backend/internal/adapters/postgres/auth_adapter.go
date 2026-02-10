package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"keepstar-admin/internal/domain"
)

type AuthAdapter struct {
	client *Client
}

func NewAuthAdapter(client *Client) *AuthAdapter {
	return &AuthAdapter{client: client}
}

func (a *AuthAdapter) GetUserByEmail(ctx context.Context, email string) (*domain.AdminUser, error) {
	query := `SELECT id, email, password_hash, tenant_id, role, created_at
		FROM admin.admin_users WHERE email = $1`

	var u domain.AdminUser
	err := a.client.pool.QueryRow(ctx, query, email).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.TenantID, &u.Role, &u.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, fmt.Errorf("query user by email: %w", err)
	}
	return &u, nil
}

func (a *AuthAdapter) GetUserByID(ctx context.Context, id string) (*domain.AdminUser, error) {
	query := `SELECT id, email, password_hash, tenant_id, role, created_at
		FROM admin.admin_users WHERE id = $1`

	var u domain.AdminUser
	err := a.client.pool.QueryRow(ctx, query, id).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.TenantID, &u.Role, &u.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, fmt.Errorf("query user by id: %w", err)
	}
	return &u, nil
}

func (a *AuthAdapter) CreateUser(ctx context.Context, user *domain.AdminUser) (*domain.AdminUser, error) {
	query := `INSERT INTO admin.admin_users (email, password_hash, tenant_id, role)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`

	err := a.client.pool.QueryRow(ctx, query,
		user.Email, user.PasswordHash, user.TenantID, user.Role,
	).Scan(&user.ID, &user.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return user, nil
}

func (a *AuthAdapter) EmailExists(ctx context.Context, email string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM admin.admin_users WHERE email = $1)`
	var exists bool
	if err := a.client.pool.QueryRow(ctx, query, email).Scan(&exists); err != nil {
		return false, fmt.Errorf("check email exists: %w", err)
	}
	return exists, nil
}
