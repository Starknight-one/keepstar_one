package operations

import (
	"context"
	"strings"
	"testing"

	"keepstar_v5/internal/domain"
)

// about_keepstar is the owner-approved EXCEPTION to state-mediation: the
// doc content returns INSIDE the OperationResult — Summary is what
// ToToolResult puts into the agent's context — so the agent answers in its
// own words. Nothing is staged, nothing touches state or the manifest.
func TestAboutKeepstarReturnsDocContent(t *testing.T) {
	ex := NewAboutKeepstarExecutor()

	res, err := ex.Execute(context.Background(), onboardingOctx(), map[string]any{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Outcome != domain.OutcomeOK {
		t.Fatalf("outcome = %s, want ok", res.Outcome)
	}
	if res.Summary == "" {
		t.Fatal("Summary empty — the doc content IS the context payload")
	}
	// The content is the embedded canonical doc, not a placeholder.
	for _, marker := range []string{"interface runtime", "storefront", "CRM"} {
		if !strings.Contains(res.Summary, marker) {
			t.Errorf("about content misses %q", marker)
		}
	}
	if got, _ := res.Output["content"].(string); got != res.Summary {
		t.Error("Output.content must equal Summary (one content, two views)")
	}
	// ToToolResult bridges Summary → tool_result content, non-error.
	tr := res.ToToolResult("tu-1")
	if tr.IsError || tr.Content != res.Summary {
		t.Errorf("tool_result = IsError %v, content match %v", tr.IsError, tr.Content == res.Summary)
	}
}

// The owner's ~2500-token ceiling: the embedded doc must fit the cap
// (a doc edit that overflows fails the build here, not silently in prod),
// and the executor reports its size.
func TestAboutKeepstarDocWithinCap(t *testing.T) {
	if len(aboutKeepstarDoc) == 0 {
		t.Fatal("embedded about doc is empty")
	}
	if len(aboutKeepstarDoc) > aboutContentCharCap {
		t.Fatalf("about doc is %d chars — over the %d cap (~2500 tokens); trim the doc", len(aboutKeepstarDoc), aboutContentCharCap)
	}
	res, err := NewAboutKeepstarExecutor().Execute(context.Background(), onboardingOctx(), map[string]any{"topic": "pricing"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Metadata["topic"] != "pricing" {
		t.Errorf("metadata = %+v", res.Metadata)
	}
}

func TestCapAboutContent(t *testing.T) {
	if got := capAboutContent("short doc", 100); got != "short doc" {
		t.Errorf("under cap changed: %q", got)
	}
	long := strings.Repeat("line one two three\n", 50)
	got := capAboutContent(long, 100)
	if len(got) > 100+len("\n[…truncated]") {
		t.Errorf("over cap not trimmed: %d chars", len(got))
	}
	if !strings.HasSuffix(got, "[…truncated]") {
		t.Errorf("truncation not marked: %q", got)
	}
}

// Universality: the canonical about-doc explains the runtime for ANY
// business — vertical shapes are library content, never doc content (same
// tripwire as the onboarding prompts).
func TestAboutKeepstarDocIsVerticalFree(t *testing.T) {
	lower := strings.ToLower(aboutKeepstarDoc)
	for _, banned := range []string{
		"realtor", "real estate", "real-estate", "listing", "broker", "showing",
	} {
		if strings.Contains(lower, banned) {
			t.Errorf("about doc hardcodes vertical term %q", banned)
		}
	}
}

// Template gates: meta kind, onboarding form only, visitor role — reachable
// exactly where onboarding visitors chat.
func TestAboutKeepstarTemplateGates(t *testing.T) {
	tmpl := NewAboutKeepstarExecutor().TemplateRow()
	if tmpl.Name != "about_keepstar" || tmpl.Kind != domain.KindMeta {
		t.Fatalf("template = %s/%s", tmpl.Name, tmpl.Kind)
	}
	if len(tmpl.Modes) != 1 || tmpl.Modes[0] != domain.ModeOnboarding {
		t.Errorf("modes = %v, want [onboarding]", tmpl.Modes)
	}
	if tmpl.MinRole != domain.RoleVisitor || !tmpl.AutoEnabled {
		t.Errorf("min_role %s auto %v, want visitor/auto-enabled", tmpl.MinRole, tmpl.AutoEnabled)
	}
}
