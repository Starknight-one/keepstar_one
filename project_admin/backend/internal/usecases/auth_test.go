package usecases

import (
	"context"
	"errors"
	"testing"
	"time"

	"keepstar-admin/internal/domain"
)

// fakeAdminCatalog implements just enough of AdminCatalogPort for Signup +
// Login + GetMe. Methods we don't need panic so a future test that drifts
// catches it loudly.
type fakeAdminCatalog struct {
	tenants    map[string]*domain.Tenant
	createErr  error
	nextID     int
	createdLog []*domain.Tenant
}

func newFakeAdminCatalog() *fakeAdminCatalog {
	return &fakeAdminCatalog{tenants: map[string]*domain.Tenant{}}
}

func (f *fakeAdminCatalog) CreateTenant(_ context.Context, t *domain.Tenant) (*domain.Tenant, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.nextID++
	t.ID = "tenant-" + t.Slug
	f.tenants[t.ID] = t
	f.createdLog = append(f.createdLog, t)
	return t, nil
}
func (f *fakeAdminCatalog) GetTenantByID(_ context.Context, id string) (*domain.Tenant, error) {
	t, ok := f.tenants[id]
	if !ok {
		return nil, errors.New("tenant not found")
	}
	return t, nil
}

// --- unused-here methods panic; keeps the interface satisfied without
//     writing stubs that quietly return zero values. ---

func (f *fakeAdminCatalog) UpdateTenantSettings(context.Context, string, domain.TenantSettings) error {
	panic("not implemented")
}
func (f *fakeAdminCatalog) ListProducts(context.Context, string, domain.AdminProductFilter) ([]domain.Product, int, error) {
	panic("not implemented")
}
func (f *fakeAdminCatalog) GetProduct(context.Context, string, string) (*domain.Product, error) {
	panic("not implemented")
}
func (f *fakeAdminCatalog) UpdateProduct(context.Context, string, string, domain.ProductUpdate) error {
	panic("not implemented")
}
func (f *fakeAdminCatalog) ListServices(context.Context, string, domain.AdminProductFilter) ([]domain.Service, int, error) {
	panic("not implemented")
}
func (f *fakeAdminCatalog) GetService(context.Context, string, string) (*domain.Service, error) {
	panic("not implemented")
}
func (f *fakeAdminCatalog) UpdateService(context.Context, string, string, domain.ProductUpdate) error {
	panic("not implemented")
}
func (f *fakeAdminCatalog) GetCategories(context.Context, string) ([]domain.Category, error) {
	panic("not implemented")
}
func (f *fakeAdminCatalog) UpsertMasterProduct(context.Context, *domain.MasterProduct) (string, error) {
	panic("not implemented")
}
func (f *fakeAdminCatalog) UpsertProductListing(context.Context, *domain.Product) (string, error) {
	panic("not implemented")
}
func (f *fakeAdminCatalog) UpsertMasterService(context.Context, *domain.MasterService) (string, error) {
	panic("not implemented")
}
func (f *fakeAdminCatalog) UpsertServiceListing(context.Context, *domain.Service) (string, error) {
	panic("not implemented")
}
func (f *fakeAdminCatalog) GetOrCreateCategory(context.Context, string, string) (string, error) {
	panic("not implemented")
}
func (f *fakeAdminCatalog) BulkUpdateStock(context.Context, string, []domain.StockUpdate) (int, error) {
	panic("not implemented")
}
func (f *fakeAdminCatalog) GetCategoryBySlug(context.Context, string) (*domain.Category, error) {
	panic("not implemented")
}
func (f *fakeAdminCatalog) GetAllMasterProducts(context.Context, string) ([]domain.MasterProduct, error) {
	panic("not implemented")
}
func (f *fakeAdminCatalog) GetUnenrichedMasterProducts(context.Context, string) ([]domain.MasterProduct, error) {
	panic("not implemented")
}
func (f *fakeAdminCatalog) UpdateMasterProductPIM(context.Context, string, string, domain.EnrichmentOutputV2) error {
	panic("not implemented")
}
func (f *fakeAdminCatalog) SoftDeleteProductBySource(context.Context, string, string, string) error {
	panic("not implemented")
}
func (f *fakeAdminCatalog) UpsertListingFromSource(context.Context, *domain.ListingFromSource) (string, error) {
	panic("not implemented")
}
func (f *fakeAdminCatalog) GetMasterProductsWithoutEmbedding(context.Context, string) ([]domain.MasterProduct, error) {
	panic("not implemented")
}
func (f *fakeAdminCatalog) GetMasterServicesWithoutEmbedding(context.Context, string) ([]domain.MasterService, error) {
	panic("not implemented")
}
func (f *fakeAdminCatalog) SeedEmbedding(context.Context, string, []float32) error {
	panic("not implemented")
}
func (f *fakeAdminCatalog) SeedServiceEmbedding(context.Context, string, []float32) error {
	panic("not implemented")
}
func (f *fakeAdminCatalog) GenerateCatalogDigest(context.Context, string) error {
	panic("not implemented")
}

