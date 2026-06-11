package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
	"keepstar_v5/internal/usecases"
)

// PipelineHandler owns POST /api/v1/pipeline.
//
// Request body: {"sessionId": "...", "query": "..."}.
// Response: {document, toolCalls, usage, latencyMs, agent1Ms, agent2Ms, spans?}.
//
// The handler extracts tenant from middleware-stamped context and hands
// everything off to PipelineExecute, which runs Agent1 (data) → Agent2
// (render) and returns a merged response. After Execute returns, the
// handler fires a best-effort goroutine that persists the trace
// (chunk 13) into v5_chat_session_traces so Curator UI can read it.
type PipelineHandler struct {
	pipeline pipelineRunner
	tracer   ports.TracePort // optional; nil disables persistence
	guard    *PipelineGuard  // optional; nil disables rate/spend limits
	log      *slog.Logger
}

// pipelineRunner is the slice of *usecases.PipelineExecute the handler
// needs — an interface so tests can stub the orchestrator.
type pipelineRunner interface {
	Execute(context.Context, usecases.PipelineExecuteRequest) (*usecases.PipelineExecuteResponse, error)
}

// NewPipelineHandler constructs the handler. tracer is optional —
// nil disables trace persistence (used by tests that don't need it).
// guard is optional — nil disables rate limiting and the daily spend cap.
// log is required so the async persist goroutine can log failures.
func NewPipelineHandler(pipeline *usecases.PipelineExecute, tracer ports.TracePort, guard *PipelineGuard, log *slog.Logger) *PipelineHandler {
	return &PipelineHandler{pipeline: pipeline, tracer: tracer, guard: guard, log: log}
}

type pipelineRequest struct {
	SessionID string `json:"sessionId"`
	Query     string `json:"query"`
}

type pipelineResponse struct {
	Document  map[string]interface{} `json:"document"`
	ToolCalls interface{}            `json:"toolCalls"`
	Usage     interface{}            `json:"usage"`
	LatencyMs int64                  `json:"latencyMs"`
	// Per-agent latency breakdown for client-side debugging.
	Agent1Ms int64 `json:"agent1Ms"`
	Agent2Ms int64 `json:"agent2Ms"`
	// Prefetch is the 1-level navigation prefetch built by the
	// orchestrator (one template per entity type + the raw entity
	// list). Frontend binds the template with the chosen entity on
	// drill click for an instant-feeling navigation. Omitted when
	// the active preset has no registered drill target or the data
	// zone is empty.
	Prefetch *usecases.PrefetchPayload `json:"prefetch,omitempty"`
	// Facets are the data-derived filter dimensions for this result set
	// (deterministic; computed from the products zone). Empty when there
	// are no usable filters. The widget renders typed controls from these.
	Facets []usecases.Facet `json:"facets,omitempty"`
	// Spans is the request waterfall captured by SpanCollector — useful
	// for client-side debugging until the /debug/traces UI ships. Empty
	// (omitted) when the logging middleware didn't attach a collector.
	Spans []domain.Span `json:"spans,omitempty"`
}

