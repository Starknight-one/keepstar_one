package handlers

// HTTP-layer pre-launch scenario verification. Each TestScenario_NNN_* maps
// to a numbered scenario in docs/pre_launch_scenarios.md. These tests
// exercise the wire surface — status codes, body parsing, JSON shapes —
// that the frontend depends on. Unit tests cover the business logic; this
// layer covers handler↔usecase↔serialization integration.
//
// Red tests document scenarios where the HTTP contract diverges from the
// document and will fail until the gap is closed.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
	"keepstar-admin/internal/usecases"
)

// ---------------------------------------------------------------------
// Inline fakes (mirrors usecases/*_test.go fakes; lives here so handlers
// tests don't depend on test-only types from another package).
// ---------------------------------------------------------------------

type httpFakeAuth struct {
	users      map[string]*domain.AdminUser
	byID       map[string]*domain.AdminUser
	byGoogle   map[string]string
	byTelegram map[int64]string
	verifiedAt map[string]bool
	totp       map[string]string
	enabled    map[string]bool
}

func newHTTPFakeAuth() *httpFakeAuth {
	return &httpFakeAuth{
		users:      map[string]*domain.AdminUser{},
		byID:       map[string]*domain.AdminUser{},
		byGoogle:   map[string]string{},
		byTelegram: map[int64]string{},
		verifiedAt: map[string]bool{},
		totp:       map[string]string{},
		enabled:    map[string]bool{},
	}
}

func (f *httpFakeAuth) GetUserByEmail(_ context.Context, email string) (*domain.AdminUser, error) {
	u, ok := f.users[strings.ToLower(email)]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return u, nil
}
func (f *httpFakeAuth) GetUserByID(_ context.Context, id string) (*domain.AdminUser, error) {
	u, ok := f.byID[id]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return u, nil
}
func (f *httpFakeAuth) CreateUser(_ context.Context, u *domain.AdminUser) (*domain.AdminUser, error) {
	u.ID = "user-" + u.Email
	f.users[strings.ToLower(u.Email)] = u
	f.byID[u.ID] = u
	return u, nil
}
func (f *httpFakeAuth) EmailExists(_ context.Context, email string) (bool, error) {
	_, ok := f.users[strings.ToLower(email)]
	return ok, nil
}
func (f *httpFakeAuth) UpdatePasswordHash(_ context.Context, id, hash string) error {
	if u, ok := f.byID[id]; ok {
		u.PasswordHash = hash
	}
	return nil
}
func (f *httpFakeAuth) MarkEmailVerified(_ context.Context, id string) error {
	f.verifiedAt[id] = true
	return nil
}
func (f *httpFakeAuth) TouchLastLogin(context.Context, string) error { return nil }
func (f *httpFakeAuth) GetUserByGoogleSub(_ context.Context, sub string) (*domain.AdminUser, error) {
	if id, ok := f.byGoogle[sub]; ok {
		return f.byID[id], nil
	}
	return nil, domain.ErrUserNotFound
}
func (f *httpFakeAuth) LinkGoogleSub(_ context.Context, id, sub, _ string) error {
	f.byGoogle[sub] = id
	return nil
}
func (f *httpFakeAuth) CreateOAuthUser(_ context.Context, u *domain.AdminUser, sub, _ string) (*domain.AdminUser, error) {
	created, _ := f.CreateUser(context.Background(), u)
	f.byGoogle[sub] = created.ID
	return created, nil
}
func (f *httpFakeAuth) GetUserByTelegramID(_ context.Context, tg int64) (*domain.AdminUser, error) {
	if id, ok := f.byTelegram[tg]; ok {
		return f.byID[id], nil
	}
	return nil, domain.ErrUserNotFound
}
func (f *httpFakeAuth) LinkTelegramID(_ context.Context, id string, tg int64, _, _ string) error {
	f.byTelegram[tg] = id
	return nil
}
func (f *httpFakeAuth) CreateTelegramUser(_ context.Context, u *domain.AdminUser, tg int64, _, _ string) (*domain.AdminUser, error) {
	created, _ := f.CreateUser(context.Background(), u)
	f.byTelegram[tg] = created.ID
	return created, nil
}
func (f *httpFakeAuth) SetTOTPSecret(_ context.Context, id, enc string) error {
	f.totp[id] = enc
	return nil
}
func (f *httpFakeAuth) GetTOTPSecret(_ context.Context, id string) (string, bool, error) {
	return f.totp[id], f.enabled[id], nil
}
func (f *httpFakeAuth) EnableTOTP(_ context.Context, id string) error {
	f.enabled[id] = true
	return nil
}
func (f *httpFakeAuth) DisableTOTP(_ context.Context, id string) error {
	delete(f.totp, id)
	delete(f.enabled, id)
	return nil
}

