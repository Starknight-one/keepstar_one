package ports

import (
	"context"

	"keepstar-admin/internal/domain"
)

type AuthPort interface {
	GetUserByEmail(ctx context.Context, email string) (*domain.AdminUser, error)
	GetUserByID(ctx context.Context, id string) (*domain.AdminUser, error)
	CreateUser(ctx context.Context, user *domain.AdminUser) (*domain.AdminUser, error)
	EmailExists(ctx context.Context, email string) (bool, error)

	// Auth v2
	UpdatePasswordHash(ctx context.Context, userID, passwordHash string) error
	MarkEmailVerified(ctx context.Context, userID string) error
	TouchLastLogin(ctx context.Context, userID string) error

	// OAuth identity linkage
	GetUserByGoogleSub(ctx context.Context, sub string) (*domain.AdminUser, error)
	LinkGoogleSub(ctx context.Context, userID, sub, avatarURL string) error
	CreateOAuthUser(ctx context.Context, user *domain.AdminUser, googleSub, avatarURL string) (*domain.AdminUser, error)

	GetUserByTelegramID(ctx context.Context, telegramID int64) (*domain.AdminUser, error)
	LinkTelegramID(ctx context.Context, userID string, telegramID int64, handle, avatarURL string) error
	CreateTelegramUser(ctx context.Context, user *domain.AdminUser, telegramID int64, handle, avatarURL string) (*domain.AdminUser, error)

	// TOTP — secret is stored encrypted; enabled_at is flipped only after a
	// valid code is presented. GetTOTP returns ("", false) when not set.
	SetTOTPSecret(ctx context.Context, userID, encryptedSecret string) error
	GetTOTPSecret(ctx context.Context, userID string) (encryptedSecret string, enabled bool, err error)
	EnableTOTP(ctx context.Context, userID string) error
	DisableTOTP(ctx context.Context, userID string) error
}
