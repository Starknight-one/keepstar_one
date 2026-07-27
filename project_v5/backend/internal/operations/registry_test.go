package operations

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"keepstar_v5/internal/domain"
)

// ─── fakes ───────────────────────────────────────────────────────────────

type fakeTenants struct {
	tenant *domain.Tenant
	calls  int
}

func (f *fakeTenants) GetTenantBySlug(_ context.Context, slug string) (*domain.Tenant, error) {
	f.calls++
	if f.tenant != nil {
		return f.tenant, nil
	}
	return &domain.Tenant{ID: "tnt-1", Slug: slug}, nil
}

type fakeStore struct {
	instances []domain.TenantOperation
	listCalls int
}

func (s *fakeStore) SearchLibrary(context.Context, []float32, string, []string, int) ([]domain.OperationTemplate, error) {
	return nil, nil
}
func (s *fakeStore) EnableOperation(context.Context, string, string, string, map[string]any) (*domain.TenantOperation, error) {
	return nil, nil
}
func (s *fakeStore) UpdateInstanceConfig(context.Context, string, string, map[string]any) error {
	return nil
}
func (s *fakeStore) DisableOperation(context.Context, string, string) error { return nil }
func (s *fakeStore) ListEnabled(context.Context, string) ([]domain.TenantOperation, error) {
	s.listCalls++
	return s.instances, nil
}
func (s *fakeStore) UpsertTemplate(_ context.Context, t domain.OperationTemplate, _ []float32) (string, error) {
	return "id-" + t.Name, nil
}
func (s *fakeStore) ListTemplates(context.Context) ([]domain.OperationTemplate, error) {
	return nil, nil
}

// stubExecutor is a native (non-passthrough) executor with a canned result.
type stubExecutor struct {
	tmpl      domain.OperationTemplate
	result    *domain.OperationResult
	err       error
	lastInput map[string]any
	lastOctx  domain.OperationContext
	calls     int
}

func (s *stubExecutor) Template() domain.OperationSpec         { return s.tmpl.Spec() }
func (s *stubExecutor) TemplateRow() domain.OperationTemplate  { return s.tmpl }
func (s *stubExecutor) SpecForTenant(_ context.Context, _ domain.Tenant, _ map[string]any) (domain.OperationSpec, error) {
	return s.tmpl.Spec(), nil
}
func (s *stubExecutor) Execute(_ context.Context, octx domain.OperationContext, input map[string]any) (*domain.OperationResult, error) {
	s.calls++
	s.lastOctx = octx
	s.lastInput = input
	if s.err != nil {
		return nil, s.err
	}
	if s.result != nil {
		return s.result, nil
	}
	return &domain.OperationResult{Outcome: domain.OutcomeOK, Summary: "ok"}, nil
}

// fakeRuns captures audit rows.
type fakeRuns struct {
	rows []domain.OperationRun
}

func (f *fakeRuns) Append(_ context.Context, run domain.OperationRun) error {
	f.rows = append(f.rows, run)
	return nil
}

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func stubTemplate(name string, kind domain.OperationKind, agent domain.AgentPlane, minRole domain.Role, modes ...domain.PipelineMode) domain.OperationTemplate {
	return domain.OperationTemplate{
		Name:        name,
		Kind:        kind,
		Title:       name,
		Description: "stub " + name,
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		Modes:       modes,
		Agent:       agent,
		MinRole:     minRole,
		AutoEnabled: true,
	}
}

// ─── DefinitionsFor ──────────────────────────────────────────────────────

