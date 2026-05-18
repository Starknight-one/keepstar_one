package usecases

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"keepstar-admin/internal/adapters/google"
	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

// fakeOAuthState supports Create + Consume with one-shot semantics so
// Complete tests can exercise replay protection (no need here, but free).
type fakeOAuthState struct {
	rows map[string]*ports.OAuthLoginState
}

func newFakeOAuthState() *fakeOAuthState {
	return &fakeOAuthState{rows: map[string]*ports.OAuthLoginState{}}
}
func (f *fakeOAuthState) Create(_ context.Context, s *ports.OAuthLoginState) error {
	f.rows[s.State] = s
	return nil
}
func (f *fakeOAuthState) Consume(_ context.Context, state string) (*ports.OAuthLoginState, error) {
	s, ok := f.rows[state]
	if !ok {
		return nil, errors.New("state not found")
	}
	delete(f.rows, state)
	return s, nil
}

// googleAuthExtAuth extends fakeAuth with google_sub linkage state.
type googleAuthFakeAuth struct {
	*fakeAuth
	bySub   map[string]string // google_sub → userID
	linkLog []string          // userIDs that got LinkGoogleSub
}

func newGoogleFakeAuth() *googleAuthFakeAuth {
	return &googleAuthFakeAuth{fakeAuth: newFakeAuth(), bySub: map[string]string{}}
}

func (f *googleAuthFakeAuth) GetUserByGoogleSub(_ context.Context, sub string) (*domain.AdminUser, error) {
	id, ok := f.bySub[sub]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return f.byID[id], nil
}
func (f *googleAuthFakeAuth) LinkGoogleSub(_ context.Context, userID, sub, _ string) error {
	f.bySub[sub] = userID
	f.linkLog = append(f.linkLog, userID)
	return nil
}
func (f *googleAuthFakeAuth) CreateOAuthUser(_ context.Context, u *domain.AdminUser, sub, _ string) (*domain.AdminUser, error) {
	u.ID = "user-" + u.Email
	f.users[strings.ToLower(u.Email)] = u
	f.byID[u.ID] = u
	f.bySub[sub] = u.ID
	return u, nil
}

func mkGoogleUC() (*GoogleAuthUseCase, *googleAuthFakeAuth, *fakeAdminCatalog, *fakeMemberships, *fakeOAuthState) {
	auth := newGoogleFakeAuth()
	cat := newFakeAdminCatalog()
	mem := newFakeMemberships()
	state := newFakeOAuthState()
	sess := newRichSessions()
	sessUC := NewSessionsUseCase(sess, auth, "test-secret-32-bytes-long-xxxxxx",
		15*time.Minute, 30*24*time.Hour, logger.New("error"))
	uc := NewGoogleAuthUseCase(
		google.NewClient("test-id", "test-secret", "http://localhost/cb"),
		state, auth, cat, mem, sessUC, logger.New("error"),
	)
	return uc, auth, cat, mem, state
}

func TestGoogle_Start_CreatesStateAndReturnsURL(t *testing.T) {
	uc, _, _, _, state := mkGoogleUC()

	res, err := uc.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if res.State == "" || res.URL == "" {
		t.Errorf("Start returned empty fields: %+v", res)
	}
	if !strings.Contains(res.URL, "state="+res.State) {
		t.Errorf("URL %q missing state param", res.URL)
	}
	if _, ok := state.rows[res.State]; !ok {
		t.Errorf("state not persisted")
	}
	stored := state.rows[res.State]
	if stored.Kind != "google_login" {
		t.Errorf("kind = %q, want google_login", stored.Kind)
	}
}

// findOrCreate exercises the 3-step cascade directly. The Complete handler is
// covered implicitly: it just wraps findOrCreate after state consume + token
// exchange, and the latter hits real Google so cannot be unit-tested.

