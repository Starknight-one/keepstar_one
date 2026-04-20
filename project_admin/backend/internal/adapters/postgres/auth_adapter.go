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
	query := `SELECT id, email, COALESCE(password_hash, ''), COALESCE(tenant_id::text, ''), role, created_at
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
	query := `SELECT id, email, COALESCE(password_hash, ''), COALESCE(tenant_id::text, ''), role, created_at
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

func (a *AuthAdapter) UpdatePasswordHash(ctx context.Context, userID, passwordHash string) error {
	q := `UPDATE admin.admin_users SET password_hash = $2, updated_at = NOW() WHERE id = $1`
	_, err := a.client.pool.Exec(ctx, q, userID, passwordHash)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	return nil
}

func (a *AuthAdapter) MarkEmailVerified(ctx context.Context, userID string) error {
	q := `UPDATE admin.admin_users SET email_verified_at = NOW(), updated_at = NOW()
		  WHERE id = $1 AND email_verified_at IS NULL`
	_, err := a.client.pool.Exec(ctx, q, userID)
	if err != nil {
		return fmt.Errorf("mark email verified: %w", err)
	}
	return nil
}

func (a *AuthAdapter) TouchLastLogin(ctx context.Context, userID string) error {
	q := `UPDATE admin.admin_users SET last_login_at = NOW() WHERE id = $1`
	_, err := a.client.pool.Exec(ctx, q, userID)
	if err != nil {
		return fmt.Errorf("touch last_login: %w", err)
	}
	return nil
}

func (a *AuthAdapter) GetUserByGoogleSub(ctx context.Context, sub string) (*domain.AdminUser, error) {
	query := `SELECT id, email, COALESCE(password_hash, ''), COALESCE(tenant_id::text, ''), role, created_at
		FROM admin.admin_users WHERE google_sub = $1`
	var u domain.AdminUser
	err := a.client.pool.QueryRow(ctx, query, sub).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.TenantID, &u.Role, &u.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, fmt.Errorf("query user by google sub: %w", err)
	}
	return &u, nil
}

func (a *AuthAdapter) LinkGoogleSub(ctx context.Context, userID, sub, avatarURL string) error {
	q := `UPDATE admin.admin_users
		SET google_sub = $2,
		    avatar_url = COALESCE(NULLIF($3,''), avatar_url),
		    email_verified_at = COALESCE(email_verified_at, NOW()),
		    updated_at = NOW()
		WHERE id = $1`
	_, err := a.client.pool.Exec(ctx, q, userID, sub, avatarURL)
	if err != nil {
		return fmt.Errorf("link google sub: %w", err)
	}
	return nil
}

func (a *AuthAdapter) GetUserByTelegramID(ctx context.Context, telegramID int64) (*domain.AdminUser, error) {
	query := `SELECT id, email, COALESCE(password_hash, ''), COALESCE(tenant_id::text, ''), role, created_at
		FROM admin.admin_users WHERE telegram_id = $1`
	var u domain.AdminUser
	err := a.client.pool.QueryRow(ctx, query, telegramID).Scan(
		&u.ID, &u.Email, &u.PasswordHash, &u.TenantID, &u.Role, &u.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, fmt.Errorf("query user by telegram id: %w", err)
	}
	return &u, nil
}

func (a *AuthAdapter) LinkTelegramID(ctx context.Context, userID string, telegramID int64, handle, avatarURL string) error {
	q := `UPDATE admin.admin_users
		SET telegram_id = $2,
		    telegram_handle = NULLIF($3,''),
		    avatar_url = COALESCE(NULLIF($4,''), avatar_url),
		    updated_at = NOW()
		WHERE id = $1`
	_, err := a.client.pool.Exec(ctx, q, userID, telegramID, handle, avatarURL)
	if err != nil {
		return fmt.Errorf("link telegram id: %w", err)
	}
	return nil
}

// CreateTelegramUser inserts a passwordless user from Telegram Login. We
// synthesize an email from the username (or numeric id) because the schema
// still treats it as NOT NULL — operators can rename later from Settings.
func (a *AuthAdapter) CreateTelegramUser(ctx context.Context, user *domain.AdminUser, telegramID int64, handle, avatarURL string) (*domain.AdminUser, error) {
	query := `INSERT INTO admin.admin_users
		(email, tenant_id, role, telegram_id, telegram_handle, avatar_url)
		VALUES ($1, $2, $3, $4, NULLIF($5,''), NULLIF($6,''))
		RETURNING id, created_at`
	err := a.client.pool.QueryRow(ctx, query,
		user.Email, user.TenantID, user.Role,
		telegramID, handle, avatarURL,
	).Scan(&user.ID, &user.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create telegram user: %w", err)
	}
	return user, nil
}

func (a *AuthAdapter) SetTOTPSecret(ctx context.Context, userID, encryptedSecret string) error {
	q := `UPDATE admin.admin_users
		SET totp_secret_encrypted = $2, totp_enabled_at = NULL, updated_at = NOW()
		WHERE id = $1`
	_, err := a.client.pool.Exec(ctx, q, userID, encryptedSecret)
	if err != nil {
		return fmt.Errorf("set totp secret: %w", err)
	}
	return nil
}

func (a *AuthAdapter) GetTOTPSecret(ctx context.Context, userID string) (string, bool, error) {
	var secret *string
	var enabledAt *string
	err := a.client.pool.QueryRow(ctx,
		`SELECT totp_secret_encrypted, totp_enabled_at::text FROM admin.admin_users WHERE id = $1`,
		userID).Scan(&secret, &enabledAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, domain.ErrUserNotFound
		}
		return "", false, fmt.Errorf("get totp secret: %w", err)
	}
	if secret == nil {
		return "", false, nil
	}
	return *secret, enabledAt != nil, nil
}

func (a *AuthAdapter) EnableTOTP(ctx context.Context, userID string) error {
	q := `UPDATE admin.admin_users SET totp_enabled_at = NOW(), updated_at = NOW()
	      WHERE id = $1 AND totp_secret_encrypted IS NOT NULL`
	tag, err := a.client.pool.Exec(ctx, q, userID)
	if err != nil {
		return fmt.Errorf("enable totp: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errors.New("totp secret not set")
	}
	return nil
}

func (a *AuthAdapter) DisableTOTP(ctx context.Context, userID string) error {
	q := `UPDATE admin.admin_users
	      SET totp_enabled_at = NULL, totp_secret_encrypted = NULL, updated_at = NOW()
	      WHERE id = $1`
	_, err := a.client.pool.Exec(ctx, q, userID)
	if err != nil {
		return fmt.Errorf("disable totp: %w", err)
	}
	return nil
}

// CreateOAuthUser inserts a passwordless user created via Google sign-in. The
// tenant must already exist. The Google sub + avatar are written atomically
// with the row — so a partial create can't leave an orphan identity.
func (a *AuthAdapter) CreateOAuthUser(ctx context.Context, user *domain.AdminUser, googleSub, avatarURL string) (*domain.AdminUser, error) {
	query := `INSERT INTO admin.admin_users
		(email, tenant_id, role, google_sub, avatar_url, email_verified_at)
		VALUES ($1, $2, $3, $4, NULLIF($5,''), NOW())
		RETURNING id, created_at`
	err := a.client.pool.QueryRow(ctx, query,
		user.Email, user.TenantID, user.Role, googleSub, avatarURL,
	).Scan(&user.ID, &user.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create oauth user: %w", err)
	}
	return user, nil
}