func TestDefinitionsForFiltersByModeAgentRoleAndSorts(t *testing.T) {
	reg := NewRegistry(RegistryConfig{Tenants: &fakeTenants{}, Log: testLogger()})
	reg.RegisterExecutor(domain.KindQuery, &stubExecutor{tmpl: stubTemplate("catalog_search", domain.KindQuery, domain.AgentData, domain.RoleVisitor, domain.ModeStorefront, domain.ModeCRM)})
	reg.RegisterExecutor(domain.KindInternal, &stubExecutor{tmpl: stubTemplate("_internal_state_filter", domain.KindInternal, domain.AgentData, domain.RoleVisitor, domain.ModeStorefront, domain.ModeCRM)})
	reg.RegisterExecutor(domain.KindInternal, &stubExecutor{tmpl: stubTemplate("_internal_history_lookup", domain.KindInternal, domain.AgentData, domain.RoleVisitor, domain.ModeStorefront, domain.ModeCRM)})
	reg.RegisterExecutor(domain.KindVisual, &stubExecutor{tmpl: stubTemplate("visual_assembly", domain.KindVisual, domain.AgentVisual, domain.RoleVisitor, domain.ModeStorefront, domain.ModeCRM, domain.ModeOnboarding)})
	reg.RegisterExecutor(domain.KindMeta, &stubExecutor{tmpl: stubTemplate("search_library", domain.KindMeta, domain.AgentData, domain.RoleVisitor, domain.ModeOnboarding)})
	reg.RegisterExecutor(domain.KindNotify, &stubExecutor{tmpl: stubTemplate("notify", domain.KindNotify, domain.AgentData, domain.RoleSystem, domain.ModeStorefront, domain.ModeCRM)})

	// Storefront data plane, visitor: exactly today's Agent1 set, sorted.
	defs := reg.DefinitionsFor(context.Background(), "acme", domain.ModeStorefront, domain.AgentData, domain.RoleVisitor)
	want := []string{"_internal_history_lookup", "_internal_state_filter", "catalog_search"}
	if len(defs) != len(want) {
		t.Fatalf("data-plane defs = %d, want %d: %+v", len(defs), len(want), defs)
	}
	for i, w := range want {
		if defs[i].Name != w {
			t.Errorf("defs[%d] = %q, want %q (byte-stable sort)", i, defs[i].Name, w)
		}
	}

	// Visual plane storefront: visual_assembly only.
	vdefs := reg.DefinitionsFor(context.Background(), "acme", domain.ModeStorefront, domain.AgentVisual, domain.RoleVisitor)
	if len(vdefs) != 1 || vdefs[0].Name != "visual_assembly" {
		t.Fatalf("visual defs = %+v, want [visual_assembly]", vdefs)
	}

	// Onboarding data plane: meta only — catalog_search excluded (R16).
	odefs := reg.DefinitionsFor(context.Background(), "acme", domain.ModeOnboarding, domain.AgentData, domain.RoleVisitor)
	if len(odefs) != 1 || odefs[0].Name != "search_library" {
		t.Fatalf("onboarding defs = %+v, want [search_library]", odefs)
	}

	// Unset mode/role default to storefront/visitor (R17/R14 defaults).
	ddefs := reg.DefinitionsFor(context.Background(), "acme", "", domain.AgentData, "")
	if len(ddefs) != len(want) {
		t.Fatalf("defaulted defs = %d, want %d", len(ddefs), len(want))
	}
}

func TestDefinitionsForIncludesEnabledInstances(t *testing.T) {
	store := &fakeStore{}
	reg := NewRegistry(RegistryConfig{Store: store, Tenants: &fakeTenants{}, Log: testLogger()})
	schedule := &stubExecutor{tmpl: func() domain.OperationTemplate {
		tmpl := stubTemplate("schedule_slot", domain.KindScheduleSlot, domain.AgentData, domain.RoleVisitor, domain.ModeStorefront, domain.ModeCRM)
		tmpl.AutoEnabled = false // executor templates are enabled per tenant (§3.1)
		return tmpl
	}()}
	reg.RegisterExecutor(domain.KindScheduleSlot, schedule)
	if err := reg.SeedTemplates(context.Background(), nil); err != nil {
		t.Fatalf("SeedTemplates: %v", err)
	}
	store.instances = []domain.TenantOperation{{
		ID: "inst-1", TenantID: "tnt-1", OperationID: "id-schedule_slot",
		Name: "book_showing", Enabled: true,
		Config:      map[string]any{"entity": "lead"},
		Description: "Book a property showing",
	}}

	defs := reg.DefinitionsFor(context.Background(), "acme", domain.ModeStorefront, domain.AgentData, domain.RoleVisitor)
	if len(defs) != 1 || defs[0].Name != "book_showing" {
		t.Fatalf("defs = %+v, want [book_showing] (template not auto-enabled, instance visible)", defs)
	}
	if defs[0].Description != "Book a property showing" {
		t.Errorf("instance description override lost: %q", defs[0].Description)
	}
}

