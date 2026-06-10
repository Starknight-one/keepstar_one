package handlers

// Unit tests for POST /api/v1/internal/presets/preview — auth gate,
// request validation, 404 mapping, and a full happy-path render against
// fake ports (no DB, no LLM).

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// --- fakes -----------------------------------------------------------------

type fakeCatalogPort struct {
	tenant   *domain.Tenant
	products []domain.Product
}

func (f *fakeCatalogPort) GetTenantBySlug(_ context.Context, slug string) (*domain.Tenant, error) {
	if f.tenant == nil || f.tenant.Slug != slug {
		return nil, domain.ErrTenantNotFound
	}
	return f.tenant, nil
}

func (f *fakeCatalogPort) ListProducts(_ context.Context, _ string, filter ports.ProductFilter) ([]domain.Product, int, error) {
	n := filter.Limit
	if n > len(f.products) {
		n = len(f.products)
	}
	return f.products[:n], len(f.products), nil
}

func (f *fakeCatalogPort) GetProduct(context.Context, string, string) (*domain.Product, error) {
	panic("GetProduct unused")
}
func (f *fakeCatalogPort) VectorSearch(context.Context, string, []float32, int, *ports.VectorFilter) ([]domain.Product, error) {
	panic("VectorSearch unused")
}
func (f *fakeCatalogPort) SearchProjection(context.Context, string, []float32, ports.ProductFilter, int) ([]domain.Product, error) {
	panic("SearchProjection unused")
}
func (f *fakeCatalogPort) BuildCatalogDigest(context.Context, string) (*domain.CatalogDigest, error) {
	panic("BuildCatalogDigest unused")
}

type fakePresetPort struct {
	presets map[string]json.RawMessage // name → doc_json
}

func (f *fakePresetPort) GetPublishedPreset(_ context.Context, _ string, name string) (*domain.Preset, error) {
	doc, ok := f.presets[name]
	if !ok {
		return nil, domain.ErrPresetNotFound
	}
	return &domain.Preset{Name: name, DocumentJSON: doc}, nil
}

func (f *fakePresetPort) ListPublishedPresets(context.Context, string) ([]domain.Preset, error) {
	panic("ListPublishedPresets unused")
}

type fakeComponentPort struct{}

func (f *fakeComponentPort) GetPublishedComponent(context.Context, string, string) (*domain.Component, error) {
	panic("GetPublishedComponent unused")
}
func (f *fakeComponentPort) ListPublishedComponents(context.Context, string) ([]domain.Component, error) {
	return nil, nil
}

// --- harness ---------------------------------------------------------------

// previewCardDoc is a minimal translate-shaped doc: a grid with one
// replicate-marked card holding a bound text + an empty actions frame.
const previewCardDoc = `{"version":"2.10","children":[{"id":"grid","type":"frame","children":[{"id":"card","type":"frame","replicate":true,"children":[{"id":"t1","type":"text","fieldBinding":"name"},{"id":"actions","type":"frame","children":[]}]}]}]}`

func previewTestHandler() *PresetHandler {
	catalog := &fakeCatalogPort{
		tenant: &domain.Tenant{ID: "11111111-2222-3333-4444-555555555555", Slug: "demo", Name: "Demo"},
		products: []domain.Product{
			{ID: "p-0", Name: "Oak Table", Price: 19900, PriceFormatted: "$199.00", Images: []string{"https://img.test/p0.jpg"}, StockQuantity: 1},
			{ID: "p-1", Name: "Walnut Shelf", Price: 34950, PriceFormatted: "$349.50", Images: []string{"https://img.test/p1.jpg"}, StockQuantity: 1},
			{ID: "p-2", Name: "Birch Chair", Price: 8999, PriceFormatted: "$89.99", Images: []string{"https://img.test/p2.jpg"}, StockQuantity: 1},
			{ID: "p-3", Name: "Pine Desk", Price: 12000, PriceFormatted: "$120.00", Images: []string{"https://img.test/p3.jpg"}, StockQuantity: 1},
		},
	}
	presetPort := &fakePresetPort{presets: map[string]json.RawMessage{
		"product_card_demo": json.RawMessage(previewCardDoc),
	}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewPresetHandler(nil, catalog, presetPort, &fakeComponentPort{}, log)
}

func doPreview(t *testing.T, h *PresetHandler, url, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, url, bytes.NewBufferString(body))
	if key != "" {
		req.Header.Set("X-Internal-Key", key)
	}
	rec := httptest.NewRecorder()
	h.Preview(rec, req)
	return rec
}

// --- tests -----------------------------------------------------------------

func TestPreviewAuthGate(t *testing.T) {
	h := previewTestHandler()

	t.Run("key unset → 503", func(t *testing.T) {
		t.Setenv("V5_INTERNAL_KEY", "")
		rec := doPreview(t, h, "/api/v1/internal/presets/preview?tenant=demo", "anything", `{"presetName":"x"}`)
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", rec.Code)
		}
	})

	t.Run("wrong key → 403", func(t *testing.T) {
		t.Setenv("V5_INTERNAL_KEY", "secret")
		rec := doPreview(t, h, "/api/v1/internal/presets/preview?tenant=demo", "wrong", `{"presetName":"x"}`)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rec.Code)
		}
	})
}

