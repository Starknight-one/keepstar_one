package usecases

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"keepstar_v5/internal/domain"
)

// PipelineExecuteRequest is the per-call input the HTTP handler hands to the
// orchestrator: a session, a tenant, and the user's natural-language query.
type PipelineExecuteRequest struct {
	SessionID  string
	TenantSlug string
	UserQuery  string
	TurnID     string
}

// PipelineExecuteResponse aggregates Agent1 + Agent2 into a single shape the
// /api/v1/pipeline handler returns.
type PipelineExecuteResponse struct {
	Document  map[string]interface{}
	ToolCalls []domain.ToolCall
	Usage     domain.LLMUsage
	LatencyMs int64

	// Per-stage breakdown for client-side debugging until /debug/traces UI ships.
	Agent1Ms int64
	Agent2Ms int64
}

// PipelineExecute is the two-agent orchestrator. Agent1 fetches/filters data
// (writes to state.Current.Data), Agent2 renders (writes to
// state.Current.Template). A short microcontext signal — generated from
// Agent1's tool name + product count — is forwarded to Agent2 so it knows
// what changed and whether to re-render.
type PipelineExecute struct {
	agent1 *Agent1Execute
	agent2 *Agent2Execute
	log    *slog.Logger
}

// NewPipelineExecute wires the orchestrator. Both agents are required.
func NewPipelineExecute(agent1 *Agent1Execute, agent2 *Agent2Execute, log *slog.Logger) *PipelineExecute {
	return &PipelineExecute{agent1: agent1, agent2: agent2, log: log}
}

// Execute runs Agent1 → Agent2 sequentially. Returns aggregated usage,
// document, and per-stage latency. Errors propagate verbatim from whichever
// agent failed — caller decides whether to retry or surface to the user.
func (uc *PipelineExecute) Execute(ctx context.Context, req PipelineExecuteRequest) (*PipelineExecuteResponse, error) {
	start := time.Now()
	endTopSpan := startSpan(ctx, "pipeline.execute")
	defer endTopSpan()

	// ── Agent1 ──
	a1, err := uc.agent1.Execute(ctx, Agent1ExecuteRequest{
		SessionID:  req.SessionID,
		TenantSlug: req.TenantSlug,
		UserQuery:  req.UserQuery,
		TurnID:     req.TurnID,
	})
	if err != nil {
		return nil, fmt.Errorf("agent1: %w", err)
	}

	microcontext := composeMicrocontext(a1)

	// ── Agent2 ──
	a2, err := uc.agent2.Execute(ctx, Agent2ExecuteRequest{
		SessionID:    req.SessionID,
		TenantSlug:   req.TenantSlug,
		UserQuery:    req.UserQuery,
		Microcontext: microcontext,
	})
	if err != nil {
		return nil, fmt.Errorf("agent2: %w", err)
	}

	// Aggregate ToolCalls (Agent1's first if it ran a tool, Agent2's all).
	toolCalls := make([]domain.ToolCall, 0, 1+len(a2.ToolCalls))
	if a1.ToolName != "" {
		toolCalls = append(toolCalls, domain.ToolCall{
			Name:  a1.ToolName,
			Input: a1.ToolInput,
		})
	}
	toolCalls = append(toolCalls, a2.ToolCalls...)

	// Sum token + cost across both agents.
	usage := domain.LLMUsage{
		InputTokens:              a1.Usage.InputTokens + a2.Usage.InputTokens,
		OutputTokens:             a1.Usage.OutputTokens + a2.Usage.OutputTokens,
		CacheCreationInputTokens: a1.Usage.CacheCreationInputTokens + a2.Usage.CacheCreationInputTokens,
		CacheReadInputTokens:     a1.Usage.CacheReadInputTokens + a2.Usage.CacheReadInputTokens,
		TotalTokens:              a1.Usage.TotalTokens + a2.Usage.TotalTokens,
		CostUSD:                  a1.Usage.CostUSD + a2.Usage.CostUSD,
		Model:                    a2.Usage.Model, // both agents use same model
	}

	return &PipelineExecuteResponse{
		Document:  a2.Document,
		ToolCalls: toolCalls,
		Usage:     usage,
		LatencyMs: time.Since(start).Milliseconds(),
		Agent1Ms:  a1.LatencyMs,
		Agent2Ms:  a2.LatencyMs,
	}, nil
}

// composeMicrocontext maps Agent1's tool result into a one-line signal
// Agent2 can use to decide whether to re-render. V4 generates the same
// shape (pipeline_execute.go in V4 — "new_search: N items found" etc.).
func composeMicrocontext(a1 *Agent1ExecuteResponse) string {
	switch a1.ToolName {
	case "catalog_search":
		return fmt.Sprintf("new_search: %d items found", a1.ProductsFound)
	case "_internal_state_filter":
		return fmt.Sprintf("filtered: %d items", a1.ProductsFound)
	case "_internal_history_lookup":
		return "history: deltas inspected"
	default:
		return "no_data_change"
	}
}