// ─── Execute: gates ──────────────────────────────────────────────────────

func TestExecuteUnknownOperationDenied(t *testing.T) {
	runs := &fakeRuns{}
	reg := NewRegistry(RegistryConfig{Tenants: &fakeTenants{}, Runs: runs, Log: testLogger()})

	res, err := reg.Execute(context.Background(), domain.OperationContext{TenantSlug: "acme"}, domain.ToolCall{Name: "nope"})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if res.Outcome != domain.OutcomeDenied {
		t.Errorf("Outcome = %q, want denied", res.Outcome)
	}
	if !res.ToToolResult("t1").IsError {
		t.Error("denied must bridge to IsError so the LLM self-corrects")
	}
	if len(runs.rows) != 1 || runs.rows[0].Outcome != domain.OutcomeDenied {
		t.Errorf("denied execution must still be audited: %+v", runs.rows)
	}
}

func TestExecuteModeGate(t *testing.T) {
	runs := &fakeRuns{}
	reg := NewRegistry(RegistryConfig{Tenants: &fakeTenants{}, Runs: runs, Log: testLogger()})
	ex := &stubExecutor{tmpl: stubTemplate("catalog_search", domain.KindQuery, domain.AgentData, domain.RoleVisitor, domain.ModeStorefront, domain.ModeCRM)}
	reg.RegisterExecutor(domain.KindQuery, ex)

	res, err := reg.Execute(context.Background(),
		domain.OperationContext{TenantSlug: "acme", Mode: domain.ModeOnboarding, Role: domain.RoleVisitor},
		domain.ToolCall{Name: "catalog_search"})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if res.Outcome != domain.OutcomeDenied {
		t.Errorf("Outcome = %q, want denied (R16: onboarding never runs catalog_search)", res.Outcome)
	}
	if ex.calls != 0 {
		t.Error("executor must not run when the mode gate fails")
	}
}

func TestExecuteRoleGate(t *testing.T) {
	runs := &fakeRuns{}
	reg := NewRegistry(RegistryConfig{Tenants: &fakeTenants{}, Runs: runs, Log: testLogger()})
	ex := &stubExecutor{tmpl: stubTemplate("advance_lead", domain.KindTransitionStatus, domain.AgentData, domain.RoleStaff, domain.ModeCRM)}
	reg.RegisterExecutor(domain.KindTransitionStatus, ex)

	// Visitor → denied.
	res, err := reg.Execute(context.Background(),
		domain.OperationContext{TenantSlug: "acme", Mode: domain.ModeCRM, Role: domain.RoleVisitor},
		domain.ToolCall{Name: "advance_lead"})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if res.Outcome != domain.OutcomeDenied || ex.calls != 0 {
		t.Errorf("visitor vs staff-op: Outcome=%q calls=%d, want denied/0 (R14)", res.Outcome, ex.calls)
	}

	// Staff → runs.
	res, err = reg.Execute(context.Background(),
		domain.OperationContext{TenantSlug: "acme", Mode: domain.ModeCRM, Role: domain.RoleStaff},
		domain.ToolCall{Name: "advance_lead"})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if res.Outcome != domain.OutcomeOK || ex.calls != 1 {
		t.Errorf("staff execution failed: Outcome=%q calls=%d", res.Outcome, ex.calls)
	}
}

// ─── Execute: validation + audit ─────────────────────────────────────────