func TestFindOrCreate_BySub_FastPath(t *testing.T) {
	uc, auth, _, _, _ := mkGoogleUC()
	// Pre-link existing user.
	existing, _ := auth.CreateUser(context.Background(), &domain.AdminUser{
		Email: "existing@x.com", TenantID: "t1", Role: domain.AdminRoleOwner,
	})
	auth.bySub["google-sub-A"] = existing.ID

	got, linked, err := uc.findOrCreate(context.Background(), &google.UserInfo{
		Sub:   "google-sub-A",
		Email: "anything-else@x.com",
		Name:  "Doesn't Matter",
	})
	if err != nil {
		t.Fatalf("findOrCreate: %v", err)
	}
	if got.ID != existing.ID {
		t.Errorf("returned user %q, want %q", got.ID, existing.ID)
	}
	if linked {
		t.Error("linked=true should be false for by-sub path (no merge happened)")
	}
}

func TestFindOrCreate_ByEmail_LinksToExisting(t *testing.T) {
	uc, auth, _, _, _ := mkGoogleUC()
	existing, _ := auth.CreateUser(context.Background(), &domain.AdminUser{
		Email: "u@x.com", TenantID: "t1", Role: domain.AdminRoleOwner,
	})
	// google_sub NOT pre-linked → forces step 2.

	got, linked, err := uc.findOrCreate(context.Background(), &google.UserInfo{
		Sub:   "fresh-sub",
		Email: "u@x.com",
		Name:  "User",
	})
	if err != nil {
		t.Fatalf("findOrCreate: %v", err)
	}
	if got.ID != existing.ID {
		t.Errorf("returned %q, want existing %q", got.ID, existing.ID)
	}
	if !linked {
		t.Error("linked=false; expected true (auto-merge)")
	}
	if len(auth.linkLog) != 1 || auth.linkLog[0] != existing.ID {
		t.Errorf("LinkGoogleSub not called for existing user: %v", auth.linkLog)
	}
	if auth.bySub["fresh-sub"] != existing.ID {
		t.Errorf("bySub not updated: %v", auth.bySub)
	}
}

func TestFindOrCreate_NewUser_CreatesTenantAndUser(t *testing.T) {
	uc, auth, cat, mem, _ := mkGoogleUC()

	got, linked, err := uc.findOrCreate(context.Background(), &google.UserInfo{
		Sub:   "new-sub",
		Email: "fresh@x.com",
		Name:  "Fresh Shop",
	})
	if err != nil {
		t.Fatalf("findOrCreate: %v", err)
	}
	if linked {
		t.Error("linked should be false on new signup")
	}
	if got.Email != "fresh@x.com" {
		t.Errorf("email = %q", got.Email)
	}
	if len(cat.createdLog) != 1 {
		t.Fatalf("expected 1 tenant created, got %d", len(cat.createdLog))
	}
	if cat.createdLog[0].Slug != "fresh-shop" {
		t.Errorf("slug = %q, want fresh-shop", cat.createdLog[0].Slug)
	}
	if _, ok, _ := mem.HasMembership(context.Background(), got.ID, got.TenantID); !ok {
		t.Errorf("owner membership not granted")
	}
	if auth.bySub["new-sub"] != got.ID {
		t.Errorf("CreateOAuthUser did not register google_sub")
	}
}

func TestFindOrCreate_NewUser_FallsBackToEmailPrefixWhenNameEmpty(t *testing.T) {
	uc, _, cat, _, _ := mkGoogleUC()

	_, _, err := uc.findOrCreate(context.Background(), &google.UserInfo{
		Sub:   "sub-2",
		Email: "owner@yoshop.com",
		Name:  "", // empty
	})
	if err != nil {
		t.Fatalf("findOrCreate: %v", err)
	}
	if cat.createdLog[0].Slug != "owner" {
		t.Errorf("expected slug from email prefix; got %q", cat.createdLog[0].Slug)
	}
}

