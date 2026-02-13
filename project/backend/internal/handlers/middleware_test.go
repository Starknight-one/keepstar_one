package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"keepstar/internal/domain"
	"keepstar/internal/handlers"
	"keepstar/internal/ports"
)

// --- minimal CatalogPort mock for middleware tests ---

type middlewareCatalogMock struct {
	tenants map[string]*domain.Tenant // slug → tenant
}

func (m *middlewareCatalogMock) GetTenantBySlug(_ context.Context, slug string) (*domain.Tenant, error) {
	if t, ok := m.tenants[slug]; ok {
		return t, nil
	}
	return nil, domain.ErrTenantNotFound
}

func (m *middlewareCatalogMock) GetCategories(context.Context) ([]domain.Category, error) {
	return nil, nil
}
func (m *middlewareCatalogMock) GetMasterProduct(context.Context, string) (*domain.MasterProduct, error) {
	return nil, nil
}
func (m *middlewareCatalogMock) ListProducts(context.Context, string, ports.ProductFilter) ([]domain.Product, int, error) {
	return nil, 0, nil
}
func (m *middlewareCatalogMock) GetProduct(context.Context, string, string) (*domain.Product, error) {
	return nil, nil
}
func (m *middlewareCatalogMock) GetStock(context.Context, string, string) (*domain.Stock, error) {
	return nil, nil
}
func (m *middlewareCatalogMock) ListServices(context.Context, string, ports.ProductFilter) ([]domain.Service, int, error) {
	return nil, 0, nil
}
func (m *middlewareCatalogMock) GetService(context.Context, string, string) (*domain.Service, error) {
	return nil, nil
}
func (m *middlewareCatalogMock) VectorSearchServices(context.Context, string, []float32, int, *ports.VectorFilter) ([]domain.Service, error) {
	return nil, nil
}
func (m *middlewareCatalogMock) GetMasterServicesWithoutEmbedding(context.Context) ([]domain.MasterService, error) {
	return nil, nil
}
func (m *middlewareCatalogMock) VectorSearch(context.Context, string, []float32, int, *ports.VectorFilter) ([]domain.Product, error) {
	return nil, nil
}
func (m *middlewareCatalogMock) SeedEmbedding(context.Context, string, []float32) error { return nil }
func (m *middlewareCatalogMock) SeedServiceEmbedding(context.Context, string, []float32) error {
	return nil
}
func (m *middlewareCatalogMock) GetMasterProductsWithoutEmbedding(context.Context) ([]domain.MasterProduct, error) {
	return nil, nil
}
func (m *middlewareCatalogMock) GenerateCatalogDigest(context.Context, string) (*domain.CatalogDigest, error) {
	return nil, nil
}
func (m *middlewareCatalogMock) GetCatalogDigest(context.Context, string) (*domain.CatalogDigest, error) {
	return nil, nil
}
func (m *middlewareCatalogMock) SaveCatalogDigest(context.Context, string, *domain.CatalogDigest) error {
	return nil
}
func (m *middlewareCatalogMock) GetAllTenants(context.Context) ([]domain.Tenant, error) {
	return nil, nil
}

// --- CORS tests ---

func TestCORSMiddleware_PreflightReturns200(t *testing.T) {
	handler := handlers.CORSMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("inner handler should not be called on OPTIONS")
	}))

	req := httptest.NewRequest("OPTIONS", "/api/v1/session/abc", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("OPTIONS: want 200, got %d", rr.Code)
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing Access-Control-Allow-Origin header")
	}
	if rr.Header().Get("Access-Control-Allow-Headers") == "" {
		t.Error("missing Access-Control-Allow-Headers header")
	}
}

func TestCORSMiddleware_NonOptionsPassesThrough(t *testing.T) {
	called := false
	handler := handlers.CORSMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("inner handler should be called for GET")
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("CORS headers should be set on regular requests too")
	}
}

