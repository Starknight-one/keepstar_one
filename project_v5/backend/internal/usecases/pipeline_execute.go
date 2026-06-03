package usecases

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
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

	// Prefetch is the 1-level navigation prefetch (adjacent template +
	// entities) for instant drill-down on the frontend. Nil when the
	// active preset has no registered drill target or the data zone is
	// empty. See PrefetchBuilder for the build rules.
	Prefetch *PrefetchPayload

	// Facets are the data-derived filter dimensions for the current result
	// set (deterministic — computed from the products zone, no LLM). Empty
	// when there are no usable filters.
	Facets []Facet
}

// PipelineExecute is the two-agent orchestrator. Agent1 fetches/filters data
// (writes to state.Current.Data), Agent2 renders (writes to
// state.Current.Template). A short microcontext signal — generated from
// Agent1's tool name + product count — is forwarded to Agent2 so it knows
// what changed and whether to re-render.
//
// After Agent2 returns, the orchestrator builds a 1-level navigation
// prefetch via PrefetchBuilder so the frontend can drill into a detail
// preset on click without a /navigation/expand round-trip. Prefetch is
// optional — nil when the active preset has no registered drill target.
type PipelineExecute struct {
	agent1   *Agent1Execute
	agent2   *Agent2Execute
	prefetch *PrefetchBuilder
	state    ports.StatePort
	log      *slog.Logger
}

// NewPipelineExecute wires the orchestrator. agent1 + agent2 are
// required; prefetch + state are optional (nil disables the prefetch
// payload). state is needed only when prefetch is non-nil — to read
// the products zone after Agent2 runs.
func NewPipelineExecute(agent1 *Agent1Execute, agent2 *Agent2Execute, state ports.StatePort, prefetch *PrefetchBuilder, log *slog.Logger) *PipelineExecute {
	return &PipelineExecute{
		agent1:   agent1,
		agent2:   agent2,
		prefetch: prefetch,
		state:    state,
		log:      log,
	}
}

// Execute runs Agent1 → Agent2 sequentially. Returns aggregated usage,
// document, and per-stage latency. Errors propagate verbatim from whichever
// agent failed — caller decides whether to retry or surface to the user.
func (uc *PipelineExecute) Execute(ctx context.Context, req PipelineExecuteRequest) (*PipelineExecuteResponse, error) {
	start := time.Now()
	ctx, topSpan := withSpan(ctx, "pipeline.execute")
	defer topSpan.End()
	if rid := domain.RequestIDFromContext(ctx); rid != "" {
		topSpan.SetAttr("request_id", rid)
	}
	if req.TurnID != "" {
		topSpan.SetAttr("turn_id", req.TurnID)
	}
	if req.TenantSlug != "" {
		topSpan.SetAttr("tenant_slug", req.TenantSlug)
	}

	// ── Agent1 ──
	a1, err := uc.agent1.Execute(ctx, Agent1ExecuteRequest{
		SessionID:  req.SessionID,
		TenantSlug: req.TenantSlug,
		UserQuery:  req.UserQuery,
		TurnID:     req.TurnID,
	})
	if err != nil {
		topSpan.SetError(err)
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
		topSpan.SetError(err)
		return nil, fmt.Errorf("agent2: %w", err)
	}

	topSpan.SetAttrs(map[string]any{
		"agent1_ms":    a1.LatencyMs,
		"agent2_ms":    a2.LatencyMs,
		"microcontext": microcontext,
	})

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

	resp := &PipelineExecuteResponse{
		Document:  a2.Document,
		ToolCalls: toolCalls,
		Usage:     usage,
		LatencyMs: time.Since(start).Milliseconds(),
		Agent1Ms:  a1.LatencyMs,
		Agent2Ms:  a2.LatencyMs,
	}

	// Data-derived filter facets + 1-level navigation prefetch — both read
	// the products zone after Agent2. Facets are deterministic and depend
	// only on the data (built on any turn with products); prefetch also
	// needs a preset with a registered drill target. Best effort: failures
	// ship empty facets / nil prefetch.
	if uc.state != nil {
		if state, err := uc.state.GetState(ctx, req.SessionID); err == nil {
			products := state.Current.Data.Products
			resp.Facets = BuildGuidedFacets(products, nil)
			if uc.prefetch != nil {
				if sourcePreset := readPresetInUse(a2.Document); sourcePreset != "" {
					resp.Prefetch = uc.prefetch.Build(ctx, req.TenantSlug, sourcePreset, products)
				}
			}
		}
	}

	return resp, nil
}

// readPresetInUse extracts the synthetic top-level marker
// visual_assembly stamps on the marshaled Document map. Empty when
// Agent2 ran the freestyle / modify path (no preset to drill into).
func readPresetInUse(doc map[string]interface{}) string {
	if doc == nil {
		return ""
	}
	v, _ := doc[domain.TemplatePresetInUseKey].(string)
	return v
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
