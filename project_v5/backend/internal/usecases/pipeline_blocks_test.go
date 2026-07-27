package usecases

// Integration test for the streamed-turn seam (§4.7 + final owner decision
// 3) through the REAL orchestrator: PipelineExecute installs the
// TurnBlockCollector on the Agent2 ctx, the registry choke point carries it
// into compose_turn, each block reaches OnBlock AS PRODUCED, and the
// aggregated ordered list lands on Response.Blocks. Reuses the fakes from
// agent1_execute_test.go / pipeline_execute_test.go (same package).

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/operations"
	"keepstar_v5/internal/tools"
)

// setupComposePipeline builds the full orchestrator on the crm form:
// Agent1's LLM calls catalog_search, Agent2's LLM calls compose_turn with
// two text blocks (text-only keeps the preset ports out of the picture —
// render-path coverage lives in tool_compose_turn_test.go).
func setupComposePipeline(t *testing.T) *PipelineExecute {
	t.Helper()
	state := newMockStatePort()
	if _, err := state.CreateState(context.Background(), "sess-1"); err != nil {
		t.Fatalf("CreateState: %v", err)
	}
	state.states["sess-1"].Current.Meta = domain.StateMeta{
		Aliases: map[string]string{"tenant_slug": "acme"},
	}

	cat := &fakeCatalog{
		tenant:   &domain.Tenant{ID: "tnt-1", Slug: "acme"},
		products: []domain.Product{{ID: "p1", Name: "Hyaluronic Serum"}},
		digest:   &domain.CatalogDigest{TotalProducts: 100},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	registry := newLegacyOpsRegistry(state, cat, log)
	registry.RegisterExecutor(domain.KindVisual,
		operations.WrapComposeTurn(tools.NewComposeTurnTool(state, nil, nil)))

	llm1 := &fakeLLM{resp: &domain.LLMResponse{
		StopReason: "tool_use",
		ToolCalls: []domain.ToolCall{
			{ID: "tool-1", Name: "catalog_search", Input: map[string]interface{}{"vector_query": "serums"}},
		},
		Usage: domain.LLMUsage{InputTokens: 100, OutputTokens: 20, CostUSD: 0.001},
	}}
	agent1 := NewAgent1Execute(llm1, state, cat, registry, NewAgent1PromptCache(cat), log)

	llm2 := &fakeLLM{resp: &domain.LLMResponse{
		StopReason: "tool_use",
		ToolCalls: []domain.ToolCall{
			{ID: "tool-2", Name: "compose_turn", Input: map[string]interface{}{
				"blocks": []interface{}{
					map[string]interface{}{"kind": "text", "text": "One new lead today."},
					map[string]interface{}{"kind": "text", "text": "Want the details?"},
				},
			}},
		},
		Usage: domain.LLMUsage{InputTokens: 100, OutputTokens: 30, CostUSD: 0.001},
	}}
	agent2 := NewAgent2Execute(llm2, state, registry, NewPromptCache(noopFieldDefPort{}, noopPresetPort{}, cat, "product"))

	return NewPipelineExecute(agent1, agent2, state, nil, log)
}

func TestPipelineComposeTurnStreamsAndAggregatesBlocks(t *testing.T) {
	uc := setupComposePipeline(t)

	var streamed []domain.TurnBlock
	resp, err := uc.Execute(context.Background(), PipelineExecuteRequest{
		SessionID:  "sess-1",
		TenantSlug: "acme",
		UserQuery:  "any new leads?",
		Mode:       domain.ModeCRM, // compose_turn gates to onboarding/crm
		OnBlock:    func(b domain.TurnBlock) { streamed = append(streamed, b) },
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if len(streamed) != 2 {
		t.Fatalf("OnBlock received %d blocks, want 2: %+v", len(streamed), streamed)
	}
	if streamed[0].Text != "One new lead today." || streamed[1].Text != "Want the details?" {
		t.Errorf("streamed order wrong: %+v", streamed)
	}
	if len(resp.Blocks) != 2 {
		t.Fatalf("Response.Blocks len = %d, want 2", len(resp.Blocks))
	}
	for i := range resp.Blocks {
		if resp.Blocks[i].BlockID != streamed[i].BlockID {
			t.Errorf("blocks[%d] id %q != streamed id %q — terminal array must be the same blocks",
				i, resp.Blocks[i].BlockID, streamed[i].BlockID)
		}
		if resp.Blocks[i].Kind != domain.BlockKindText {
			t.Errorf("blocks[%d].Kind = %q, want text", i, resp.Blocks[i].Kind)
		}
	}
}

// TestPipelineComposeTurnNilOnBlockStillAggregates — plain POST /pipeline
// (no streaming sink) must still return the full ordered blocks array.
func TestPipelineComposeTurnNilOnBlockStillAggregates(t *testing.T) {
	uc := setupComposePipeline(t)

	resp, err := uc.Execute(context.Background(), PipelineExecuteRequest{
		SessionID:  "sess-1",
		TenantSlug: "acme",
		UserQuery:  "any new leads?",
		Mode:       domain.ModeCRM,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if len(resp.Blocks) != 2 {
		t.Fatalf("Response.Blocks len = %d, want 2 with OnBlock nil", len(resp.Blocks))
	}
}

// TestPipelineLegacyTurnHasNoBlocks — a storefront turn (visual plane =
// visual_assembly path; here Agent2 ends without tools) keeps Blocks nil,
// so the wire omits the field and old bundles see the legacy shape.
func TestPipelineLegacyTurnHasNoBlocks(t *testing.T) {
	uc := setupPipeline(t, nil, []domain.Product{{ID: "p1", Name: "Hyaluronic Serum"}})

	var streamed int
	resp, err := uc.Execute(context.Background(), PipelineExecuteRequest{
		SessionID:  "sess-1",
		TenantSlug: "acme",
		UserQuery:  "show me serums",
		OnBlock:    func(domain.TurnBlock) { streamed++ },
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if streamed != 0 {
		t.Errorf("OnBlock fired %d times on a legacy turn, want 0", streamed)
	}
	if resp.Blocks != nil {
		t.Errorf("Response.Blocks = %+v on a legacy turn, want nil", resp.Blocks)
	}
}
