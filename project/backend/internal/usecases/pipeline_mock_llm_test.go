package usecases_test

import (
	"context"
	"testing"

	"keepstar/internal/adapters/postgres"
	"keepstar/internal/domain"
	"keepstar/internal/logger"
	"keepstar/internal/presets"
	"keepstar/internal/testutil"
	"keepstar/internal/tools"
	"keepstar/internal/usecases"
)

// pipelineSetup creates DB-backed adapters + mock LLM for pipeline tests.
func pipelineSetup(t *testing.T, llmResponses ...*domain.LLMResponse) (
	context.Context, *usecases.PipelineExecuteUseCase, *postgres.StateAdapter, string,
) {
	t.Helper()
	client := testutil.TestDB(t)
	log := logger.New("error")

	stateAdapter := postgres.NewStateAdapter(client, log)
	catalogAdapter := postgres.NewCatalogAdapter(client)
	cacheAdapter := postgres.NewCacheAdapter(client)
	presetRegistry := presets.NewPresetRegistry()
	toolRegistry := tools.NewRegistry(stateAdapter, catalogAdapter, presetRegistry, nil)
	mockLLM := testutil.NewMockLLMClient(llmResponses...)

	sessionID := testutil.TestStateWithProducts(t, client, 4)

	// Ensure session exists in cache for pipeline FK
	sess := &domain.Session{
		ID:     sessionID,
		Status: domain.SessionStatusActive,
	}
	if err := cacheAdapter.SaveSession(context.Background(), sess); err != nil {
		// Session may already exist from TestSession helper
		t.Logf("save session (may already exist): %v", err)
	}

	pipeline := usecases.NewPipelineExecuteUseCase(
		mockLLM, stateAdapter, cacheAdapter, nil, catalogAdapter,
		toolRegistry, presetRegistry, log,
	)

	return context.Background(), pipeline, stateAdapter, sessionID
}

// TestPipeline_MockLLM_RenderOnly tests pipeline with mock LLM that calls render_product_preset.
// Agent1 returns render_product_preset tool call, Agent2 returns end_turn.
func TestPipeline_MockLLM_RenderOnly(t *testing.T) {
	agent1Response := &domain.LLMResponse{
		ToolCalls: []domain.ToolCall{
			{
				ID:   "call-mock-1",
				Name: "render_product_preset",
				Input: map[string]interface{}{
					"preset": "product_grid",
				},
			},
		},
		StopReason: "tool_use",
		Usage: domain.LLMUsage{
			InputTokens: 100, OutputTokens: 20, TotalTokens: 120,
			Model: "mock", CostUSD: 0.001,
		},
	}
	agent2Response := &domain.LLMResponse{
		Text:       "Вот ваши товары",
		StopReason: "end_turn",
		Usage: domain.LLMUsage{
			InputTokens: 50, OutputTokens: 10, TotalTokens: 60,
			Model: "mock", CostUSD: 0.0005,
		},
	}

	ctx, pipeline, stateAdapter, sessionID := pipelineSetup(t, agent1Response, agent2Response)

	resp, err := pipeline.Execute(ctx, usecases.PipelineExecuteRequest{
		SessionID: sessionID,
		Query:     "покажи товары",
		TurnID:    "turn-pipeline-1",
	})
	if err != nil {
		t.Fatalf("pipeline: %v", err)
	}

	// Verify pipeline completed
	if resp.TotalMs <= 0 {
		t.Error("expected positive TotalMs")
	}

	// Verify state has template
	state, err := stateAdapter.GetState(ctx, sessionID)
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	if state.Current.Template == nil {
		t.Error("expected template after pipeline")
	}

	// Verify deltas from this turn
	deltas, err := stateAdapter.GetDeltas(ctx, sessionID)
	if err != nil {
		t.Fatalf("get deltas: %v", err)
	}
	turnDeltas := 0
	for _, d := range deltas {
		if d.TurnID == "turn-pipeline-1" {
			turnDeltas++
		}
	}
	if turnDeltas == 0 {
		t.Error("expected at least 1 delta with turn-pipeline-1")
	}
	t.Logf("pipeline complete: tool=%s, totalMs=%d, turnDeltas=%d", resp.ToolCalled, resp.TotalMs, turnDeltas)
}

// TestPipeline_MockLLM_NoToolCall tests pipeline when LLM returns text without tool call.
func TestPipeline_MockLLM_NoToolCall(t *testing.T) {
	agent1Response := &domain.LLMResponse{
		Text:       "Привет! Чем могу помочь?",
		StopReason: "end_turn",
		Usage: domain.LLMUsage{
			InputTokens: 100, OutputTokens: 20, TotalTokens: 120,
			Model: "mock", CostUSD: 0.001,
		},
	}
	agent2Response := &domain.LLMResponse{
		Text:       "response",
		StopReason: "end_turn",
		Usage: domain.LLMUsage{
			InputTokens: 50, OutputTokens: 10, TotalTokens: 60,
			Model: "mock", CostUSD: 0.0005,
		},
	}

	ctx, pipeline, _, sessionID := pipelineSetup(t, agent1Response, agent2Response)

	resp, err := pipeline.Execute(ctx, usecases.PipelineExecuteRequest{
		SessionID: sessionID,
		Query:     "привет",
		TurnID:    "turn-pipeline-2",
	})
	if err != nil {
		t.Fatalf("pipeline: %v", err)
	}

	if resp.ToolCalled != "" {
		t.Errorf("expected no tool called, got %s", resp.ToolCalled)
	}
	if resp.ProductsFound != 0 {
		t.Errorf("expected 0 products, got %d", resp.ProductsFound)
	}
}

// TestPipeline_MockLLM_TurnIDGrouping verifies deltas from pipeline share the same TurnID.
func TestPipeline_MockLLM_TurnIDGrouping(t *testing.T) {
	agent1Response := &domain.LLMResponse{
		ToolCalls: []domain.ToolCall{
			{
				ID:    "call-group-1",
				Name:  "render_product_preset",
				Input: map[string]interface{}{"preset": "product_grid"},
			},
		},
		StopReason: "tool_use",
		Usage:      domain.LLMUsage{Model: "mock"},
	}
	agent2Response := &domain.LLMResponse{
		ToolCalls: []domain.ToolCall{
			{
				ID:    "call-group-2",
				Name:  "render_product_preset",
				Input: map[string]interface{}{"preset": "product_grid"},
			},
		},
		StopReason: "tool_use",
		Usage:      domain.LLMUsage{Model: "mock"},
	}

	ctx, pipeline, stateAdapter, sessionID := pipelineSetup(t, agent1Response, agent2Response)

	_, err := pipeline.Execute(ctx, usecases.PipelineExecuteRequest{
		SessionID: sessionID,
		Query:     "покажи товары",
		TurnID:    "turn-grouped",
	})
	if err != nil {
		t.Fatalf("pipeline: %v", err)
	}

	deltas, _ := stateAdapter.GetDeltas(ctx, sessionID)
	grouped := 0
	for _, d := range deltas {
		if d.TurnID == "turn-grouped" {
			grouped++
		}
	}
	if grouped < 1 {
		t.Errorf("expected deltas with turn-grouped, got %d", grouped)
	}
	t.Logf("deltas with turn-grouped: %d / total: %d", grouped, len(deltas))
}
