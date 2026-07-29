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

// The prompt must teach the full meta-op vocabulary (all 13 wire names) and
// the R4 turn cap — the sequencing depends on both.
func TestOnboardingAgent1PromptCoversMetaOps(t *testing.T) {
	for _, op := range []string{
		"about_keepstar",
		"search_library", "create_tenant", "define_entity", "define_value_set",
		"define_automation", "enable_operations", "adopt_presets",
		"issue_ingest_door", "register_user", "seed_demo_data",
		"issue_surface_urls", "apply_manifest",
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

// Owner flow ruling 2026-07-28: the user's own action (registration submit,
// file upload) IS the approval — the server auto-applies the staged
// manifest. A live bug proved the failure mode: a form submit 409'd with
// "step not ready" after the model SAID "applying now" without calling the
// tool. Both prompts must encode the rule, or the flow regresses to
// model-gated forms.
func TestOnboardingPromptsEncodeUserActionApproval(t *testing.T) {
	for _, want := range []string{"IS approval", "NEVER locked behind apply_manifest"} {
		if !strings.Contains(OnboardingAgent1SystemPrompt, want) {
			t.Errorf("Agent1 onboarding prompt misses the auto-approve rule %q", want)
		}
	}
	// Agent1 must never assume an apply happened without the tool call —
	// the "applying now" bug class.
	if !strings.Contains(OnboardingAgent1SystemPrompt, "CALL the tool") {
		t.Error("Agent1 onboarding prompt misses the call-don't-assume apply rule")
	}
	// Agent2 must never narrate actions as happening.
	if !strings.Contains(ComposeTurnAgent2Addition, "Never claim an action is happening") {
		t.Error("Agent2 compose_turn addition misses the never-claim-actions rule")
	}
}

// Owner flow ruling 2026-07-28, case 1: the visitor who is only exploring
// gets about_keepstar — content into context, answered in the agent's own
// words — and NOTHING staged.
func TestOnboardingPromptsEncodeAboutCase(t *testing.T) {
	if !strings.Contains(OnboardingAgent1SystemPrompt, "Stage NOTHING while the visitor is only exploring") {
		t.Error("Agent1 onboarding prompt misses the explore-don't-stage rule")
	}
	// Agent2 keys case 1 off the <about_keepstar> microcontext envelope
	// (ComposeOnboardingMicrocontextFromResults).
	if !strings.Contains(OnboardingAgent2Addition, "<about_keepstar>") {
		t.Error("Agent2 onboarding addition never names the <about_keepstar> envelope")
	}
	if !strings.Contains(OnboardingAgent2Addition, "OWN WORDS") {
		t.Error("Agent2 onboarding addition misses the own-words rule")
	}
}

// Owner flow ruling 2026-07-28, case 2: operations are presented as
// business-language bullets; the Input/Does/Output spec cards render ONLY
// on an explicit ask. And all user-facing text mirrors the user's language.
func TestOnboardingAgent2CaseTwoShape(t *testing.T) {
	if !strings.Contains(OnboardingAgent2Addition, "BUSINESS") {
		t.Error("Agent2 addition misses the business-bullets rule")
	}
	if !strings.Contains(OnboardingAgent2Addition, "Do NOT render operation_card here") {
		t.Error("Agent2 addition misses the spec-cards-only-on-request rule")
	}
	if !strings.Contains(ComposeTurnAgent2Addition, "language\n    the user writes in") &&
		!strings.Contains(ComposeTurnAgent2Addition, "language the user writes in") {
		t.Error("compose_turn addition misses the mirror-the-user's-language rule")
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
		{
			// enable_operations carries its instance names so Agent2 can
			// write the case-2 business bullets from real names (owner flow
			// ruling 2026-07-28). []any mirrors the JSONB round-trip shape.
			name: "enable_operations instance names",
			m: &domain.OnboardingManifest{Steps: []domain.ManifestStep{
				{Op: "enable_operations", Status: domain.ManifestStepProposed,
					Params: map[string]any{"operations": []any{
						map[string]any{"template": "query", "instance": "find_orders"},
						map[string]any{"template": "notify", "instance": "notify_staff"},
					}}},
			}},
			want: "staged: enable_operations(find_orders, notify_staff)",
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

// ComposeOnboardingMicrocontextFromResults is the drop-in results-aware
// composer: turn ops derive from the structured results, and an OK
// about_keepstar result appends its content in the <about_keepstar>
// envelope — the owner-approved exception to state-mediation that lets
// Agent2 answer "what is this?" in its own words.
func TestComposeOnboardingMicrocontextFromResults(t *testing.T) {
	about := &domain.OperationResult{
		Operation: "about_keepstar",
		Outcome:   domain.OutcomeOK,
		Summary:   "Keepstar is an interface runtime.",
		Output:    map[string]any{"content": "Keepstar is an interface runtime."},
	}

	got := ComposeOnboardingMicrocontextFromResults(nil, []*domain.OperationResult{about})
	want := "turn: about_keepstar | manifest: empty\n<about_keepstar>\nKeepstar is an interface runtime.\n</about_keepstar>"
	if got != want {
		t.Errorf("about turn:\ngot  %q\nwant %q", got, want)
	}

	// Without an about result the output is byte-identical to the
	// name-based composer — no envelope, no drift.
	search := &domain.OperationResult{Operation: "search_library", Outcome: domain.OutcomeOK}
	got = ComposeOnboardingMicrocontextFromResults(nil, []*domain.OperationResult{search, nil})
	if want := ComposeOnboardingMicrocontext(nil, []string{"search_library"}); got != want {
		t.Errorf("plain turn:\ngot  %q\nwant %q", got, want)
	}

	// A failed about load must not inject an envelope.
	bad := &domain.OperationResult{Operation: "about_keepstar", Outcome: domain.OutcomeError, Summary: "error: boom"}
	if got := ComposeOnboardingMicrocontextFromResults(nil, []*domain.OperationResult{bad}); strings.Contains(got, "<about_keepstar>") {
		t.Errorf("error outcome leaked an envelope: %q", got)
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
