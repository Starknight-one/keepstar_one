package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"keepstar_v5/internal/domain"
)

// fakeThemePort models the v5_themes store: a present row returns its override
// merged over defaults (IsDefault=false); a missing row returns the canonical
// defaults (IsDefault=true) — exactly the contract's no-row semantics, without
// a DB. UpsertTheme records the last write so PUT round-trips can be asserted.
type fakeThemePort struct {
	override   *domain.ThemeTokens // nil ⇒ no row ⇒ defaults
	notFound   bool                // simulate unknown tenant
	lastUpsert *domain.ThemeTokens
}

func (f *fakeThemePort) GetTheme(_ context.Context, _ string) (domain.Theme, error) {
	if f.notFound {
		return domain.Theme{}, domain.ErrTenantNotFound
	}
	if f.override == nil {
		return domain.DefaultTheme(), nil
	}
	return domain.Theme{
		Tokens:    domain.MergeThemeTokensOverDefaults(*f.override),
		IsDefault: false,
	}, nil
}

func (f *fakeThemePort) UpsertTheme(_ context.Context, _ string, tokens domain.ThemeTokens) error {
	if f.notFound {
		return domain.ErrTenantNotFound
	}
	cp := tokens
	f.lastUpsert = &cp
	f.override = &cp // subsequent GetTheme reflects the write
	return nil
}

// TestResolveThemeMap_NoRowDefaults asserts the SINGLE attach helper returns
// the canonical default tokens with isDefault:true when the tenant has no row,
// and that those tokens equal the contract values (byte-identical guard at the
// wire-map level the frontend actually reads).
func TestResolveThemeMap_NoRowDefaults(t *testing.T) {
	m := resolveThemeMap(context.Background(), &fakeThemePort{}, "tenant-uuid", discardLog())

	if got := m["isDefault"]; got != true {
		t.Fatalf("isDefault = %v, want true", got)
	}
	tokens, ok := m["tokens"].(map[string]interface{})
	if !ok {
		t.Fatalf("tokens missing or wrong type: %#v", m["tokens"])
	}
	colors, _ := tokens["colors"].(map[string]interface{})
	if colors["blue"] != "#5BA4D9" {
		t.Errorf("colors.blue = %v, want #5BA4D9", colors["blue"])
	}
	if colors["text"] != "#1a1a1a" {
		t.Errorf("colors.text = %v, want #1a1a1a", colors["text"])
	}
	// JSON numbers decode to float64 — assert the radius default 8.
	radius, _ := tokens["radius"].(map[string]interface{})
	if radius["base"] != float64(8) {
		t.Errorf("radius.base = %v, want 8", radius["base"])
	}
}

// TestResolveThemeMap_FailOpen asserts an unknown tenant (port error) does NOT
// break render — the helper falls back to default tokens (isDefault:true).
func TestResolveThemeMap_FailOpen(t *testing.T) {
	m := resolveThemeMap(context.Background(), &fakeThemePort{notFound: true}, "ghost", discardLog())
	if m["isDefault"] != true {
		t.Fatalf("fail-open did not yield defaults: %#v", m)
	}
}

// TestResolveThemeMap_NilPort asserts a nil port (optional dependency) attaches
// defaults rather than panicking.
func TestResolveThemeMap_NilPort(t *testing.T) {
	m := resolveThemeMap(context.Background(), nil, "tenant-uuid", discardLog())
	if m["isDefault"] != true {
		t.Fatalf("nil port did not yield defaults: %#v", m)
	}
}

// TestAttachThemeToDocument asserts the assembled Document carries doc.theme —
// the exact key the frontend shadow-root injector reads (resp.document.theme).
func TestAttachThemeToDocument(t *testing.T) {
	doc := map[string]interface{}{"version": "1", "children": []interface{}{}}
	tenant := &domain.Tenant{ID: "11111111-1111-1111-1111-111111111111", Slug: "acme"}

	attachThemeToDocument(context.Background(), doc, &fakeThemePort{}, tenant, discardLog())

	theme, ok := doc["theme"].(map[string]interface{})
	if !ok {
		t.Fatalf("doc.theme missing or wrong type: %#v", doc["theme"])
	}
	if theme["isDefault"] != true {
		t.Errorf("doc.theme.isDefault = %v, want true", theme["isDefault"])
	}
	if _, ok := theme["tokens"]; !ok {
		t.Errorf("doc.theme.tokens missing: %#v", theme)
	}
	// Original document keys are untouched (theme is an additive passenger).
	if doc["version"] != "1" {
		t.Errorf("attach mutated unrelated keys: version=%v", doc["version"])
	}
}