func TestExecuteValidatesNativeInput(t *testing.T) {
	runs := &fakeRuns{}
	reg := NewRegistry(RegistryConfig{Tenants: &fakeTenants{}, Runs: runs, Log: testLogger()})
	tmpl := stubTemplate("create_lead", domain.KindCreateRecord, domain.AgentData, domain.RoleVisitor, domain.ModeStorefront)
	tmpl.InputSchema = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":  map[string]any{"type": "string"},
			"price": map[string]any{"type": "number", domain.SchemaKeyUnit: string(domain.UnitUSD)},
		},
		"required": []string{"name"},
	}
	ex := &stubExecutor{tmpl: tmpl}
	reg.RegisterExecutor(domain.KindCreateRecord, ex)
	octx := domain.OperationContext{TenantSlug: "acme", Mode: domain.ModeStorefront, Role: domain.RoleVisitor}

	// Missing required → OutcomeInvalid, executor untouched, audited.
	res, err := reg.Execute(context.Background(), octx, domain.ToolCall{Name: "create_lead", Input: map[string]any{}})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if res.Outcome != domain.OutcomeInvalid || ex.calls != 0 {
		t.Errorf("invalid input: Outcome=%q calls=%d, want invalid/0", res.Outcome, ex.calls)
	}
	if !res.ToToolResult("t1").IsError {
		t.Error("invalid must bridge to IsError so the LLM self-corrects")
	}
	if len(runs.rows) != 1 || runs.rows[0].Outcome != domain.OutcomeInvalid {
		t.Errorf("invalid execution must be audited: %+v", runs.rows)
	}

	// Valid input → coerced (usd → cents) before the executor sees it.
	if _, err := reg.Execute(context.Background(), octx, domain.ToolCall{
		Name:  "create_lead",
		Input: map[string]any{"name": "Ann", "price": 12.5},
	}); err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if ex.lastInput["price"] != 1250 {
		t.Errorf("usd not coerced to cents at the choke point: %#v", ex.lastInput["price"])
	}
}

// TestExecuteRedactsSensitiveInAudit is the registry half of the R6 duty:
// x-sensitive keys never reach the persisted run row.
func TestExecuteRedactsSensitiveInAudit(t *testing.T) {
	runs := &fakeRuns{}
	reg := NewRegistry(RegistryConfig{Tenants: &fakeTenants{}, Runs: runs, Log: testLogger()})
	tmpl := stubTemplate("register", domain.KindCreateRecord, domain.AgentData, domain.RoleVisitor, domain.ModeStorefront)
	tmpl.InputSchema = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"email":    map[string]any{"type": "string"},
			"password": map[string]any{"type": "string", domain.SchemaKeySensitive: true},
		},
	}
	ex := &stubExecutor{tmpl: tmpl, result: &domain.OperationResult{Outcome: domain.OutcomeOK, Summary: "ok"}}
	reg.RegisterExecutor(domain.KindCreateRecord, ex)

	if _, err := reg.Execute(context.Background(),
		domain.OperationContext{TenantSlug: "acme", Mode: domain.ModeStorefront, Role: domain.RoleVisitor},
		domain.ToolCall{Name: "register", Input: map[string]any{"email": "a@b.c", "password": "hunter2"}},
	); err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if len(runs.rows) != 1 {
		t.Fatalf("want 1 audit row, got %d", len(runs.rows))
	}
	row := runs.rows[0]
	if row.Input["password"] != redactedPlaceholder {
		t.Errorf("password leaked into the audit row: %#v", row.Input)
	}
	if row.Input["email"] != "a@b.c" {
		t.Errorf("unflagged key mangled: %#v", row.Input)
	}
	// The executor still received the real value.
	if ex.lastInput["password"] != "hunter2" {
		t.Errorf("executor must receive the real value: %#v", ex.lastInput)
	}
}

