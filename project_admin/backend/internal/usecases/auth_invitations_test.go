package usecases

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

// fakeInvitations is an in-memory InvitationPort.
type fakeInvitations struct {
	byHash    map[string]*ports.Invitation
	byID      map[string]*ports.Invitation
	preview   map[string]*ports.InvitationPreview
	createCnt int
}

func newFakeInvitations() *fakeInvitations {
	return &fakeInvitations{
		byHash:  map[string]*ports.Invitation{},
		byID:    map[string]*ports.Invitation{},
		preview: map[string]*ports.InvitationPreview{},
	}
}

func (f *fakeInvitations) Create(_ context.Context, inv *ports.Invitation) error {
	f.createCnt++
	inv.ID = "inv-" + inv.TokenHash[:8]
	inv.CreatedAt = time.Now()
	f.byHash[inv.TokenHash] = inv
	f.byID[inv.ID] = inv
	return nil
}
func (f *fakeInvitations) FindActive(_ context.Context, tokenHash string) (*ports.Invitation, error) {
	inv, ok := f.byHash[tokenHash]
	if !ok {
		return nil, nil
	}
	return inv, nil
}
func (f *fakeInvitations) MarkAccepted(_ context.Context, id string) error {
	inv, ok := f.byID[id]
	if !ok {
		return errors.New("not found")
	}
	now := time.Now()
	inv.AcceptedAt = &now
	return nil
}
func (f *fakeInvitations) ListForTenant(_ context.Context, tenantID string) ([]*ports.Invitation, error) {
	var out []*ports.Invitation
	for _, inv := range f.byID {
		if inv.TenantID == tenantID {
			out = append(out, inv)
		}
	}
	return out, nil
}
func (f *fakeInvitations) Preview(_ context.Context, tokenHash string) (*ports.InvitationPreview, error) {
	inv, ok := f.byHash[tokenHash]
	if !ok {
		return nil, nil
	}
	return &ports.InvitationPreview{
		Email:     inv.Email,
		Role:      inv.Role,
		TenantID:  inv.TenantID,
		ExpiresAt: inv.ExpiresAt,
	}, nil
}
func (f *fakeInvitations) CountRecentByInviter(_ context.Context, inviterID string, _ time.Time) (int, error) {
	cnt := 0
	for _, inv := range f.byID {
		if inv.InviterID == inviterID {
			cnt++
		}
	}
	return cnt, nil
}

func mkInvitesUC() (*InvitationsUseCase, *fakeInvitations, *statefulFakeAuth, *fakeMemberships, *fakeAdminCatalog, *fakeMailer) {
	inv := newFakeInvitations()
	auth := newStatefulFakeAuth()
	mem := newFakeMemberships()
	cat := newFakeAdminCatalog()
	mail := &fakeMailer{}
	sess := newRichSessions()
	sessUC := NewSessionsUseCase(sess, auth, "test-secret-32-bytes-long-xxxxxx",
		15*time.Minute, 30*24*time.Hour, logger.New("error"))
	uc := NewInvitationsUseCase(inv, auth, mem, cat, sessUC, mail,
		"https://app.example.com", 7*24*time.Hour, logger.New("error"))
	return uc, inv, auth, mem, cat, mail
}

