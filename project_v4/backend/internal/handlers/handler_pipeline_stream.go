package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"keepstar_v4/internal/domain"
	"keepstar_v4/internal/logger"
	"keepstar_v4/internal/usecases"
)

// PipelineStreamHandler serves POST /api/v1/pipeline/stream as SSE.
type PipelineStreamHandler struct {
	streamUC *usecases.PipelineStreamUseCase
	log      *logger.Logger
}

// NewPipelineStreamHandler creates the SSE pipeline handler.
func NewPipelineStreamHandler(streamUC *usecases.PipelineStreamUseCase, log *logger.Logger) *PipelineStreamHandler {
	return &PipelineStreamHandler{streamUC: streamUC, log: log}
}

// HandlePipelineStream handles POST /api/v1/pipeline/stream with SSE.
// Events emitted: session_init, widget_provisional, formation_complete, error.
func (h *PipelineStreamHandler) HandlePipelineStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	var req PipelineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if req.Query == "" {
		http.Error(w, "Query is required", http.StatusBadRequest)
		return
	}

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	// SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	ctx := logger.WithSessionID(r.Context(), sessionID)

	var tenantSlug string
	if tenant := GetTenantFromContext(ctx); tenant != nil {
		tenantSlug = tenant.Slug
	}

	var screenCtx *usecases.ScreenContext
	if req.ScreenContext != nil {
		screenCtx = &usecases.ScreenContext{
			Mode:        req.ScreenContext.Mode,
			WidgetCount: req.ScreenContext.WidgetCount,
			Fields:      req.ScreenContext.Fields,
		}
	}

	events := make(chan usecases.StreamEvent, 16)
	done := make(chan struct{})

	// Writer goroutine: consume events, serialize to SSE frames, flush.
	go func() {
		defer close(done)
		for ev := range events {
			payload, err := json.Marshal(ev)
			if err != nil {
				h.log.Error("sse_marshal_failed", "error", err)
				continue
			}
			if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Kind, payload); err != nil {
				h.log.Warn("sse_write_failed", "error", err)
				return
			}
			flusher.Flush()
		}
	}()

	turnID := uuid.New().String()
	_, err := h.streamUC.Execute(ctx, usecases.PipelineExecuteRequest{
		SessionID:     sessionID,
		Query:         req.Query,
		TenantSlug:    tenantSlug,
		TurnID:        turnID,
		ScreenContext: screenCtx,
	}, events)
	close(events)
	<-done

	if err != nil {
		h.log.Error("pipeline_stream_failed", "error", err)
	}
	_ = domain.PipelineTrace{} // silence unused import if trimmed later
}
