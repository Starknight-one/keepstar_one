package operations

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/engine/presets"
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

// Stage-time repair of adopt_presets (handoff tail #4). The model invents
// preset names (live smoke: "lead_cards"). Apply-time only SKIPS them
// (f84e0a0), so the workspace goes live missing surfaces the conversation
// promised and nobody is left in the loop — hence catching it at STAGE time
// (V2_SPEC L11).
//
// But catching it must not mean dropping the whole step. Agent1 has no
// in-turn re-prompt (pipeline_execute.go calls the model once, runs the
// emitted batch, returns), and the auto-apply triggers — registration submit,
// upload completion — fire before the model gets another turn. So an
// all-or-nothing rejection does NOT buy a corrected re-stage: it leaves the
// manifest with no adopt_presets step at all and the tenant applies with zero
// presets, which is strictly worse than the apply-time skip it replaced.
//
// The contract these cases pin: keep every real name, drop the invented ones
// from the PERSISTED params (the apply must never carry a hallucination
// forward), and put the offenders in the OK summary so the miss is visible
// rather than silent. Reject only when nothing real is left.
func TestStagedAdoptPresetsRepairsNames(t *testing.T) {
	library := []string{"booking_form", "lead_table", "surface_links"}

	cases := []struct {
		name        string
		library     func() []string
		presets     []any
		wantOutcome domain.OpOutcome
		wantStaged  bool
		// wantPresets is the presets[] the STEP must persist (nil = don't check).
		wantPresets []string
		// wantInSummary are substrings the LLM needs to self-correct.
		wantInSummary []string
	}{
		{
			name:        "every name in the library stages untouched",
			library:     func() []string { return library },
			presets:     []any{"lead_table", "booking_form"},
			wantOutcome: domain.OutcomeOK,
			wantStaged:  true,
			wantPresets: []string{"lead_table", "booking_form"},
		},
		{
			name:        "one invented name is dropped, the real siblings still stage",
			library:     func() []string { return library },
			presets:     []any{"lead_table", "lead_cards"},
			wantOutcome: domain.OutcomeOK,
			wantStaged:  true,
			wantPresets: []string{"lead_table"},
			// The offender AND the candidates: the model reads this on its
			// next turn and can re-stage what it actually meant.
			wantInSummary: []string{"lead_cards", "booking_form", "lead_table", "surface_links"},
		},
		{
			// Nothing real left — the apply cannot honour an empty presets[],
			// so this one IS a rejection.
			name:          "every name invented rejects the staging",
			library:       func() []string { return library },
			presets:       []any{"deal_grid", "pipeline_board"},
			wantOutcome:   domain.OutcomeInvalid,
			wantStaged:    false,
			wantInSummary: []string{"deal_grid", "pipeline_board", "lead_table"},
		},
		{
			// A library that cannot be read is an infrastructure problem, not
			// proof that every requested name is wrong — dropping them all
			// there would dead-end the flow.
			name:        "empty library falls through to the apply-time belt",
			library:     func() []string { return nil },
			presets:     []any{"lead_cards"},
			wantOutcome: domain.OutcomeOK,
			wantStaged:  true,
			wantPresets: []string{"lead_cards"},
		},
		{
			name:        "unwired library falls through to the apply-time belt",
			library:     nil,
			presets:     []any{"lead_cards"},
			wantOutcome: domain.OutcomeOK,
			wantStaged:  true,
			wantPresets: []string{"lead_cards"},
		},
		{
			name:        "no presets at all is invalid",
			library:     func() []string { return library },
			presets:     []any{},
			wantOutcome: domain.OutcomeInvalid,
			wantStaged:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ob := &fakeOnboardingState{}
			deps := metaDeps(ob, newOpsStatePort())
			deps.PresetLibrary = tc.library
			ex := stagedExecutor(deps, "adopt_presets")

			res, err := ex.Execute(context.Background(), onboardingOctx(),
				map[string]any{"presets": tc.presets})
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if res.Outcome != tc.wantOutcome {
				t.Fatalf("outcome = %s, want %s (%s)", res.Outcome, tc.wantOutcome, res.Summary)
			}
			for _, want := range tc.wantInSummary {
				if !strings.Contains(res.Summary, want) {
					t.Errorf("summary %q does not carry %q — the model cannot self-correct", res.Summary, want)
				}
			}
			staged := ob.manifest != nil && len(ob.manifest.Steps) > 0
			if staged != tc.wantStaged {
				t.Fatalf("staged = %v, want %v (manifest %+v)", staged, tc.wantStaged, ob.manifest)
			}
			if !tc.wantStaged && len(ob.infos) != 0 {
				t.Errorf("a rejected staging still wrote %d delta(s)", len(ob.infos))
			}
			if tc.wantPresets == nil {
				return
			}
			got := cfgStringSlice(ob.manifest.Steps[0].Params, "presets")
			if strings.Join(got, ",") != strings.Join(tc.wantPresets, ",") {
				t.Errorf("staged presets = %v, want %v — the apply must not carry an "+
					"invented name forward", got, tc.wantPresets)
			}
		})
	}
}

