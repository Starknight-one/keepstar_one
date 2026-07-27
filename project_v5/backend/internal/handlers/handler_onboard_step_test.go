package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
	"keepstar_v5/internal/usecases"
)

// --- onboarding fakes shared by the step-submit and upload tests ---

// onboardStateFake implements ports.OnboardingStatePort, recording every
// persisted byte so the R6 assertion can grep the WHOLE persistence surface.
type onboardStateFake struct {
	manifest  *domain.OnboardingManifest
	snapshots [][]byte
	deltas    []domain.DeltaInfo
}

func (s *onboardStateFake) GetOnboarding(context.Context, string) (*domain.OnboardingManifest, error) {
	if s.manifest == nil {
		return nil, nil
	}
	raw, _ := json.Marshal(s.manifest)
	var m domain.OnboardingManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *onboardStateFake) UpdateOnboarding(_ context.Context, _ string, m *domain.OnboardingManifest, info domain.DeltaInfo) (int, error) {
	raw, err := json.Marshal(m)
	if err != nil {
		return 0, err
	}
	s.snapshots = append(s.snapshots, raw)
	var cp domain.OnboardingManifest
	if err := json.Unmarshal(raw, &cp); err != nil {
		return 0, err
	}
	s.manifest = &cp
	s.deltas = append(s.deltas, info)
	return len(s.snapshots), nil
}

func (s *onboardStateFake) persistedJSON(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	for _, snap := range s.snapshots {
		b.Write(snap)
	}
	raw, err := json.Marshal(s.deltas)
	if err != nil {
		t.Fatalf("marshal deltas: %v", err)
	}
	b.Write(raw)
	return b.String()
}

// onboardGatewayFake implements ports.AdminGatewayPort.
type onboardGatewayFake struct {
	seenPassword string
	provisionErr error
	createErr    error
	importedBody string
	importedFile string
	importSlug   string
	statusResp   *ports.AdminImportStatus
}

func (g *onboardGatewayFake) CreateTenant(context.Context, string, string) (*ports.AdminTenant, error) {
	if g.createErr != nil {
		return nil, g.createErr
	}
	return &ports.AdminTenant{TenantID: "t-1", Slug: "acme-realty"}, nil
}
func (g *onboardGatewayFake) ProvisionUser(_ context.Context, _, _, password, _ string) (string, error) {
	if g.provisionErr != nil {
		return "", g.provisionErr
	}
	g.seenPassword = password
	return "user-7", nil
}
func (g *onboardGatewayFake) AdoptPresets(context.Context, string, []string) (*ports.AdminAdoptResult, error) {
	return &ports.AdminAdoptResult{}, nil
}
func (g *onboardGatewayFake) StartImport(_ context.Context, slug, fileName string, file io.Reader) (*ports.AdminImportJob, error) {
	raw, _ := io.ReadAll(file)
	g.importedBody = string(raw)
	g.importedFile = fileName
	g.importSlug = slug
	return &ports.AdminImportJob{JobID: "job-1", Status: "pending", TotalItems: 2}, nil
}
func (g *onboardGatewayFake) ImportStatus(context.Context, string, string) (*ports.AdminImportStatus, error) {
	if g.statusResp == nil {
		return nil, errors.New("no status stubbed")
	}
	return g.statusResp, nil
}

// onboardTokensFake implements ports.OnboardingTokenPort.
type onboardTokensFake struct {
	token *ports.IngestToken
	used  []string
}

func (f *onboardTokensFake) MintIngestToken(_ context.Context, sessionID, tenantID string, formats []string, _ time.Duration) (*ports.IngestToken, error) {
	return &ports.IngestToken{Token: "minted", SessionID: sessionID, TenantID: tenantID,
		Formats: formats, ExpiresAt: time.Now().Add(time.Hour)}, nil
}
func (f *onboardTokensFake) GetIngestToken(_ context.Context, token string) (*ports.IngestToken, error) {
	if f.token == nil || f.token.Token != token {
		return nil, errors.New("not found")
	}
	return f.token, nil
}
func (f *onboardTokensFake) MarkIngestTokenUsed(_ context.Context, token string) error {
	f.used = append(f.used, token)
	return nil
}
func (f *onboardTokensFake) MintSurfaceToken(context.Context, string) (string, error) {
	return "surf-1", nil
}

