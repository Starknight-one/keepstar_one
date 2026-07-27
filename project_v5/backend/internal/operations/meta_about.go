package operations

import (
	"context"
	_ "embed"
	"strings"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// AboutKeepstarExecutor — the second IMMEDIATE meta-operation (owner ruling
// 2026-07-28): when an onboarding visitor is just chatting — "what is this?",
// "how does it work?", "what do I get?" — the agent loads the canonical
// about-doc and answers from it in its own words.
//
// This is the owner-approved EXCEPTION to state-mediation: the doc content
// returns INSIDE the OperationResult (Summary → tool_result → the agent's
// context), never through a state zone or a rendered widget. Follow-up
// questions go the same way — call the operation again. Nothing is staged,
// nothing mutates the world; the manifest is untouched.
//
// The doc is package-embedded (single binary, no runtime file path) and
// capped at ~2500 tokens (aboutContentCharCap) so a future doc edit cannot
// silently flood the context window.

//go:embed docs/about_keepstar.md
var aboutKeepstarDoc string

// aboutContentCharCap bounds the returned content: ~4 chars/token puts
// 10 000 chars at the owner's ~2500-token ceiling.
const aboutContentCharCap = 10000

// AboutKeepstarExecutor implements ports.Executor for about_keepstar.
type AboutKeepstarExecutor struct {
	tmpl domain.OperationTemplate
}

var _ ports.Executor = (*AboutKeepstarExecutor)(nil)

// NewAboutKeepstarExecutor builds the executor over its §3.1 seed row.
func NewAboutKeepstarExecutor() *AboutKeepstarExecutor {
	return &AboutKeepstarExecutor{tmpl: seedRow("about_keepstar")}
}

func (e *AboutKeepstarExecutor) Template() domain.OperationSpec        { return e.tmpl.Spec() }
func (e *AboutKeepstarExecutor) TemplateRow() domain.OperationTemplate { return e.tmpl }

func (e *AboutKeepstarExecutor) SpecForTenant(context.Context, domain.Tenant, map[string]any) (domain.OperationSpec, error) {
	return e.tmpl.Spec(), nil
}

// Execute returns the capped about-doc content. The optional "topic" input
// is advisory only — the whole doc always returns and the agent selects
// what the question needs; it is echoed in Metadata for trace readability.
func (e *AboutKeepstarExecutor) Execute(_ context.Context, _ domain.OperationContext, input map[string]any) (*domain.OperationResult, error) {
	content := capAboutContent(aboutKeepstarDoc, aboutContentCharCap)
	meta := map[string]any{"chars": len(content)}
	if topic := stringInput(input, "topic"); topic != "" {
		meta["topic"] = topic
	}
	return &domain.OperationResult{
		Kind:     e.tmpl.Kind,
		Outcome:  domain.OutcomeOK,
		Count:    1,
		Summary:  content,
		Output:   map[string]any{"content": content},
		Metadata: meta,
	}, nil
}

// capAboutContent trims oversized content at the last line boundary before
// the cap (falling back to a hard cut), marking the truncation so the agent
// never mistakes a cut doc for a complete one. No-op under the cap.
func capAboutContent(doc string, limit int) string {
	doc = strings.TrimSpace(doc)
	if len(doc) <= limit {
		return doc
	}
	cut := doc[:limit]
	if i := strings.LastIndexByte(cut, '\n'); i > 0 {
		cut = cut[:i]
	}
	return strings.TrimSpace(cut) + "\n[…truncated]"
}