type httpFakeMemberships struct {
	rows map[string]string
}

func newHTTPFakeMemberships() *httpFakeMemberships {
	return &httpFakeMemberships{rows: map[string]string{}}
}

func (f *httpFakeMemberships) ListForUser(_ context.Context, uid string) ([]ports.UserTenantMembership, error) {
	var out []ports.UserTenantMembership
	for k, role := range f.rows {
		parts := strings.SplitN(k, "|", 2)
		if parts[0] == uid {
			out = append(out, ports.UserTenantMembership{TenantID: parts[1], Role: role})
		}
	}
	return out, nil
}
func (f *httpFakeMemberships) HasMembership(_ context.Context, uid, tid string) (string, bool, error) {
	r, ok := f.rows[uid+"|"+tid]
	return r, ok, nil
}
func (f *httpFakeMemberships) Add(_ context.Context, uid, tid, role, _ string) error {
	f.rows[uid+"|"+tid] = role
	return nil
}
func (f *httpFakeMemberships) Remove(_ context.Context, uid, tid string) error {
	delete(f.rows, uid+"|"+tid)
	return nil
}
func (f *httpFakeMemberships) SoftDeleteEmptyOrphanTenants(_ context.Context, _, _ string) ([]string, error) {
	return nil, nil
}

type httpFakeCatalog struct {
	byID map[string]*domain.Tenant
}

func newHTTPFakeCatalog() *httpFakeCatalog {
	return &httpFakeCatalog{byID: map[string]*domain.Tenant{}}
}

