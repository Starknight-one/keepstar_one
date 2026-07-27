package usecases

// The onboarding pipeline response carries the manifest status summary
// (owner 2026-07-28: contextual CTAs — the shell shows Accept only while a
// plan is staged and unapplied). Populated on the onboarding form only;
// storefront/crm turns keep a nil field, so the wire omits it. Reuses the
// fakes from agent1_execute_test.go / pipeline_execute_test.go (same
// package).

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"keepstar_v5/internal/domain"
)

// manifestStateFake layers ports.OnboardingStatePort over the shared
// mockStatePort so PipelineExecute's interface assertion finds the manifest.
type manifestStateFake struct {
	*mockStatePort
	manifest *domain.OnboardingManifest
}

func (s *manifestStateFake) GetOnboarding(context.Context, string) (*domain.OnboardingManifest, error) {
	return s.manifest, nil
}

func (s *manifestStateFake) UpdateOnboarding(_ context.Context, _ string, m *domain.OnboardingManifest, _ domain.DeltaInfo) (int, error) {
	s.manifest = m
	return 1, nil
}

// setupManifestPipeline builds the orchestrator over a manifest-capable
// state. Agent1's LLM ends the turn without tools (mode-agnostic — no
// registry gating in the way); Agent2's LLM ends the turn too.
func setupManifestPipeline(t *testing.T, m *domain.OnboardingManifest) *PipelineExecute {
	t.Helper()
	state := &manifestStateFake{mockStatePort: newMockStatePort(), manifest: m}
	if _, err := state.CreateState(context.Background(), "sess-1"); err != nil {
		t.Fatalf("CreateState: %v", err)
	}
	state.states["sess-1"].Current.Meta = domain.StateMeta{
		Aliases: map[string]string{"tenant_slug": "acme"},
	}

	cat := &fakeCatalog{
		tenant: &domain.Tenant{ID: "tnt-1", Slug: "acme"},
		digest: &domain.CatalogDigest{TotalProducts: 100},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	registry := newLegacyOpsRegistry(state, cat, log)

	llm1 := &fakeLLM{resp: &domain.LLMResponse{StopReason: "end_turn"}}
	agent1 := NewAgent1Execute(llm1, state, cat, registry, NewAgent1PromptCache(cat), log)
	llm2 := &fakeLLM{resp: &domain.LLMResponse{StopReason: "end_turn"}}
	agent2 := NewAgent2Execute(llm2, state, registry, NewPromptCache(noopFieldDefPort{}, noopPresetPort{}, cat, "product"))

	return NewPipelineExecute(agent1, agent2, state, nil, log)
}

func summaryManifest() *domain.OnboardingManifest {
	return &domain.OnboardingManifest{Steps: []domain.ManifestStep{
		{ID: "s1", Op: "create_tenant", Status: domain.ManifestStepProposed},
		{ID: "s2", Op: "define_entity", Status: domain.ManifestStepProposed},
		{ID: "s3", Op: "issue_ingest_door", Status: domain.ManifestStepApplied},
		{ID: "s4", Op: "adopt_presets", Status: domain.ManifestStepFailed, Error: "boom"},
	}}
}

func TestPipelineOnboardingManifestSummary(t *testing.T) {
	uc := setupManifestPipeline(t, summaryManifest())

	resp, err := uc.Execute(context.Background(), PipelineExecuteRequest{
		SessionID:  "sess-1",
		TenantSlug: "acme",
		UserQuery:  "what's in the plan?",
		Mode:       domain.ModeOnboarding,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	s := resp.Manifest
	if s == nil {
		t.Fatalf("onboarding response has no manifest summary")
	}
	if s.Staged != 2 || s.Applied != 1 || s.Failed != 1 || s.Total != 4 {
		t.Fatalf("summary = %+v, want staged 2 / applied 1 / failed 1 / total 4", s)
	}
	if s.FailedStep != "adopt_presets" || s.FailedReason != "boom" {
		t.Fatalf("failure detail = %s/%s, want adopt_presets/boom", s.FailedStep, s.FailedReason)
	}
}

func TestPipelineStorefrontOmitsManifestSummary(t *testing.T) {
	uc := setupManifestPipeline(t, summaryManifest())

	resp, err := uc.Execute(context.Background(), PipelineExecuteRequest{
		SessionID:  "sess-1",
		TenantSlug: "acme",
		UserQuery:  "show me listings",
		Mode:       domain.ModeStorefront,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if resp.Manifest != nil {
		t.Fatalf("storefront turn carries a manifest summary: %+v", resp.Manifest)
	}
}
