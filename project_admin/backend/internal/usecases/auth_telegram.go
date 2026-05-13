package usecases

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"keepstar-admin/internal/adapters/telegram"
	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

// TelegramAuthUseCase handles both the legacy Login Widget flow and the new
// OIDC redirect flow. The OIDC client is optional — when nil the widget path
// remains active (backward compat). When both are configured, OIDC takes
// precedence in the frontend; the widget endpoint stays registered for safety.
type TelegramAuthUseCase struct {
	botToken    string
	oidcClient  *telegram.OIDCClient
	states      ports.OAuthLoginStatePort
	users       ports.AuthPort
	catalog     ports.AdminCatalogPort
	memberships ports.UserTenantsPort
	sessions    *SessionsUseCase
	stateTTL    time.Duration
	log         *logger.Logger
}

func NewTelegramAuthUseCase(
	botToken string,
	oidcClient *telegram.OIDCClient,
	states ports.OAuthLoginStatePort,
	users ports.AuthPort,
	catalog ports.AdminCatalogPort,
	memberships ports.UserTenantsPort,
	sessions *SessionsUseCase,
	log *logger.Logger,
) *TelegramAuthUseCase {
	return &TelegramAuthUseCase{
		botToken:    botToken,
		oidcClient:  oidcClient,
		states:      states,
		users:       users,
		catalog:     catalog,
		memberships: memberships,
		sessions:    sessions,
		stateTTL:    10 * time.Minute,
		log:         log,
	}
}

// HasOIDC reports whether the OIDC redirect flow is configured.
func (uc *TelegramAuthUseCase) HasOIDC() bool { return uc.oidcClient != nil }

// ---- OIDC redirect flow ----

type TelegramStartResult struct {
	URL string `json:"url"`
}

// Start mints a state nonce and returns the Telegram consent URL.
func (uc *TelegramAuthUseCase) Start(ctx context.Context) (*TelegramStartResult, error) {
	if uc.oidcClient == nil {
		return nil, errors.New("telegram oidc not configured")
	}
	state := randomHex(32)
	now := time.Now().UTC()
	if err := uc.states.Create(ctx, &ports.OAuthLoginState{
		State:     state,
		Kind:      "telegram_login",
		CreatedAt: now,
		ExpiresAt: now.Add(uc.stateTTL),
	}); err != nil {
		return nil, fmt.Errorf("persist state: %w", err)
	}
	return &TelegramStartResult{URL: uc.oidcClient.BuildAuthURL(state)}, nil
}

// CompleteOIDC is the callback-side handler for the OIDC redirect flow.
func (uc *TelegramAuthUseCase) CompleteOIDC(ctx context.Context, code, state, ua, ip string) (*AuthResponse, error) {
	if uc.oidcClient == nil {
		return nil, errors.New("telegram oidc not configured")
	}
	if code == "" || state == "" {
		return nil, errors.New("missing code or state")
	}
	record, err := uc.states.Consume(ctx, state)
	if err != nil {
		return nil, fmt.Errorf("invalid or expired state")
	}
	if record.Kind != "telegram_login" {
		return nil, errors.New("state kind mismatch")
	}

	info, err := uc.oidcClient.Exchange(ctx, code)
	if err != nil {
		uc.log.Warn("telegram_oidc_exchange_failed", "error", err)
		return nil, fmt.Errorf("telegram rejected the authorization")
	}

	user, err := uc.findOrCreateOIDC(ctx, info)
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

func (uc *TelegramAuthUseCase) findOrCreateOIDC(ctx context.Context, info *telegram.OIDCUserInfo) (*domain.AdminUser, error) {
	tgID, err := strconv.ParseInt(info.Sub, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid telegram sub: %w", err)
	}

	if u, err := uc.users.GetUserByTelegramID(ctx, tgID); err == nil {
		return u, nil
	} else if !errors.Is(err, domain.ErrUserNotFound) {
		return nil, fmt.Errorf("lookup by telegram_id: %w", err)
	}

	synthLocal := info.Username
	if synthLocal == "" {
		synthLocal = fmt.Sprintf("tg%d", tgID)
	}
	syntheticEmail := strings.ToLower(synthLocal) + "@telegram.keepstar.local"

	companyName := strings.TrimSpace(info.FirstName + " " + info.LastName)
	if companyName == "" {
		companyName = synthLocal
	}
	tenant, err := uc.catalog.CreateTenant(ctx, &domain.Tenant{
		Slug:     slugify(companyName + "-" + fmt.Sprint(tgID)),
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
	user, err = uc.users.CreateTelegramUser(ctx, user, tgID, info.Username, info.PhotoURL)
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

// ---- Legacy Login Widget flow (kept for backward compat) ----

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
