package usecases

// Unit tests for the pipeline orchestrator's OnStage hook: nil keeps the
// old path intact, non-nil fires exactly once between agents with the
// StageEvent fields derived from Agent1's outcome (incl. the "empty:
// previous data preserved" case where Count still shows the stale data).

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
	"keepstar_v5/internal/tools"
)

// --- fakes -----------------------------------------------------------------

// noopFieldDefPort returns no published definitions/samples so the Agent2
// prompt builds without a DB.
type noopFieldDefPort struct{}

func (noopFieldDefPort) ListFieldDefinitions(context.Context, string, domain.EntityType) ([]ports.FieldDefinition, error) {
	return nil, nil
}
func (noopFieldDefPort) GetFieldDefinition(context.Context, string, domain.EntityType, string) (*ports.FieldDefinition, error) {
	panic("GetFieldDefinition unused")
}
func (noopFieldDefPort) SampleFieldValues(context.Context, string, domain.EntityType, int) (map[string][]interface{}, error) {
	return nil, nil
}

type noopPresetPort struct{}

func (noopPresetPort) GetPublishedPreset(context.Context, string, string) (*domain.Preset, error) {
	panic("GetPublishedPreset unused")
}
func (noopPresetPort) ListPublishedPresets(context.Context, string) ([]domain.Preset, error) {
	return nil, nil
}

// --- harness ---------------------------------------------------------------

// setupPipeline builds a full orchestrator over fakes: Agent1's LLM always
// calls catalog_search, Agent2's LLM ends the turn without tools.
// stateProducts preload the session (stale data), catalogProducts are what
// the search will find.
func setupPipeline(t *testing.T, stateProducts, catalogProducts []domain.Product) *PipelineExecute {
	t.Helper()
	state := newMockStatePort()
	if _, err := state.CreateState(context.Background(), "sess-1"); err != nil {
		t.Fatalf("CreateState: %v", err)
	}
	state.states["sess-1"].Current.Data = domain.StateData{Products: stateProducts}
	state.states["sess-1"].Current.Meta = domain.StateMeta{
		ProductCount: len(stateProducts),
		Aliases:      map[string]string{"tenant_slug": "acme"},
	}

	cat := &fakeCatalog{
		tenant:   &domain.Tenant{ID: "tnt-1", Slug: "acme"},
		products: catalogProducts,
		digest:   &domain.CatalogDigest{TotalProducts: 100},
	}
	registry := tools.NewRegistry()
	registry.Register(tools.NewCatalogSearchTool(state, cat, nil))
	registry.Register(tools.NewStateFilterTool(state))
	registry.Register(tools.NewHistoryLookupTool(state))

	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	llm1 := &fakeLLM{resp: &domain.LLMResponse{
		StopReason: "tool_use",
		ToolCalls: []domain.ToolCall{
			{ID: "tool-1", Name: "catalog_search", Input: map[string]interface{}{"vector_query": "serums"}},
		},
		Usage: domain.LLMUsage{InputTokens: 100, OutputTokens: 20, CostUSD: 0.001},
	}}
	agent1 := NewAgent1Execute(llm1, state, cat, registry, NewAgent1PromptCache(cat), log)

	llm2 := &fakeLLM{resp: &domain.LLMResponse{StopReason: "end_turn"}}
	agent2 := NewAgent2Execute(llm2, state, registry, NewPromptCache(noopFieldDefPort{}, noopPresetPort{}, "product"))

	return NewPipelineExecute(agent1, agent2, state, nil, log)
}

func pipelineReq(onStage func(StageEvent)) PipelineExecuteRequest {
	return PipelineExecuteRequest{
		SessionID:  "sess-1",
		TenantSlug: "acme",
		UserQuery:  "show me serums",
		OnStage:    onStage,
	}
}

// --- tests -----------------------------------------------------------------

func TestPipelineOnStageNilUnchanged(t *testing.T) {
	uc := setupPipeline(t, nil, []domain.Product{
		{ID: "p1", Name: "Hyaluronic Serum"},
		{ID: "p2", Name: "Niacinamide Serum"},
	})

	resp, err := uc.Execute(context.Background(), pipelineReq(nil))
	if err != nil {
		t.Fatalf("unexpected error with OnStage nil: %v", err)
	}
	if resp == nil {
		t.Fatal("nil response")
	}
	if resp.Agent1Ms < 0 || resp.Agent2Ms < 0 {
		t.Errorf("latency breakdown missing: a1=%d a2=%d", resp.Agent1Ms, resp.Agent2Ms)
	}
}

