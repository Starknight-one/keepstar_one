package operations

import (
	"context"
	"strings"
	"testing"

	"keepstar_v5/internal/domain"
)

// applyFixture returns a staged manifest (what GetOnboarding sees before
// the run) and the applier's post-run result.
func applyFixture() (*domain.OnboardingManifest, *domain.OnboardingManifest) {
	staged := &domain.OnboardingManifest{Version: 1, Steps: []domain.ManifestStep{
		{ID: "create_tenant-1", Op: "create_tenant", Status: domain.ManifestStepProposed},
		{ID: "issue_ingest_door-2", Op: "issue_ingest_door", Status: domain.ManifestStepProposed},
		{ID: "register_user-3", Op: "register_user", Status: domain.ManifestStepProposed},
		{ID: "issue_surface_urls-4", Op: "issue_surface_urls", Status: domain.ManifestStepProposed},
	}}
	applied := &domain.OnboardingManifest{
		Version: 1,
		Tenant:  domain.ManifestTenant{ID: "tnt-1", Slug: "sunrise-bakery"},
		Steps: []domain.ManifestStep{
			{ID: "create_tenant-1", Op: "create_tenant", Status: domain.ManifestStepApplied,
				Result: map[string]any{"tenantId": "tnt-1", "slug": "sunrise-bakery"}},
			{ID: "issue_ingest_door-2", Op: "issue_ingest_door", Status: domain.ManifestStepAccepted,
				Result: map[string]any{"token": "tok-1"}},
			{ID: "register_user-3", Op: "register_user", Status: domain.ManifestStepAccepted},
			{ID: "issue_surface_urls-4", Op: "issue_surface_urls", Status: domain.ManifestStepAccepted,
				Result: map[string]any{"waiting": "pending steps: issue_ingest_door, register_user"}},
		},
	}
	return staged, applied
}

// A partial apply digests honestly: applied count, waiting ops, per-step
// line — and refreshes the synthetic manifestStep set the manifest_summary
// preset binds.
func TestApplyManifestDigestsWaitingRun(t *testing.T) {
	staged, applied := applyFixture()
	ob := &fakeOnboardingState{manifest: staged}
	state := newOpsStatePort()
	applier := &fakeApplier{manifest: applied}
	deps := metaDeps(ob, state)
	deps.Applier = applier
	ex := NewApplyManifestExecutor(deps)

	res, err := ex.Execute(context.Background(), onboardingOctx(), map[string]any{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if applier.calls != 1 {
		t.Fatalf("applier calls = %d", applier.calls)
	}
	if res.Outcome != domain.OutcomeOK {
		t.Fatalf("outcome = %s (%s)", res.Outcome, res.Summary)
	}
	for _, want := range []string{"applied 1/4", "waiting on issue_ingest_door, register_user, issue_surface_urls", "slug=sunrise-bakery"} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("summary %q missing %q", res.Summary, want)
		}
	}
	if res.Output["applied"] != 1 || res.Output["total"] != 4 {
		t.Errorf("output = %+v", res.Output)
	}

	var set *domain.EntitySet
	for i := range state.state.Current.Data.Entities {
		if state.state.Current.Data.Entities[i].Slug == "manifestStep" {
			set = &state.state.Current.Data.Entities[i]
		}
	}
	if set == nil || !set.Synthetic || len(set.Records) != 4 {
		t.Fatalf("manifestStep set = %+v", set)
	}
	rec := set.Records[0]
	if rec.Data["op"] != "create_tenant" || rec.Data["status"] != "applied" || rec.Data["statusLabel"] != "Done" {
		t.Errorf("manifestStep record = %+v", rec.Data)
	}
	if rec.Data["title"] == "" || rec.Data["title"] == "create_tenant" {
		t.Errorf("title not resolved from seed row: %v", rec.Data["title"])
	}
	// No URLs issued yet → no surfaceLink set.
	for _, s := range state.state.Current.Data.Entities {
		if s.Slug == "surfaceLink" {
			t.Error("surfaceLink set written before URLs were issued")
		}
	}
}