// Pipeline handles POST /api/v1/pipeline.
func (h *PipelineHandler) Pipeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Guard the only paid-LLM endpoint: per-IP rate limit + global daily
	// spend ceiling. Checked before we read the body so abusive callers are
	// rejected cheaply.
	if ok, reason := h.guard.Allow(clientIP(r)); !ok {
		switch reason {
		case "budget":
			h.log.Warn("pipeline_daily_budget_exceeded", "ip", clientIP(r))
			http.Error(w, "service temporarily unavailable, try again later", http.StatusServiceUnavailable)
		default:
			http.Error(w, "rate limit exceeded, slow down", http.StatusTooManyRequests)
		}
		return
	}

	var req pipelineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.SessionID == "" || req.Query == "" {
		http.Error(w, "sessionId and query are required", http.StatusBadRequest)
		return
	}

	tenant := TenantFromContext(r.Context())
	if tenant == nil {
		http.Error(w, "tenant unresolved", http.StatusInternalServerError)
		return
	}

	resp, err := h.pipeline.Execute(r.Context(), usecases.PipelineExecuteRequest{
		SessionID:  req.SessionID,
		TenantSlug: tenant.Slug,
		UserQuery:  req.Query,
	})
	if err != nil {
		// Killed session: a curator kill stamped killed_at and GetState now
		// refuses it. Return a clean 409 (not a 500) and skip the error-trace —
		// this is an intentional close, not a failure.
		if errors.Is(err, domain.ErrSessionKilled) {
			http.Error(w, "session closed", http.StatusConflict)
			return
		}
		// Persist the failed turn too — operators need to see error
		// traces in Curator (the dominant debugging signal).
		h.persistTrace(r.Context(), req, tenant, nil, err)
		// Log the real error server-side; return a generic message so we
		// don't leak internals (DB errors, prompt detail) to the client.
		h.log.Error("pipeline_execute_failed", "err", err, "session_id", req.SessionID, "tenant", tenant.Slug)
		http.Error(w, "pipeline failed", http.StatusInternalServerError)
		return
	}

	out := pipelineResponse{
		Document:  resp.Document,
		ToolCalls: resp.ToolCalls,
		Usage:     resp.Usage,
		LatencyMs: resp.LatencyMs,
		Agent1Ms:  resp.Agent1Ms,
		Agent2Ms:  resp.Agent2Ms,
		Prefetch:  resp.Prefetch,
		Facets:    resp.Facets,
	}
	if sc := domain.SpanFromContext(r.Context()); sc != nil {
		out.Spans = sc.Spans()
	}
	writeJSON(w, http.StatusOK, out)

	// Record the turn's actual cost against the daily spend ceiling.
	h.guard.RecordCost(resp.Usage.CostUSD)

	// Trace persistence — fire after the response is on the wire so
	// the user never waits on this. Caller's request context may be
	// cancelled after writeJSON returns, so the goroutine carries its
	// own derived ctx with a generous timeout.
	h.persistTrace(r.Context(), req, tenant, resp, nil)
}

// persistTrace assembles a TraceRow from the in-memory span collector
// + pipeline response and fires a best-effort INSERT in the
// background. Never propagates errors back to the caller — the user
// already got the pipeline response.
func (h *PipelineHandler) persistTrace(
	ctx context.Context,
	req pipelineRequest,
	tenant *domain.Tenant,
	resp *usecases.PipelineExecuteResponse,
	pipelineErr error,
) {
	if h.tracer == nil {
		return
	}
	sc := domain.SpanFromContext(ctx)
	if sc == nil {
		return
	}
	rid := domain.RequestIDFromContext(ctx)
	if rid == "" {
		return
	}
	spans := sc.Spans()

	row := domain.TraceRow{
		SessionID: req.SessionID,
		RequestID: rid,
		TenantID:  tenant.ID,
		UserQuery: req.Query,
		Spans:     spans,
		SpanCount: len(spans),
		Status:    "ok",
	}
	if resp != nil {
		row.LatencyMs = resp.LatencyMs
		row.Agent1Ms = resp.Agent1Ms
		row.Agent2Ms = resp.Agent2Ms
		row.TokensInput = int64(resp.Usage.InputTokens)
		row.TokensOutput = int64(resp.Usage.OutputTokens)
		row.TokensCacheRead = int64(resp.Usage.CacheReadInputTokens)
		row.TokensCacheCreation = int64(resp.Usage.CacheCreationInputTokens)
		row.CostUSD = resp.Usage.CostUSD
	}
	if pipelineErr != nil {
		row.Status = "error"
		row.ErrorMessage = pipelineErr.Error()
	}

	go func() {
		// Detached ctx — caller's request ctx may already be cancelled
		// by the time this goroutine actually runs.
		bg, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		defer func() {
			if rcv := recover(); rcv != nil {
				h.log.Error("trace persist panic", "request_id", rid, "panic", rcv)
			}
		}()
		if err := h.tracer.SaveTrace(bg, row); err != nil {
			h.log.Warn("trace persist failed", "request_id", rid, "err", err)
		}
	}()
}
