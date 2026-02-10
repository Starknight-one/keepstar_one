package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"keepstar-admin/internal/usecases"
)

type ImportHandler struct {
	importUC *usecases.ImportUseCase
}

func NewImportHandler(importUC *usecases.ImportUseCase) *ImportHandler {
	return &ImportHandler{importUC: importUC}
}

func (h *ImportHandler) HandleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}

	tenantID := TenantID(r.Context())

	var req usecases.ImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	job, err := h.importUC.Upload(r.Context(), tenantID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"jobId":      job.ID,
		"status":     job.Status,
		"totalItems": job.TotalItems,
	})
}

func (h *ImportHandler) HandleGetJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}

	tenantID := TenantID(r.Context())
	jobID := strings.TrimPrefix(r.URL.Path, "/admin/api/catalog/import/")
	jobID = strings.TrimSuffix(jobID, "/")

	job, err := h.importUC.GetJob(r.Context(), tenantID, jobID)
	if err != nil {
		writeError(w, http.StatusNotFound, "import job not found")
		return
	}

	writeJSON(w, http.StatusOK, job)
}

func (h *ImportHandler) HandleListJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}

	tenantID := TenantID(r.Context())
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	if limit <= 0 {
		limit = 20
	}

	jobs, total, err := h.importUC.ListJobs(r.Context(), tenantID, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list imports")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"imports": jobs,
		"total":   total,
	})
}