func TestPipelineOnStageInvokedOnce(t *testing.T) {
	uc := setupPipeline(t, nil, []domain.Product{
		{ID: "p1", Name: "Hyaluronic Serum"},
		{ID: "p2", Name: "Niacinamide Serum"},
	})

	var events []StageEvent
	if _, err := uc.Execute(context.Background(), pipelineReq(func(ev StageEvent) {
		events = append(events, ev)
	})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("OnStage invoked %d times, want exactly 1: %+v", len(events), events)
	}
	ev := events[0]
	if ev.Phase != "data_done" {
		t.Errorf("Phase = %q, want data_done", ev.Phase)
	}
	if ev.Signal != "new_search: 2 items found" {
		t.Errorf("Signal = %q, want new_search: 2 items found", ev.Signal)
	}
	if ev.ToolName != "catalog_search" {
		t.Errorf("ToolName = %q, want catalog_search", ev.ToolName)
	}
	if ev.Count != 2 {
		t.Errorf("Count = %d, want 2", ev.Count)
	}
	if ev.Empty {
		t.Error("Empty = true on a 2-hit search, want false")
	}
	if ev.Bypassed {
		t.Error("Bypassed = true on an LLM-routed search, want false")
	}
	if ev.Agent1Ms < 0 {
		t.Errorf("Agent1Ms = %d, want >= 0", ev.Agent1Ms)
	}
}

func TestPipelineOnStageEmptySearchPreservesStaleCount(t *testing.T) {
	// Session already holds 2 products; the new search finds nothing →
	// tool reports "empty: 0 results, previous data preserved" and
	// ProductsFound still counts the stale data. Empty must flag that.
	stale := []domain.Product{
		{ID: "p1", Name: "Hyaluronic Serum"},
		{ID: "p2", Name: "Niacinamide Serum"},
	}
	uc := setupPipeline(t, stale, nil)

	var events []StageEvent
	if _, err := uc.Execute(context.Background(), pipelineReq(func(ev StageEvent) {
		events = append(events, ev)
	})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("OnStage invoked %d times, want exactly 1", len(events))
	}
	ev := events[0]
	if !ev.Empty {
		t.Error("Empty = false, want true on an empty: tool result")
	}
	if ev.ToolName != "catalog_search" {
		t.Errorf("ToolName = %q, want catalog_search", ev.ToolName)
	}
	if ev.Count != 2 {
		t.Errorf("Count = %d, want 2 (stale data preserved)", ev.Count)
	}
}

func TestPipelineOnStageEmptyStateFilterFlagsEmpty(t *testing.T) {
	// A state filter that matches nothing also preserves data and reports
	// "empty: 0 results from N products, data preserved" — Empty must be
	// true so the widget doesn't announce "Narrowed down to N".
	stale := []domain.Product{
		{ID: "p1", Name: "Hyaluronic Serum"},
		{ID: "p2", Name: "Niacinamide Serum"},
	}
	uc := setupPipeline(t, stale, nil)
	uc.agent1.llm = &fakeLLM{resp: &domain.LLMResponse{
		StopReason: "tool_use",
		ToolCalls: []domain.ToolCall{
			{ID: "tool-1", Name: "_internal_state_filter", Input: map[string]interface{}{"text_match": "no-such-product-xyz"}},
		},
		Usage: domain.LLMUsage{InputTokens: 100, OutputTokens: 20, CostUSD: 0.001},
	}}

	var events []StageEvent
	if _, err := uc.Execute(context.Background(), pipelineReq(func(ev StageEvent) {
		events = append(events, ev)
	})); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("OnStage invoked %d times, want exactly 1", len(events))
	}
	ev := events[0]
	if ev.ToolName != "_internal_state_filter" {
		t.Errorf("ToolName = %q, want _internal_state_filter", ev.ToolName)
	}
	if !ev.Empty {
		t.Error("Empty = false, want true on an empty: state-filter result")
	}
}