// --- helpers ---

func mkAuthUC() (*AuthUseCase, *fakeAuth, *fakeAdminCatalog, *fakeMemberships) {
	auth := newFakeAuth()
	cat := newFakeAdminCatalog()
	mem := newFakeMemberships()
	uc := NewAuthUseCase(auth, cat, "test-secret-32-bytes-long-xxxxxx")
	uc.SetMemberships(mem)
	return uc, auth, cat, mem
}

// --- Signup ---

func TestSignup_HappyPath(t *testing.T) {
	uc, auth, cat, mem := mkAuthUC()

	resp, err := uc.Signup(context.Background(), SignupRequest{
		Email:       "new@example.com",
		Password:    "supersecret",
		CompanyName: "Acme Shop",
	})
	if err != nil {
		t.Fatalf("Signup: %v", err)
	}
	if resp == nil || resp.User == nil {
		t.Fatalf("Signup returned nil response")
	}
	if resp.User.Email != "new@example.com" {
		t.Errorf("user email = %q, want new@example.com", resp.User.Email)
	}
	if resp.User.Role != domain.AdminRoleOwner {
		t.Errorf("role = %q, want owner", resp.User.Role)
	}
	if resp.Token == "" {
		t.Errorf("token empty (sessions not wired → fallback JWT expected)")
	}
	// Tenant created with slug derived from company name.
	if len(cat.createdLog) != 1 || cat.createdLog[0].Slug != "acme-shop" {
		t.Errorf("expected one tenant 'acme-shop', got %+v", cat.createdLog)
	}
	// Owner membership granted.
	if _, ok, _ := mem.HasMembership(context.Background(), resp.User.ID, resp.User.TenantID); !ok {
		t.Errorf("owner membership not granted")
	}
	// User stored.
	if _, err := auth.GetUserByEmail(context.Background(), "new@example.com"); err != nil {
		t.Errorf("user not stored: %v", err)
	}
}

func TestSignup_DuplicateEmailRejects(t *testing.T) {
	uc, auth, _, _ := mkAuthUC()
	_, _ = auth.CreateUser(context.Background(), &domain.AdminUser{Email: "taken@x.com"})

	_, err := uc.Signup(context.Background(), SignupRequest{
		Email:       "taken@x.com",
		Password:    "supersecret",
		CompanyName: "X",
	})
	if !errors.Is(err, domain.ErrEmailExists) {
		t.Errorf("err = %v, want ErrEmailExists", err)
	}
}

func TestSignup_RejectsMissingFields(t *testing.T) {
	uc, _, _, _ := mkAuthUC()
	cases := []SignupRequest{
		{Email: "", Password: "p", CompanyName: "C"},
		{Email: "e@e", Password: "", CompanyName: "C"},
		{Email: "e@e", Password: "p", CompanyName: ""},
	}
	for _, c := range cases {
		if _, err := uc.Signup(context.Background(), c); err == nil {
			t.Errorf("Signup(%+v) should reject", c)
		}
	}
}

func TestSignup_RejectsShortPassword(t *testing.T) {
	uc, _, _, _ := mkAuthUC()
	_, err := uc.Signup(context.Background(), SignupRequest{
		Email:       "e@e.com",
		Password:    "12345", // < 6
		CompanyName: "X",
	})
	if err == nil {
		t.Error("expected rejection for short password")
	}
}

