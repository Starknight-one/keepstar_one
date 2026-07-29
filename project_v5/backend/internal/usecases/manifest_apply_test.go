package usecases

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// --- fakes (applier-prefixed: the package's other fakes serve other suites) ---

// applierStatePort records every persisted manifest snapshot AND every delta
// so tests can assert the FULL persistence surface (the R6 test greps it).
// Mutex-guarded so the concurrency regression test can hammer it under -race.
type applierStatePort struct {
	mu        sync.Mutex
	manifest  *domain.OnboardingManifest
	snapshots [][]byte // marshaled manifest at each UpdateOnboarding
	deltas    []domain.DeltaInfo
	failWrite bool
}

func (s *applierStatePort) GetOnboarding(_ context.Context, _ string) (*domain.OnboardingManifest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.manifest == nil {
		return nil, nil
	}
	// Round-trip like the DB does so tests catch anything non-serializable.
	raw, err := json.Marshal(s.manifest)
	if err != nil {
		return nil, err
	}
	var m domain.OnboardingManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (s *applierStatePort) UpdateOnboarding(_ context.Context, _ string, m *domain.OnboardingManifest, info domain.DeltaInfo) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failWrite {
		return 0, errors.New("state write refused")
	}
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

// persistedJSON is everything the fake ever wrote: manifest snapshots +
// deltas — the surface the R6 password assertion scans.
func (s *applierStatePort) persistedJSON(t *testing.T) string {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
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

type applierTokens struct {
	ingestMints  int
	surfaceMints int
	usedTokens   []string
	// minted records every ingest token by value so GetIngestToken behaves
	// like the store; tests may pre-seed or mutate entries (expire, consume).
	minted map[string]*ports.IngestToken
}

func (f *applierTokens) MintIngestToken(_ context.Context, sessionID, tenantID string, formats []string, _ time.Duration) (*ports.IngestToken, error) {
	f.ingestMints++
	t := &ports.IngestToken{
		Token: fmt.Sprintf("ingest-token-%d", f.ingestMints), SessionID: sessionID,
		TenantID: tenantID, Formats: formats, ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if f.minted == nil {
		f.minted = map[string]*ports.IngestToken{}
	}
	f.minted[t.Token] = t
	return t, nil
}
func (f *applierTokens) GetIngestToken(_ context.Context, token string) (*ports.IngestToken, error) {
	t, ok := f.minted[token]
	if !ok {
		return nil, errors.New("token not found")
	}
	return t, nil
}
func (f *applierTokens) MarkIngestTokenUsed(_ context.Context, token string) error {
	f.usedTokens = append(f.usedTokens, token)
	return nil
}
func (f *applierTokens) MintSurfaceToken(context.Context, string) (string, error) {
	f.surfaceMints++
	return fmt.Sprintf("surface-token-%d", f.surfaceMints), nil
}

type applierGateway struct {
	mu             sync.Mutex
	createCalls    int
	provisionCalls int
	adoptCalls     int
	// seenPassword records what crossed into ProvisionUser — the ONE place
	// the password is allowed to reach (R6).
	seenPassword string
	provisionErr error
	createErr    error
}

func (g *applierGateway) CreateTenant(_ context.Context, name, _ string) (*ports.AdminTenant, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.createCalls++
	if g.createErr != nil {
		return nil, g.createErr
	}
	return &ports.AdminTenant{TenantID: "11111111-2222-3333-4444-555555555555", Slug: "acme-realty"}, nil
}
func (g *applierGateway) ProvisionUser(_ context.Context, _, _, password, _ string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.provisionCalls++
	if g.provisionErr != nil {
		return "", g.provisionErr
	}
	g.seenPassword = password
	return "user-42", nil
}
func (g *applierGateway) AdoptPresets(_ context.Context, _ string, names []string) (*ports.AdminAdoptResult, error) {
	g.adoptCalls++
	return &ports.AdminAdoptResult{Adopted: names, BindingReports: []map[string]any{}, Invalidated: true}, nil
}
func (g *applierGateway) StartImport(context.Context, string, string, io.Reader) (*ports.AdminImportJob, error) {
	return nil, errors.New("not implemented")
}
func (g *applierGateway) ImportStatus(context.Context, string, string) (*ports.AdminImportStatus, error) {
	return nil, errors.New("not implemented")
}

// applierEntities implements ports.EntityPort with call recording and
// injectable failure on UpsertValueSet (halt-and-retry test).
type applierEntities struct {
	calls       []string
	valueSetErr error
}

func (e *applierEntities) UpsertEntityDefinition(_ context.Context, def *domain.EntityDefinition) error {
	e.calls = append(e.calls, "entity:"+def.Slug+":"+def.TenantID)
	return nil
}
func (e *applierEntities) GetEntityDefinition(context.Context, string, string) (*domain.EntityDefinition, error) {
	return nil, errors.New("not implemented")
}
func (e *applierEntities) ListEntityDefinitions(context.Context, string) ([]domain.EntityDefinition, error) {
	return nil, nil
}
func (e *applierEntities) UpsertValueSet(_ context.Context, vs *domain.ValueSet) error {
	if e.valueSetErr != nil {
		return e.valueSetErr
	}
	e.calls = append(e.calls, "valueset:"+vs.Slug)
	return nil
}
func (e *applierEntities) GetValueSet(context.Context, string, string) (*domain.ValueSet, error) {
	return nil, errors.New("not implemented")
}
func (e *applierEntities) ListValueSets(context.Context, string) ([]domain.ValueSet, error) {
	return nil, nil
}
func (e *applierEntities) CreateRecord(context.Context, *domain.EntityRecord) (*domain.EntityRecord, *domain.RuntimeEvent, error) {
	return nil, nil, errors.New("not implemented")
}
func (e *applierEntities) UpdateRecord(context.Context, string, string, map[string]any) (*domain.EntityRecord, *domain.RuntimeEvent, error) {
	return nil, nil, errors.New("not implemented")
}
func (e *applierEntities) TransitionStatus(context.Context, string, string, string) (*domain.EntityRecord, *domain.RuntimeEvent, error) {
	return nil, nil, errors.New("not implemented")
}
func (e *applierEntities) GetRecord(context.Context, string, string) (*domain.EntityRecord, error) {
	return nil, errors.New("not implemented")
}
func (e *applierEntities) QueryRecords(context.Context, string, string, domain.RecordFilter) ([]domain.EntityRecord, int, error) {
	return nil, 0, errors.New("not implemented")
}
func (e *applierEntities) UpsertAutomation(_ context.Context, a *domain.Automation) error {
	e.calls = append(e.calls, "automation:"+a.Name)
	return nil
}
func (e *applierEntities) ListAutomations(context.Context, string, string) ([]domain.Automation, error) {
	return nil, nil
}
func (e *applierEntities) MarkEventProcessed(context.Context, int64) error { return nil }

type applierOps struct{ enabled []string }

func (o *applierOps) SearchLibrary(context.Context, []float32, string, []string, int) ([]domain.OperationTemplate, error) {
	return nil, nil
}
func (o *applierOps) EnableOperation(_ context.Context, tenantID, template, instance string, _ map[string]any) (*domain.TenantOperation, error) {
	o.enabled = append(o.enabled, template+"/"+instance)
	return &domain.TenantOperation{TenantID: tenantID, Name: instance}, nil
}
func (o *applierOps) UpdateInstanceConfig(context.Context, string, string, map[string]any) error {
	return nil
}
func (o *applierOps) DisableOperation(context.Context, string, string) error { return nil }
func (o *applierOps) ListEnabled(context.Context, string) ([]domain.TenantOperation, error) {
	return nil, nil
}

type applierThemes struct{ upserts []string }

func (t *applierThemes) GetTheme(context.Context, string) (domain.Theme, error) {
	return domain.DefaultTheme(), nil
}
func (t *applierThemes) UpsertTheme(_ context.Context, slugOrID string, _ domain.ThemeTokens) error {
	t.upserts = append(t.upserts, slugOrID)
	return nil
}

type applierRegistry struct{ invalidated []string }

func (r *applierRegistry) RegisterExecutor(domain.OperationKind, ports.Executor) {}
func (r *applierRegistry) DefinitionsFor(context.Context, string, domain.PipelineMode, domain.AgentPlane, domain.Role) []domain.ToolDefinition {
	return nil
}
func (r *applierRegistry) Get(context.Context, string, string) (*domain.OperationSpec, error) {
	return nil, errors.New("not implemented")
}
func (r *applierRegistry) Execute(context.Context, domain.OperationContext, domain.ToolCall) (*domain.OperationResult, error) {
	return nil, errors.New("not implemented")
}
func (r *applierRegistry) InvalidateTenant(slug string) { r.invalidated = append(r.invalidated, slug) }

// --- helpers ---

type applierFixture struct {
	state    *applierStatePort
	tokens   *applierTokens
	gateway  *applierGateway
	entities *applierEntities
	ops      *applierOps
	themes   *applierThemes
	registry *applierRegistry
	ap       *ManifestApplier
}

func newApplierFixture(m *domain.OnboardingManifest) *applierFixture {
	f := &applierFixture{
		state:    &applierStatePort{manifest: m},
		tokens:   &applierTokens{},
		gateway:  &applierGateway{},
		entities: &applierEntities{},
		ops:      &applierOps{},
		themes:   &applierThemes{},
		registry: &applierRegistry{},
	}
	f.ap = NewManifestApplier(ManifestApplierConfig{
		State:          f.state,
		Tokens:         f.tokens,
		Gateway:        f.gateway,
		Entities:       f.entities,
		Ops:            f.ops,
		Themes:         f.themes,
		Registry:       f.registry,
		SurfaceBaseURL: "https://v5.example.test",
		Log:            slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return f
}

func step(id, op string, params map[string]any) domain.ManifestStep {
	return domain.ManifestStep{ID: id, Op: op, Params: params, Status: domain.ManifestStepProposed}
}

func stepByID(t *testing.T, m *domain.OnboardingManifest, id string) *domain.ManifestStep {
	t.Helper()
	for i := range m.Steps {
		if m.Steps[i].ID == id {
			return &m.Steps[i]
		}
	}
	t.Fatalf("step %q not found", id)
	return nil
}

func realtorManifest() *domain.OnboardingManifest {
	return &domain.OnboardingManifest{
		Version: 1,
		Steps: []domain.ManifestStep{
			// Deliberately staged with create_tenant NOT first — the applier
			// must force it first regardless of LLM emission order.
			step("s-entity", "define_entity", map[string]any{
				"entity": map[string]any{
					"slug": "lead", "name": "Lead",
					"fields": []any{map[string]any{"key": "name", "label": "Name", "type": "text"}},
				},
			}),
			step("s-tenant", "create_tenant", map[string]any{"name": "Acme Realty", "vertical": "real estate agency"}),
			step("s-vset", "define_value_set", map[string]any{
				"slug": "lead_pipeline", "name": "Lead pipeline",
				"values": []any{map[string]any{"value": "new", "label": "New"}},
			}),
			step("s-ops", "enable_operations", map[string]any{
				"operations": []any{map[string]any{"template": "schedule_slot", "instance": "book_showing", "config": map[string]any{"entity": "lead"}}},
			}),
			step("s-door", "issue_ingest_door", map[string]any{"formats": []any{"csv", "json"}}),
			step("s-reg", "register_user", map[string]any{"role": "owner"}),
			step("s-urls", "issue_surface_urls", map[string]any{}),
		},
	}
}

// --- tests ---

// The applier must run create_tenant FIRST (R22: stage order with
// create_tenant forced first) even when the LLM staged it later, and the
// tenant identity must reach every subsequent in-process write.
func TestApplyForcesCreateTenantFirst(t *testing.T) {
	f := newApplierFixture(realtorManifest())
	m, err := f.ap.Apply(context.Background(), "sess-1", "")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if f.gateway.createCalls != 1 {
		t.Fatalf("create tenant calls = %d, want 1", f.gateway.createCalls)
	}
	if m.Tenant.Slug != "acme-realty" || m.Tenant.ID == "" {
		t.Fatalf("tenant identity not recorded: %+v", m.Tenant)
	}
	// define_entity ran with the created tenant id — proof of ordering.
	if len(f.entities.calls) == 0 || f.entities.calls[0] != "entity:lead:"+m.Tenant.ID {
		t.Fatalf("entity write missing tenant id (ordering broken): %v", f.entities.calls)
	}
	// R22: brand-default theme written in-process at create_tenant time.
	if len(f.themes.upserts) != 1 || f.themes.upserts[0] != m.Tenant.ID {
		t.Fatalf("brand-default theme not written for tenant: %v", f.themes.upserts)
	}
	// Registry spec cache dropped after tenant mutations (§6.1).
	if len(f.registry.invalidated) == 0 {
		t.Errorf("registry.InvalidateTenant never called after mutations")
	}
}

// A failed step halts the run (steps after it stay un-applied) and a retry
// re-runs FROM it without repeating earlier side effects (idempotency:
// create_tenant must not mint a second tenant).
func TestApplyHaltsOnFailureAndRetryResumes(t *testing.T) {
	f := newApplierFixture(realtorManifest())
	f.entities.valueSetErr = errors.New("db exploded")

	m, err := f.ap.Apply(context.Background(), "sess-1", "")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := stepByID(t, m, "s-vset").Status; got != domain.ManifestStepFailed {
		t.Fatalf("value-set step status = %q, want failed", got)
	}
	if got := stepByID(t, m, "s-ops").Status; got != domain.ManifestStepAccepted {
		t.Fatalf("post-failure step status = %q, want accepted (halt means NOT applied)", got)
	}
	if len(f.ops.enabled) != 0 {
		t.Fatalf("enable_operations ran past a halted run: %v", f.ops.enabled)
	}

	// Retry: the failure is gone; the run resumes from the failed step.
	f.entities.valueSetErr = nil
	m, err = f.ap.Apply(context.Background(), "sess-1", "")
	if err != nil {
		t.Fatalf("retry Apply: %v", err)
	}
	if got := stepByID(t, m, "s-vset").Status; got != domain.ManifestStepApplied {
		t.Fatalf("value-set step after retry = %q, want applied", got)
	}
	if f.gateway.createCalls != 1 {
		t.Fatalf("create tenant called %d times across retry, want 1 (idempotency guard)", f.gateway.createCalls)
	}
	if vsStep := stepByID(t, m, "s-vset"); vsStep.Attempt != 2 {
		t.Errorf("failed step attempt = %d, want 2", vsStep.Attempt)
	}
}

// issue_surface_urls refuses (rests at accepted, run does NOT halt) until
// every other step applied; once they are, it mints the CRM token and both
// URLs.
func TestApplySurfaceURLsWaitThenIssue(t *testing.T) {
	f := newApplierFixture(realtorManifest())
	m, err := f.ap.Apply(context.Background(), "sess-1", "")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	urls := stepByID(t, m, "s-urls")
	if urls.Status != domain.ManifestStepAccepted {
		t.Fatalf("surface step status = %q, want accepted (waiting)", urls.Status)
	}
	if _, ok := urls.Result["waiting"]; !ok {
		t.Fatalf("waiting reason missing: %v", urls.Result)
	}
	if f.tokens.surfaceMints != 0 {
		t.Fatalf("surface token minted while steps pending")
	}

	// Complete the two trigger steps out of band.
	if _, err := f.ap.ExecuteStep(context.Background(), "sess-1", "s-reg", map[string]any{
		"email": "owner@acme.test", "password": "pw",
	}); err != nil {
		t.Fatalf("ExecuteStep: %v", err)
	}
	if err := f.ap.CompleteIngestStep(context.Background(), "sess-1", &ports.AdminImportStatus{
		Status: "completed", Processed: 20, ProjectionRows: 20, Invalidated: true,
	}); err != nil {
		t.Fatalf("CompleteIngestStep: %v", err)
	}

	m, err = f.ap.Apply(context.Background(), "sess-1", "")
	if err != nil {
		t.Fatalf("final Apply: %v", err)
	}
	urls = stepByID(t, m, "s-urls")
	if urls.Status != domain.ManifestStepApplied {
		t.Fatalf("surface step = %q, want applied; result %v", urls.Status, urls.Result)
	}
	sf, _ := urls.Result["storefrontUrl"].(string)
	crm, _ := urls.Result["crmUrl"].(string)
	if sf != "https://v5.example.test/s/acme-realty" {
		t.Errorf("storefront url = %q", sf)
	}
	if !strings.HasPrefix(crm, "https://v5.example.test/crm/acme-realty?k=surface-token-") {
		t.Errorf("crm url = %q", crm)
	}
}

// THE R6 test (REQUIRED by the work order): the registration password
// reaches the admin gateway and NOTHING ELSE — not one persisted manifest
// snapshot, not one delta. The assertion greps the full persistence surface.
func TestExecuteStepRegisterUser_PasswordNeverPersisted(t *testing.T) {
	const password = "sup3r-secret-PW-!"
	f := newApplierFixture(realtorManifest())
	if _, err := f.ap.Apply(context.Background(), "sess-1", ""); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	st, err := f.ap.ExecuteStep(context.Background(), "sess-1", "s-reg", map[string]any{
		"name": "Olga", "email": "owner@acme.test", "password": password,
	})
	if err != nil {
		t.Fatalf("ExecuteStep: %v", err)
	}
	if f.gateway.seenPassword != password {
		t.Fatalf("gateway did not receive the password — the ONE allowed path is broken")
	}
	if st.Status != domain.ManifestStepApplied {
		t.Fatalf("register step status = %q, want applied", st.Status)
	}
	if st.Result["userId"] != "user-42" || st.Result["email"] != "owner@acme.test" {
		t.Fatalf("register step result = %v", st.Result)
	}

	persisted := f.state.persistedJSON(t)
	if strings.Contains(persisted, password) {
		t.Fatalf("PASSWORD LEAKED into persisted manifest/delta state (R6 violation)")
	}
	if strings.Contains(persisted, `"password"`) {
		t.Fatalf("a 'password' key reached persisted state (R6 violation)")
	}
}

// A provisioning failure keeps the step accepted (re-submittable) and still
// never persists the password.
func TestExecuteStepRegisterUser_FailureStaysAccepted(t *testing.T) {
	const password = "another-secret-9"
	f := newApplierFixture(realtorManifest())
	if _, err := f.ap.Apply(context.Background(), "sess-1", ""); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	f.gateway.provisionErr = errors.New("admin responded 400: weak password policy")

	_, err := f.ap.ExecuteStep(context.Background(), "sess-1", "s-reg", map[string]any{
		"email": "owner@acme.test", "password": password,
	})
	if err == nil {
		t.Fatalf("ExecuteStep succeeded despite gateway failure")
	}
	m, _ := f.state.GetOnboarding(context.Background(), "sess-1")
	if got := stepByID(t, m, "s-reg").Status; got != domain.ManifestStepAccepted {
		t.Fatalf("register step after failure = %q, want accepted (re-submittable)", got)
	}
	if strings.Contains(f.state.persistedJSON(t), password) {
		t.Fatalf("PASSWORD LEAKED on the failure path (R6 violation)")
	}
}

// ExecuteStep error contract for the handler mapping. (A fully-proposed
// manifest is NOT an error anymore — the auto-apply tests below own that
// path, owner decision 2026-07-28.)
func TestExecuteStepErrors(t *testing.T) {
	f := newApplierFixture(realtorManifest())
	ctx := context.Background()

	if _, err := f.ap.ExecuteStep(ctx, "sess-1", "nope", nil); !errors.Is(err, ErrStepNotFound) {
		t.Errorf("unknown step: err = %v, want ErrStepNotFound", err)
	}
	if _, err := f.ap.ExecuteStep(ctx, "sess-1", "s-entity", nil); !errors.Is(err, ErrStepNotSubmittable) {
		t.Errorf("non-register step: err = %v, want ErrStepNotSubmittable", err)
	}

	if _, err := f.ap.Apply(ctx, "sess-1", ""); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := f.ap.ExecuteStep(ctx, "sess-1", "s-reg", map[string]any{"email": "", "password": ""}); !errors.Is(err, ErrInvalidSubmit) {
		t.Errorf("empty creds: err = %v, want ErrInvalidSubmit", err)
	}

	empty := &applierStatePort{}
	f2 := newApplierFixture(nil)
	f2.state = empty
	f2.ap = NewManifestApplier(ManifestApplierConfig{State: empty, Log: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if _, err := f2.ap.ExecuteStep(ctx, "sess-x", "s-reg", nil); !errors.Is(err, ErrNoManifest) {
		t.Errorf("no manifest: err = %v, want ErrNoManifest", err)
	}
}

// Owner decision 2026-07-28 (the live-bug class): submitting the
// registration form against a fully-PROPOSED manifest IS the approval —
// ExecuteStep auto-applies the staged manifest, then registers. The 409
// "step not ready: apply the manifest first" after the model merely SAID
// "applying now" (without calling apply_manifest) must never recur.
// Addressed by the seeded form's op-alias, like the real widget does.
func TestExecuteStepAutoAppliesProposedManifest(t *testing.T) {
	const password = "auto-apply-secret-1"
	f := newApplierFixture(realtorManifest())
	ctx := context.Background()

	st, err := f.ap.ExecuteStep(ctx, "sess-1", "register_user", map[string]any{
		"email": "owner@acme.test", "password": password,
	})
	if err != nil {
		t.Fatalf("ExecuteStep on a fully-proposed manifest: %v (this is the live 409 bug)", err)
	}
	if st.Status != domain.ManifestStepApplied {
		t.Fatalf("register step = %q, want applied", st.Status)
	}
	if f.gateway.createCalls != 1 || f.gateway.provisionCalls != 1 {
		t.Fatalf("create=%d provision=%d, want 1/1 (one auto-apply, then the registration)",
			f.gateway.createCalls, f.gateway.provisionCalls)
	}

	// The whole plan ran — not just the register step.
	m, _ := f.state.GetOnboarding(ctx, "sess-1")
	for _, id := range []string{"s-tenant", "s-entity", "s-vset", "s-ops"} {
		if got := stepByID(t, m, id).Status; got != domain.ManifestStepApplied {
			t.Errorf("step %s = %q, want applied (auto-apply must run the full plan)", id, got)
		}
	}
	if tok, _ := stepByID(t, m, "s-door").Result["token"].(string); tok == "" {
		t.Errorf("ingest door not armed by the auto-apply: %v", stepByID(t, m, "s-door").Result)
	}

	// R6 discipline survives the auto-apply path.
	if strings.Contains(f.state.persistedJSON(t), password) {
		t.Fatalf("PASSWORD LEAKED on the auto-apply path (R6 violation)")
	}
}

// A halted auto-apply (create_tenant fails → no tenant) still refuses the
// registration with a 409-class sentinel that CARRIES the failed step's
// reason — never a bare "apply the manifest first" (owner 2026-07-28).
func TestExecuteStepAutoApplyHaltSurfacesReason(t *testing.T) {
	f := newApplierFixture(realtorManifest())
	f.gateway.createErr = errors.New("admin unreachable")

	_, err := f.ap.ExecuteStep(context.Background(), "sess-1", "s-reg", map[string]any{
		"email": "owner@acme.test", "password": "pw",
	})
	if !errors.Is(err, ErrTenantNotProvisioned) {
		t.Fatalf("err = %v, want ErrTenantNotProvisioned (handler maps it to 409)", err)
	}
	if !strings.Contains(err.Error(), "create_tenant failed: admin unreachable") {
		t.Fatalf("409 reason does not name the failed step: %v", err)
	}

	// The step is still re-submittable once the failure is fixed.
	m, _ := f.state.GetOnboarding(context.Background(), "sess-1")
	if got := stepByID(t, m, "s-reg").Status; got != domain.ManifestStepAccepted {
		t.Fatalf("register step after halted auto-apply = %q, want accepted", got)
	}
}

// A MID-chain failure (tenant fine, an unrelated step broke) must not block
// the user's registration — their action proceeds; the failed step stays
// retryable on the manifest.
func TestExecuteStepAutoApplyMidChainFailureStillRegisters(t *testing.T) {
	f := newApplierFixture(realtorManifest())
	f.entities.valueSetErr = errors.New("db exploded")

	st, err := f.ap.ExecuteStep(context.Background(), "sess-1", "s-reg", map[string]any{
		"email": "owner@acme.test", "password": "pw",
	})
	if err != nil {
		t.Fatalf("registration blocked by an unrelated failed step: %v", err)
	}
	if st.Status != domain.ManifestStepApplied {
		t.Fatalf("register step = %q, want applied", st.Status)
	}
	m, _ := f.state.GetOnboarding(context.Background(), "sess-1")
	if got := stepByID(t, m, "s-vset").Status; got != domain.ManifestStepFailed {
		t.Fatalf("value-set step = %q, want failed (recorded for retry)", got)
	}
}

// The user's upload IS the approval (owner 2026-07-28): a fully-proposed
// manifest resolves to a live ingest token by auto-applying (which mints
// the door); a second call returns the SAME token without re-minting.
func TestResolveIngestTokenAutoApplies(t *testing.T) {
	f := newApplierFixture(realtorManifest())
	ctx := context.Background()

	tok, err := f.ap.ResolveIngestToken(ctx, "sess-1")
	if err != nil {
		t.Fatalf("ResolveIngestToken on a fully-proposed manifest: %v", err)
	}
	if f.tokens.ingestMints != 1 {
		t.Fatalf("ingest mints = %d, want 1 (minted by the auto-apply)", f.tokens.ingestMints)
	}
	if tok.TenantSlug != "acme-realty" {
		t.Fatalf("token tenant slug = %q, want acme-realty (admin route address)", tok.TenantSlug)
	}
	m, _ := f.state.GetOnboarding(ctx, "sess-1")
	door := stepByID(t, m, "s-door")
	if got, _ := door.Result["token"].(string); got != tok.Token {
		t.Fatalf("step token %q != resolved token %q", got, tok.Token)
	}

	tok2, err := f.ap.ResolveIngestToken(ctx, "sess-1")
	if err != nil {
		t.Fatalf("second ResolveIngestToken: %v", err)
	}
	if tok2.Token != tok.Token || f.tokens.ingestMints != 1 {
		t.Fatalf("second resolve re-minted: token %q vs %q, mints %d", tok2.Token, tok.Token, f.tokens.ingestMints)
	}
}

// A stale recorded token re-mints server-side and stamps the fresh token
// back onto the step (persisted — poll endpoint and re-renders agree).
func TestResolveIngestTokenReMintsStale(t *testing.T) {
	m := realtorManifest()
	m.Tenant = domain.ManifestTenant{ID: "11111111-2222-3333-4444-555555555555", Slug: "acme-realty", Name: "Acme Realty"}
	for i := range m.Steps {
		m.Steps[i].Status = domain.ManifestStepApplied
	}
	door := stepByID(t, m, "s-door")
	door.Status = domain.ManifestStepAccepted
	door.Result = map[string]any{"token": "stale-1"}

	f := newApplierFixture(m)
	f.tokens.minted = map[string]*ports.IngestToken{
		"stale-1": {Token: "stale-1", SessionID: "sess-1", ExpiresAt: time.Now().Add(-time.Minute)},
	}

	tok, err := f.ap.ResolveIngestToken(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("ResolveIngestToken with a stale recorded token: %v", err)
	}
	if tok.Token == "stale-1" {
		t.Fatalf("stale token returned as live")
	}
	if f.tokens.ingestMints != 1 {
		t.Fatalf("ingest mints = %d, want 1 (fresh mint)", f.tokens.ingestMints)
	}
	mm, _ := f.state.GetOnboarding(context.Background(), "sess-1")
	if got, _ := stepByID(t, mm, "s-door").Result["token"].(string); got != tok.Token {
		t.Fatalf("fresh token not stamped on the step: %q vs %q", got, tok.Token)
	}
}

// ResolveIngestToken failure surface: a halted apply names the failed step;
// a manifest without an uploader step is a distinct, mappable error.
func TestResolveIngestTokenErrors(t *testing.T) {
	ctx := context.Background()

	f := newApplierFixture(realtorManifest())
	f.gateway.createErr = errors.New("admin down")
	_, err := f.ap.ResolveIngestToken(ctx, "sess-1")
	if !errors.Is(err, ErrTenantNotProvisioned) {
		t.Fatalf("halted apply: err = %v, want ErrTenantNotProvisioned", err)
	}
	if !strings.Contains(err.Error(), "create_tenant failed: admin down") {
		t.Fatalf("halted apply reason missing: %v", err)
	}

	// Owner's law (2026-07-28): a file arriving at the uploader IS the
	// intent — a manifest without an issue_ingest_door step gets one
	// auto-staged (persisted) and the resolve succeeds instead of 404ing.
	noDoor := realtorManifest()
	steps := noDoor.Steps[:0]
	for _, s := range noDoor.Steps {
		if s.Op != "issue_ingest_door" {
			steps = append(steps, s)
		}
	}
	noDoor.Steps = steps
	f2 := newApplierFixture(noDoor)
	tok, err := f2.ap.ResolveIngestToken(ctx, "sess-1")
	if err != nil {
		t.Fatalf("no door staged must auto-stage, got err = %v", err)
	}
	if tok == nil || tok.Token == "" {
		t.Fatalf("auto-staged door must mint a token, got %+v", tok)
	}
	if idx := findStepIndexByOp(f2.state.manifest, "issue_ingest_door"); idx < 0 {
		t.Fatalf("auto-staged issue_ingest_door step was not persisted")
	}
}

// ManifestStatusOf digests the manifest into the contextual-CTA summary
// (owner 2026-07-28): staged>0 ⇒ the shell may show Accept; failed>0 ⇒
// retry + the FIRST failure's op and reason; nil manifest ⇒ nil (wire
// omits the field).
func TestManifestStatusOf(t *testing.T) {
	if s := ManifestStatusOf(nil); s != nil {
		t.Fatalf("nil manifest: summary = %+v, want nil", s)
	}
	if s := ManifestStatusOf(&domain.OnboardingManifest{}); s != nil {
		t.Fatalf("empty manifest: summary = %+v, want nil", s)
	}

	m := &domain.OnboardingManifest{Steps: []domain.ManifestStep{
		{ID: "1", Op: "create_tenant", Status: domain.ManifestStepApplied},
		{ID: "2", Op: "define_entity", Status: domain.ManifestStepFailed, Error: "boom"},
		{ID: "3", Op: "define_value_set", Status: domain.ManifestStepFailed, Error: "later"},
		{ID: "4", Op: "issue_ingest_door", Status: domain.ManifestStepAccepted},
		{ID: "5", Op: "enable_operations", Status: domain.ManifestStepProposed},
		{ID: "6", Op: "adopt_presets", Status: domain.ManifestStepProposed},
	}}
	s := ManifestStatusOf(m)
	if s.Staged != 2 || s.Applied != 1 || s.Failed != 2 || s.Total != 6 {
		t.Fatalf("summary = %+v, want staged 2 / applied 1 / failed 2 / total 6", s)
	}
	if s.FailedStep != "define_entity" || s.FailedReason != "boom" {
		t.Fatalf("first failure = %s/%s, want define_entity/boom", s.FailedStep, s.FailedReason)
	}
}

// upTo applies only the ordered prefix up to and including the named step.
func TestApplyUpTo(t *testing.T) {
	f := newApplierFixture(realtorManifest())
	m, err := f.ap.Apply(context.Background(), "sess-1", "s-entity")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// Order is tenant → entity (forced-first tenant, then stage order).
	if got := stepByID(t, m, "s-tenant").Status; got != domain.ManifestStepApplied {
		t.Fatalf("tenant step = %q, want applied", got)
	}
	if got := stepByID(t, m, "s-entity").Status; got != domain.ManifestStepApplied {
		t.Fatalf("entity step = %q, want applied", got)
	}
	if got := stepByID(t, m, "s-vset").Status; got != domain.ManifestStepProposed {
		t.Fatalf("beyond-upTo step = %q, want proposed (untouched)", got)
	}
}

// The upload-door lifecycle: mint at apply (rests accepted), job recorded,
// completion applies the step, consumes the token, and is idempotent.
func TestIngestDoorLifecycle(t *testing.T) {
	f := newApplierFixture(realtorManifest())
	ctx := context.Background()
	m, err := f.ap.Apply(ctx, "sess-1", "")
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	door := stepByID(t, m, "s-door")
	if door.Status != domain.ManifestStepAccepted {
		t.Fatalf("door step = %q, want accepted", door.Status)
	}
	tok, _ := door.Result["token"].(string)
	if tok == "" {
		t.Fatalf("no ingest token minted: %v", door.Result)
	}

	// Re-apply must NOT mint a second token (armed uploader already bound it).
	if _, err := f.ap.Apply(ctx, "sess-1", ""); err != nil {
		t.Fatalf("re-Apply: %v", err)
	}
	if f.tokens.ingestMints != 1 {
		t.Fatalf("ingest mints = %d, want 1", f.tokens.ingestMints)
	}

	if err := f.ap.RecordUploadJob(ctx, "sess-1", "job-7", "listings.csv"); err != nil {
		t.Fatalf("RecordUploadJob: %v", err)
	}
	// A failed import keeps the door open (R25).
	if err := f.ap.RecordIngestFailure(ctx, "sess-1", []string{"row 3: no name"}); err != nil {
		t.Fatalf("RecordIngestFailure: %v", err)
	}
	m, _ = f.state.GetOnboarding(ctx, "sess-1")
	door = stepByID(t, m, "s-door")
	if door.Status != domain.ManifestStepAccepted {
		t.Fatalf("door after failure = %q, want accepted", door.Status)
	}
	if door.Result["lastError"] == "" {
		t.Fatalf("failure not recorded: %v", door.Result)
	}

	st := &ports.AdminImportStatus{Status: "completed", Processed: 20, ProjectionRows: 20, Invalidated: true}
	if err := f.ap.CompleteIngestStep(ctx, "sess-1", st); err != nil {
		t.Fatalf("CompleteIngestStep: %v", err)
	}
	if err := f.ap.CompleteIngestStep(ctx, "sess-1", st); err != nil { // idempotent
		t.Fatalf("second CompleteIngestStep: %v", err)
	}
	m, _ = f.state.GetOnboarding(ctx, "sess-1")
	door = stepByID(t, m, "s-door")
	if door.Status != domain.ManifestStepApplied {
		t.Fatalf("door after completion = %q, want applied", door.Status)
	}
	if door.Result["projectionRows"] != float64(20) { // JSON round-trip: numbers → float64
		t.Fatalf("honest outcome missing: %v", door.Result)
	}
	if len(f.tokens.usedTokens) != 1 || f.tokens.usedTokens[0] != tok {
		t.Fatalf("ingest token not consumed exactly once: %v", f.tokens.usedTokens)
	}
}

// --- concurrency (m3 security review, 2026-07-28) ---

// A double-clicked registration submit races two ExecuteStep calls, BOTH of
// which would auto-apply a still-proposed manifest — without per-session
// serialization that is two CreateTenant calls (admin suffixes slugs → two
// tenants, split-brain manifest). The session lock must reduce the race to
// one apply + one provision; the loser sees the applied step and replays
// idempotently.
func TestExecuteStepConcurrentDoubleSubmitAppliesOnce(t *testing.T) {
	f := newApplierFixture(realtorManifest())
	ctx := context.Background()

	var wg sync.WaitGroup
	errs := make([]error, 2)
	steps := make([]*domain.ManifestStep, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			steps[i], errs[i] = f.ap.ExecuteStep(ctx, "sess-1", "s-reg", map[string]any{
				"email": "owner@acme.test", "password": "pw-concurrent",
			})
		}(i)
	}
	wg.Wait()

	for i := 0; i < 2; i++ {
		if errs[i] != nil {
			t.Fatalf("submit %d failed: %v", i, errs[i])
		}
		if steps[i].Status != domain.ManifestStepApplied {
			t.Fatalf("submit %d step = %q, want applied", i, steps[i].Status)
		}
	}
	if f.gateway.createCalls != 1 {
		t.Fatalf("create tenant calls = %d, want 1 (concurrent auto-apply must serialize)", f.gateway.createCalls)
	}
	if f.gateway.provisionCalls != 1 {
		t.Fatalf("provision calls = %d, want 1 (loser must replay idempotently)", f.gateway.provisionCalls)
	}
}

// Concurrent upload + registration submit (both owner-decided auto-apply
// triggers) must also converge on ONE apply run: one tenant, one door mint.
func TestConcurrentUploadAndSubmitApplyOnce(t *testing.T) {
	f := newApplierFixture(realtorManifest())
	ctx := context.Background()

	var wg sync.WaitGroup
	var submitErr, uploadErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, submitErr = f.ap.ExecuteStep(ctx, "sess-1", "s-reg", map[string]any{
			"email": "owner@acme.test", "password": "pw-race",
		})
	}()
	go func() {
		defer wg.Done()
		_, uploadErr = f.ap.ResolveIngestToken(ctx, "sess-1")
	}()
	wg.Wait()

	if submitErr != nil {
		t.Fatalf("submit failed: %v", submitErr)
	}
	if uploadErr != nil {
		t.Fatalf("upload door resolution failed: %v", uploadErr)
	}
	if f.gateway.createCalls != 1 {
		t.Fatalf("create tenant calls = %d, want 1", f.gateway.createCalls)
	}
	if f.tokens.ingestMints != 1 {
		t.Fatalf("ingest mints = %d, want 1 (the single apply mints the door once)", f.tokens.ingestMints)
	}
}

// EnsureZeroInputStep is the render-side face of the owner's law (handoff
// 2026-07-28, law #1): the model rendered the block whose data only
// issue_surface_urls can produce, but never staged it and never applied.
// The server must stage it, run the manifest and end with live URLs — the
// business user's handover cannot hinge on a tool call the model skipped.
func TestEnsureZeroInputStepStagesAndApplies(t *testing.T) {
	ctx := context.Background()

	// A realtor plan WITHOUT the surface-URL step, everything else ready to
	// apply (the two trigger steps completed out of band below).
	noURLs := realtorManifest()
	steps := noURLs.Steps[:0]
	for _, s := range noURLs.Steps {
		if s.Op != "issue_surface_urls" {
			steps = append(steps, s)
		}
	}
	noURLs.Steps = steps
	f := newApplierFixture(noURLs)
	if _, err := f.ap.Apply(ctx, "sess-1", ""); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := f.ap.ExecuteStep(ctx, "sess-1", "s-reg", map[string]any{
		"email": "owner@acme.test", "password": "pw",
	}); err != nil {
		t.Fatalf("ExecuteStep: %v", err)
	}
	if err := f.ap.CompleteIngestStep(ctx, "sess-1", &ports.AdminImportStatus{
		Status: "completed", Processed: 10, ProjectionRows: 10, Invalidated: true,
	}); err != nil {
		t.Fatalf("CompleteIngestStep: %v", err)
	}
	if idx := findStepIndexByOp(f.state.manifest, "issue_surface_urls"); idx >= 0 {
		t.Fatalf("precondition: the step must NOT be staged yet")
	}

	changed, err := f.ap.EnsureZeroInputStep(ctx, "sess-1", "issue_surface_urls")
	if err != nil {
		t.Fatalf("EnsureZeroInputStep: %v", err)
	}
	if !changed {
		t.Fatal("changed = false — the caller would bind stale state and render an empty card")
	}

	idx := findStepIndexByOp(f.state.manifest, "issue_surface_urls")
	if idx < 0 {
		t.Fatal("auto-staged issue_surface_urls step was not persisted")
	}
	st := f.state.manifest.Steps[idx]
	if st.Status != domain.ManifestStepApplied {
		t.Fatalf("step status = %q, want applied; result %v, err %q", st.Status, st.Result, st.Error)
	}
	if sf, _ := st.Result["storefrontUrl"].(string); sf != "https://v5.example.test/s/acme-realty" {
		t.Errorf("storefront url = %q", sf)
	}
	if crm, _ := st.Result["crmUrl"].(string); !strings.HasPrefix(crm, "https://v5.example.test/crm/acme-realty?k=surface-token-") {
		t.Errorf("crm url = %q", crm)
	}

	// Second render of the same block must be a no-op: applied is terminal,
	// so no second surface token and no apply churn (loop guard).
	changed, err = f.ap.EnsureZeroInputStep(ctx, "sess-1", "issue_surface_urls")
	if err != nil {
		t.Fatalf("second EnsureZeroInputStep: %v", err)
	}
	if changed {
		t.Error("changed = true on an already-applied step — the render loop would re-apply every turn")
	}
	if f.tokens.surfaceMints != 1 {
		t.Errorf("surface token mints = %d, want 1", f.tokens.surfaceMints)
	}
}

// The guard rails: only zero-input ops may be auto-staged (anything else
// would need business values the server cannot invent), and a session with
// no plan at all is left alone rather than given an orphan step.
func TestEnsureZeroInputStepGuards(t *testing.T) {
	ctx := context.Background()

	f := newApplierFixture(realtorManifest())
	if _, err := f.ap.EnsureZeroInputStep(ctx, "sess-1", "create_tenant"); err == nil {
		t.Fatal("create_tenant needs name+vertical — auto-staging it must fail loud")
	}

	empty := newApplierFixture(nil)
	changed, err := empty.ap.EnsureZeroInputStep(ctx, "sess-1", "issue_surface_urls")
	if err != nil {
		t.Fatalf("session without a manifest must be a no-op, got err = %v", err)
	}
	if changed {
		t.Error("changed = true on a session with no manifest")
	}
	if empty.state.manifest != nil {
		t.Error("an orphan step was persisted into a session that has no plan")
	}
}

// The readiness gate. Unlike the other two faces of the owner's law — form
// submit and file upload, both USER actions — this trigger is a MODEL choice:
// agent2 deciding to compose a surface_links block. So it must be incapable
// of provisioning anything on its own. EnsureZeroInputStep runs the applier,
// and the applier runs the WHOLE manifest, so the guard is a precondition:
// every other step must already be terminal (applied or skipped). While one
// is not, the render is left to come out empty and honest.
//
// Each case below is a way the model can fire the trigger against a plan that
// is not ready, and none of them may touch admin.
func TestEnsureZeroInputStepRefusesAnUnreadyPlan(t *testing.T) {
	ctx := context.Background()

	// A surface_links block emitted before the user approved anything: the
	// whole plan is still `proposed`. Applying here would create the tenant.
	t.Run("nothing approved yet", func(t *testing.T) {
		f := newApplierFixture(realtorManifest())
		changed, err := f.ap.EnsureZeroInputStep(ctx, "sess-1", "issue_surface_urls")
		if err != nil {
			t.Fatalf("EnsureZeroInputStep: %v", err)
		}
		if changed {
			t.Error("changed = true on an unapproved plan")
		}
		if f.gateway.createCalls != 0 {
			t.Errorf("createTenant calls = %d — a tenant was provisioned in admin off a "+
				"model's render choice, with no user action", f.gateway.createCalls)
		}
		if idx := findStepIndexByOp(f.state.manifest, "issue_surface_urls"); idx >= 0 &&
			f.state.manifest.Steps[idx].Status != domain.ManifestStepProposed {
			t.Errorf("issue_surface_urls ran against an unapproved plan: %+v", f.state.manifest.Steps[idx])
		}
	})

	// The two steps that REST mid-flow: register_user waits for the form,
	// issue_ingest_door waits for the upload. A surface_links render while
	// either is outstanding must not re-run the apply chain (and could not
	// issue anything anyway — applyIssueSurfaceURLs refuses a pending plan).
	t.Run("a step is still resting at accepted", func(t *testing.T) {
		f := newApplierFixture(realtorManifest())
		if _, err := f.ap.Apply(ctx, "sess-1", ""); err != nil {
			t.Fatalf("Apply: %v", err)
		}
		before := f.gateway.adoptCalls
		changed, err := f.ap.EnsureZeroInputStep(ctx, "sess-1", "issue_surface_urls")
		if err != nil {
			t.Fatalf("EnsureZeroInputStep: %v", err)
		}
		if changed {
			t.Error("changed = true while register_user is still waiting for the form")
		}
		if f.gateway.adoptCalls != before {
			t.Errorf("adopt calls %d → %d — the render re-ran the whole apply chain",
				before, f.gateway.adoptCalls)
		}
		if f.tokens.surfaceMints != 0 {
			t.Errorf("surface token mints = %d, want 0 — nothing to hand over yet", f.tokens.surfaceMints)
		}
	})

	// A step that FAILED (admin timeout on create_tenant, say) is the worst
	// case: without the gate every subsequent composed turn retries it
	// unattended, and the tenant-ID idempotency guard only holds if admin
	// never created the tenant it errored on.
	t.Run("a failed step is not retried on every render", func(t *testing.T) {
		f := newApplierFixture(realtorManifest())
		f.gateway.createErr = errors.New("admin timeout")
		if _, err := f.ap.Apply(ctx, "sess-1", ""); err != nil {
			t.Fatalf("Apply: %v", err)
		}
		if f.gateway.createCalls != 1 {
			t.Fatalf("precondition: createTenant calls = %d, want 1", f.gateway.createCalls)
		}
		for i := 0; i < 3; i++ {
			changed, err := f.ap.EnsureZeroInputStep(ctx, "sess-1", "issue_surface_urls")
			if err != nil {
				t.Fatalf("EnsureZeroInputStep %d: %v", i, err)
			}
			if changed {
				t.Fatalf("changed = true on render %d over a failed plan", i)
			}
		}
		if f.gateway.createCalls != 1 {
			t.Errorf("createTenant calls = %d, want 1 — the failed step was retried "+
				"unattended, once per composed turn", f.gateway.createCalls)
		}
	})
}
