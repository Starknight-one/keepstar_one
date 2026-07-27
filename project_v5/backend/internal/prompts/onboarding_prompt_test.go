package prompts

import (
	"strings"
	"testing"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/engine/presets"
)

// The universality ruling (RUNTIME_SPEC.md §1): the onboarding agent's
// prompt contains ZERO vertical content — vertical shapes live in the
// library, not the prompt. This test is the tripwire: a "helpful" realtor
// example added to the prompt fails the build.
func TestOnboardingAgent1PromptIsVerticalFree(t *testing.T) {
	lower := strings.ToLower(OnboardingAgent1SystemPrompt)
	for _, banned := range []string{
		"realtor", "real estate", "real-estate", "listing", "lead",
		"apartment", "property", "properties", "broker", "showing",
	} {
		if strings.Contains(lower, banned) {
			t.Errorf("Agent1 onboarding prompt hardcodes vertical term %q", banned)
		}
	}
}

// The prompt must teach the full meta-op vocabulary (all 11 wire names) and
// the R4 turn cap — the sequencing depends on both.
func TestOnboardingAgent1PromptCoversMetaOps(t *testing.T) {
	for _, op := range []string{
		"search_library", "create_tenant", "define_entity", "define_value_set",
		"define_automation", "enable_operations", "adopt_presets",
		"issue_ingest_door", "register_user", "issue_surface_urls", "apply_manifest",
	} {
		if !strings.Contains(OnboardingAgent1SystemPrompt, op) {
			t.Errorf("Agent1 onboarding prompt never names %q", op)
		}
	}
	if !strings.Contains(OnboardingAgent1SystemPrompt, "8 tool calls") {
		t.Error("Agent1 onboarding prompt does not state the 8-call cap (R4)")
	}
	// R6: the credential rule must be stated.
	if !strings.Contains(OnboardingAgent1SystemPrompt, "password") {
		t.Error("Agent1 onboarding prompt misses the credential rule (R6)")
	}
}

// Every preset the Agent2 choreography names must exist in the system
// registry — a renamed or missing seed otherwise degrades silently to a
// warn+empty render at demo time. surface_links and manifest_summary are
// surfaces-lane deliverables still in flight (R20); they are allowed as
// PENDING here and this test starts enforcing them the moment they land.
func TestOnboardingAgent2AdditionPresetsExist(t *testing.T) {
	named := []string{
		"uploader_card", "design_system_preview", "operation_card",
		"registration_form", "success_plaque", "surface_links", "manifest_summary",
	}
	full := ComposeTurnAgent2Addition + OnboardingAgent2Addition
	for _, name := range named {
		if !strings.Contains(full, name) {
			t.Errorf("Agent2 onboarding addition never names preset %q", name)
			continue
		}
		if _, ok := presets.SystemPresetSeeds[name]; !ok {
			t.Errorf("Agent2 onboarding addition names preset %q that is not in the system registry", name)
		}
	}
	// The synthetic replicate sources (R23) the choreography relies on.
	for _, slug := range []string{"opCard", "manifestStep", "surfaceLink"} {
		if !strings.Contains(full, slug) {
			t.Errorf("Agent2 addition never names synthetic set %q", slug)
		}
	}
}

func TestComposeOnboardingMicrocontext(t *testing.T) {
	manifest := &domain.OnboardingManifest{Steps: []domain.ManifestStep{
		{Op: "create_tenant", Status: domain.ManifestStepApplied},
		{Op: "define_value_set", Status: domain.ManifestStepApplied,
			Params: map[string]any{"slug": "order_pipeline"}},
		{Op: "define_entity", Status: domain.ManifestStepProposed,
			Params: map[string]any{"entity": map[string]any{"slug": "order"}}},
		{Op: "register_user", Status: domain.ManifestStepAccepted},
		{Op: "adopt_presets", Status: domain.ManifestStepFailed, Error: "unresolved binding"},
	}}

	cases := []struct {
		name string
		m    *domain.OnboardingManifest
		ops  []string
		want string
	}{
		{
			name: "nil manifest",
			m:    nil,
			want: "manifest: empty",
		},
		{
			name: "nil manifest with turn ops",
			m:    nil,
			ops:  []string{"search_library"},
			want: "turn: search_library | manifest: empty",
		},
		{
			name: "mixed statuses",
			m:    manifest,
			ops:  []string{"define_entity", "apply_manifest"},
			want: "turn: define_entity, apply_manifest | staged: define_entity(order) | applied: 2/5 | waiting: register_user | failed: adopt_presets (unresolved binding)",
		},
		{
			name: "staged only",
			m: &domain.OnboardingManifest{Steps: []domain.ManifestStep{
				{Op: "create_tenant", Status: domain.ManifestStepProposed},
				{Op: "define_automation", Status: domain.ManifestStepProposed,
					Params: map[string]any{"name": "notify_on_order"}},
			}},
			want: "staged: create_tenant, define_automation(notify_on_order)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ComposeOnboardingMicrocontext(tc.m, tc.ops); got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

// OnboardingAgent2SystemPrompt keeps the storefront base intact and appends
// both blocks — the storefront form's prompt bytes stay untouched (§3.1
// stability warning).
func TestOnboardingAgent2SystemPromptAssembly(t *testing.T) {
	got := OnboardingAgent2SystemPrompt()
	if !strings.HasPrefix(got, Agent2SystemPrompt) {
		t.Error("assembled prompt does not start with the base Agent2 prompt")
	}
	if !strings.Contains(got, "COMPOSE_TURN") || !strings.Contains(got, "ONBOARDING CHOREOGRAPHY") {
		t.Error("assembled prompt misses an addition block")
	}
}