// onboardThemesFake implements ports.ThemePort — the applier's create_tenant
// step writes the brand-default theme in-process (R22).
type onboardThemesFake struct{}

func (onboardThemesFake) GetTheme(context.Context, string) (domain.Theme, error) {
	return domain.DefaultTheme(), nil
}
func (onboardThemesFake) UpsertTheme(context.Context, string, domain.ThemeTokens) error { return nil }

// registeredManifest is a manifest mid-beat-3: tenant provisioned, the
// register step accepted (rendered form), the ingest door armed.
func registeredManifest() *domain.OnboardingManifest {
	return &domain.OnboardingManifest{
		Version: 1,
		Tenant:  domain.ManifestTenant{ID: "t-1", Slug: "acme-realty", Name: "Acme Realty"},
		Steps: []domain.ManifestStep{
			{ID: "s-tenant", Op: "create_tenant", Status: domain.ManifestStepApplied},
			{ID: "s-door", Op: "issue_ingest_door", Status: domain.ManifestStepAccepted,
				Result: map[string]any{"token": "door-token-1", "formats": []any{"csv", "json"}}},
			{ID: "s-reg", Op: "register_user", Status: domain.ManifestStepAccepted,
				Params: map[string]any{"role": "owner"}},
		},
	}
}

// proposedManifest is a manifest as the model staged it mid-beat-2: nothing
// applied, nothing accepted. The USER's next action (registration submit or
// file upload) is the approval and must auto-apply it (owner 2026-07-28) —
// never a 409 "apply the manifest first".
func proposedManifest() *domain.OnboardingManifest {
	return &domain.OnboardingManifest{
		Version: 1,
		Steps: []domain.ManifestStep{
			{ID: "s-tenant", Op: "create_tenant", Status: domain.ManifestStepProposed,
				Params: map[string]any{"name": "Acme Realty", "vertical": "real estate agency"}},
			{ID: "s-door", Op: "issue_ingest_door", Status: domain.ManifestStepProposed,
				Params: map[string]any{"formats": []any{"csv", "json"}}},
			{ID: "s-reg", Op: "register_user", Status: domain.ManifestStepProposed,
				Params: map[string]any{"role": "owner"}},
		},
	}
}

// newOnboardTestHandler builds an OnboardHandler with a REAL ManifestApplier
// over the fakes — the step-submit and upload paths run end to end minus
// the DB and admin HTTP.
func newOnboardTestHandler(state *onboardStateFake, gw *onboardGatewayFake, tokens *onboardTokensFake) *OnboardHandler {
	h := NewOnboardHandler(nil, nil, nil, onboardTestLog())
	applier := usecases.NewManifestApplier(usecases.ManifestApplierConfig{
		State:          state,
		Tokens:         tokens,
		Gateway:        gw,
		Themes:         onboardThemesFake{},
		SurfaceBaseURL: "https://v5.example.test",
		Log:            onboardTestLog(),
	})
	h.SetOnboardingDeps(OnboardDeps{
		OnboardState: state,
		Tokens:       tokens,
		Gateway:      gw,
		Applier:      applier,
	})
	return h
}

func stepSubmitRequest(t *testing.T, stepID, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/onboard/step/"+stepID+"/submit", strings.NewReader(body))
	req.SetPathValue("stepId", stepID)
	req.AddCookie(&http.Cookie{Name: onboardCookieName, Value: expectedOnboardCookie("pw")})
	return req
}