// TestAttachThemeToDocument_OverrideWins asserts a tenant override flows onto
// doc.theme with isDefault:false and the overridden value present.
func TestAttachThemeToDocument_OverrideWins(t *testing.T) {
	doc := map[string]interface{}{"version": "1"}
	port := &fakeThemePort{override: &domain.ThemeTokens{Colors: map[string]string{"blue": "#0A0B0C"}}}
	tenant := &domain.Tenant{ID: "t-id", Slug: "acme"}

	attachThemeToDocument(context.Background(), doc, port, tenant, discardLog())

	theme := doc["theme"].(map[string]interface{})
	if theme["isDefault"] != false {
		t.Errorf("override doc.theme.isDefault = %v, want false", theme["isDefault"])
	}
	tokens := theme["tokens"].(map[string]interface{})
	colors := tokens["colors"].(map[string]interface{})
	if colors["blue"] != "#0A0B0C" {
		t.Errorf("override color not on doc.theme: %v", colors["blue"])
	}
	if colors["orange"] != "#F0924A" {
		t.Errorf("merged default color missing on doc.theme: %v", colors["orange"])
	}
}

// --- admin GET/PUT theme endpoints ---

func TestThemeHandler_GetDefaults(t *testing.T) {
	t.Setenv("V5_INTERNAL_KEY", "secret")
	h := NewThemeHandler(&fakeThemePort{}, discardLog())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/internal/theme?tenant=acme", nil)
	req.Header.Set("X-Internal-Key", "secret")
	rec := httptest.NewRecorder()
	h.Theme(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Theme domain.Theme `json:"theme"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Theme.IsDefault {
		t.Errorf("GET no-row theme.isDefault = false, want true")
	}
	if body.Theme.Tokens.Colors["blue"] != "#5BA4D9" {
		t.Errorf("GET default colors.blue = %q", body.Theme.Tokens.Colors["blue"])
	}
}

// TestThemeHandler_PutRoundTrip asserts an upserted override survives a
// round-trip: PUT stores it, the PUT response reflects the merged result, and
// a subsequent GET returns isDefault:false with the overridden value.
func TestThemeHandler_PutRoundTrip(t *testing.T) {
	t.Setenv("V5_INTERNAL_KEY", "secret")
	port := &fakeThemePort{}
	h := NewThemeHandler(port, discardLog())

	putBody := `{"tokens":{"colors":{"blue":"#123456"},"gap":{"md":20}}}`
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/internal/theme?tenant=acme", strings.NewReader(putBody))
	putReq.Header.Set("X-Internal-Key", "secret")
	putRec := httptest.NewRecorder()
	h.Theme(putRec, putReq)

	if putRec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200; body=%s", putRec.Code, putRec.Body.String())
	}
	// The store recorded the override exactly as sent.
	if port.lastUpsert == nil || port.lastUpsert.Colors["blue"] != "#123456" || port.lastUpsert.Gap["md"] != 20 {
		t.Fatalf("upsert did not persist override: %#v", port.lastUpsert)
	}

	var putBodyResp struct {
		Theme domain.Theme `json:"theme"`
	}
	if err := json.Unmarshal(putRec.Body.Bytes(), &putBodyResp); err != nil {
		t.Fatalf("decode PUT resp: %v", err)
	}
	if putBodyResp.Theme.IsDefault {
		t.Errorf("PUT resp theme.isDefault = true, want false")
	}
	if putBodyResp.Theme.Tokens.Colors["blue"] != "#123456" {
		t.Errorf("PUT resp colors.blue = %q, want #123456", putBodyResp.Theme.Tokens.Colors["blue"])
	}
	if putBodyResp.Theme.Tokens.Gap["md"] != 20 {
		t.Errorf("PUT resp gap.md = %d, want 20", putBodyResp.Theme.Tokens.Gap["md"])
	}
	// Non-overridden keys still merged from defaults.
	if putBodyResp.Theme.Tokens.Colors["orange"] != "#F0924A" {
		t.Errorf("PUT resp lost default colors.orange = %q", putBodyResp.Theme.Tokens.Colors["orange"])
	}

	// GET after PUT reflects the override.
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/internal/theme?tenant=acme", nil)
	getReq.Header.Set("X-Internal-Key", "secret")
	getRec := httptest.NewRecorder()
	h.Theme(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", getRec.Code)
	}
	var getBody struct {
		Theme domain.Theme `json:"theme"`
	}
	_ = json.Unmarshal(getRec.Body.Bytes(), &getBody)
	if getBody.Theme.IsDefault || getBody.Theme.Tokens.Colors["blue"] != "#123456" {
		t.Errorf("GET after PUT did not reflect override: isDefault=%v blue=%q",
			getBody.Theme.IsDefault, getBody.Theme.Tokens.Colors["blue"])
	}
}

func TestThemeHandler_AuthGate(t *testing.T) {
	t.Setenv("V5_INTERNAL_KEY", "secret")
	h := NewThemeHandler(&fakeThemePort{}, discardLog())

	// Wrong key → 403.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/internal/theme?tenant=acme", nil)
	req.Header.Set("X-Internal-Key", "wrong")
	rec := httptest.NewRecorder()
	h.Theme(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("wrong key status = %d, want 403", rec.Code)
	}

	// Missing tenant param → 400.
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/internal/theme", nil)
	req2.Header.Set("X-Internal-Key", "secret")
	rec2 := httptest.NewRecorder()
	h.Theme(rec2, req2)
	if rec2.Code != http.StatusBadRequest {
		t.Errorf("missing tenant status = %d, want 400", rec2.Code)
	}
}
