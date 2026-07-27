package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/usecases"
)

// stageDataDoneFrame is the wire shape of the `stage` frame emitted between
// Agent1 and Agent2 — the StageEvent the orchestrator reports, camelCased.
type stageDataDoneFrame struct {
	Phase    string `json:"phase"`
	Signal   string `json:"signal"`
	ToolName string `json:"toolName"`
	Count    int    `json:"count"`
	Empty    bool   `json:"empty"`
	Bypassed bool   `json:"bypassed"`
	Agent1Ms int64  `json:"agent1Ms"`
}

// streamErrorFrame is the wire shape of the terminal `error` frame for
// failures that occur after SSE headers are already on the wire.
type streamErrorFrame struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

// PipelineStream handles POST /api/v1/pipeline/stream — the SSE-over-POST
// variant of Pipeline. Guard, body validation, and tenant resolution mirror
// Pipeline() exactly (plain JSON errors before any frame is sent). After
// that the response is a text/event-stream:
//
//	event: stage   {"phase":"data_start"}            — emitted immediately
//	event: stage   {"phase":"data_done", ...}        — after Agent1
//	event: result  {<full pipelineResponse JSON>}    — terminal
//
// Mid-stream failures emit a terminal `error` frame ({409,"session closed"}
// or {500,"pipeline failed"}) instead. No LLM tokens are streamed and no
// agent is skipped — frames mark stage boundaries only.
func (h *PipelineHandler) PipelineStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

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

	// From here on the response is a stream: SSE headers, never a
	// Content-Length, and writeJSON must not be used.
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	rc := http.NewResponseController(w)
	// Drop the server write deadline so a slow turn can't kill the stream
	// mid-frame. ErrNotSupported is fine — bare recorders in tests don't
	// implement deadlines.
	if err := rc.SetWriteDeadline(time.Time{}); err != nil && !errors.Is(err, http.ErrNotSupported) {
		h.log.Warn("pipeline_stream_set_deadline_failed", "err", err)
	}

	writeFrame := func(event string, payload any) {
		// Client gone — Execute aborts via its ctx; just stop writing.
		if r.Context().Err() != nil {
			return
		}
		data, err := json.Marshal(payload)
		if err != nil {
			h.log.Error("pipeline_stream_marshal_failed", "event", event, "err", err)
			return
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
		// ErrNotSupported is ignored so bare recorders (no Flusher) still
		// accumulate the frames in their buffered body.
		if err := rc.Flush(); err != nil && !errors.Is(err, http.ErrNotSupported) {
			h.log.Warn("pipeline_stream_flush_failed", "err", err)
		}
	}

	writeFrame("stage", map[string]string{"phase": "data_start"})

	// R17: mode + role from the session row (mirrors Pipeline()) —
	// tenant-scoped: a session from another tenant lends nothing.
	mode, role := h.sessionFormRole(r.Context(), req.SessionID, tenant.ID)

	resp, err := h.pipeline.Execute(r.Context(), usecases.PipelineExecuteRequest{
		SessionID:  req.SessionID,
		TenantSlug: tenant.Slug,
		UserQuery:  req.Query,
		Mode:       mode,
		Role:       role,
		OnStage: func(ev usecases.StageEvent) {
			writeFrame("stage", stageDataDoneFrame{
				Phase:    ev.Phase,
				Signal:   ev.Signal,
				ToolName: ev.ToolName,
				Count:    ev.Count,
				Empty:    ev.Empty,
				Bypassed: ev.Bypassed,
				Agent1Ms: ev.Agent1Ms,
			})
		},
	})
	if err != nil {
		// Killed session: an intentional close, not a failure — clean 409
		// frame and no error trace (mirrors Pipeline()).
		if errors.Is(err, domain.ErrSessionKilled) {
			writeFrame("error", streamErrorFrame{Status: http.StatusConflict, Message: "session closed"})
			return
		}
		h.persistTrace(r.Context(), req, tenant, mode, nil, err)
		h.log.Error("pipeline_execute_failed", "err", err, "session_id", req.SessionID, "tenant", tenant.Slug)
		writeFrame("error", streamErrorFrame{Status: http.StatusInternalServerError, Message: "pipeline failed"})
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
	attachThemeToDocument(r.Context(), out.Document, h.themes, tenant, h.log)
	if sc := domain.SpanFromContext(r.Context()); sc != nil {
		out.Spans = sc.Spans()
	}
	writeFrame("result", out)

	h.guard.RecordCost(resp.Usage.CostUSD)
	h.persistTrace(r.Context(), req, tenant, mode, resp, nil)
}
