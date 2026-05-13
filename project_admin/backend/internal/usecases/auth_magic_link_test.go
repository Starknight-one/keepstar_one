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

// --- in-memory fakes ------------------------------------------------------

type fakeAuth struct {
	users   map[string]*domain.AdminUser // by email
	byID    map[string]*domain.AdminUser
	failGet bool
}

func newFakeAuth() *fakeAuth {
	return &fakeAuth{users: map[string]*domain.AdminUser{}, byID: map[string]*domain.AdminUser{}}
}

func (f *fakeAuth) GetUserByEmail(_ context.Context, email string) (*domain.AdminUser, error) {
	if f.failGet {
		return nil, errors.New("boom")
	}
	u, ok := f.users[strings.ToLower(email)]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return u, nil
}
func (f *fakeAuth) GetUserByID(_ context.Context, id string) (*domain.AdminUser, error) {
	u, ok := f.byID[id]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return u, nil
}
func (f *fakeAuth) CreateUser(_ context.Context, u *domain.AdminUser) (*domain.AdminUser, error) {
	u.ID = "user-" + u.Email
	f.users[strings.ToLower(u.Email)] = u
	f.byID[u.ID] = u
	return u, nil
}
func (f *fakeAuth) EmailExists(_ context.Context, email string) (bool, error) {
	_, ok := f.users[strings.ToLower(email)]
	return ok, nil
}
func (f *fakeAuth) UpdatePasswordHash(context.Context, string, string) error      { return nil }
func (f *fakeAuth) MarkEmailVerified(context.Context, string) error               { return nil }
func (f *fakeAuth) TouchLastLogin(context.Context, string) error                  { return nil }
func (f *fakeAuth) GetUserByGoogleSub(context.Context, string) (*domain.AdminUser, error) {
	return nil, domain.ErrUserNotFound
}
func (f *fakeAuth) LinkGoogleSub(context.Context, string, string, string) error { return nil }
func (f *fakeAuth) CreateOAuthUser(_ context.Context, u *domain.AdminUser, _, _ string) (*domain.AdminUser, error) {
	return f.CreateUser(context.Background(), u)
}
func (f *fakeAuth) GetUserByTelegramID(context.Context, int64) (*domain.AdminUser, error) {
	return nil, domain.ErrUserNotFound
}
func (f *fakeAuth) LinkTelegramID(context.Context, string, int64, string, string) error { return nil }
func (f *fakeAuth) CreateTelegramUser(_ context.Context, u *domain.AdminUser, _ int64, _, _ string) (*domain.AdminUser, error) {
	return f.CreateUser(context.Background(), u)
}
func (f *fakeAuth) SetTOTPSecret(context.Context, string, string) error { return nil }
func (f *fakeAuth) GetTOTPSecret(context.Context, string) (string, bool, error) {
	return "", false, nil
}
func (f *fakeAuth) EnableTOTP(context.Context, string) error  { return nil }
func (f *fakeAuth) DisableTOTP(context.Context, string) error { return nil }

// fakeMemberships supports a fail-on-Add toggle to exercise the race-fix branch.
type fakeMemberships struct {
	rows    map[string]string // userID|tenantID -> role
	failAdd bool
}

func newFakeMemberships() *fakeMemberships { return &fakeMemberships{rows: map[string]string{}} }

func (f *fakeMemberships) ListForUser(_ context.Context, userID string) ([]ports.UserTenantMembership, error) {
	var out []ports.UserTenantMembership
	for k, role := range f.rows {
		parts := strings.SplitN(k, "|", 2)
		if parts[0] == userID {
			out = append(out, ports.UserTenantMembership{TenantID: parts[1], Role: role})
		}
	}
	return out, nil
}
func (f *fakeMemberships) HasMembership(_ context.Context, userID, tenantID string) (string, bool, error) {
	role, ok := f.rows[userID+"|"+tenantID]
	return role, ok, nil
}
func (f *fakeMemberships) Add(_ context.Context, userID, tenantID, role, _ string) error {
	if f.failAdd {
		return errors.New("add failed")
	}
	f.rows[userID+"|"+tenantID] = role
	return nil
}
func (f *fakeMemberships) Remove(_ context.Context, userID, tenantID string) error {
	delete(f.rows, userID+"|"+tenantID)
	return nil
}
func (f *fakeMemberships) SoftDeleteEmptyOrphanTenants(_ context.Context, _, _ string) ([]string, error) {
	return nil, nil
}