func (f *httpFakeCatalog) CreateTenant(_ context.Context, t *domain.Tenant) (*domain.Tenant, error) {
	t.ID = "tenant-" + t.Slug
	f.byID[t.ID] = t
	return t, nil
}
func (f *httpFakeCatalog) GetTenantByID(_ context.Context, id string) (*domain.Tenant, error) {
	t, ok := f.byID[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return t, nil
}
func (f *httpFakeCatalog) UpdateTenantSettings(context.Context, string, domain.TenantSettings) error {
	return nil
}
func (f *httpFakeCatalog) ListProducts(context.Context, string, domain.AdminProductFilter) ([]domain.Product, int, error) {
	return nil, 0, nil
}
func (f *httpFakeCatalog) GetProduct(context.Context, string, string) (*domain.Product, error) {
	return nil, nil
}
func (f *httpFakeCatalog) UpdateProduct(context.Context, string, string, domain.ProductUpdate) error {
	return nil
}
func (f *httpFakeCatalog) GetCategories(context.Context, string) ([]domain.Category, error) {
	return nil, nil
}
func (f *httpFakeCatalog) UpsertMasterProduct(context.Context, *domain.MasterProduct) (string, error) {
	return "", nil
}
func (f *httpFakeCatalog) UpsertProductListing(context.Context, *domain.Product) (string, error) {
	return "", nil
}
func (f *httpFakeCatalog) GetOrCreateCategory(context.Context, string, string) (string, error) {
	return "", nil
}
func (f *httpFakeCatalog) BulkUpdateStock(context.Context, string, []domain.StockUpdate) (int, error) {
	return 0, nil
}
func (f *httpFakeCatalog) GetCategoryBySlug(context.Context, string) (*domain.Category, error) {
	return nil, nil
}
func (f *httpFakeCatalog) GetAllMasterProducts(context.Context, string) ([]domain.MasterProduct, error) {
	return nil, nil
}
func (f *httpFakeCatalog) GetUnenrichedMasterProducts(context.Context, string) ([]domain.MasterProduct, error) {
	return nil, nil
}
func (f *httpFakeCatalog) UpdateMasterProductPIM(context.Context, string, string, domain.EnrichmentOutputV2) error {
	return nil
}
func (f *httpFakeCatalog) SoftDeleteProductBySource(context.Context, string, string, string) error {
	return nil
}
func (f *httpFakeCatalog) UpsertListingFromSource(context.Context, *domain.ListingFromSource) (string, error) {
	return "", nil
}
func (f *httpFakeCatalog) GetMasterProductsWithoutEmbedding(context.Context, string) ([]domain.MasterProduct, error) {
	return nil, nil
}
func (f *httpFakeCatalog) SeedEmbedding(context.Context, string, []float32) error { return nil }
func (f *httpFakeCatalog) GenerateCatalogDigest(context.Context, string) error    { return nil }

type httpFakeSessions struct {
	byID    map[string]*ports.Session
	byHash  map[string]string
	nextID  int
	revoked map[string]bool
}

func newHTTPFakeSessions() *httpFakeSessions {
	return &httpFakeSessions{byID: map[string]*ports.Session{}, byHash: map[string]string{}, revoked: map[string]bool{}}
}

func (s *httpFakeSessions) Create(_ context.Context, sess *ports.Session) error {
	s.nextID++
	sess.ID = fmt.Sprintf("sess-%d", s.nextID)
	s.byID[sess.ID] = sess
	s.byHash[sess.TokenHash] = sess.ID
	return nil
}
func (s *httpFakeSessions) FindActive(_ context.Context, hash string) (*ports.Session, error) {
	id, ok := s.byHash[hash]
	if !ok {
		return nil, nil
	}
	return s.byID[id], nil
}
func (s *httpFakeSessions) FindByID(_ context.Context, id string) (*ports.Session, error) {
	return s.byID[id], nil
}
func (s *httpFakeSessions) Revoke(_ context.Context, id string) error {
	if sess, ok := s.byID[id]; ok {
		now := time.Now()
		sess.RevokedAt = &now
	}
	return nil
}
func (s *httpFakeSessions) RevokeAllForUser(_ context.Context, uid string) error {
	now := time.Now()
	for _, sess := range s.byID {
		if sess.UserID == uid {
			sess.RevokedAt = &now
		}
	}
	return nil
}
func (s *httpFakeSessions) ListActiveForUser(_ context.Context, uid string) ([]*ports.Session, error) {
	var out []*ports.Session
	for _, sess := range s.byID {
		if sess.UserID == uid && sess.RevokedAt == nil {
			out = append(out, sess)
		}
	}
	return out, nil
}

type httpFakeChallenges struct {
	byHash map[string]*ports.Challenge
}

func newHTTPFakeChallenges() *httpFakeChallenges {
	return &httpFakeChallenges{byHash: map[string]*ports.Challenge{}}
}

func (c *httpFakeChallenges) Create(_ context.Context, ch *ports.Challenge) error {
	ch.ID = "ch-" + ch.CodeHash[:8]
	c.byHash[ch.CodeHash] = ch
	return nil
}
func (c *httpFakeChallenges) FindActive(_ context.Context, kind, hash string) (*ports.Challenge, error) {
	ch, ok := c.byHash[hash]
	if !ok || ch.Kind != kind || ch.ConsumedAt != nil || time.Now().After(ch.ExpiresAt) {
		return nil, nil
	}
	return ch, nil
}
func (c *httpFakeChallenges) Consume(_ context.Context, id string) error {
	for _, ch := range c.byHash {
		if ch.ID == id {
			now := time.Now()
			ch.ConsumedAt = &now
			return nil
		}
	}
	return nil
}
func (c *httpFakeChallenges) DeleteExpired(context.Context) error { return nil }

type httpFakeMailer struct {
	sent []ports.EmailMessage
}

func (m *httpFakeMailer) Send(_ context.Context, msg ports.EmailMessage) error {
	m.sent = append(m.sent, msg)
	return nil
}

// ---------------------------------------------------------------------
// Test fixture: builds usecases + handlers wired for HTTP tests.
// ---------------------------------------------------------------------

type httpFixture struct {
	auth        *httpFakeAuth
	catalog     *httpFakeCatalog
	memberships *httpFakeMemberships
	sessions    *httpFakeSessions
	challenges  *httpFakeChallenges
	mailer      *httpFakeMailer

	authHandler     *AuthHandler
	sessionsHandler *SessionsHandler
	magicHandler    *MagicLinkHandler

	authUC     *usecases.AuthUseCase
	sessionsUC *usecases.SessionsUseCase
	magicUC    *usecases.MagicLinkUseCase
}

func newHTTPFixture() *httpFixture {
	auth := newHTTPFakeAuth()
	cat := newHTTPFakeCatalog()
	mem := newHTTPFakeMemberships()
	sess := newHTTPFakeSessions()
	ch := newHTTPFakeChallenges()
	mail := &httpFakeMailer{}
	log := logger.New("error")

	const secret = "test-secret-32-bytes-long-xxxxxx"
	sessUC := usecases.NewSessionsUseCase(sess, auth, secret, 15*time.Minute, 30*24*time.Hour, log)
	authUC := usecases.NewAuthUseCase(auth, cat, secret)
	authUC.SetSessions(sessUC)
	authUC.SetMemberships(mem)

	magicUC := usecases.NewMagicLinkUseCase(auth, mem, ch, mail, sessUC, "https://app.example.com", 24*time.Hour, log)

	flags := AuthFeatureFlags{Google: false, Email: true}
	authHandler := NewAuthHandler(authUC, log, flags)
	sessionsHandler := NewSessionsHandler(sessUC, log)
	magicHandler := NewMagicLinkHandler(magicUC, log)

	return &httpFixture{
		auth: auth, catalog: cat, memberships: mem, sessions: sess,
		challenges: ch, mailer: mail,
		authHandler: authHandler, sessionsHandler: sessionsHandler, magicHandler: magicHandler,
		authUC: authUC, sessionsUC: sessUC, magicUC: magicUC,
	}
}

// withUser returns a context that mimics what AuthMiddleware would inject.
func withUser(userID, tenantID string) context.Context {
	ctx := context.WithValue(context.Background(), ctxUserID, userID)
	ctx = context.WithValue(ctx, ctxTenantID, tenantID)
	return ctx
}

func postJSON(t *testing.T, h http.HandlerFunc, ctx context.Context, body any) *httptest.ResponseRecorder {
	t.Helper()
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(buf))
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func getJSON(t *testing.T, h http.HandlerFunc, ctx context.Context) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

// ---------------------------------------------------------------------
// Scenarios from sec 1 — Signup
// ---------------------------------------------------------------------

// TestScenarioHTTP_001_Signup_Returns201 verifies scenario 1 at HTTP layer:
// POST /admin/api/auth/signup with valid body returns 201 + AuthResponse JSON.
func TestScenarioHTTP_001_Signup_Returns201(t *testing.T) {
	fx := newHTTPFixture()
	rec := postJSON(t, fx.authHandler.HandleSignup, nil, map[string]string{
		"email": "new@example.com", "password": "supersecret", "companyName": "Acme",
	})
	if rec.Code != http.StatusCreated {
		t.Errorf("status=%d, want 201; body=%s", rec.Code, rec.Body.String())
	}
	var resp usecases.AuthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
	}
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Errorf("token pair missing: %+v", resp)
	}
	if resp.User == nil || resp.User.Email != "new@example.com" {
		t.Errorf("user wrong: %+v", resp.User)
	}
}

