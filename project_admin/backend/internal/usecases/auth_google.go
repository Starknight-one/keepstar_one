package usecases

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"keepstar-admin/internal/adapters/google"
	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

// GoogleAuthUseCase handles "Continue with Google" at sign-in and sign-up. It
// mints a pre-auth state, redirects the user to Google's consent screen, and
// on callback:
//   - verifies+consumes the state (anti-CSRF)
//   - exchanges the authorization code for id_token claims
//   - links to an existing user (by google_sub, then by email) OR creates a
//     new account with a fresh tenant (first-login flow)
//   - issues a refresh+access pair via SessionsUseCase
type GoogleAuthUseCase struct {
	client      *google.Client
	states      ports.OAuthLoginStatePort
	users       ports.AuthPort
	catalog     ports.AdminCatalogPort
	memberships ports.UserTenantsPort
	sessions    *SessionsUseCase
	stateTTL    time.Duration
	log         *logger.Logger
}

func NewGoogleAuthUseCase(
	client *google.Client,
	states ports.OAuthLoginStatePort,
	users ports.AuthPort,
	catalog ports.AdminCatalogPort,
	memberships ports.UserTenantsPort,
	sessions *SessionsUseCase,
	log *logger.Logger,
) *GoogleAuthUseCase {
	return &GoogleAuthUseCase{
		client:      client,
		states:      states,
		users:       users,
		catalog:     catalog,
		memberships: memberships,
		sessions:    sessions,
		stateTTL:    10 * time.Minute,
		log:         log,
	}
}

type GoogleStartResult struct {
	URL   string `json:"url"`
	State string `json:"state"`
}

func (uc *GoogleAuthUseCase) Start(ctx context.Context) (*GoogleStartResult, error) {
	state := randomHex(32)
	now := time.Now().UTC()
	if err := uc.states.Create(ctx, &ports.OAuthLoginState{
		State:     state,
		Kind:      "google_login",
		CreatedAt: now,
		ExpiresAt: now.Add(uc.stateTTL),
	}); err != nil {
		return nil, fmt.Errorf("persist state: %w", err)
	}
	return &GoogleStartResult{
		URL:   uc.client.BuildAuthURL(state),
		State: state,
	}, nil
}

// Complete is the callback-side handler. Returns the freshly minted token
// pair plus the user record. Errors map 1:1 to the UI AuthErrorPage
// state-mismatch / expired / google_rejected reasons.
func (uc *GoogleAuthUseCase) Complete(ctx context.Context, code, state, ua, ip string) (*AuthResponse, error) {
	if code == "" || state == "" {
		return nil, errors.New("missing code or state")
	}
	record, err := uc.states.Consume(ctx, state)
	if err != nil {
		return nil, fmt.Errorf("invalid or expired state")
	}
	if record.Kind != "google_login" {
		return nil, errors.New("state kind mismatch")
	}

	info, err := uc.client.Exchange(ctx, code)
	if err != nil {
		uc.log.Warn("google_exchange_failed", "error", err)
		return nil, fmt.Errorf("google rejected the authorization")
	}

	user, err := uc.findOrCreate(ctx, info)
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

// findOrCreate resolves a google_sub to an admin user in three steps:
//  1. Lookup by google_sub — fastest path, already-linked account.
//  2. Lookup by email — a user who originally signed up with email/password
//     and is now using Google for the first time. We link the sub to their
//     row so subsequent logins hit step 1.
//  3. New user — create a fresh tenant + passwordless user.
func (uc *GoogleAuthUseCase) findOrCreate(ctx context.Context, info *google.UserInfo) (*domain.AdminUser, error) {
	email := strings.ToLower(strings.TrimSpace(info.Email))

	// Step 1
	if u, err := uc.users.GetUserByGoogleSub(ctx, info.Sub); err == nil {
		return u, nil
	} else if !errors.Is(err, domain.ErrUserNotFound) {
		return nil, fmt.Errorf("lookup by google_sub: %w", err)
	}

	// Step 2
	if u, err := uc.users.GetUserByEmail(ctx, email); err == nil {
		if err := uc.users.LinkGoogleSub(ctx, u.ID, info.Sub, info.Picture); err != nil {
			return nil, fmt.Errorf("link google_sub: %w", err)
		}
		return u, nil
	} else if !errors.Is(err, domain.ErrUserNotFound) {
		return nil, fmt.Errorf("lookup by email: %w", err)
	}

	// Step 3 — fresh signup. Use Google's name as the company name placeholder;
	// the user can rename the workspace later from Settings.
	companyName := info.Name
	if companyName == "" {
		companyName = strings.SplitN(email, "@", 2)[0]
	}
	tenant, err := uc.catalog.CreateTenant(ctx, &domain.Tenant{
		Slug:     slugify(companyName),
		Name:     companyName,
		Type:     "retailer",
		Settings: map[string]any{"currency": "USD"},
	})
	if err != nil {
		return nil, fmt.Errorf("create tenant: %w", err)
	}
	user := &domain.AdminUser{
		Email:    email,
		TenantID: tenant.ID,
		Role:     domain.AdminRoleOwner,
	}
	user, err = uc.users.CreateOAuthUser(ctx, user, info.Sub, info.Picture)
	if err != nil {
		return nil, fmt.Errorf("create oauth user: %w", err)
	}
	if uc.memberships != nil {
		if err := uc.memberships.Add(ctx, user.ID, tenant.ID, "owner", ""); err != nil {
			return nil, fmt.Errorf("grant owner membership: %w", err)
		}
	}
	return user, nil
}