type fakeChallenges struct {
	byHash map[string]*ports.Challenge // by code hash
	calls  int
}

func newFakeChallenges() *fakeChallenges { return &fakeChallenges{byHash: map[string]*ports.Challenge{}} }

func (f *fakeChallenges) Create(_ context.Context, c *ports.Challenge) error {
	f.calls++
	c.ID = "ch-" + c.CodeHash[:8]
	f.byHash[c.CodeHash] = c
	return nil
}
func (f *fakeChallenges) FindActive(_ context.Context, kind, codeHash string) (*ports.Challenge, error) {
	c, ok := f.byHash[codeHash]
	if !ok || c.Kind != kind || c.ConsumedAt != nil || time.Now().After(c.ExpiresAt) {
		return nil, nil
	}
	return c, nil
}
func (f *fakeChallenges) Consume(_ context.Context, id string) error {
	for _, c := range f.byHash {
		if c.ID == id {
			now := time.Now()
			c.ConsumedAt = &now
			return nil
		}
	}
	return errors.New("challenge not found")
}
func (f *fakeChallenges) DeleteExpired(context.Context) error { return nil }

type fakeMailer struct {
	sent []ports.EmailMessage
	fail bool
}

func (f *fakeMailer) Send(_ context.Context, msg ports.EmailMessage) error {
	if f.fail {
		return errors.New("smtp down")
	}
	f.sent = append(f.sent, msg)
	return nil
}

type fakeSessions struct {
	created []*ports.Session
}

func (f *fakeSessions) Create(_ context.Context, s *ports.Session) error {
	s.ID = "sess-" + s.UserID
	f.created = append(f.created, s)
	return nil
}
func (f *fakeSessions) FindActive(context.Context, string) (*ports.Session, error)  { return nil, nil }
func (f *fakeSessions) FindByID(context.Context, string) (*ports.Session, error)    { return nil, nil }
func (f *fakeSessions) Revoke(context.Context, string) error                        { return nil }
func (f *fakeSessions) RevokeAllForUser(context.Context, string) error              { return nil }
func (f *fakeSessions) ListActiveForUser(context.Context, string) ([]*ports.Session, error) {
	return nil, nil
}

func newMagicLinkUC(auth *fakeAuth, mem *fakeMemberships, ch *fakeChallenges, mail *fakeMailer) (*MagicLinkUseCase, *fakeSessions, *fakeMailer) {
	if mail == nil {
		mail = &fakeMailer{}
	}
	sess := &fakeSessions{}
	log := logger.New("error")
	sessionsUC := NewSessionsUseCase(sess, auth, "test-secret-32-bytes-long-xxxxxx", 15*time.Minute, 30*24*time.Hour, log)
	uc := NewMagicLinkUseCase(auth, mem, ch, mail, sessionsUC, "https://app.example.com", 24*time.Hour, log)
	return uc, sess, mail
}

// --- Issue ----------------------------------------------------------------

func TestMagicLink_Issue_CreatesChallengeAndSendsEmail(t *testing.T) {
	auth := newFakeAuth()
	user, _ := auth.CreateUser(context.Background(), &domain.AdminUser{Email: "a@b.com", TenantID: "t1"})
	ch := newFakeChallenges()
	uc, _, mail := newMagicLinkUC(auth, newFakeMemberships(), ch, nil)

	uc.Issue(context.Background(), user.ID, user.Email)

	if ch.calls != 1 {
		t.Fatalf("expected 1 challenge created, got %d", ch.calls)
	}
	if len(mail.sent) != 1 {
		t.Fatalf("expected 1 email sent, got %d", len(mail.sent))
	}
	if mail.sent[0].Kind != "magic_link" {
		t.Errorf("email kind = %q, want magic_link", mail.sent[0].Kind)
	}
	link, _ := mail.sent[0].Data["Link"].(string)
	if !strings.HasPrefix(link, "https://app.example.com/auth/magic?code=") {
		t.Errorf("email link malformed: %q", link)
	}
}