// The fully-applied run surfaces both URLs (the handover beat) and writes
// the surfaceLink set the surface_links preset binds.
func TestApplyManifestIssuesURLs(t *testing.T) {
	staged, applied := applyFixture()
	for i := range applied.Steps {
		applied.Steps[i].Status = domain.ManifestStepApplied
	}
	applied.Steps[3].Result = map[string]any{
		"storefrontUrl": "https://v5.example/s/sunrise-bakery",
		"crmUrl":        "https://v5.example/crm/sunrise-bakery?k=tok",
	}
	ob := &fakeOnboardingState{manifest: staged}
	state := newOpsStatePort()
	deps := metaDeps(ob, state)
	deps.Applier = &fakeApplier{manifest: applied}
	ex := NewApplyManifestExecutor(deps)

	res, err := ex.Execute(context.Background(), onboardingOctx(), map[string]any{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Outcome != domain.OutcomeOK || !strings.Contains(res.Summary, "fully applied 4/4") {
		t.Fatalf("result = %+v", res)
	}
	if !strings.Contains(res.Summary, "https://v5.example/s/sunrise-bakery") ||
		!strings.Contains(res.Summary, "https://v5.example/crm/sunrise-bakery?k=tok") {
		t.Errorf("summary misses URLs: %q", res.Summary)
	}
	if res.Output["storefrontUrl"] != "https://v5.example/s/sunrise-bakery" {
		t.Errorf("output = %+v", res.Output)
	}

	var links *domain.EntitySet
	for i := range state.state.Current.Data.Entities {
		if state.state.Current.Data.Entities[i].Slug == "surfaceLink" {
			links = &state.state.Current.Data.Entities[i]
		}
	}
	if links == nil || len(links.Records) != 2 {
		t.Fatalf("surfaceLink set = %+v", links)
	}
	if links.Records[0].Data["url"] != "https://v5.example/s/sunrise-bakery" ||
		links.Records[1].Data["surface"] != "crm" {
		t.Errorf("surfaceLink records = %+v", links.Records)
	}
}

// A failed step is an error outcome (IsError tool_result → the agent
// relays it) carrying the op and its error.
func TestApplyManifestFailedStep(t *testing.T) {
	staged, applied := applyFixture()
	applied.Steps[1].Status = domain.ManifestStepFailed
	applied.Steps[1].Error = "admin unreachable"
	ob := &fakeOnboardingState{manifest: staged}
	deps := metaDeps(ob, newOpsStatePort())
	deps.Applier = &fakeApplier{manifest: applied}
	ex := NewApplyManifestExecutor(deps)

	res, err := ex.Execute(context.Background(), onboardingOctx(), map[string]any{"upTo": "register_user-3"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Outcome != domain.OutcomeError {
		t.Fatalf("outcome = %s", res.Outcome)
	}
	if !strings.Contains(res.Summary, "halted at issue_ingest_door") || !strings.Contains(res.Summary, "admin unreachable") {
		t.Errorf("summary = %q", res.Summary)
	}
	if deps.Applier.(*fakeApplier).lastUpTo != "register_user-3" {
		t.Errorf("upTo = %q", deps.Applier.(*fakeApplier).lastUpTo)
	}
}

// Guard rails: applying with nothing staged is a self-correctable invalid;
// a missing applier is a loud wiring error — never a silent no-op.
func TestApplyManifestGuards(t *testing.T) {
	deps := metaDeps(&fakeOnboardingState{}, newOpsStatePort())
	applier := &fakeApplier{}
	deps.Applier = applier
	ex := NewApplyManifestExecutor(deps)

	res, err := ex.Execute(context.Background(), onboardingOctx(), map[string]any{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Outcome != domain.OutcomeInvalid || !strings.Contains(res.Summary, "nothing staged") {
		t.Errorf("empty-session result = %+v", res)
	}
	if applier.calls != 0 {
		t.Errorf("applier ran on an empty session")
	}

	staged, _ := applyFixture()
	deps2 := metaDeps(&fakeOnboardingState{manifest: staged}, newOpsStatePort())
	res, err = NewApplyManifestExecutor(deps2).Execute(context.Background(), onboardingOctx(), map[string]any{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Outcome != domain.OutcomeError || !strings.Contains(res.Summary, "not wired") {
		t.Errorf("nil-applier result = %+v", res)
	}
}
