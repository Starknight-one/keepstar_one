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
}
