package usecases

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

// MagicLinkUseCase issues and consumes single-use sign-in links.
//
// The primary caller today is the Shopify install flow: a merchant who arrives
// from the App Store has no account in our system; after OAuth completes we
// fetch their shop email, create a passwordless admin_user, and email a magic
// link they can click from any device to land logged in.
//
// Issue is best-effort by design — it never fails the calling flow. If SMTP
// is misconfigured or the user lookup fails, we log and continue: the install
// itself shouldn't break because email delivery wobbled. Users always have a
// fallback path (Google/Telegram OAuth using the same email).
type MagicLinkUseCase struct {
	users       ports.AuthPort
	memberships ports.UserTenantsPort
	challenges  ports.ChallengePort
	mailer      ports.MailerPort
	sessions    *SessionsUseCase
	baseURL     string
	ttl         time.Duration
	log         *logger.Logger
}

func NewMagicLinkUseCase(
	users ports.AuthPort,
	memberships ports.UserTenantsPort,
	ch ports.ChallengePort,
	mailer ports.MailerPort,
	sessions *SessionsUseCase,
	baseURL string,
	ttl time.Duration,
	log *logger.Logger,
) *MagicLinkUseCase {
	return &MagicLinkUseCase{
		users:       users,
		memberships: memberships,
		challenges:  ch,
		mailer:      mailer,
		sessions:    sessions,
		baseURL:     baseURL,
		ttl:         ttl,
		log:         log,
	}
}

// Issue creates a magic-link challenge for the given user and emails it.
// Errors are logged and swallowed — the caller (e.g. Shopify install) treats
// magic-link delivery as a side effect, not a hard requirement.
func (uc *MagicLinkUseCase) Issue(ctx context.Context, userID, email string) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || userID == "" {
		uc.log.Warn("magic_link_issue_skipped_empty", "user_id", userID, "email_present", email != "")
		return
	}

	token := randomHex(32)
	ch := &ports.Challenge{
		UserID:    userID,
		Email:     email,
		Kind:      ports.ChallengeMagicLink,
		CodeHash:  hashCode(token),
		ExpiresAt: time.Now().Add(uc.ttl),
	}
	if err := uc.challenges.Create(ctx, ch); err != nil {
		uc.log.Error("magic_link_challenge_create_failed", "email", email, "error", err)
		return
	}

	link := fmt.Sprintf("%s/auth/magic?code=%s", uc.baseURL, token)
	if uc.mailer == nil {
		uc.log.Warn("magic_link_mailer_unconfigured", "email", email, "user_id", userID)
		return
	}
	if err := uc.mailer.Send(ctx, ports.EmailMessage{
		To:   email,
		Kind: "magic_link",
		Data: map[string]any{"Email": email, "Link": link, "TTL": int(uc.ttl.Minutes())},
	}); err != nil {
		uc.log.Error("magic_link_mail_failed", "email", email, "error", err)
		return
	}
	uc.log.Info("magic_link_issued", "email", email, "user_id", userID, "ttl_min", int(uc.ttl.Minutes()))
}

// ProvisionShopOwner is the install-flow user-provisioning entry point.
// Caller (the Shopify install completion hook in main.go) has already
// fetched the shop owner email; we own the user/membership/magic-link side.
//
//  1. Find or create an admin_user for the email. Reusing existing users by
//     email lets a merchant who already has a Keepstar account from
//     Google/Telegram attach this Shopify install to that account.
//  2. Grant owner membership on the freshly-provisioned tenant.
//  3. Issue + email a magic-link so the merchant can sign in from any device.
//
// All steps are best-effort: failures are logged but never propagated, since
// the integration row itself is already persisted upstream and a transient
// SMTP/DB error shouldn't undo a working install.
func (uc *MagicLinkUseCase) ProvisionShopOwner(ctx context.Context, tenantID, email string) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || tenantID == "" {
		uc.log.Warn("install_provision_skipped_empty", "tenant_id", tenantID, "email_present", email != "")
		return
	}

	user, err := uc.users.GetUserByEmail(ctx, email)
	if err != nil && !errors.Is(err, domain.ErrUserNotFound) {
		uc.log.Error("install_provision_user_lookup_failed",
			"tenant_id", tenantID, "email", email, "error", err)
		return
	}
	if user == nil || errors.Is(err, domain.ErrUserNotFound) {
		newUser := &domain.AdminUser{
			Email:    email,
			TenantID: tenantID,
			Role:     domain.AdminRoleOwner,
		}
		user, err = uc.users.CreateUser(ctx, newUser)
		if err != nil {
			uc.log.Error("install_provision_user_create_failed",
				"tenant_id", tenantID, "email", email, "error", err)
			return
		}
		uc.log.Info("install_provision_user_created",
			"tenant_id", tenantID, "user_id", user.ID, "email", email)
	}

	if uc.memberships != nil {
		if _, ok, _ := uc.memberships.HasMembership(ctx, user.ID, tenantID); !ok {
			if err := uc.memberships.Add(ctx, user.ID, tenantID, "owner", ""); err != nil {
				uc.log.Error("install_provision_membership_add_failed",
					"tenant_id", tenantID, "user_id", user.ID, "error", err)
				// fall through — still issue the magic-link; user just won't see
				// this tenant in their workspace picker until membership lands.
			}
		}
	}

	uc.Issue(ctx, user.ID, email)
}

// Consume validates a clicked link, mints a refresh+access pair, and consumes
// the challenge so the link can't be reused. ua/ip are optional metadata for
// the session list display.
func (uc *MagicLinkUseCase) Consume(ctx context.Context, code, ua, ip string) (*TokenPair, *domain.AdminUser, error) {
	if code == "" {
		return nil, nil, fmt.Errorf("missing code")
	}
	ch, err := uc.challenges.FindActive(ctx, ports.ChallengeMagicLink, hashCode(code))
	if err != nil {
		return nil, nil, fmt.Errorf("find magic-link challenge: %w", err)
	}
	if ch == nil {
		return nil, nil, domain.ErrInvalidCredentials
	}

	user, err := uc.users.GetUserByID(ctx, ch.UserID)
	if err != nil {
		return nil, nil, fmt.Errorf("load user: %w", err)
	}

	pair, err := uc.sessions.Issue(ctx, user, ua, ip)
	if err != nil {
		return nil, nil, fmt.Errorf("issue session: %w", err)
	}
	if err := uc.challenges.Consume(ctx, ch.ID); err != nil {
		// Token already minted — log but don't fail the user. A leaked challenge
		// row will expire on its own via DeleteExpired.
		uc.log.Error("magic_link_consume_cleanup_failed", "challenge_id", ch.ID, "error", err)
	}
	_ = uc.users.TouchLastLogin(ctx, user.ID)
	return pair, user, nil
}
