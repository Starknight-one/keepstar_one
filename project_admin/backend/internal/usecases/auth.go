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
	auth    ports.AuthPort
	catalog ports.AdminCatalogPort
	secret  string
}

func NewAuthUseCase(auth ports.AuthPort, catalog ports.AdminCatalogPort, jwtSecret string) *AuthUseCase {
	return &AuthUseCase{auth: auth, catalog: catalog, secret: jwtSecret}
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
	Token string           `json:"token"`
	User  *domain.AdminUser `json:"user"`
}

func (uc *AuthUseCase) Signup(ctx context.Context, req SignupRequest) (*AuthResponse, error) {
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
		Settings: map[string]any{"currency": "RUB"},
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

	token, err := uc.generateToken(user)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{Token: token, User: user}, nil
}

func (uc *AuthUseCase) Login(ctx context.Context, req LoginRequest) (*AuthResponse, error) {
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

	token, err := uc.generateToken(user)
	if err != nil {
		return nil, err
	}

	return &AuthResponse{Token: token, User: user}, nil
}

func (uc *AuthUseCase) GetMe(ctx context.Context, userID string) (*domain.AdminUser, error) {
	return uc.auth.GetUserByID(ctx, userID)
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