func TestCORSMiddleware_AllowsXTenantSlugHeader(t *testing.T) {
	handler := handlers.CORSMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	req := httptest.NewRequest("OPTIONS", "/api/v1/session/abc", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	allowHeaders := rr.Header().Get("Access-Control-Allow-Headers")
	if allowHeaders == "" {
		t.Fatal("missing Allow-Headers")
	}
	// X-Tenant-Slug must be allowed
	found := false
	for _, h := range []string{"X-Tenant-Slug", "Content-Type"} {
		if contains(allowHeaders, h) {
			found = true
		}
	}
	if !found {
		t.Errorf("Allow-Headers should contain X-Tenant-Slug, got %q", allowHeaders)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// --- Tenant middleware: ResolveTenant (from path) ---

func newMockCatalog() *middlewareCatalogMock {
	return &middlewareCatalogMock{
		tenants: map[string]*domain.Tenant{
			"nike": {ID: "t1", Slug: "nike", Name: "Nike Store"},
		},
	}
}

func TestTenantMiddleware_ResolveTenantFromPath_Found(t *testing.T) {
	mw := handlers.NewTenantMiddleware(newMockCatalog())

	var gotTenant *domain.Tenant
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTenant = handlers.GetTenantFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := mw.ResolveTenant(inner)
	req := httptest.NewRequest("GET", "/api/v1/tenants/nike/products", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	if gotTenant == nil {
		t.Fatal("expected tenant in context")
	}
	if gotTenant.Slug != "nike" {
		t.Errorf("want slug nike, got %s", gotTenant.Slug)
	}
}

func TestTenantMiddleware_ResolveTenantFromPath_NotFound(t *testing.T) {
	mw := handlers.NewTenantMiddleware(newMockCatalog())

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("inner handler should not be called when tenant not found")
	})

	handler := mw.ResolveTenant(inner)
	req := httptest.NewRequest("GET", "/api/v1/tenants/unknown/products", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rr.Code)
	}
}

func TestTenantMiddleware_ResolveTenantFromPath_MissingSlug(t *testing.T) {
	mw := handlers.NewTenantMiddleware(newMockCatalog())

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("inner handler should not be called when slug missing")
	})

	handler := mw.ResolveTenant(inner)
	req := httptest.NewRequest("GET", "/api/v1/products", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

// --- Tenant middleware: ResolveFromHeader ---

func TestTenantMiddleware_ResolveFromHeader_WithHeader(t *testing.T) {
	mw := handlers.NewTenantMiddleware(newMockCatalog())

	var gotTenant *domain.Tenant
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTenant = handlers.GetTenantFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := mw.ResolveFromHeader("")(inner)
	req := httptest.NewRequest("GET", "/api/v1/session/abc", nil)
	req.Header.Set("X-Tenant-Slug", "nike")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	if gotTenant == nil || gotTenant.Slug != "nike" {
		t.Errorf("expected tenant nike in context, got %v", gotTenant)
	}
}

func TestTenantMiddleware_ResolveFromHeader_DefaultFallback(t *testing.T) {
	mw := handlers.NewTenantMiddleware(newMockCatalog())

	var gotTenant *domain.Tenant
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTenant = handlers.GetTenantFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := mw.ResolveFromHeader("nike")(inner)
	req := httptest.NewRequest("GET", "/api/v1/session/abc", nil)
	// No X-Tenant-Slug header — should fall back to default "nike"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
	if gotTenant == nil || gotTenant.Slug != "nike" {
		t.Errorf("expected default tenant nike, got %v", gotTenant)
	}
}

func TestTenantMiddleware_ResolveFromHeader_InvalidTenantContinues(t *testing.T) {
	mw := handlers.NewTenantMiddleware(newMockCatalog())

	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		tenant := handlers.GetTenantFromContext(r.Context())
		if tenant != nil {
			t.Error("expected nil tenant for unknown slug")
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := mw.ResolveFromHeader("")(inner)
	req := httptest.NewRequest("GET", "/api/v1/session/abc", nil)
	req.Header.Set("X-Tenant-Slug", "nonexistent")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("inner handler should be called even with unknown tenant (graceful fallback)")
	}
}