func TestInvite_CreateHappyPath(t *testing.T) {
	uc, inv, auth, _, cat, mail := mkInvitesUC()
	// Pre-seed inviter + tenant so the email rendering paths run.
	inviter, _ := auth.CreateUser(context.Background(), &domain.AdminUser{Email: "owner@x.com"})
	cat.tenants["t-1"] = &domain.Tenant{ID: "t-1", Slug: "acme", Name: "Acme"}

	if err := uc.Create(context.Background(), "t-1", inviter.ID, "guest@x.com", "admin"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if inv.createCnt != 1 {
		t.Errorf("invitations stored = %d, want 1", inv.createCnt)
	}
	if len(mail.sent) != 1 {
		t.Fatalf("emails sent = %d, want 1", len(mail.sent))
	}
	if mail.sent[0].Kind != "invitation" {
		t.Errorf("kind = %q, want invitation", mail.sent[0].Kind)
	}
	link, _ := mail.sent[0].Data["Link"].(string)
	if !strings.HasPrefix(link, "https://app.example.com/auth/accept-invite?token=") {
		t.Errorf("link malformed: %q", link)
	}
}

func TestInvite_CreateRejectsEmptyEmail(t *testing.T) {
	uc, _, _, _, _, _ := mkInvitesUC()
	if err := uc.Create(context.Background(), "t-1", "u1", "", "admin"); err == nil {
		t.Error("expected err on empty email")
	}
}

func TestInvite_CreateRejectsInvalidRole(t *testing.T) {
	uc, _, _, _, _, _ := mkInvitesUC()
	if err := uc.Create(context.Background(), "t-1", "u1", "x@x.com", "superuser"); err == nil {
		t.Error("expected err on invalid role")
	}
}

func TestInvite_CreateLowercasesEmail(t *testing.T) {
	uc, inv, auth, _, cat, _ := mkInvitesUC()
	inviter, _ := auth.CreateUser(context.Background(), &domain.AdminUser{Email: "owner@x.com"})
	cat.tenants["t-1"] = &domain.Tenant{ID: "t-1", Slug: "acme", Name: "Acme"}

	_ = uc.Create(context.Background(), "t-1", inviter.ID, "  CASE@X.COM  ", "admin")

	var stored *ports.Invitation
	for _, i := range inv.byID {
		stored = i
	}
	if stored == nil || stored.Email != "case@x.com" {
		t.Errorf("email not normalized: %+v", stored)
	}
}

func TestInvite_Preview(t *testing.T) {
	uc, inv, auth, _, cat, mail := mkInvitesUC()
	inviter, _ := auth.CreateUser(context.Background(), &domain.AdminUser{Email: "o@x"})
	cat.tenants["t-1"] = &domain.Tenant{ID: "t-1", Slug: "acme", Name: "Acme"}
	_ = uc.Create(context.Background(), "t-1", inviter.ID, "guest@x.com", "admin")

	link, _ := mail.sent[0].Data["Link"].(string)
	token := strings.TrimPrefix(link, "https://app.example.com/auth/accept-invite?token=")

	p, err := uc.Preview(context.Background(), token)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if p.Email != "guest@x.com" || p.Role != "admin" || p.TenantID != "t-1" {
		t.Errorf("preview = %+v", p)
	}
	_ = inv // silence unused
}

func TestInvite_PreviewUnknownToken(t *testing.T) {
	uc, _, _, _, _, _ := mkInvitesUC()
	if _, err := uc.Preview(context.Background(), "ghost-token"); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("unknown token: err = %v, want ErrInvalidCredentials", err)
	}
}