func TestExecutePassthroughSkipsValidation(t *testing.T) {
	reg := NewRegistry(RegistryConfig{Tenants: &fakeTenants{}, Log: testLogger()})
	tool := &stubTool{
		def:    domain.ToolDefinition{Name: "catalog_search", Description: "d", InputSchema: map[string]any{"type": "object", "properties": map[string]any{"vector_query": map[string]any{"type": "string"}}, "required": []string{"vector_query"}}},
		result: &domain.ToolResult{Content: "ok: found 0 products"},
	}
	reg.RegisterExecutor(domain.KindQuery, WrapCatalogSearch(tool, nil))

	// Empty input despite required[] — the legacy retry path depends on
	// this reaching the tool (byte-identical behavior duty).
	res, err := reg.Execute(context.Background(),
		domain.OperationContext{TenantSlug: "acme", Mode: domain.ModeStorefront, Role: domain.RoleVisitor},
		domain.ToolCall{Name: "catalog_search", Input: map[string]any{}})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if res.Outcome != domain.OutcomeOK {
		t.Errorf("passthrough must skip validation, got %q (%s)", res.Outcome, res.Summary)
	}
	if tool.lastArgs == nil {
		t.Error("legacy tool never ran")
	}
}

// ─── Execute: instance resolution + config ───────────────────────────────

func TestExecuteResolvesInstanceWithConfig(t *testing.T) {
	store := &fakeStore{}
	reg := NewRegistry(RegistryConfig{Store: store, Tenants: &fakeTenants{}, Log: testLogger()})
	tmpl := stubTemplate("schedule_slot", domain.KindScheduleSlot, domain.AgentData, domain.RoleVisitor, domain.ModeStorefront, domain.ModeCRM)
	tmpl.AutoEnabled = false
	ex := &stubExecutor{tmpl: tmpl, result: &domain.OperationResult{Outcome: domain.OutcomeOK, Summary: "booked"}}
	reg.RegisterExecutor(domain.KindScheduleSlot, ex)
	if err := reg.SeedTemplates(context.Background(), nil); err != nil {
		t.Fatalf("SeedTemplates: %v", err)
	}
	cfg := map[string]any{"entity": "lead", "datetime_field": "preferredTime"}
	store.instances = []domain.TenantOperation{{
		ID: "inst-1", TenantID: "tnt-1", OperationID: "id-schedule_slot",
		Name: "book_showing", Enabled: true, Config: cfg,
	}}

	// The template name itself is NOT callable (not auto-enabled)…
	res, err := reg.Execute(context.Background(),
		domain.OperationContext{TenantSlug: "acme", Mode: domain.ModeStorefront, Role: domain.RoleVisitor},
		domain.ToolCall{Name: "schedule_slot"})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if res.Outcome != domain.OutcomeDenied {
		t.Errorf("non-auto-enabled template must deny direct calls, got %q", res.Outcome)
	}

	// …the instance is, and carries its config into the executor context.
	res, err = reg.Execute(context.Background(),
		domain.OperationContext{TenantSlug: "acme", Mode: domain.ModeStorefront, Role: domain.RoleVisitor},
		domain.ToolCall{Name: "book_showing"})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if res.Outcome != domain.OutcomeOK {
		t.Fatalf("instance call failed: %q (%s)", res.Outcome, res.Summary)
	}
	if ex.lastOctx.Config["entity"] != "lead" {
		t.Errorf("instance config not passed: %#v", ex.lastOctx.Config)
	}
	if ex.lastOctx.TenantID != "tnt-1" {
		t.Errorf("tenant id not stamped on context: %q", ex.lastOctx.TenantID)
	}
}

// ─── Spec cache: TTL + invalidation ──────────────────────────────────────

func TestInvalidateTenantDropsCachedInstances(t *testing.T) {
	store := &fakeStore{}
	reg := NewRegistry(RegistryConfig{Store: store, Tenants: &fakeTenants{}, Log: testLogger()})
	reg.RegisterExecutor(domain.KindQuery, &stubExecutor{tmpl: stubTemplate("catalog_search", domain.KindQuery, domain.AgentData, domain.RoleVisitor, domain.ModeStorefront)})

	reg.DefinitionsFor(context.Background(), "acme", domain.ModeStorefront, domain.AgentData, domain.RoleVisitor)
	reg.DefinitionsFor(context.Background(), "acme", domain.ModeStorefront, domain.AgentData, domain.RoleVisitor)
	if store.listCalls != 1 {
		t.Fatalf("instances must be cached per tenant: %d list calls", store.listCalls)
	}

	reg.InvalidateTenant("acme")
	reg.DefinitionsFor(context.Background(), "acme", domain.ModeStorefront, domain.AgentData, domain.RoleVisitor)
	if store.listCalls != 2 {
		t.Errorf("InvalidateTenant must force a rebuild: %d list calls", store.listCalls)
	}
}

