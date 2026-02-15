package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/usecases"
)

type EnrichmentHandler struct {
	enrichUC *usecases.EnrichmentUseCase
	log      *logger.Logger
}

func NewEnrichmentHandler(enrichUC *usecases.EnrichmentUseCase, log *logger.Logger) *EnrichmentHandler {
	return &EnrichmentHandler{enrichUC: enrichUC, log: log}
}

func (h *EnrichmentHandler) HandleEnrich(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("handler.enrich")
		defer endSpan()
	}

	switch r.Method {
	case http.MethodPost:
		h.startEnrichment(w, r)
	case http.MethodGet:
		h.getStatus(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "GET or POST only")
	}
}

type enrichRequest struct {
	FilePath string `json:"filePath"`
}

func (h *EnrichmentHandler) startEnrichment(w http.ResponseWriter, r *http.Request) {
	reqLog := h.log.FromContext(r.Context())

	// Check if already running
	if job := h.enrichUC.GetStatus(); job != nil && job.Status == "processing" {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "enrichment already running",
			"job":   job,
		})
		return
	}

	var req enrichRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.FilePath == "" {
		writeError(w, http.StatusBadRequest, "filePath is required")
		return
	}

	// Run in background
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		job, err := h.enrichUC.EnrichFile(bgCtx, req.FilePath)
		if err != nil {
			reqLog.Error("enrichment_failed", "error", err)
			return
		}
		if job != nil {
			reqLog.Info("enrichment_done",
				"enriched", job.EnrichedProducts,
				"cost_usd", job.EstimatedCostUSD)
		}
	}()

	// Wait briefly for job to initialize
	time.Sleep(50 * time.Millisecond)

	if status := h.enrichUC.GetStatus(); status != nil {
		writeJSON(w, http.StatusAccepted, status)
	} else {
		writeJSON(w, http.StatusAccepted, map[string]any{"status": "started"})
	}
}

func (h *EnrichmentHandler) getStatus(w http.ResponseWriter, r *http.Request) {
	job := h.enrichUC.GetStatus()
	if job == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":  "idle",
			"message": "No enrichment job has been run yet",
		})
		return
	}
	writeJSON(w, http.StatusOK, job)
}