// TestScenarioHTTP_002_Signup_DuplicateEmail_Returns409 verifies scenario 2:
// re-signup with taken email returns 409 with a "email already used" message.
func TestScenarioHTTP_002_Signup_DuplicateEmail_Returns409(t *testing.T) {
	fx := newHTTPFixture()
	// Pre-seed user.
	_, _ = fx.auth.CreateUser(context.Background(), &domain.AdminUser{Email: "taken@x.com"})

	rec := postJSON(t, fx.authHandler.HandleSignup, nil, map[string]string{
		"email": "taken@x.com", "password": "supersecret", "companyName": "X",
	})
	if rec.Code != http.StatusConflict {
		t.Errorf("status=%d, want 409; body=%s", rec.Code, rec.Body.String())
	}
}

// TestScenarioHTTP_004_Signup_MissingFields_Returns400 verifies scenario 4:
// missing field → 400.
func TestScenarioHTTP_004_Signup_MissingFields_Returns400(t *testing.T) {
	fx := newHTTPFixture()
	rec := postJSON(t, fx.authHandler.HandleSignup, nil, map[string]string{
		"email": "", "password": "supersecret", "companyName": "X",
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------
// Scenarios from sec 2 — Login
// ---------------------------------------------------------------------

// TestScenarioHTTP_009_Login_Returns200WithTokenPair verifies scenario 9.
func TestScenarioHTTP_009_Login_Returns200WithTokenPair(t *testing.T) {
	fx := newHTTPFixture()
	// Seed user via Signup (so the password gets hashed correctly).
	_ = postJSON(t, fx.authHandler.HandleSignup, nil, map[string]string{
		"email": "u@x.com", "password": "supersecret", "companyName": "C",
	})

	rec := postJSON(t, fx.authHandler.HandleLogin, nil, map[string]string{
		"email": "u@x.com", "password": "supersecret",
	})
	if rec.Code != http.StatusOK {
		t.Errorf("status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp usecases.AuthResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Errorf("token pair missing: %+v", resp)
	}
}

// TestScenarioHTTP_010_Login_UnknownEmail_Returns401 verifies anti-
// enumeration: unknown email returns same 401 as wrong password.
func TestScenarioHTTP_010_Login_UnknownEmail_Returns401(t *testing.T) {
	fx := newHTTPFixture()
	rec := postJSON(t, fx.authHandler.HandleLogin, nil, map[string]string{
		"email": "ghost@nowhere.com", "password": "anything",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status=%d, want 401; body=%s", rec.Code, rec.Body.String())
	}
}

// TestScenarioHTTP_011_Login_WrongPassword_Returns401 verifies scenario 11.
func TestScenarioHTTP_011_Login_WrongPassword_Returns401(t *testing.T) {
	fx := newHTTPFixture()
	_ = postJSON(t, fx.authHandler.HandleSignup, nil, map[string]string{
		"email": "u@x.com", "password": "supersecret", "companyName": "C",
	})
	rec := postJSON(t, fx.authHandler.HandleLogin, nil, map[string]string{
		"email": "u@x.com", "password": "wrong-password",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status=%d, want 401; body=%s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------
// Scenarios from sec 3 — Sessions / refresh
// ---------------------------------------------------------------------

// TestScenarioHTTP_016_RefreshRotates verifies scenario 16: refresh exchanges
// old refresh token for a new pair.
func TestScenarioHTTP_016_RefreshRotatesAndIssuesNew(t *testing.T) {
	fx := newHTTPFixture()
	signupRec := postJSON(t, fx.authHandler.HandleSignup, nil, map[string]string{
		"email": "rot@x.com", "password": "supersecret", "companyName": "Rot",
	})
	var auth usecases.AuthResponse
	_ = json.Unmarshal(signupRec.Body.Bytes(), &auth)

	rec := postJSON(t, fx.sessionsHandler.HandleRefresh, nil, map[string]string{
		"refresh_token": auth.RefreshToken,
	})
	if rec.Code != http.StatusOK {
		t.Errorf("status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var pair usecases.TokenPair
	_ = json.Unmarshal(rec.Body.Bytes(), &pair)
	if pair.RefreshToken == "" || pair.RefreshToken == auth.RefreshToken {
		t.Errorf("refresh did not rotate: orig=%s rotated=%s", auth.RefreshToken, pair.RefreshToken)
	}
}

// TestScenarioHTTP_017_RefreshTokenReuse_TriggersBreach verifies scenario 17.
func TestScenarioHTTP_017_RefreshTokenReuse_Returns401(t *testing.T) {
	fx := newHTTPFixture()
	signupRec := postJSON(t, fx.authHandler.HandleSignup, nil, map[string]string{
		"email": "breach@x.com", "password": "supersecret", "companyName": "B",
	})
	var auth usecases.AuthResponse
	_ = json.Unmarshal(signupRec.Body.Bytes(), &auth)
	// First refresh rotates.
	_ = postJSON(t, fx.sessionsHandler.HandleRefresh, nil, map[string]string{
		"refresh_token": auth.RefreshToken,
	})
	// Second refresh with same original token → 401 (breach detected).
	rec := postJSON(t, fx.sessionsHandler.HandleRefresh, nil, map[string]string{
		"refresh_token": auth.RefreshToken,
	})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status=%d, want 401; body=%s", rec.Code, rec.Body.String())
	}
}

// TestScenarioHTTP_015_Logout_Always200 verifies scenario 15: logout endpoint
// is idempotent — even unknown tokens return 200 (no enumeration).
func TestScenarioHTTP_015_Logout_Always200(t *testing.T) {
	fx := newHTTPFixture()
	rec := postJSON(t, fx.sessionsHandler.HandleLogout, nil, map[string]string{
		"refresh_token": "anything",
	})
	if rec.Code != http.StatusOK {
		t.Errorf("status=%d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}

// TestScenarioHTTP_019_SessionList_LacksParsedUA verifies scenario 19 at the
// HTTP shape layer. EXPECTED TO FAIL: the JSON view of a session today only
// includes UserAgent + IP raw strings — no parsed browser/OS, no geo, no
// current-device marker. Document specifies these fields exist.
func TestScenarioHTTP_019_SessionList_LacksParsedUA(t *testing.T) {
	fx := newHTTPFixture()
	signupRec := postJSON(t, fx.authHandler.HandleSignup, nil, map[string]string{
		"email": "list@x.com", "password": "supersecret", "companyName": "L",
	})
	var auth usecases.AuthResponse
	_ = json.Unmarshal(signupRec.Body.Bytes(), &auth)

	rec := getJSON(t, fx.sessionsHandler.HandleList, withUser(auth.User.ID, auth.User.TenantID))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	// These JSON keys must appear in the response per scenario 19. Today they
	// don't — only `user_agent` / `ip` strings are returned.
	for _, want := range []string{"browser", "os", "geo", "current_session"} {
		if !strings.Contains(body, want) {
			t.Errorf("scenario 19: response JSON missing %q field — UI cannot render parsed/geo. body=%s", want, body)
		}
	}
}

// ---------------------------------------------------------------------
// Scenarios from sec 6 — Magic link
// ---------------------------------------------------------------------

// TestScenarioHTTP_038to43_MagicConsume_Returns401ForUsedOrExpired verifies
// scenarios 40 + 41 at HTTP: used/expired link → 401 with friendly message
// "link expired or already used".
func TestScenarioHTTP_040_MagicConsume_UsedLink_Returns401(t *testing.T) {
	fx := newHTTPFixture()
	user, _ := fx.auth.CreateUser(context.Background(), &domain.AdminUser{Email: "m@x.com", TenantID: "t1"})
	fx.magicUC.Issue(context.Background(), user.ID, user.Email)
	link, _ := fx.mailer.sent[0].Data["Link"].(string)
	code := strings.TrimPrefix(link, "https://app.example.com/auth/magic?code=")

	// First consume succeeds.
	rec1 := postJSON(t, fx.magicHandler.HandleConsume, nil, map[string]string{"code": code})
	if rec1.Code != http.StatusOK {
		t.Fatalf("first consume: status=%d, body=%s", rec1.Code, rec1.Body.String())
	}
	// Second consume → 401 with friendly message.
	rec2 := postJSON(t, fx.magicHandler.HandleConsume, nil, map[string]string{"code": code})
	if rec2.Code != http.StatusUnauthorized {
		t.Errorf("status=%d, want 401; body=%s", rec2.Code, rec2.Body.String())
	}
	if !strings.Contains(rec2.Body.String(), "expired or already used") {
		t.Errorf("scenario 40: error message should be friendly, got %s", rec2.Body.String())
	}
}

// ---------------------------------------------------------------------
// Scenarios from sec 9 — Shopify pending_link
// ---------------------------------------------------------------------

// TestScenarioHTTP_060_PendingShopLinkConsume_NoAuth_Returns401 verifies
// scenarios 57-60 at HTTP: pending_link consume requires auth.
func TestScenarioHTTP_060_PendingShopLinkConsume_NoAuth_Returns401(t *testing.T) {
	fx := newHTTPFixture()
	rec := postJSON(t, fx.magicHandler.HandleConsumePendingShopLink, nil, map[string]string{"token": "any"})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status=%d, want 401; body=%s", rec.Code, rec.Body.String())
	}
}

// TestScenarioHTTP_060_PendingShopLinkConsume_Auth_HappyPath verifies
// scenario 60 backend path: authenticated consume returns {tenant_id}.
func TestScenarioHTTP_060_PendingShopLinkConsume_Auth_HappyPath(t *testing.T) {
	fx := newHTTPFixture()
	// Pre-seed a user.
	user, _ := fx.auth.CreateUser(context.Background(), &domain.AdminUser{Email: "pl@x.com", TenantID: "t-other"})
	token, err := fx.magicUC.IssuePendingShopLink(context.Background(), "t-shopify-pending", "shop.myshopify.com")
	if err != nil {
		t.Fatalf("IssuePendingShopLink: %v", err)
	}

	rec := postJSON(t, fx.magicHandler.HandleConsumePendingShopLink,
		withUser(user.ID, user.TenantID),
		map[string]string{"token": token})
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		TenantID string `json:"tenant_id"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body.TenantID != "t-shopify-pending" {
		t.Errorf("tenant_id=%q, want t-shopify-pending; body=%s", body.TenantID, rec.Body.String())
	}
}

// ---------------------------------------------------------------------
// Method enforcement / 405 paths — defensive shape check.
// ---------------------------------------------------------------------

// TestScenarioHTTP_MethodNotAllowed_Returns405 verifies all public POST
// endpoints reject GET with 405 (not 404 / 200). Frontend regressions
// have shipped before because a route was wired but rejecting GET.
func TestScenarioHTTP_MethodNotAllowed_Returns405(t *testing.T) {
	fx := newHTTPFixture()
	cases := []struct {
		name string
		h    http.HandlerFunc
	}{
		{"signup", fx.authHandler.HandleSignup},
		{"login", fx.authHandler.HandleLogin},
		{"refresh", fx.sessionsHandler.HandleRefresh},
		{"logout", fx.sessionsHandler.HandleLogout},
		{"magic-consume", fx.magicHandler.HandleConsume},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c.h(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: GET status=%d, want 405; body=%s", c.name, rec.Code, rec.Body.String())
		}
	}
}