func TestInvite_PreviewExpired(t *testing.T) {
	uc, inv, _, _, _, _ := mkInvitesUC()
	// Inject an expired invitation directly.
	inv.byHash["expired-hash"] = &ports.Invitation{
		ID: "inv-1", TenantID: "t-1", Email: "x@x.com", Role: "admin",
		TokenHash: "expired-hash", ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	if _, err := uc.Preview(context.Background(), "expired"); err == nil {
		t.Error("expected err on expired token")
	}
}

func TestInvite_AcceptLoggedOut_CreatesUserAndSession(t *testing.T) {
	uc, _, _, mem, cat, mail := mkInvitesUC()
	owner, _ := uc.users.(*statefulFakeAuth).CreateUser(context.Background(), &domain.AdminUser{Email: "o@x"})
	cat.tenants["t-2"] = &domain.Tenant{ID: "t-2", Slug: "biz", Name: "Biz"}
	_ = uc.Create(context.Background(), "t-2", owner.ID, "new@invite.com", "member")

	link, _ := mail.sent[0].Data["Link"].(string)
	token := strings.TrimPrefix(link, "https://app.example.com/auth/accept-invite?token=")

	resp, err := uc.Accept(context.Background(), AcceptRequest{
		Token:    token,
		Password: "supersecret",
	})
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if resp == nil || resp.User == nil || resp.User.Email != "new@invite.com" {
		t.Fatalf("user not created or wrong email: %+v", resp)
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Errorf("token pair empty: %+v", resp)
	}
	if _, ok, _ := mem.HasMembership(context.Background(), resp.User.ID, "t-2"); !ok {
		t.Errorf("membership not added")
	}
}

func TestInvite_AcceptLoggedIn_OnlyAddsMembership(t *testing.T) {
	uc, _, _, mem, cat, mail := mkInvitesUC()
	owner, _ := uc.users.(*statefulFakeAuth).CreateUser(context.Background(), &domain.AdminUser{Email: "o@x"})
	cat.tenants["t-3"] = &domain.Tenant{ID: "t-3", Slug: "z", Name: "Z"}
	_ = uc.Create(context.Background(), "t-3", owner.ID, "existing@x.com", "viewer")

	// Pre-create the invitee.
	existing, _ := uc.users.(*statefulFakeAuth).CreateUser(context.Background(), &domain.AdminUser{Email: "existing@x.com"})

	link, _ := mail.sent[0].Data["Link"].(string)
	token := strings.TrimPrefix(link, "https://app.example.com/auth/accept-invite?token=")

	resp, err := uc.Accept(context.Background(), AcceptRequest{
		Token:         token,
		CurrentUserID: existing.ID, // logged-in path
	})
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	// No new session pair issued in path A.
	if resp.AccessToken != "" {
		t.Errorf("logged-in path issued access token unexpectedly: %+v", resp)
	}
	if _, ok, _ := mem.HasMembership(context.Background(), existing.ID, "t-3"); !ok {
		t.Errorf("membership not added")
	}
}

func TestInvite_AcceptRejectsExpired(t *testing.T) {
	uc, inv, _, _, _, _ := mkInvitesUC()
	inv.byHash["expired"] = &ports.Invitation{
		ID: "inv-expired", TenantID: "t-1", Email: "x@x.com", Role: "admin",
		TokenHash: "expired", ExpiresAt: time.Now().Add(-1 * time.Hour),
	}
	inv.byID["inv-expired"] = inv.byHash["expired"]
	_, err := uc.Accept(context.Background(), AcceptRequest{Token: "rawToken"})
	if err == nil {
		t.Error("expected err on expired token")
	}
}

func TestInvite_AcceptRejectsReuse(t *testing.T) {
	uc, _, _, _, cat, mail := mkInvitesUC()
	owner, _ := uc.users.(*statefulFakeAuth).CreateUser(context.Background(), &domain.AdminUser{Email: "o@x"})
	cat.tenants["t-1"] = &domain.Tenant{ID: "t-1", Slug: "a", Name: "A"}
	_ = uc.Create(context.Background(), "t-1", owner.ID, "guest@x.com", "admin")

	link, _ := mail.sent[0].Data["Link"].(string)
	token := strings.TrimPrefix(link, "https://app.example.com/auth/accept-invite?token=")

	if _, err := uc.Accept(context.Background(), AcceptRequest{Token: token, Password: "supersecret"}); err != nil {
		t.Fatalf("first accept: %v", err)
	}
	// Second accept must fail (already accepted).
	if _, err := uc.Accept(context.Background(), AcceptRequest{Token: token, Password: "supersecret"}); err == nil {
		t.Error("expected err on reused token")
	}
}

func TestInvite_AcceptRejectsUnknownToken(t *testing.T) {
	uc, _, _, _, _, _ := mkInvitesUC()
	_, err := uc.Accept(context.Background(), AcceptRequest{Token: "ghost"})
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestInvite_AcceptLoggedOutRejectsWeakPassword(t *testing.T) {
	uc, _, _, _, cat, mail := mkInvitesUC()
	owner, _ := uc.users.(*statefulFakeAuth).CreateUser(context.Background(), &domain.AdminUser{Email: "o@x"})
	cat.tenants["t-1"] = &domain.Tenant{ID: "t-1", Slug: "a", Name: "A"}
	_ = uc.Create(context.Background(), "t-1", owner.ID, "g@x.com", "member")

	link, _ := mail.sent[0].Data["Link"].(string)
	token := strings.TrimPrefix(link, "https://app.example.com/auth/accept-invite?token=")

	_, err := uc.Accept(context.Background(), AcceptRequest{Token: token, Password: "1"})
	if err == nil {
		t.Error("expected err on weak password")
	}
}

// =====================================================================
// Pre-launch scenario verification (docs/pre_launch_scenarios.md sec 12)
// =====================================================================

// TestScenario_082_InviteMailerFail_RetryNeeded verifies:
// «Если mailer недоступен при Create — invitation row создаётся, но email
// не уходит. Известный gap — invitee никогда не узнает что был приглашён.
// Нужен retry-job или UI "отправить ещё раз".» (sec 12, scenario 82)
//
// EXPECTED TO FAIL: scenario 82 NOT implemented. There is no Resend
// invitation API and no background retry. This test verifies (a) the
// invitation row IS created on mailer failure (recoverable state), and
// (b) NO retry mechanism exists — which is the gap.
func TestScenario_082_InviteMailerFail_LeavesRowButNoRetry(t *testing.T) {
	uc, inv, auth, _, cat, mail := mkInvitesUC()
	mail.fail = true
	inviter, _ := auth.CreateUser(context.Background(), &domain.AdminUser{Email: "owner@x.com"})
	cat.tenants["t-1"] = &domain.Tenant{ID: "t-1", Slug: "acme", Name: "Acme"}

	// Create returns nil error even when mailer fails (best-effort).
	_ = uc.Create(context.Background(), "t-1", inviter.ID, "invited@x.com", "member")

	// Row should be persisted so a future resend can use it.
	if inv.createCnt != 1 {
		t.Errorf("invitation row not persisted on mailer fail: createCnt=%d", inv.createCnt)
	}
	// No email actually sent (mailer rejected).
	if len(mail.sent) != 0 {
		t.Errorf("mail.fail=true but %d email captured (mock should reject)", len(mail.sent))
	}
	// Gap: there's no ResendInvitation or RetryFailedInvitation method on the
	// InvitationsUseCase. Document the absence.
	t.Errorf("scenario 82: no Resend/Retry path exists for invitations whose initial mailer.Send failed — invitee stranded")
}

// TestScenario_072_InviteRateLimitEnforced verifies:
// «Я как owner вижу rate-limit "invite quota exceeded" если выслал больше
// 20 приглашений за 24h.» (sec 12, scenario 72)
//
// The unit test confirms the COUNT-based limit gate fires. CountRecentByInviter
// is a port method; the fake counts all rows (no time filter), so the gate
// trips at 20 even though our fake doesn't replicate the 24h cutoff exactly.
func TestScenario_072_InviteRateLimit_EnforcedAt20Per24h(t *testing.T) {
	uc, _, auth, _, cat, _ := mkInvitesUC()
	owner, _ := auth.CreateUser(context.Background(), &domain.AdminUser{Email: "o@x"})
	cat.tenants["t-1"] = &domain.Tenant{ID: "t-1", Slug: "a", Name: "A"}

	for i := 0; i < 20; i++ {
		email := strings.Replace("guest-N@x.com", "N", strings.Trim(time.Now().Format(".000"), "."), 1)
		email = strings.Replace(email, ".", "", -1) + "-" + strings.Repeat("a", i+1) + "@x.com"
		if err := uc.Create(context.Background(), "t-1", owner.ID, email, "member"); err != nil {
			t.Fatalf("invite #%d: %v", i, err)
		}
	}

	// 21st should be rejected.
	err := uc.Create(context.Background(), "t-1", owner.ID, "overflow@x.com", "member")
	if err == nil {
		t.Error("scenario 72: 21st invitation must be rejected by rate limit")
	}
}
