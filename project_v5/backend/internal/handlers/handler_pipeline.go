package handlers

import (
	"encoding/json"
	"net/http"

	"keepstar_v5/internal/usecases"
)

// PipelineHandler owns POST /api/v1/pipeline.
//
// Request body: {"sessionId": "...", "query": "..."}.
// Response: {document, toolCalls, usage, latencyMs}.
//
// The handler extracts tenant from middleware-stamped context and hands
// everything off to Agent2Execute. Tool execution + state writes happen
// inside the use case.
type PipelineHandler struct {
	agent2 *usecases.Agent2Execute
}

// NewPipelineHandler constructs the handler. agent2 is the only dep —
// state, presets, components, LLM, tools all live behind it.
func NewPipelineHandler(agent2 *usecases.Agent2Execute) *PipelineHandler {
	return &PipelineHandler{agent2: agent2}
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

	resp, err := h.agent2.Execute(r.Context(), usecases.Agent2ExecuteRequest{
		SessionID:  req.SessionID,
		TenantSlug: tenant.Slug,
		UserQuery:  req.Query,
	})
	if err != nil {
		http.Error(w, "pipeline failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, pipelineResponse{
		Document:  resp.Document,
		ToolCalls: resp.ToolCalls,
		Usage:     resp.Usage,
		LatencyMs: resp.LatencyMs,
	})
}
