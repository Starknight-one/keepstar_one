package usecases

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// fakeOpRegistry captures Execute calls (ports.OperationRegistry).
type fakeOpRegistry struct {
	lastOctx domain.OperationContext
	lastCall domain.ToolCall
	calls    int
	result   *domain.OperationResult
	err      error
}

var _ ports.OperationRegistry = (*fakeOpRegistry)(nil)

func (f *fakeOpRegistry) RegisterExecutor(domain.OperationKind, ports.Executor) {}
func (f *fakeOpRegistry) DefinitionsFor(context.Context, string, domain.PipelineMode, domain.AgentPlane, domain.Role) []domain.ToolDefinition {
	return nil
}
func (f *fakeOpRegistry) Get(context.Context, string, string) (*domain.OperationSpec, error) {
	return nil, nil
}
func (f *fakeOpRegistry) Execute(_ context.Context, octx domain.OperationContext, call domain.ToolCall) (*domain.OperationResult, error) {
	f.calls++
	f.lastOctx = octx
	f.lastCall = call
	if f.err != nil {
		return nil, f.err
	}
	if f.result != nil {
		return f.result, nil
	}
	return &domain.OperationResult{Outcome: domain.OutcomeOK, Summary: "ok"}, nil
}
func (f *fakeOpRegistry) InvalidateTenant(string) {}

func newTestRunner(reg ports.OperationRegistry) *OperationRunner {
	return NewOperationRunner(reg, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// The v1 automation-facing allowlist is notify only (§4.2) — anything else
// is refused before it can reach the registry.
func TestOperationRunnerAllowlist(t *testing.T) {
	reg := &fakeOpRegistry{}
	r := newTestRunner(reg)

	if err := r.Run(context.Background(), "tnt-1", "acme", "create_record", nil, nil); err == nil {
		t.Fatal("non-notify operation must be refused")
	}
	if reg.calls != 0 {
		t.Fatalf("refused operation must never reach the registry")
	}
}

// Run substitutes {field} placeholders from the event payload (snapshot
// fields + payload scalars), executes under the synthetic system context
// and leaves unknown placeholders visible.
func TestOperationRunnerSubstitutesAndExecutes(t *testing.T) {
	reg := &fakeOpRegistry{}
	r := newTestRunner(reg)

	err := r.Run(context.Background(), "tnt-1", "acme", "notify",
		map[string]any{
			"title": "Showing request — {refTitle}",
			"body":  "{name} · {phone}",
			"ref":   "{recordId}",
			"note":  "{unknownField}",
		},
		map[string]any{
			"actor":    "visitor:s1",
			"recordId": "rec-1",
			"snapshot": map[string]any{"name": "Ann", "phone": "+14155550101", "refTitle": "Sea View 2BR"},
		})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if reg.lastOctx.Role != domain.RoleSystem {
		t.Errorf("role = %q, want system", reg.lastOctx.Role)
	}
	if reg.lastOctx.TenantID != "tnt-1" || reg.lastOctx.TenantSlug != "acme" {
		t.Errorf("tenant identity = %q/%q", reg.lastOctx.TenantID, reg.lastOctx.TenantSlug)
	}
	if reg.lastOctx.ActorID != "system:automation" {
		t.Errorf("actor = %q", reg.lastOctx.ActorID)
	}
	in := reg.lastCall.Input
	if in["title"] != "Showing request — Sea View 2BR" {
		t.Errorf("title = %v", in["title"])
	}
	if in["body"] != "Ann · +14155550101" {
		t.Errorf("body = %v", in["body"])
	}
	if in["ref"] != "rec-1" {
		t.Errorf("ref = %v", in["ref"])
	}
	if in["note"] != "{unknownField}" {
		t.Errorf("unknown placeholder must stay visible: %v", in["note"])
	}
}

// The diff shape (updates / transitions) substitutes from the after-image.
func TestOperationRunnerSubstitutesFromDiffAfter(t *testing.T) {
	reg := &fakeOpRegistry{}
	r := newTestRunner(reg)

	err := r.Run(context.Background(), "tnt-1", "acme", "notify",
		map[string]any{"title": "Lead now {status}"},
		map[string]any{
			"actor": "user:u1",
			"diff": map[string]any{
				"before": map[string]any{"status": "new"},
				"after":  map[string]any{"status": "contacted"},
			},
		})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := reg.lastCall.Input["title"]; got != "Lead now contacted" {
		t.Errorf("title = %v", got)
	}
}

// A non-ok outcome is an error to the dispatcher — DispatchEvent logs it
// and moves on; it must not look like success.
func TestOperationRunnerNonOKOutcomeIsError(t *testing.T) {
	reg := &fakeOpRegistry{result: &domain.OperationResult{
		Outcome: domain.OutcomeDenied, Summary: "denied: unknown operation",
	}}
	r := newTestRunner(reg)

	if err := r.Run(context.Background(), "tnt-1", "acme", "notify",
		map[string]any{"title": "x"}, nil); err == nil {
		t.Fatal("denied outcome must surface as an error")
	}
}