func TestSignup_TenantCreateFailRollsUp(t *testing.T) {
	uc, _, cat, _ := mkAuthUC()
	cat.createErr = errors.New("db down")
	_, err := uc.Signup(context.Background(), SignupRequest{
		Email: "e@e.com", Password: "supersecret", CompanyName: "X",
	})
	if err == nil {
		t.Error("expected error when tenant create fails")
	}
}

// --- Login ---

func TestLogin_HappyPath(t *testing.T) {
	uc, auth, _, _ := mkAuthUC()
	// Use Signup to get a properly-hashed user, then Login on the same creds.
	if _, err := uc.Signup(context.Background(), SignupRequest{
		Email: "u@e.com", Password: "supersecret", CompanyName: "C",
	}); err != nil {
		t.Fatalf("seed Signup: %v", err)
	}
	_ = auth // not needed directly
	resp, err := uc.Login(context.Background(), LoginRequest{
		Email: "u@e.com", Password: "supersecret",
	})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if resp.Token == "" {
		t.Error("token empty")
	}
	if resp.User == nil || resp.User.Email != "u@e.com" {
		t.Errorf("user wrong: %+v", resp.User)
	}
}

func TestLogin_EmailNotFound(t *testing.T) {
	uc, _, _, _ := mkAuthUC()
	_, err := uc.Login(context.Background(), LoginRequest{
		Email: "ghost@nowhere.com", Password: "anything",
	})
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	uc, _, _, _ := mkAuthUC()
	if _, err := uc.Signup(context.Background(), SignupRequest{
		Email: "u@e.com", Password: "supersecret", CompanyName: "C",
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, err := uc.Login(context.Background(), LoginRequest{
		Email: "u@e.com", Password: "wrong-password",
	})
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestLogin_MissingFields(t *testing.T) {
	uc, _, _, _ := mkAuthUC()
	if _, err := uc.Login(context.Background(), LoginRequest{Email: "", Password: ""}); err == nil {
		t.Error("expected rejection for missing fields")
	}
}

// --- GetMe / GetTenant ---

func TestGetMe_HappyPath(t *testing.T) {
	uc, auth, _, _ := mkAuthUC()
	created, _ := auth.CreateUser(context.Background(), &domain.AdminUser{
		Email:    "me@x.com",
		TenantID: "t1",
		Role:     domain.AdminRoleOwner,
	})
	got, err := uc.GetMe(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetMe: %v", err)
	}
	if got.Email != "me@x.com" {
		t.Errorf("email = %q, want me@x.com", got.Email)
	}
}

func TestGetMe_NotFound(t *testing.T) {
	uc, _, _, _ := mkAuthUC()
	_, err := uc.GetMe(context.Background(), "user-missing")
	if !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("err = %v, want ErrUserNotFound", err)
	}
}

func TestGetTenant_HappyPath(t *testing.T) {
	uc, _, cat, _ := mkAuthUC()
	cat.tenants["t-1"] = &domain.Tenant{ID: "t-1", Slug: "acme", Name: "Acme"}
	got, err := uc.GetTenant(context.Background(), "t-1")
	if err != nil {
		t.Fatalf("GetTenant: %v", err)
	}
	if got.Slug != "acme" {
		t.Errorf("slug = %q, want acme", got.Slug)
	}
}

// --- slugify regression ---

func TestSlugify(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Acme Shop", "acme-shop"},
		{"  whitespace   tour ", "whitespace-tour"},
		{"!!!", "store"}, // all-punctuation → fallback
		{"My Store 2026", "my-store-2026"},
		{"already-good", "already-good"},
		{"Stone & Steel", "stone-steel"},
	}
	for _, c := range cases {
		if got := slugify(c.in); got != c.want {
			t.Errorf("slugify(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// --- SetTwoFactor + pre-2FA branch -- need TwoFactor + Sessions wired -----

// The pre-2FA branch is exercised in detail by auth_2fa_test.go and the
// magic-link suite. Here we just smoke-check that issueTokens picks the
// pre-2FA path when 2FA is enabled.
func TestIssueTokens_PreTFAWhen2FAEnabled(t *testing.T) {
	t.Skip("pre-2FA branch covered by auth_2fa_test.go; placeholder for completeness")
	// pre2faTTL accessed indirectly via SetTwoFactor — leave to dedicated test.
	_ = time.Minute
}

// =====================================================================
// Pre-launch scenario verification (docs/pre_launch_scenarios.md sec 1)
// =====================================================================

// TestScenario_005_SignupGarbageCompanyName_SlugFallsBackToStore verifies:
// «Я как пользователь регистрируюсь с companyName "!!!" (мусор) — slug
// fallback'ается в `store`.» (sec 1, scenario 5)
func TestScenario_005_SignupGarbageCompanyName_SlugFallsBackToStore(t *testing.T) {
	uc, _, cat, _ := mkAuthUC()
	if _, err := uc.Signup(context.Background(), SignupRequest{
		Email: "junk@example.com", Password: "supersecret", CompanyName: "!!!",
	}); err != nil {
		t.Fatalf("Signup: %v", err)
	}
	if len(cat.createdLog) != 1 {
		t.Fatalf("tenant not created")
	}
	if cat.createdLog[0].Slug != "store" {
		t.Errorf("slug = %q, want 'store' (junk-name fallback)", cat.createdLog[0].Slug)
	}
}

// TestScenario_006_SignupCompanyNameWithAmpersand_NormalizedSlug verifies:
// «Я как пользователь регистрируюсь с companyName "Stone & Steel" — slug =
// `stone-steel` (амперсанд нормализуется).» (sec 1, scenario 6)
func TestScenario_006_SignupCompanyNameAmpersand_Normalized(t *testing.T) {
	uc, _, cat, _ := mkAuthUC()
	if _, err := uc.Signup(context.Background(), SignupRequest{
		Email: "stone@example.com", Password: "supersecret", CompanyName: "Stone & Steel",
	}); err != nil {
		t.Fatalf("Signup: %v", err)
	}
	if len(cat.createdLog) != 1 {
		t.Fatalf("tenant not created")
	}
	if cat.createdLog[0].Slug != "stone-steel" {
		t.Errorf("slug = %q, want 'stone-steel'", cat.createdLog[0].Slug)
	}
}

// TestScenario_007_SignupUppercaseEmail_StoredLowercaseAndCaseInsensitiveLogin verifies:
// «Я как пользователь регистрируюсь с email в верхнем регистре `USER@X.com`
// — в БД хранится lowercased; при следующем login регистро-нечувствительно.»
// (sec 1, scenario 7)
//
// EXPECTED TO FAIL if Signup does NOT lowercase email on insert. Today's
// AuthUseCase.Signup() passes Email through verbatim to CreateUser(),
// relying on the adapter to lowercase. fakeAuth.CreateUser ALSO doesn't
// lowercase the canonical stored key in this test (it lowercases for the
// `users` map but stores u.Email as given). So the assertion below is the
// document's contract — if it fails, signup is leaking the original case.
func TestScenario_007_SignupUppercaseEmail_StoredAndLoginCaseInsensitive(t *testing.T) {
	uc, auth, _, _ := mkAuthUC()
	if _, err := uc.Signup(context.Background(), SignupRequest{
		Email: "USER@X.COM", Password: "supersecret", CompanyName: "Acme",
	}); err != nil {
		t.Fatalf("Signup: %v", err)
	}
	stored, err := auth.GetUserByEmail(context.Background(), "user@x.com")
	if err != nil {
		t.Fatalf("GetUserByEmail(lowercased): %v", err)
	}
	if stored.Email != "user@x.com" {
		t.Errorf("scenario 7: stored email = %q, want 'user@x.com' (lowercased)", stored.Email)
	}
	// Login with the original case AND with all-lowercase must both work.
	if _, err := uc.Login(context.Background(), LoginRequest{Email: "USER@X.COM", Password: "supersecret"}); err != nil {
		t.Errorf("Login(uppercase): %v", err)
	}
	if _, err := uc.Login(context.Background(), LoginRequest{Email: "user@x.com", Password: "supersecret"}); err != nil {
		t.Errorf("Login(lowercase): %v", err)
	}
}