func TestPreviewValidation(t *testing.T) {
	h := previewTestHandler()
	t.Setenv("V5_INTERNAL_KEY", "secret")

	cases := []struct {
		name string
		url  string
		body string
		want int
	}{
		{"missing tenant param", "/api/v1/internal/presets/preview", `{"presetName":"x"}`, http.StatusBadRequest},
		{"invalid JSON body", "/api/v1/internal/presets/preview?tenant=demo", `{nope`, http.StatusBadRequest},
		{"neither docJson nor presetName", "/api/v1/internal/presets/preview?tenant=demo", `{}`, http.StatusBadRequest},
		{"both docJson and presetName", "/api/v1/internal/presets/preview?tenant=demo", `{"presetName":"x","docJson":` + previewCardDoc + `}`, http.StatusBadRequest},
		{"negative count", "/api/v1/internal/presets/preview?tenant=demo", `{"presetName":"product_card_demo","count":-1}`, http.StatusBadRequest},
		{"docJson not a document", "/api/v1/internal/presets/preview?tenant=demo", `{"docJson":"not-an-object"}`, http.StatusBadRequest},
		{"unknown tenant", "/api/v1/internal/presets/preview?tenant=ghost", `{"presetName":"product_card_demo"}`, http.StatusNotFound},
		{"unknown preset", "/api/v1/internal/presets/preview?tenant=demo", `{"presetName":"ghost"}`, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := doPreview(t, h, tc.url, "secret", tc.body)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d (body: %s)", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

// previewResponse mirrors the wire shape for assertions.
type previewResponse struct {
	Document     map[string]interface{} `json:"document"`
	ProductCount int                    `json:"productCount"`
}

// decodePreview parses the 200 response.
func decodePreview(t *testing.T, rec *httptest.ResponseRecorder) previewResponse {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var out previewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return out
}

// gridCards digs document.children[0].children (the replicated cards).
func gridCards(t *testing.T, doc map[string]interface{}) []interface{} {
	t.Helper()
	children, _ := doc["children"].([]interface{})
	if len(children) != 1 {
		t.Fatalf("expected 1 top-level grid, got %d", len(children))
	}
	grid, _ := children[0].(map[string]interface{})
	cards, _ := grid["children"].([]interface{})
	return cards
}

func TestPreviewHappyPathDraftDoc(t *testing.T) {
	h := previewTestHandler()
	t.Setenv("V5_INTERNAL_KEY", "secret")

	rec := doPreview(t, h, "/api/v1/internal/presets/preview?tenant=demo", "secret", `{"docJson":`+previewCardDoc+`}`)
	out := decodePreview(t, rec)

	if out.ProductCount != 3 { // default count
		t.Fatalf("productCount = %d, want 3", out.ProductCount)
	}
	cards := gridCards(t, out.Document)
	if len(cards) != 3 {
		t.Fatalf("expected 3 replicated cards, got %d", len(cards))
	}
	wantNames := []string{"Oak Table", "Walnut Shelf", "Birch Chair"}
	for i, raw := range cards {
		card, _ := raw.(map[string]interface{})
		kids, _ := card["children"].([]interface{})
		if len(kids) != 2 {
			t.Fatalf("card[%d] expected 2 children (text+actions), got %d", i, len(kids))
		}
		text, _ := kids[0].(map[string]interface{})
		if got, _ := text["content"].(string); got != wantNames[i] {
			t.Errorf("card[%d] text content = %q, want %q", i, got, wantNames[i])
		}
		actions, _ := kids[1].(map[string]interface{})
		buttons, _ := actions["children"].([]interface{})
		if len(buttons) != 2 {
			t.Errorf("card[%d] actions frame expected 2 injected buttons, got %d", i, len(buttons))
		}
	}
}

func TestPreviewHappyPathPublishedPreset(t *testing.T) {
	h := previewTestHandler()
	t.Setenv("V5_INTERNAL_KEY", "secret")

	rec := doPreview(t, h, "/api/v1/internal/presets/preview?tenant=demo", "secret",
		`{"presetName":"product_card_demo","count":2}`)
	out := decodePreview(t, rec)

	if out.ProductCount != 2 {
		t.Fatalf("productCount = %d, want 2", out.ProductCount)
	}
	if cards := gridCards(t, out.Document); len(cards) != 2 {
		t.Fatalf("expected 2 replicated cards, got %d", len(cards))
	}
}

func TestPreviewCountCappedAt12(t *testing.T) {
	h := previewTestHandler()
	t.Setenv("V5_INTERNAL_KEY", "secret")

	// Fake catalog has 4 products; count=50 must be capped to 12, then
	// bounded by what the catalog returns (4).
	rec := doPreview(t, h, "/api/v1/internal/presets/preview?tenant=demo", "secret",
		`{"presetName":"product_card_demo","count":50}`)
	out := decodePreview(t, rec)
	if out.ProductCount != 4 {
		t.Fatalf("productCount = %d, want 4 (capped fetch over a 4-product catalog)", out.ProductCount)
	}
}
