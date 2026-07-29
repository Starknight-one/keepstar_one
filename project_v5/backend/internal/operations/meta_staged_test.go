package operations

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// Staging appends a proposed step, proposes the tenant identity and writes
// the onboarding delta with IDENTITY-ONLY params — the curator-visible
// assembly log never carries step params (R6 discipline).
func TestStagedMetaExecutorStagesCreateTenant(t *testing.T) {
	ob := &fakeOnboardingState{}
	deps := metaDeps(ob, newOpsStatePort())
	ex := stagedExecutor(deps, "create_tenant")

	res, err := ex.Execute(context.Background(), onboardingOctx(), map[string]any{
		"name": "Sunrise Bakery", "vertical": "bakery",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Outcome != domain.OutcomeOK {
		t.Fatalf("outcome = %s (%s)", res.Outcome, res.Summary)
	}
	if want := `staged: create_tenant "Sunrise Bakery" (bakery)`; res.Summary != want {
		t.Errorf("summary = %q, want %q", res.Summary, want)
	}

	m := ob.manifest
	if m == nil || len(m.Steps) != 1 {
		t.Fatalf("manifest steps = %+v", m)
	}
	st := m.Steps[0]
	if st.Op != "create_tenant" || st.Status != domain.ManifestStepProposed || st.ID != "create_tenant-1" {
		t.Errorf("step = %+v", st)
	}
	if m.Tenant.Name != "Sunrise Bakery" || m.Tenant.Vertical != "bakery" {
		t.Errorf("tenant proposal = %+v", m.Tenant)
	}
	if got := res.Output["stepId"]; got != "create_tenant-1" {
		t.Errorf("output stepId = %v", got)
	}

	if len(ob.infos) != 1 {
		t.Fatalf("deltas = %d", len(ob.infos))
	}
	info := ob.infos[0]
	if info.Path != "onboarding" || info.Action.Type != ports.OnboardingStepAction {
		t.Errorf("delta info = %+v", info)
	}
	// Identity-only delta params: step params (business name etc.) stay out.
	raw, _ := json.Marshal(info.Action.Params)
	if strings.Contains(string(raw), "Sunrise") {
		t.Errorf("delta params leak step params: %s", raw)
	}
	for _, key := range []string{"stepId", "op", "status"} {
		if _, ok := info.Action.Params[key]; !ok {
			t.Errorf("delta params missing %q: %s", key, raw)
		}
	}
}

// Re-staging the same natural key REPLACES the step (stable id, fresh
// params, back to proposed) instead of duplicating it; a different key
// appends a second step.
func TestStagedMetaExecutorRestageSemantics(t *testing.T) {
	ob := &fakeOnboardingState{}
	deps := metaDeps(ob, newOpsStatePort())
	ex := stagedExecutor(deps, "define_value_set")
	octx := onboardingOctx()

	first := map[string]any{"slug": "order_pipeline", "name": "Order pipeline",
		"values": []any{map[string]any{"value": "new", "label": "New"}}}
	if _, err := ex.Execute(context.Background(), octx, first); err != nil {
		t.Fatalf("stage 1: %v", err)
	}
	// Simulate a prior failed apply: the re-stage must clear it.
	ob.manifest.Steps[0].Status = domain.ManifestStepFailed
	ob.manifest.Steps[0].Error = "boom"

	second := map[string]any{"slug": "order_pipeline", "name": "Order pipeline",
		"values": []any{
			map[string]any{"value": "new", "label": "New"},
			map[string]any{"value": "done", "label": "Done"},
		}}
	res, err := ex.Execute(context.Background(), octx, second)
	if err != nil {
		t.Fatalf("stage 2: %v", err)
	}
	if !strings.Contains(res.Summary, "2 values") {
		t.Errorf("summary = %q", res.Summary)
	}
	if len(ob.manifest.Steps) != 1 {
		t.Fatalf("restage duplicated the step: %+v", ob.manifest.Steps)
	}
	st := ob.manifest.Steps[0]
	if st.ID != "define_value_set-1" || st.Status != domain.ManifestStepProposed || st.Error != "" {
		t.Errorf("replaced step = %+v", st)
	}
	if vals, _ := st.Params["values"].([]any); len(vals) != 2 {
		t.Errorf("params not replaced: %+v", st.Params)
	}

	// Different natural key → append.
	other := map[string]any{"slug": "request_types", "name": "Request types",
		"values": []any{map[string]any{"value": "call", "label": "Call"}}}
	if _, err := ex.Execute(context.Background(), octx, other); err != nil {
		t.Fatalf("stage 3: %v", err)
	}
	if len(ob.manifest.Steps) != 2 {
		t.Fatalf("distinct key did not append: %+v", ob.manifest.Steps)
	}
	if ob.manifest.Steps[1].ID != "define_value_set-2" {
		t.Errorf("second id = %q", ob.manifest.Steps[1].ID)
	}
}

// An applied step is immutable: staging it again neither mutates the
// manifest nor persists anything, and says so.
func TestStagedMetaExecutorAppliedIsImmutable(t *testing.T) {
	ob := &fakeOnboardingState{manifest: &domain.OnboardingManifest{
		Version: 1,
		Tenant:  domain.ManifestTenant{ID: "tnt-1", Slug: "sunrise-bakery", Name: "Sunrise Bakery", Vertical: "bakery"},
		Steps: []domain.ManifestStep{{
			ID: "create_tenant-1", Op: "create_tenant", Status: domain.ManifestStepApplied,
			Params: map[string]any{"name": "Sunrise Bakery", "vertical": "bakery"},
		}},
	}}
	deps := metaDeps(ob, newOpsStatePort())
	ex := stagedExecutor(deps, "create_tenant")

	res, err := ex.Execute(context.Background(), onboardingOctx(), map[string]any{
		"name": "Renamed", "vertical": "cafe",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Outcome != domain.OutcomeOK || !strings.Contains(res.Summary, "already applied") {
		t.Errorf("result = %+v", res)
	}
	if got := res.Output["staged"]; got != false {
		t.Errorf("output staged = %v", got)
	}
	if len(ob.infos) != 0 {
		t.Errorf("immutable step persisted a delta: %+v", ob.infos)
	}
	if ob.manifest.Tenant.Name != "Sunrise Bakery" {
		t.Errorf("provisioned tenant identity mutated: %+v", ob.manifest.Tenant)
	}
}

// R6: register_user staging strips credential-class keys even if they were
// smuggled into the tool input — nothing credential-shaped may reach the
// persisted manifest or its delta.
func TestRegisterUserStagingScrubsCredentials(t *testing.T) {
	ob := &fakeOnboardingState{}
	deps := metaDeps(ob, newOpsStatePort())
	ex := stagedExecutor(deps, "register_user")

	res, err := ex.Execute(context.Background(), onboardingOctx(), map[string]any{
		"role": "owner", "email": "owner@example.com", "password": "hunter2", "name": "Olive Owner",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Outcome != domain.OutcomeOK {
		t.Fatalf("outcome = %s (%s)", res.Outcome, res.Summary)
	}

	stored, _ := json.Marshal(ob.manifest)
	for _, leak := range []string{"owner@example.com", "hunter2", "Olive"} {
		if strings.Contains(string(stored), leak) {
			t.Errorf("manifest leaks credential %q: %s", leak, stored)
		}
	}
	st := ob.manifest.Steps[0]
	if got := st.Params["role"]; got != "owner" {
		t.Errorf("role param = %v", got)
	}
	if _, ok := st.Params["password"]; ok {
		t.Error("password key survived the scrub")
	}
}

// enable_operations / adopt_presets key on their content: the same set
// re-staged replaces, a different set appends.
func TestEnableOperationsKeysOnInstanceSet(t *testing.T) {
	ob := &fakeOnboardingState{}
	deps := metaDeps(ob, newOpsStatePort())
	ex := stagedExecutor(deps, "enable_operations")
	octx := onboardingOctx()

	ops := map[string]any{"operations": []any{
		map[string]any{"template": "query", "instance": "order_search", "config": map[string]any{"source": "entity", "entity": "order"}},
		map[string]any{"template": "notify", "instance": "notify_staff", "config": map[string]any{"channel": "runtime"}},
	}}
	res, err := ex.Execute(context.Background(), octx, ops)
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	if want := "staged: enable_operations 2 operations: notify_staff, order_search"; res.Summary != want {
		t.Errorf("summary = %q", res.Summary)
	}
	if _, err := ex.Execute(context.Background(), octx, ops); err != nil {
		t.Fatalf("restage: %v", err)
	}
	if len(ob.manifest.Steps) != 1 {
		t.Fatalf("same set duplicated: %+v", ob.manifest.Steps)
	}
}

// RegisterMetaExecutors exposes all 13 meta-ops to the onboarding form's
// data agent — and to nothing else (mode gate lives on the seed rows).
func TestRegisterMetaExecutorsVisibility(t *testing.T) {
	reg := NewRegistry(RegistryConfig{Tenants: &fakeTenants{}, Log: testLogger()})
	RegisterMetaExecutors(reg, metaDeps(&fakeOnboardingState{}, newOpsStatePort()))

	defs := reg.DefinitionsFor(context.Background(), "keepstar-onboarding",
		domain.ModeOnboarding, domain.AgentData, domain.RoleVisitor)
	want := []string{
		"about_keepstar",
		"adopt_presets", "apply_manifest", "create_tenant", "define_automation",
		"define_entity", "define_value_set", "enable_operations",
		"issue_ingest_door", "issue_surface_urls", "register_user", "search_library",
		"seed_demo_data",
	}
	if len(defs) != len(want) {
		t.Fatalf("onboarding defs = %d, want %d", len(defs), len(want))
	}
	for i, def := range defs {
		if def.Name != want[i] {
			t.Errorf("defs[%d] = %q, want %q", i, def.Name, want[i])
		}
	}

	if got := reg.DefinitionsFor(context.Background(), "acme",
		domain.ModeStorefront, domain.AgentData, domain.RoleVisitor); len(got) != 0 {
		t.Errorf("meta ops leaked into the storefront form: %+v", got)
	}
}