func TestMagicLink_Issue_NoMailerLogsAndExits(t *testing.T) {
	auth := newFakeAuth()
	user, _ := auth.CreateUser(context.Background(), &domain.AdminUser{Email: "a@b.com", TenantID: "t1"})
	ch := newFakeChallenges()
	log := logger.New("error")
	sessionsUC := NewSessionsUseCase(&fakeSessions{}, auth, "secret", 15*time.Minute, 30*24*time.Hour, log)

	uc := NewMagicLinkUseCase(auth, newFakeMemberships(), ch, nil /* mailer */, sessionsUC, "https://app.example.com", 24*time.Hour, log)
	uc.Issue(context.Background(), user.ID, user.Email)

	if ch.calls != 1 {
		t.Errorf("challenge should still be created so a re-attempted Send can succeed; got %d", ch.calls)
	}
}

func TestMagicLink_Issue_EmptyEmailIsNoop(t *testing.T) {
	auth := newFakeAuth()
	ch := newFakeChallenges()
	uc, _, mail := newMagicLinkUC(auth, newFakeMemberships(), ch, nil)

	uc.Issue(context.Background(), "user-1", "")

	if ch.calls != 0 || len(mail.sent) != 0 {
		t.Errorf("empty email must not create challenge or send mail")
	}
}

// --- Consume --------------------------------------------------------------

func TestMagicLink_Consume_ValidCodeIssuesSession(t *testing.T) {
	auth := newFakeAuth()
	user, _ := auth.CreateUser(context.Background(), &domain.AdminUser{Email: "a@b.com", TenantID: "t1"})
	ch := newFakeChallenges()
	uc, sess, mail := newMagicLinkUC(auth, newFakeMemberships(), ch, nil)

	uc.Issue(context.Background(), user.ID, user.Email)
	link, _ := mail.sent[0].Data["Link"].(string)
	code := strings.TrimPrefix(link, "https://app.example.com/auth/magic?code=")

	pair, gotUser, err := uc.Consume(context.Background(), code, "ua", "ip")
	if err != nil {
		t.Fatalf("Consume returned err: %v", err)
	}
	if pair == nil || pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatalf("Consume returned empty token pair: %+v", pair)
	}
	if gotUser == nil || gotUser.ID != user.ID {
		t.Errorf("Consume returned wrong user: %+v", gotUser)
	}
	if len(sess.created) != 1 {
		t.Errorf("expected 1 session, got %d", len(sess.created))
	}
}

func TestMagicLink_Consume_MissingCodeErrors(t *testing.T) {
	auth := newFakeAuth()
	uc, _, _ := newMagicLinkUC(auth, newFakeMemberships(), newFakeChallenges(), nil)

	_, _, err := uc.Consume(context.Background(), "", "ua", "ip")
	if err == nil {
		t.Error("expected error on empty code")
	}
}

func TestMagicLink_Consume_BadCodeErrors(t *testing.T) {
	auth := newFakeAuth()
	uc, _, _ := newMagicLinkUC(auth, newFakeMemberships(), newFakeChallenges(), nil)

	_, _, err := uc.Consume(context.Background(), "not-a-real-token", "ua", "ip")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestMagicLink_Consume_ReusedCodeErrors(t *testing.T) {
	auth := newFakeAuth()
	user, _ := auth.CreateUser(context.Background(), &domain.AdminUser{Email: "a@b.com", TenantID: "t1"})
	ch := newFakeChallenges()
	uc, _, mail := newMagicLinkUC(auth, newFakeMemberships(), ch, nil)

	uc.Issue(context.Background(), user.ID, user.Email)
	link, _ := mail.sent[0].Data["Link"].(string)
	code := strings.TrimPrefix(link, "https://app.example.com/auth/magic?code=")

	if _, _, err := uc.Consume(context.Background(), code, "ua", "ip"); err != nil {
		t.Fatalf("first Consume should succeed: %v", err)
	}
	_, _, err := uc.Consume(context.Background(), code, "ua", "ip")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("second Consume should reject reused code, got %v", err)
	}
}

// --- ProvisionShopOwner ---------------------------------------------------

