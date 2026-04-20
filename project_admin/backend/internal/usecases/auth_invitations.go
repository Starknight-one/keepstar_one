package usecases

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

// InvitationsUseCase handles creating workspace invites, previewing them by
// token, and accepting them — both for brand-new users (who set a password)
// and for users who are already signed in (just adds the membership row).
type InvitationsUseCase struct {
	invites      ports.InvitationPort
	users        ports.AuthPort
	memberships  ports.UserTenantsPort
	catalog      ports.AdminCatalogPort
	sessions     *SessionsUseCase
	mailer       ports.MailerPort
	baseURL      string
	inviteTTL    time.Duration
	maxPerDay    int
	log          *logger.Logger
}

func NewInvitationsUseCase(
	invites ports.InvitationPort,
	users ports.AuthPort,
	memberships ports.UserTenantsPort,
	catalog ports.AdminCatalogPort,
	sessions *SessionsUseCase,
	mailer ports.MailerPort,
	baseURL string,
	inviteTTL time.Duration,
	log *logger.Logger,
) *InvitationsUseCase {
	return &InvitationsUseCase{
		invites:     invites,
		users:       users,
		memberships: memberships,
		catalog:     catalog,
		sessions:    sessions,
		mailer:      mailer,
		baseURL:     baseURL,
		inviteTTL:   inviteTTL,
		maxPerDay:   20,
		log:         log,
	}
}

// Create is called by a workspace owner/admin. inviterID is the authenticated
// caller; tenantID is the workspace they are inviting someone into. The
// caller's role has already been verified by the handler.
func (uc *InvitationsUseCase) Create(ctx context.Context, tenantID, inviterID, email, role string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return errors.New("email required")
	}
	if !isValidRole(role) {
		return errors.New("invalid role")
	}

	// Rate-limit: at most N invites per inviter per 24h.
	since := time.Now().Add(-24 * time.Hour)
	n, err := uc.invites.CountRecentByInviter(ctx, inviterID, since)
	if err == nil && n >= uc.maxPerDay {
		return errors.New("invite quota exceeded — try again tomorrow")
	}

	token := randomHex(32)
	inv := &ports.Invitation{
		TenantID:  tenantID,
		Email:     email,
		Role:      role,
		TokenHash: hashCode(token),
		InviterID: inviterID,
		ExpiresAt: time.Now().Add(uc.inviteTTL),
	}
	if err := uc.invites.Create(ctx, inv); err != nil {
		return fmt.Errorf("create invitation: %w", err)
	}

	link := fmt.Sprintf("%s/auth/accept-invite?token=%s", uc.baseURL, token)
	if uc.mailer != nil {
		tenant, _ := uc.catalog.GetTenantByID(ctx, tenantID)
		tenantName := tenantID
		if tenant != nil {
			tenantName = tenant.Name
		}
		inviter, _ := uc.users.GetUserByID(ctx, inviterID)
		inviterName := "Someone"
		if inviter != nil {
			inviterName = inviter.Email
		}
		if err := uc.mailer.Send(ctx, ports.EmailMessage{
			To:   email,
			Kind: "invitation",
			Data: map[string]any{
				"Inviter":    inviterName,
				"TenantName": tenantName,
				"Role":       role,
				"Link":       link,
			},
		}); err != nil {
			uc.log.Error("invitation_mail_failed", "email", email, "error", err)
		}
	}
	return nil
}

// Preview returns tenant + inviter details for the /auth/accept-invite page.
// Does NOT consume the invitation.
func (uc *InvitationsUseCase) Preview(ctx context.Context, token string) (*ports.InvitationPreview, error) {
	p, err := uc.invites.Preview(ctx, hashCode(token))
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, domain.ErrInvalidCredentials
	}
	if time.Now().After(p.ExpiresAt) {
		return nil, domain.ErrInvalidCredentials
	}
	return p, nil
}

// AcceptRequest holds everything the accept endpoint might need. For a new
// user we require Password; for an already-authenticated user we ignore it
// and just link the membership to their CurrentUserID.
type AcceptRequest struct {
	Token          string
	Password       string
	CurrentUserID  string // empty when the invitee is logged out
	UserAgent      string
	IP             string
}

// Accept redeems a token:
//   - logged-out invitee → create a passwordful user (or link if email
//     already exists) + add membership + issue session
//   - logged-in invitee → just add membership to their account (password
//     from the request is ignored)
func (uc *InvitationsUseCase) Accept(ctx context.Context, req AcceptRequest) (*AuthResponse, error) {
	inv, err := uc.invites.FindActive(ctx, hashCode(req.Token))
	if err != nil {
		return nil, fmt.Errorf("find invitation: %w", err)
	}
	if inv == nil {
		return nil, domain.ErrInvalidCredentials
	}
	if inv.AcceptedAt != nil {
		return nil, errors.New("invitation already accepted")
	}
	if time.Now().After(inv.ExpiresAt) {
		return nil, errors.New("invitation expired")
	}

	// Path A — caller is already signed in. Just link membership.
	if req.CurrentUserID != "" {
		if err := uc.memberships.Add(ctx, req.CurrentUserID, inv.TenantID, inv.Role, inv.InviterID); err != nil {
			return nil, fmt.Errorf("add membership: %w", err)
		}
		if err := uc.invites.MarkAccepted(ctx, inv.ID); err != nil {
			return nil, err
		}
		user, err := uc.users.GetUserByID(ctx, req.CurrentUserID)
		if err != nil {
			return nil, err
		}
		return &AuthResponse{User: user}, nil
	}

	// Path B — invitee is logged out. Either they have an account already
	// (link + sign them in) or we create a new one from the invite email.
	existing, err := uc.users.GetUserByEmail(ctx, inv.Email)
	if err != nil && !errors.Is(err, domain.ErrUserNotFound) {
		return nil, fmt.Errorf("lookup by email: %w", err)
	}

	var user *domain.AdminUser
	if existing != nil {
		// Don't touch the password — they already have one. Just add membership.
		user = existing
	} else {
		if err := validatePasswordStrength(req.Password); err != nil {
			return nil, err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return nil, fmt.Errorf("hash password: %w", err)
		}
		newUser := &domain.AdminUser{
			Email:        inv.Email,
			PasswordHash: string(hash),
			TenantID:     inv.TenantID,
			Role:         domain.AdminRole(inv.Role),
		}
		user, err = uc.users.CreateUser(ctx, newUser)
		if err != nil {
			return nil, fmt.Errorf("create invited user: %w", err)
		}
	}

	if err := uc.memberships.Add(ctx, user.ID, inv.TenantID, inv.Role, inv.InviterID); err != nil {
		return nil, fmt.Errorf("add membership: %w", err)
	}
	if err := uc.invites.MarkAccepted(ctx, inv.ID); err != nil {
		return nil, err
	}

	_ = uc.users.TouchLastLogin(ctx, user.ID)
	pair, err := uc.sessions.IssueForTenant(ctx, user, inv.TenantID, domain.AdminRole(inv.Role), req.UserAgent, req.IP)
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

func isValidRole(r string) bool {
	switch r {
	case "owner", "admin", "member", "viewer":
		return true
	}
	return false
}
