package handlers

import (
	"encoding/json"
	"net/http"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/usecases"
)

// PipelineHandler owns POST /api/v1/pipeline.
//
// Request body: {"sessionId": "...", "query": "..."}.
// Response: {document, toolCalls, usage, latencyMs, agent1Ms, agent2Ms, spans?}.
//
// The handler extracts tenant from middleware-stamped context and hands
// everything off to PipelineExecute, which runs Agent1 (data) → Agent2
// (render) and returns a merged response.
type PipelineHandler struct {
	pipeline *usecases.PipelineExecute
}

// NewPipelineHandler constructs the handler. pipeline is the only dep —
// agents, state, presets, components, LLM, tools all live behind it.
func NewPipelineHandler(pipeline *usecases.PipelineExecute) *PipelineHandler {
	return &PipelineHandler{pipeline: pipeline}
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
		http.Error(w, "pipeline failed: "+err.Error(), http.StatusInternalServerError)
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
	}
	if sc := domain.SpanFromContext(r.Context()); sc != nil {
		out.Spans = sc.Spans()
	}
	writeJSON(w, http.StatusOK, out)
}