func TestProvisionShopOwner_NewUser_HappyPath(t *testing.T) {
	auth := newFakeAuth()
	mem := newFakeMemberships()
	ch := newFakeChallenges()
	uc, _, mail := newMagicLinkUC(auth, mem, ch, nil)

	uc.ProvisionShopOwner(context.Background(), "tenant-1", "owner@shop.com")

	user, err := auth.GetUserByEmail(context.Background(), "owner@shop.com")
	if err != nil {
		t.Fatalf("user not created: %v", err)
	}
	if _, ok, _ := mem.HasMembership(context.Background(), user.ID, "tenant-1"); !ok {
		t.Errorf("membership not granted")
	}
	if len(mail.sent) != 1 {
		t.Errorf("expected 1 magic-link email, got %d", len(mail.sent))
	}
}

func TestProvisionShopOwner_NewUser_AddFails_BlocksEmail(t *testing.T) {
	auth := newFakeAuth()
	mem := newFakeMemberships()
	mem.failAdd = true // simulate the race-fix scenario
	ch := newFakeChallenges()
	uc, _, mail := newMagicLinkUC(auth, mem, ch, nil)

	uc.ProvisionShopOwner(context.Background(), "tenant-1", "owner@shop.com")

	// User row was created (we can't know which lookup will hit first; safer
	// to leave the row than orphan a tenant), but no email goes out — that
	// is the whole point of the race fix.
	if _, err := auth.GetUserByEmail(context.Background(), "owner@shop.com"); err != nil {
		t.Fatalf("user should still exist for manual recovery: %v", err)
	}
	if len(mail.sent) != 0 {
		t.Errorf("expected 0 emails (newly-created + Add failed → no link), got %d", len(mail.sent))
	}
	if ch.calls != 0 {
		t.Errorf("expected 0 challenges (issuance aborted), got %d", ch.calls)
	}
}

func TestProvisionShopOwner_ExistingUser_AddFails_StillEmails(t *testing.T) {
	// A user who already has memberships from another flow (Google/email-pwd)
	// is safe even if Add fails for this new tenant — picker stays non-empty.
	auth := newFakeAuth()
	existing, _ := auth.CreateUser(context.Background(), &domain.AdminUser{Email: "owner@shop.com", TenantID: "tenant-other"})
	mem := newFakeMemberships()
	_ = mem.Add(context.Background(), existing.ID, "tenant-other", "owner", "")
	mem.failAdd = true // new tenant Add fails
	ch := newFakeChallenges()
	uc, _, mail := newMagicLinkUC(auth, mem, ch, nil)

	uc.ProvisionShopOwner(context.Background(), "tenant-new", "owner@shop.com")

	if len(mail.sent) != 1 {
		t.Errorf("existing user with another membership should still receive link, got %d emails", len(mail.sent))
	}
}

func TestProvisionShopOwner_EmptyEmail_NoOp(t *testing.T) {
	auth := newFakeAuth()
	ch := newFakeChallenges()
	uc, _, mail := newMagicLinkUC(auth, newFakeMemberships(), ch, nil)

	uc.ProvisionShopOwner(context.Background(), "tenant-1", "")

	if ch.calls != 0 || len(mail.sent) != 0 {
		t.Error("empty shop email must not provision or send")
	}
}

func TestProvisionShopOwner_ExistingUser_Reused(t *testing.T) {
	// Email match → reuse user, just add membership for the new tenant.
	auth := newFakeAuth()
	existing, _ := auth.CreateUser(context.Background(), &domain.AdminUser{Email: "owner@shop.com", TenantID: "tenant-1"})
	mem := newFakeMemberships()
	ch := newFakeChallenges()
	uc, _, mail := newMagicLinkUC(auth, mem, ch, nil)

	uc.ProvisionShopOwner(context.Background(), "tenant-2", "owner@shop.com")

	// No new user (only one row in fake)
	if len(auth.byID) != 1 {
		t.Errorf("expected 1 user row (reuse), got %d", len(auth.byID))
	}
	if _, ok, _ := mem.HasMembership(context.Background(), existing.ID, "tenant-2"); !ok {
		t.Errorf("membership for new tenant not added")
	}
	if len(mail.sent) != 1 {
		t.Errorf("expected magic-link to existing user, got %d emails", len(mail.sent))
	}
}