func TestSpecCacheTTLActsAsMiss(t *testing.T) {
	store := &fakeStore{}
	reg := NewRegistry(RegistryConfig{Store: store, Tenants: &fakeTenants{}, SpecTTL: time.Nanosecond, Log: testLogger()})
	reg.RegisterExecutor(domain.KindQuery, &stubExecutor{tmpl: stubTemplate("catalog_search", domain.KindQuery, domain.AgentData, domain.RoleVisitor, domain.ModeStorefront)})

	reg.DefinitionsFor(context.Background(), "acme", domain.ModeStorefront, domain.AgentData, domain.RoleVisitor)
	time.Sleep(2 * time.Nanosecond)
	reg.DefinitionsFor(context.Background(), "acme", domain.ModeStorefront, domain.AgentData, domain.RoleVisitor)
	if store.listCalls < 2 {
		t.Errorf("expired entry must rebuild (§6.1 TTL net): %d list calls", store.listCalls)
	}
}

// ─── Seeding ─────────────────────────────────────────────────────────────

type fakeEmbedder struct {
	texts []string
}

func (f *fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	f.texts = texts
	out := make([][]float32, len(texts))
	for i := range out {
		out[i] = []float32{0.1, 0.2}
	}
	return out, nil
}

func TestSeedTemplatesUpsertsExecutorAndExtraRows(t *testing.T) {
	store := &fakeStore{}
	emb := &fakeEmbedder{}
	reg := NewRegistry(RegistryConfig{Store: store, Tenants: &fakeTenants{}, Embedder: emb, Log: testLogger()})
	reg.RegisterExecutor(domain.KindQuery, &stubExecutor{tmpl: stubTemplate("catalog_search", domain.KindQuery, domain.AgentData, domain.RoleVisitor, domain.ModeStorefront)})

	extra := []domain.OperationTemplate{stubTemplate("search_library", domain.KindMeta, domain.AgentData, domain.RoleVisitor, domain.ModeOnboarding)}
	if err := reg.SeedTemplates(context.Background(), extra); err != nil {
		t.Fatalf("SeedTemplates: %v", err)
	}
	if len(emb.texts) != 2 {
		t.Errorf("descriptions must embed in one batch: %d texts", len(emb.texts))
	}

	// Seeded-but-executor-less rows are library content: Execute → denied.
	res, err := reg.Execute(context.Background(),
		domain.OperationContext{TenantSlug: "acme", Mode: domain.ModeOnboarding, Role: domain.RoleVisitor},
		domain.ToolCall{Name: "search_library"})
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if res.Outcome != domain.OutcomeDenied {
		t.Errorf("executor-less template must deny, got %q", res.Outcome)
	}
}

func TestSeedTemplatesRequiresStore(t *testing.T) {
	reg := NewRegistry(RegistryConfig{Tenants: &fakeTenants{}, Log: testLogger()})
	if err := reg.SeedTemplates(context.Background(), nil); err == nil {
		t.Error("seeding without a store must fail loud")
	}
}

// ─── Execute: R6 credential scrub at the audit boundary ──────────────────

