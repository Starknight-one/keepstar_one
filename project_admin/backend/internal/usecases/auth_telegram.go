package usecases

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"keepstar-admin/internal/adapters/telegram"
	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

// TelegramAuthUseCase wraps the Telegram Login Widget flow. The widget posts
// a payload directly to the frontend which forwards it to `Complete` — there
// is no server-side state since the HMAC signature authenticates the request.
type TelegramAuthUseCase struct {
	botToken    string
	users       ports.AuthPort
	catalog     ports.AdminCatalogPort
	memberships ports.UserTenantsPort
	sessions    *SessionsUseCase
	log         *logger.Logger
}

func NewTelegramAuthUseCase(
	botToken string,
	users ports.AuthPort,
	catalog ports.AdminCatalogPort,
	memberships ports.UserTenantsPort,
	sessions *SessionsUseCase,
	log *logger.Logger,
) *TelegramAuthUseCase {
	return &TelegramAuthUseCase{
		botToken:    botToken,
		users:       users,
		catalog:     catalog,
		memberships: memberships,
		sessions:    sessions,
		log:         log,
	}
}

func (uc *TelegramAuthUseCase) Complete(ctx context.Context, fields map[string]string, ua, ip string) (*AuthResponse, error) {
	tUser, err := telegram.Verify(fields, uc.botToken)
	if err != nil {
		uc.log.Warn("telegram_verify_failed", "error", err)
		return nil, fmt.Errorf("telegram verification failed")
	}
	user, err := uc.findOrCreate(ctx, tUser)
	if err != nil {
		return nil, err
	}
	_ = uc.users.TouchLastLogin(ctx, user.ID)

	pair, err := uc.sessions.Issue(ctx, user, ua, ip)
	if err != nil {
		return nil, err
	}
	return &AuthResponse{
		Token:        pair.AccessToken,
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		ExpiresIn:    pair.ExpiresIn,
		User:         user,
	}, nil
}

func (uc *TelegramAuthUseCase) findOrCreate(ctx context.Context, t *telegram.User) (*domain.AdminUser, error) {
	if u, err := uc.users.GetUserByTelegramID(ctx, t.ID); err == nil {
		return u, nil
	} else if !errors.Is(err, domain.ErrUserNotFound) {
		return nil, fmt.Errorf("lookup by telegram_id: %w", err)
	}

	// Telegram doesn't expose email — we synthesize a placeholder based on id /
	// username to satisfy admin_users.email NOT NULL. Users can rename later.
	synthLocal := t.Username
	if synthLocal == "" {
		synthLocal = fmt.Sprintf("tg%d", t.ID)
	}
	syntheticEmail := strings.ToLower(synthLocal) + "@telegram.keepstar.local"

	companyName := strings.TrimSpace(t.FirstName + " " + t.LastName)
	if companyName == "" {
		companyName = synthLocal
	}
	tenant, err := uc.catalog.CreateTenant(ctx, &domain.Tenant{
		Slug:     slugify(companyName + "-" + fmt.Sprint(t.ID)),
		Name:     companyName,
		Type:     "retailer",
		Settings: map[string]any{"currency": "USD"},
	})
	if err != nil {
		return nil, fmt.Errorf("create tenant: %w", err)
	}
	user := &domain.AdminUser{
		Email:    syntheticEmail,
		TenantID: tenant.ID,
		Role:     domain.AdminRoleOwner,
	}
	user, err = uc.users.CreateTelegramUser(ctx, user, t.ID, t.Username, t.PhotoURL)
	if err != nil {
		return nil, fmt.Errorf("create telegram user: %w", err)
	}
	if uc.memberships != nil {
		if err := uc.memberships.Add(ctx, user.ID, tenant.ID, "owner", ""); err != nil {
			return nil, fmt.Errorf("grant owner membership: %w", err)
		}
	}
	return user, nil
}