// THE R6 transport-level test (REQUIRED by the work order): the password
// submitted to the step-submit endpoint reaches the admin gateway and is
// NEVER persisted — no manifest snapshot, no delta carries the value or a
// "password" key.
func TestStepSubmit_PasswordChoke(t *testing.T) {
	const password = "transport-secret-PW#1"
	t.Setenv("ONBOARDING_PASSWORD", "pw")
	state := &onboardStateFake{manifest: registeredManifest()}
	gw := &onboardGatewayFake{}
	h := newOnboardTestHandler(state, gw, &onboardTokensFake{})

	rec := httptest.NewRecorder()
	h.StepSubmit(rec, stepSubmitRequest(t, "s-reg",
		`{"sessionId":"sess-1","name":"Olga","email":"o@acme.test","password":"`+password+`","formId":"f1"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body %q", rec.Code, rec.Body.String())
	}

	var resp struct {
		Status string `json:"status"`
		Step   struct {
			Status string `json:"status"`
		} `json:"step"`
		Apply []map[string]any `json:"apply"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "ok" || resp.Step.Status != domain.ManifestStepApplied {
		t.Fatalf("response = %+v", resp)
	}
	// No presets wired in the test handler → plaque render degrades to the
	// form ack (documented fallback).
	if len(resp.Apply) != 1 || resp.Apply[0]["target"] != "form" || resp.Apply[0]["status"] != "ok" {
		t.Fatalf("apply = %v", resp.Apply)
	}

	if gw.seenPassword != password {
		t.Fatalf("gateway did not receive the password — the ONE allowed path is broken")
	}
	persisted := state.persistedJSON(t)
	if strings.Contains(persisted, password) {
		t.Fatalf("PASSWORD LEAKED into persisted state (R6 violation)")
	}
	if strings.Contains(persisted, `"password"`) {
		t.Fatalf("a 'password' key reached persisted state (R6 violation)")
	}
	// The response wire must not echo the password either.
	if strings.Contains(rec.Body.String(), password) {
		t.Fatalf("PASSWORD echoed on the response wire (R6 violation)")
	}
}

// Submit error surface: cookie gate, missing fields, unknown step, and the
// retryable provisioning failure (200 {status:error} + form apply, step
// stays accepted, password still never persisted).
func TestStepSubmitErrors(t *testing.T) {
	t.Setenv("ONBOARDING_PASSWORD", "pw")

	// No cookie → 403 before anything else.
	h := newOnboardTestHandler(&onboardStateFake{manifest: registeredManifest()}, &onboardGatewayFake{}, &onboardTokensFake{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/onboard/step/s-reg/submit", strings.NewReader(`{}`))
	req.SetPathValue("stepId", "s-reg")
	rec := httptest.NewRecorder()
	h.StepSubmit(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("no cookie: status = %d, want 403", rec.Code)
	}

	// Missing fields → 400.
	rec = httptest.NewRecorder()
	h.StepSubmit(rec, stepSubmitRequest(t, "s-reg", `{"sessionId":"sess-1","email":"o@a.t"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing password: status = %d, want 400", rec.Code)
	}

	// Unknown step → 404.
	rec = httptest.NewRecorder()
	h.StepSubmit(rec, stepSubmitRequest(t, "nope", `{"sessionId":"sess-1","email":"o@a.t","password":"x"}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown step: status = %d, want 404", rec.Code)
	}

	// Non-register step → 400.
	rec = httptest.NewRecorder()
	h.StepSubmit(rec, stepSubmitRequest(t, "s-door", `{"sessionId":"sess-1","email":"o@a.t","password":"x"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("non-register step: status = %d, want 400", rec.Code)
	}

	// Gateway failure → 200 {status:error} with a form apply; step stays
	// accepted; no password persisted.
	const password = "fail-path-secret"
	state := &onboardStateFake{manifest: registeredManifest()}
	gw := &onboardGatewayFake{provisionErr: errors.New("admin responded 500")}
	h = newOnboardTestHandler(state, gw, &onboardTokensFake{})
	rec = httptest.NewRecorder()
	h.StepSubmit(rec, stepSubmitRequest(t, "s-reg",
		`{"sessionId":"sess-1","email":"o@a.t","password":"`+password+`","formId":"f1"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("gateway failure: status = %d, want 200", rec.Code)
	}
	var resp struct {
		Status string           `json:"status"`
		Apply  []map[string]any `json:"apply"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Status != "error" || len(resp.Apply) != 1 || resp.Apply[0]["target"] != "form" || resp.Apply[0]["status"] != "error" {
		t.Fatalf("failure response = %v", resp)
	}
	m, _ := state.GetOnboarding(context.Background(), "sess-1")
	if m.Steps[2].Status != domain.ManifestStepAccepted {
		t.Fatalf("register step after failure = %q, want accepted", m.Steps[2].Status)
	}
	if strings.Contains(state.persistedJSON(t), password) {
		t.Fatalf("PASSWORD LEAKED on the failure path (R6 violation)")
	}
}

// The live 2026-07-28 bug, transport level: the model SAID "applying now"
// but never called apply_manifest, so the manifest was still fully
// proposed — the form submit then 409'd "step not ready: apply the
// manifest first". The submit is the approval: the server auto-applies and
// registers, 200. Addressed by op alias, exactly as the seeded
// registration_form posts it.
func TestStepSubmit_AutoAppliesProposedManifest(t *testing.T) {
	const password = "auto-apply-transport-pw"
	t.Setenv("ONBOARDING_PASSWORD", "pw")
	state := &onboardStateFake{manifest: proposedManifest()}
	gw := &onboardGatewayFake{}
	h := newOnboardTestHandler(state, gw, &onboardTokensFake{})

	rec := httptest.NewRecorder()
	h.StepSubmit(rec, stepSubmitRequest(t, "register_user",
		`{"sessionId":"sess-1","email":"o@acme.test","password":"`+password+`","formId":"f1"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body %q — the auto-apply must prevent this 409", rec.Code, rec.Body.String())
	}
	var resp struct {
		Status string `json:"status"`
		Step   struct {
			Status string `json:"status"`
		} `json:"step"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != "ok" || resp.Step.Status != domain.ManifestStepApplied {
		t.Fatalf("response = %+v, want ok/applied", resp)
	}
	if gw.seenPassword != password {
		t.Fatalf("gateway did not receive the password")
	}

	// The plan actually ran: tenant provisioned, door armed.
	m, _ := state.GetOnboarding(context.Background(), "sess-1")
	if m.Tenant.Slug != "acme-realty" {
		t.Fatalf("tenant not provisioned by the auto-apply: %+v", m.Tenant)
	}
	if tok, _ := m.Steps[1].Result["token"].(string); tok == "" {
		t.Fatalf("ingest door not armed by the auto-apply: %+v", m.Steps[1])
	}
	// R6 discipline holds on this path too.
	if strings.Contains(state.persistedJSON(t), password) {
		t.Fatalf("PASSWORD LEAKED on the auto-apply path (R6 violation)")
	}
}

// A halted auto-apply still 409s — but the body must name the FAILED step
// and its reason (owner 2026-07-28), never a bare "apply the manifest
// first".
func TestStepSubmit_HaltedAutoApply409CarriesReason(t *testing.T) {
	t.Setenv("ONBOARDING_PASSWORD", "pw")
	state := &onboardStateFake{manifest: proposedManifest()}
	gw := &onboardGatewayFake{createErr: errors.New("admin unreachable")}
	h := newOnboardTestHandler(state, gw, &onboardTokensFake{})

	rec := httptest.NewRecorder()
	h.StepSubmit(rec, stepSubmitRequest(t, "register_user",
		`{"sessionId":"sess-1","email":"o@acme.test","password":"pw1"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "create_tenant failed: admin unreachable") {
		t.Fatalf("409 body does not carry the failed step's reason: %q", rec.Body.String())
	}
}
