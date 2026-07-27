package usecases

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
	"keepstar_v5/internal/prompts"
)

// PipelineExecuteRequest is the per-call input the HTTP handler hands to the
// orchestrator: a session, a tenant, and the user's natural-language query.
// Mode/Role/ActorID come from the session row (R17): the handler reads
// v5_chat_sessions.mode and the session role and passes them through; empty
// values default to the storefront form / visitor role downstream.
type PipelineExecuteRequest struct {
	SessionID  string
	TenantSlug string
	UserQuery  string
	TurnID     string
	Mode       domain.PipelineMode
	Role       domain.Role
	ActorID    string

	// OnStage, when non-nil, receives a progress signal between pipeline
	// stages (currently once, after Agent1) so callers can stream progress
	// to the client. Invoked synchronously on the request goroutine — it
	// must be fast, anything slow delays Agent2. nil = exactly the current
	// (non-streaming) behavior.
	OnStage func(StageEvent)

	// OnBlock, when non-nil, receives every TurnBlock the turn's
	// compose_turn call emits, AS PRODUCED (R9 as overridden by final owner
	// decision 3: real per-block SSE streaming, no batching). Invoked
	// synchronously on the request goroutine — keep it fast. nil callers
	// still get the full ordered list on Response.Blocks.
	OnBlock func(domain.TurnBlock)
}

// StageEvent describes one completed pipeline stage for OnStage consumers.
// Mirrors the microcontext signal handed to Agent2 plus the Agent1
// diagnostics the streaming handler puts on the wire.
type StageEvent struct {
	Phase    string
	Signal   string
	ToolName string
	Ops      []string // every executed Agent1 operation, in order (multi-op turns, R4)
	Count    int
	Empty    bool
	Bypassed bool
	Agent1Ms int64
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

	// Blocks is the ordered TurnBlock list of a composed turn (§4.7): the
	// same blocks the streaming transport already emitted as `event: block`
	// frames, aggregated for the terminal `result` frame / POST /pipeline
	// JSON body. Nil on legacy single-document turns (visual_assembly-only)
	// — the wire omits the field and old bundles are unaffected.
	Blocks []domain.TurnBlock
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
		Mode:       req.Mode,
		Role:       req.Role,
		ActorID:    req.ActorID,
	})
	if err != nil {
		topSpan.SetError(err)
		return nil, fmt.Errorf("agent1: %w", err)
	}

	microcontext := composeMicrocontext(a1.Results)
	if req.Mode == domain.ModeOnboarding {
		microcontext = uc.onboardingMicrocontext(ctx, req.SessionID, a1.Results, microcontext)
	}

	if req.OnStage != nil {
		// Empty comes from the structured outcome, not ProductsFound: on
		// "0 hits, previous data preserved" the count still shows the
		// stale data while the operation reports OutcomeEmpty.
		req.OnStage(StageEvent{
			Phase:    "data_done",
			Signal:   microcontext,
			ToolName: a1.ToolName,
			Ops:      opNames(a1.Results),
			Count:    a1.ProductsFound,
			Empty:    anyEmpty(a1.Results),
			Bypassed: a1.Bypassed,
			Agent1Ms: a1.LatencyMs,
		})
	}

	// ── Agent2 ──
	// The turn-block collector rides the ctx through the registry choke
	// point into compose_turn (§4.7): each block it produces is forwarded
	// to OnBlock AS PRODUCED (the SSE `event: block` frames) and retained
	// for Response.Blocks. Installed on BOTH transports — plain POST
	// /pipeline (OnBlock nil) still aggregates, and compose_turn's
	// one-call-per-turn guard reads the same collector.
	blockSink := domain.NewTurnBlockCollector(req.OnBlock)
	a2, err := uc.agent2.Execute(domain.WithTurnBlockCollector(ctx, blockSink), Agent2ExecuteRequest{
		SessionID:    req.SessionID,
		TenantSlug:   req.TenantSlug,
		UserQuery:    req.UserQuery,
		Mode:         req.Mode,
		Role:         req.Role,
		ActorID:      req.ActorID,
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

	// Aggregate ToolCalls (Agent1's executed calls — one on storefront/crm,
	// up to 8 on the onboarding form (R4) — then Agent2's all). IDs are
	// stripped from Agent1's calls, matching the pre-registry wire shape.
	toolCalls := make([]domain.ToolCall, 0, len(a1.ToolCalls)+len(a2.ToolCalls))
	for _, tc := range a1.ToolCalls {
		toolCalls = append(toolCalls, domain.ToolCall{
			Name:  tc.Name,
			Input: tc.Input,
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
		Blocks:    blockSink.Blocks(),
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

// onboardingMicrocontext folds the session's staged manifest into the
// Agent2 signal on the onboarding form (§4.3): "turn: … | staged: … |
// applied: n/m | waiting: …". The manifest is read through the
// OnboardingStatePort side of the state adapter; when the port or the
// manifest is unavailable the generic microcontext passes through (fallback,
// logged — never a silent empty signal).
func (uc *PipelineExecute) onboardingMicrocontext(ctx context.Context, sessionID string, results []*domain.OperationResult, fallback string) string {
	ob, ok := uc.state.(ports.OnboardingStatePort)
	if !ok {
		return fallback
	}
	m, err := ob.GetOnboarding(ctx, sessionID)
	if err != nil {
		if uc.log != nil {
			uc.log.Warn("pipeline: onboarding manifest read failed — generic microcontext", "session", sessionID, "err", err)
		}
		return fallback
	}
	return prompts.ComposeOnboardingMicrocontext(m, opNames(results))
}

// composeMicrocontext maps Agent1's structured operation results into a
// one-line signal Agent2 can use to decide whether to re-render. The
// per-kind phrase table preserves V4's strings byte-compatible
// ("new_search: N items found", "filtered: N items") and adds the entity
// phrases ("lead_search: N leads found"). Catalog/state counts arrive
// pre-stamped from the reloaded state (stampStateCounts) so the stale-data
// semantics of an empty search survive the registry swap.
func composeMicrocontext(results []*domain.OperationResult) string {
	var phrases []string
	for _, r := range results {
		if r == nil {
			continue
		}
		switch {
		case r.Kind == domain.KindQuery && r.EntityKind != "":
			phrases = append(phrases, fmt.Sprintf("%s: %d %ss found", r.Operation, r.Count, r.EntityKind))
		case r.Kind == domain.KindQuery:
			phrases = append(phrases, fmt.Sprintf("new_search: %d items found", r.Count))
		case r.Operation == "_internal_state_filter":
			phrases = append(phrases, fmt.Sprintf("filtered: %d items", r.Count))
		case r.Operation == "_internal_history_lookup":
			phrases = append(phrases, "history: deltas inspected")
		}
	}
	if len(phrases) == 0 {
		return "no_data_change"
	}
	return strings.Join(phrases, "; ")
}

// opNames lists the executed operations in order for StageEvent.Ops.
func opNames(results []*domain.OperationResult) []string {
	if len(results) == 0 {
		return nil
	}
	names := make([]string, 0, len(results))
	for _, r := range results {
		if r != nil {
			names = append(names, r.Operation)
		}
	}
	return names
}

// anyEmpty reports whether any executed operation came back OutcomeEmpty —
// the structured replacement for the old "empty:" prefix sniffing.
func anyEmpty(results []*domain.OperationResult) bool {
	for _, r := range results {
		if r != nil && r.Outcome == domain.OutcomeEmpty {
			return true
		}
	}
	return false
}
