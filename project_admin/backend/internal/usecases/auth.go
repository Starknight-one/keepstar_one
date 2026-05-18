package usecases

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/ports"
)

type AuthUseCase struct {
	auth        ports.AuthPort
	catalog     ports.AdminCatalogPort
	memberships ports.UserTenantsPort // optional; when set, Signup adds the creator as 'owner'
	secret      string
	sessions    *SessionsUseCase   // optional; if set, Login/Signup returns a real session pair
	twoFactor   *TwoFactorUseCase  // optional; if set, Login returns pre-2FA token when enabled
	pre2faTTL   time.Duration
}

func NewAuthUseCase(auth ports.AuthPort, catalog ports.AdminCatalogPort, jwtSecret string) *AuthUseCase {
	return &AuthUseCase{auth: auth, catalog: catalog, secret: jwtSecret, pre2faTTL: 5 * time.Minute}
}

// SetSessions wires the session usecase in after both have been constructed.
// When set, Login/Signup will issue a (access, refresh) pair via SessionsUseCase
// instead of a standalone 24h JWT. Legacy `token` field stays populated with
// the access token for one release.
func (uc *AuthUseCase) SetSessions(s *SessionsUseCase) { uc.sessions = s }

// SetMemberships wires the many-to-many membership adapter so Signup can
// record the creator as 'owner' in admin.user_tenants alongside the legacy
// admin_users.tenant_id write.
func (uc *AuthUseCase) SetMemberships(m ports.UserTenantsPort) { uc.memberships = m }

// SetTwoFactor wires the 2FA usecase so Login can branch into the pre-2FA
// flow when the user has TOTP enabled. When nil, Login always issues a full
// session pair (backwards-compat path).
func (uc *AuthUseCase) SetTwoFactor(t *TwoFactorUseCase, pre2faTTL time.Duration) {
	uc.twoFactor = t
	if pre2faTTL > 0 {
		uc.pre2faTTL = pre2faTTL
	}
}

type SignupRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	CompanyName string `json:"companyName"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	// Legacy single-token field — kept as alias of AccessToken for one release
	// so old frontends don't break. New clients read access_token/refresh_token.
	Token        string            `json:"token"`
	AccessToken  string            `json:"access_token,omitempty"`
	RefreshToken string            `json:"refresh_token,omitempty"`
	ExpiresIn    int64             `json:"expires_in,omitempty"`
	User         *domain.AdminUser `json:"user"`

	// When the user has 2FA enabled, Login returns these instead of a full
	// pair. Frontend sends Pre2FAToken to /2fa/verify/* to complete login.
	Requires2FA  bool   `json:"requires_2fa,omitempty"`
	Pre2FAToken  string `json:"pre_2fa_token,omitempty"`
	Has2FAEmail  bool   `json:"has_2fa_email,omitempty"`

	// LinkedFromEmail is set when an OAuth callback auto-merged a brand-new
	// provider identity (e.g. google_sub) onto an existing email-account.
	// Frontend renders a "Welcome back — we connected X to your account at
	// {LinkedFromEmail}" banner when non-empty. Empty value means no merge
	// happened (fresh signup or already-linked provider).
	LinkedFromEmail string `json:"linked_from_email,omitempty"`
}

func (uc *AuthUseCase) Signup(ctx context.Context, req SignupRequest) (*AuthResponse, error) {
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" || req.Password == "" || req.CompanyName == "" {
		return nil, fmt.Errorf("email, password, and companyName are required")
	}
	if len(req.Password) < 6 {
		return nil, fmt.Errorf("password must be at least 6 characters")
	}

	exists, err := uc.auth.EmailExists(ctx, req.Email)
	if err != nil {
		return nil, fmt.Errorf("check email: %w", err)
	}
	if exists {
		return nil, domain.ErrEmailExists
	}

	// Create tenant
	tenant := &domain.Tenant{
		Slug:     slugify(req.CompanyName),
		Name:     req.CompanyName,
		Type:     "retailer",
		Settings: map[string]any{"currency": "USD"},
	}
	tenant, err = uc.catalog.CreateTenant(ctx, tenant)
	if err != nil {
		return nil, fmt.Errorf("create tenant: %w", err)
	}

	// Hash password
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	// Create user
	user := &domain.AdminUser{
		Email:        req.Email,
		PasswordHash: string(hash),
		TenantID:     tenant.ID,
		Role:         domain.AdminRoleOwner,
	}
	user, err = uc.auth.CreateUser(ctx, user)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	if uc.memberships != nil {
		if err := uc.memberships.Add(ctx, user.ID, tenant.ID, "owner", ""); err != nil {
			return nil, fmt.Errorf("grant owner membership: %w", err)
		}
	}

	return uc.issueTokens(ctx, user, "", "")
}

func (uc *AuthUseCase) Login(ctx context.Context, req LoginRequest) (*AuthResponse, error) {
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" || req.Password == "" {
		return nil, fmt.Errorf("email and password are required")
	}

	user, err := uc.auth.GetUserByEmail(ctx, req.Email)
	if err != nil {
		return nil, domain.ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, domain.ErrInvalidCredentials
	}

	return uc.issueTokens(ctx, user, "", "")
}

// LoginWithMeta is the same as Login but threads user-agent + IP into the new
// session row so session list shows useful device metadata.
func (uc *AuthUseCase) LoginWithMeta(ctx context.Context, req LoginRequest, ua, ip string) (*AuthResponse, error) {
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" || req.Password == "" {
		return nil, fmt.Errorf("email and password are required")
	}
	user, err := uc.auth.GetUserByEmail(ctx, req.Email)
	if err != nil {
		return nil, domain.ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, domain.ErrInvalidCredentials
	}
	_ = uc.auth.TouchLastLogin(ctx, user.ID)
	return uc.issueTokens(ctx, user, ua, ip)
}

func (uc *AuthUseCase) issueTokens(ctx context.Context, user *domain.AdminUser, ua, ip string) (*AuthResponse, error) {
	// If 2FA is enabled for this user, we stop here and hand back a pre-2FA
	// token — frontend collects a code and calls /2fa/verify/*.
	if uc.twoFactor != nil {
		enabled, err := uc.twoFactor.Enabled(ctx, user.ID)
		if err == nil && enabled {
			pre, err := uc.generatePre2FAToken(user)
			if err != nil {
				return nil, err
			}
			return &AuthResponse{
				Requires2FA: true,
				Pre2FAToken: pre,
				User:        user,
			}, nil
		}
	}
	if uc.sessions != nil {
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
	// Fallback path — sessions usecase not wired, issue a 24h self-signed JWT.
	token, err := uc.generateToken(user)
	if err != nil {
		return nil, err
	}
	return &AuthResponse{Token: token, AccessToken: token, User: user}, nil
}

func (uc *AuthUseCase) generatePre2FAToken(user *domain.AdminUser) (string, error) {
	claims := jwt.MapClaims{
		"uid":   user.ID,
		"scope": "pre_2fa",
		"exp":   time.Now().Add(uc.pre2faTTL).Unix(),
		"iat":   time.Now().Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(uc.secret))
	if err != nil {
		return "", fmt.Errorf("sign pre-2fa token: %w", err)
	}
	return signed, nil
}

func (uc *AuthUseCase) GetMe(ctx context.Context, userID string) (*domain.AdminUser, error) {
	return uc.auth.GetUserByID(ctx, userID)
}

// SetPasswordForPasswordless lets an authenticated user who currently has no
// password (signed up via magic-link / OAuth) define one for the first time.
// Refuses if a password already exists — those users go through the regular
// password-reset flow.
//
// Scenario 39: shown as a one-shot promo after first magic-link / OAuth
// sign-in so the user can fall back to email+password later.
func (uc *AuthUseCase) SetPasswordForPasswordless(ctx context.Context, userID, newPassword string) error {
	if err := validatePasswordStrength(newPassword); err != nil {
		return err
	}
	user, err := uc.auth.GetUserByID(ctx, userID)
	if err != nil {
		return fmt.Errorf("load user: %w", err)
	}
	if user.PasswordHash != "" {
		return fmt.Errorf("password already set — use the reset flow")
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	return uc.auth.UpdatePasswordHash(ctx, userID, string(hashed))
}

func (uc *AuthUseCase) GetTenant(ctx context.Context, tenantID string) (*domain.Tenant, error) {
	return uc.catalog.GetTenantByID(ctx, tenantID)
}

func (uc *AuthUseCase) generateToken(user *domain.AdminUser) (string, error) {
	claims := jwt.MapClaims{
		"uid":  user.ID,
		"tid":  user.TenantID,
		"role": string(user.Role),
		"exp":  time.Now().Add(24 * time.Hour).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(uc.secret))
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return signed, nil
}

var nonAlphanumeric = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || r == '-' {
			b.WriteRune(r)
		}
	}
	slug := nonAlphanumeric.ReplaceAllString(b.String(), "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "store"
	}
	return slug
}
