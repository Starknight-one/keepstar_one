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
	"keepstar_v5/internal/engine/presets"
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

// --- GET /api/v1/internal/presets/system ------------------------------------

func doListSystem(t *testing.T, h *PresetHandler, key string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/internal/presets/system", nil)
	if key != "" {
		req.Header.Set("X-Internal-Key", key)
	}
	rec := httptest.NewRecorder()
	h.ListSystem(rec, req)
	return rec
}

func TestListSystemAuthGate(t *testing.T) {
	h := previewTestHandler()

	t.Run("key unset → 503", func(t *testing.T) {
		t.Setenv("V5_INTERNAL_KEY", "")
		rec := doListSystem(t, h, "anything")
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", rec.Code)
		}
	})

	t.Run("wrong key → 403", func(t *testing.T) {
		t.Setenv("V5_INTERNAL_KEY", "secret")
		rec := doListSystem(t, h, "wrong")
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", rec.Code)
		}
	})
}

// TestListSystemContract pins the frozen cross-repo wire shape the admin
// canvas consumes: 14 entries (12 presets + 2 components), sorted by
// name, exact field names, canonical categories. If the registry gains a
// preset without taxonomy coverage, or a field is renamed, this fails.
func TestListSystemContract(t *testing.T) {
	h := previewTestHandler()
	t.Setenv("V5_INTERNAL_KEY", "secret")

	rec := doListSystem(t, h, "secret")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}

	// Decode into raw maps so we can assert the EXACT field names of the
	// contract, not just whatever a Go struct happens to tolerate.
	var out struct {
		Presets []map[string]interface{} `json:"presets"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	wantNames := []string{
		"component_brand_badge",
		"component_price_rating",
		"empty_not_found",
		"error_generic",
		"product_card",
		"product_card_compact",
		"product_card_horizontal",
		"product_card_list_row",
		"product_carousel",
		"product_comparison",
		"product_detail",
		"product_detail_accordion",
		"product_detail_horizontal",
		"text_explainer",
	}
	if len(out.Presets) != len(wantNames) {
		t.Fatalf("entries = %d, want %d (body: %s)", len(out.Presets), len(wantNames), rec.Body.String())
	}

	wantFields := []string{"name", "description", "category", "defaultReplicate", "kind"}
	canonical := map[string]bool{"cards": true, "details": true, "components": true, "states": true, "narrative": true}
	byName := map[string]map[string]interface{}{}
	for i, entry := range out.Presets {
		name, _ := entry["name"].(string)
		if name != wantNames[i] {
			t.Errorf("entry[%d] name = %q, want %q (sorted ascending)", i, name, wantNames[i])
		}
		if len(entry) != len(wantFields) {
			t.Errorf("entry %q has %d fields, want %d: %v", name, len(entry), len(wantFields), entry)
		}
		for _, f := range wantFields {
			if _, ok := entry[f]; !ok {
				t.Errorf("entry %q missing contract field %q", name, f)
			}
		}
		if desc, _ := entry["description"].(string); desc == "" {
			t.Errorf("entry %q has empty description", name)
		}
		if cat, _ := entry["category"].(string); !canonical[cat] {
			t.Errorf("entry %q category = %q, not canonical", name, cat)
		}
		byName[name] = entry
	}

	// Spot-check taxonomy + flags per the frozen contract.
	spots := []struct {
		name      string
		category  string
		replicate bool
		kind      string
	}{
		{"product_card", "cards", true, "preset"},
		{"product_detail_accordion", "details", false, "preset"},
		{"empty_not_found", "states", false, "preset"},
		{"text_explainer", "narrative", false, "preset"},
		{"component_price_rating", "components", false, "component"},
	}
	for _, s := range spots {
		entry := byName[s.name]
		if entry == nil {
			t.Errorf("entry %q missing", s.name)
			continue
		}
		if got, _ := entry["category"].(string); got != s.category {
			t.Errorf("%s category = %q, want %q", s.name, got, s.category)
		}
		if got, _ := entry["defaultReplicate"].(bool); got != s.replicate {
			t.Errorf("%s defaultReplicate = %v, want %v", s.name, got, s.replicate)
		}
		if got, _ := entry["kind"].(string); got != s.kind {
			t.Errorf("%s kind = %q, want %q", s.name, got, s.kind)
		}
	}

	// Preset descriptions must be the Agent2-visible ones (the same text
	// SystemPresetsBlock injects into the Agent2 prompt), not a parallel
	// copy that could drift.
	if got, _ := byName["product_card"]["description"].(string); got != presets.SystemPresetDescriptions["product_card"] {
		t.Errorf("product_card description = %q, want SystemPresetDescriptions text %q",
			got, presets.SystemPresetDescriptions["product_card"])
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