// A repaired staging must still be REPLACEABLE. The step's natural key is
// built from the sanitized presets[], and those go through JSONB, so a
// re-stage of the same repaired set has to match its own predecessor instead
// of appending a second adopt_presets step (two steps = the apply adopts
// twice and the digest lies about the plan).
func TestStagedAdoptPresetsRepairedStepIsReStageable(t *testing.T) {
	ob := &fakeOnboardingState{}
	deps := metaDeps(ob, newOpsStatePort())
	deps.PresetLibrary = func() []string { return []string{"booking_form", "lead_table"} }
	ex := stagedExecutor(deps, "adopt_presets")

	for i := 0; i < 2; i++ {
		res, err := ex.Execute(context.Background(), onboardingOctx(),
			map[string]any{"presets": []any{"lead_table", "lead_cards"}})
		if err != nil {
			t.Fatalf("stage %d: %v", i+1, err)
		}
		if res.Outcome != domain.OutcomeOK {
			t.Fatalf("stage %d outcome = %s (%s)", i+1, res.Outcome, res.Summary)
		}
	}
	if n := len(ob.manifest.Steps); n != 1 {
		t.Fatalf("adopt_presets steps = %d, want 1 — the repaired key does not "+
			"survive a round-trip, so re-staging duplicates the step", n)
	}
}

// The summary must carry the LIVE library, not a copy that can drift: a name
// the seeds advertise has to stage, and one they do not has to be dropped and
// named, against presets.SystemPresetNames as wired in main.go.
func TestStagedAdoptPresetsUsesTheRealLibrary(t *testing.T) {
	ob := &fakeOnboardingState{}
	deps := metaDeps(ob, newOpsStatePort())
	deps.PresetLibrary = presets.SystemPresetNames
	ex := stagedExecutor(deps, "adopt_presets")

	res, err := ex.Execute(context.Background(), onboardingOctx(),
		map[string]any{"presets": []any{"lead_table", "surface_links"}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Outcome != domain.OutcomeOK {
		t.Fatalf("real library rejected its own presets: %s", res.Summary)
	}
	if got := cfgStringSlice(ob.manifest.Steps[0].Params, "presets"); len(got) != 2 {
		t.Fatalf("real library dropped its own presets: %v", got)
	}

	ob2 := &fakeOnboardingState{}
	deps2 := metaDeps(ob2, newOpsStatePort())
	deps2.PresetLibrary = presets.SystemPresetNames
	res2, err := stagedExecutor(deps2, "adopt_presets").Execute(context.Background(), onboardingOctx(),
		map[string]any{"presets": []any{"lead_cards"}})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res2.Outcome != domain.OutcomeInvalid {
		t.Fatalf("outcome = %s, want invalid — nothing real was left", res2.Outcome)
	}
	if !strings.Contains(res2.Summary, "lead_table") {
		t.Errorf("summary does not advertise the real library: %q", res2.Summary)
	}
}