func TestFindOrCreate_NormalizesEmailCase(t *testing.T) {
	uc, auth, _, _, _ := mkGoogleUC()
	existing, _ := auth.CreateUser(context.Background(), &domain.AdminUser{
		Email: "case@x.com", TenantID: "t1",
	})

	got, linked, err := uc.findOrCreate(context.Background(), &google.UserInfo{
		Sub:   "case-sub",
		Email: "  CASE@X.COM  ",
		Name:  "Case",
	})
	if err != nil {
		t.Fatalf("findOrCreate: %v", err)
	}
	if got.ID != existing.ID || !linked {
		t.Errorf("expected merge with existing case@x.com; got %q linked=%v", got.ID, linked)
	}
}

func TestComplete_RejectsMissingCodeOrState(t *testing.T) {
	uc, _, _, _, _ := mkGoogleUC()
	if _, err := uc.Complete(context.Background(), "", "state", "", ""); err == nil {
		t.Error("expected err on empty code")
	}
	if _, err := uc.Complete(context.Background(), "code", "", "", ""); err == nil {
		t.Error("expected err on empty state")
	}
}

func TestComplete_RejectsUnknownState(t *testing.T) {
	uc, _, _, _, _ := mkGoogleUC()
	if _, err := uc.Complete(context.Background(), "code", "ghost-state", "", ""); err == nil {
		t.Error("expected err on unknown state")
	}
}

func TestComplete_RejectsWrongKind(t *testing.T) {
	uc, _, _, _, state := mkGoogleUC()
	// Pre-insert a state with the wrong kind.
	_ = state.Create(context.Background(), &ports.OAuthLoginState{
		State:     "abc",
		Kind:      "telegram_login",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(10 * time.Minute),
	})
	if _, err := uc.Complete(context.Background(), "code", "abc", "", ""); err == nil {
		t.Error("expected err on kind mismatch")
	}
}

// =====================================================================
// Pre-launch scenario verification (docs/pre_launch_scenarios.md sec 4)
// =====================================================================

// TestScenario_028_GoogleStateWithTelegramKind_Rejected verifies:
// «Я как пользователь возвращаюсь с state но kind=`telegram_login`
// (попытка cross-kind) — отклоняется.» (sec 4, scenario 28)
//
// Already covered logically by TestComplete_RejectsWrongKind; this aliased
// test name lets the scenario report match the document 1-to-1.
func TestScenario_028_GoogleCallback_CrossKindStateRejected(t *testing.T) {
	uc, _, _, _, state := mkGoogleUC()
	_ = state.Create(context.Background(), &ports.OAuthLoginState{
		State:     "xkind",
		Kind:      "telegram_login",
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(10 * time.Minute),
	})
	_, err := uc.Complete(context.Background(), "code", "xkind", "", "")
	if err == nil {
		t.Fatal("scenario 28: cross-kind state must be rejected by Google callback")
	}
	if !strings.Contains(err.Error(), "state kind mismatch") {
		t.Errorf("err = %q, want 'state kind mismatch'", err.Error())
	}
}

// TestScenario_031_GoogleSignupWithoutName_FallsBackToEmailPrefix verifies:
// «Я как пользователь регистрируюсь через Google без `name` в профиле —
// companyName fallback к email-prefix; пользователь сможет переименовать
// workspace в Settings потом.» (sec 4, scenario 31)
//
// Already covered by TestFindOrCreate_NewUser_FallsBackToEmailPrefixWhenNameEmpty;
// re-stated here with the scenario-numbered name for the test report.
func TestScenario_031_GoogleSignup_NoName_FallsBackToEmailPrefix(t *testing.T) {
	uc, _, cat, _, _ := mkGoogleUC()
	if _, _, err := uc.findOrCreate(context.Background(), &google.UserInfo{
		Sub:   "no-name-sub",
		Email: "anon@store.example.com",
		Name:  "",
	}); err != nil {
		t.Fatalf("findOrCreate: %v", err)
	}
	if len(cat.createdLog) != 1 {
		t.Fatalf("tenant not created")
	}
	if cat.createdLog[0].Slug != "anon" {
		t.Errorf("scenario 31: slug = %q, want 'anon' (email-prefix fallback)", cat.createdLog[0].Slug)
	}
}