// THE R6 audit-boundary test (m3 security review, 2026-07-28): a
// model-smuggled password key must NEVER reach v5_operation_runs — on ANY
// audit path. The dangerous paths are denied and invalid, which persist the
// RAW pre-validation input: register_user's schema declares only `role`, so
// schema-driven x-sensitive redaction cannot see a smuggled `password` key.
// The name-based credential scrub (RedactCredentialKeys) must cover top-level
// keys, nested objects and objects inside arrays.
func TestAuditScrubsSmuggledCredentials(t *testing.T) {
	const password = "smuggled-secret-PW#9"

	smuggled := func() map[string]any {
		return map[string]any{
			"role":     "admin", // enum violation → OutcomeInvalid
			"password": password,
			"values":   map[string]any{"password": password, "note": "keep"},
			"accounts": []any{map[string]any{"secret": password}},
		}
	}

	assertScrubbed := func(t *testing.T, run domain.OperationRun) {
		t.Helper()
		raw, err := json.Marshal(run.Input)
		if err != nil {
			t.Fatalf("marshal run input: %v", err)
		}
		if strings.Contains(string(raw), password) {
			t.Fatalf("password VALUE reached the v5_operation_runs row (R6 violation): %s", raw)
		}
		if !strings.Contains(string(raw), `"password":"[REDACTED]"`) {
			t.Errorf("credential key should survive with a redacted value: %s", raw)
		}
	}

	tmpl := stubTemplate("register_user", domain.KindMeta, domain.AgentData, domain.RoleVisitor, domain.ModeOnboarding)
	tmpl.InputSchema = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"role": map[string]any{"type": "string", "enum": []string{"owner"}},
		},
		"required": []string{"role"},
	}

	t.Run("invalid path persists scrubbed raw input", func(t *testing.T) {
		runs := &fakeRuns{}
		reg := NewRegistry(RegistryConfig{Tenants: &fakeTenants{}, Runs: runs, Log: testLogger()})
		reg.RegisterExecutor(domain.KindMeta, &stubExecutor{tmpl: tmpl})

		res, err := reg.Execute(context.Background(),
			domain.OperationContext{TenantSlug: "acme", Mode: domain.ModeOnboarding, Role: domain.RoleVisitor},
			domain.ToolCall{Name: "register_user", Input: smuggled()})
		if err != nil {
			t.Fatalf("unexpected Go error: %v", err)
		}
		if res.Outcome != domain.OutcomeInvalid {
			t.Fatalf("Outcome = %q, want invalid (enum violation)", res.Outcome)
		}
		if len(runs.rows) != 1 {
			t.Fatalf("audit rows = %d, want 1", len(runs.rows))
		}
		assertScrubbed(t, runs.rows[0])
	})

	t.Run("denied (mode gate) path persists scrubbed raw input", func(t *testing.T) {
		runs := &fakeRuns{}
		reg := NewRegistry(RegistryConfig{Tenants: &fakeTenants{}, Runs: runs, Log: testLogger()})
		reg.RegisterExecutor(domain.KindMeta, &stubExecutor{tmpl: tmpl})

		res, err := reg.Execute(context.Background(),
			domain.OperationContext{TenantSlug: "acme", Mode: domain.ModeStorefront, Role: domain.RoleVisitor},
			domain.ToolCall{Name: "register_user", Input: smuggled()})
		if err != nil {
			t.Fatalf("unexpected Go error: %v", err)
		}
		if res.Outcome != domain.OutcomeDenied {
			t.Fatalf("Outcome = %q, want denied", res.Outcome)
		}
		if len(runs.rows) != 1 {
			t.Fatalf("audit rows = %d, want 1", len(runs.rows))
		}
		assertScrubbed(t, runs.rows[0])
	})

	t.Run("unknown-operation path persists scrubbed raw input", func(t *testing.T) {
		runs := &fakeRuns{}
		reg := NewRegistry(RegistryConfig{Tenants: &fakeTenants{}, Runs: runs, Log: testLogger()})

		res, err := reg.Execute(context.Background(),
			domain.OperationContext{TenantSlug: "acme", Mode: domain.ModeOnboarding, Role: domain.RoleVisitor},
			domain.ToolCall{Name: "no_such_op", Input: smuggled()})
		if err != nil {
			t.Fatalf("unexpected Go error: %v", err)
		}
		if res.Outcome != domain.OutcomeDenied {
			t.Fatalf("Outcome = %q, want denied", res.Outcome)
		}
		if len(runs.rows) != 1 {
			t.Fatalf("audit rows = %d, want 1", len(runs.rows))
		}
		assertScrubbed(t, runs.rows[0])
	})
}
