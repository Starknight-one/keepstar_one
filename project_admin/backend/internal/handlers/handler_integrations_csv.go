package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/usecases"
)

// CSVIntegrationsHandler serves the two CSV endpoints used by the onboarding
// UI: suggest-mapping (LLM proposes column→field map) and import (transform
// confirmed mapping into ImportItems and schedule an ImportJob).
type CSVIntegrationsHandler struct {
	mapping      *usecases.CSVMappingUseCase // may be nil when ANTHROPIC_API_KEY absent
	importer     *usecases.ImportUseCase
	integrations *usecases.IntegrationsUseCase
	log          *logger.Logger
}

func NewCSVIntegrationsHandler(mapping *usecases.CSVMappingUseCase, importer *usecases.ImportUseCase, integrations *usecases.IntegrationsUseCase, log *logger.Logger) *CSVIntegrationsHandler {
	return &CSVIntegrationsHandler{mapping: mapping, importer: importer, integrations: integrations, log: log}
}

type csvSuggestRequest struct {
	Headers    []string   `json:"headers"`
	SampleRows [][]string `json:"sampleRows"`
}

// HandleSuggestMapping — POST /admin/api/integrations/csv/suggest-mapping
func (h *CSVIntegrationsHandler) HandleSuggestMapping(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	if h.mapping == nil {
		writeError(w, http.StatusServiceUnavailable, "AI mapping unavailable — ANTHROPIC_API_KEY not configured")
		return
	}
	var req csvSuggestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if len(req.Headers) == 0 {
		writeError(w, http.StatusBadRequest, "no headers")
		return
	}
	suggestion, err := h.mapping.SuggestMapping(r.Context(), req.Headers, req.SampleRows)
	if err != nil {
		h.log.FromContext(r.Context()).Error("csv_suggest_failed", "error", err)
		writeError(w, http.StatusInternalServerError, "mapping suggestion failed")
		return
	}
	writeJSON(w, http.StatusOK, suggestion)
}

type csvImportRequest struct {
	Headers  []string          `json:"headers"`
	Rows     [][]string        `json:"rows"`
	Mapping  map[string]string `json:"mapping"`
	Filename string            `json:"filename"`
}

const csvRowLimit = 25000 // hard cap — above this, recommend Shopify path

// HandleImport — POST /admin/api/integrations/csv/import
func (h *CSVIntegrationsHandler) HandleImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var req csvImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if len(req.Headers) == 0 || len(req.Rows) == 0 {
		writeError(w, http.StatusBadRequest, "headers and rows required")
		return
	}
	if len(req.Rows) > csvRowLimit {
		writeError(w, http.StatusRequestEntityTooLarge,
			"CSV exceeds 25,000 rows — split the file or use the Shopify integration")
		return
	}
	tenantID := TenantID(r.Context())

	filename := strings.TrimSpace(req.Filename)
	if filename == "" {
		filename = "upload.csv"
	}

	// Transform confirmed mapping into ImportItems (authoritative on server).
	sourcePrefix := strings.TrimSuffix(filename, ".csv") + "-" + time.Now().UTC().Format("20060102T150405")
	items := usecases.ApplyMapping(req.Headers, req.Rows, req.Mapping, "csv", sourcePrefix)
	if len(items) == 0 {
		writeError(w, http.StatusBadRequest, "no valid rows after applying mapping")
		return
	}

	job, err := h.importer.UploadWithJobName(r.Context(), tenantID, filename, items)
	if err != nil {
		h.log.FromContext(r.Context()).Error("csv_import_upload_failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to start import")
		return
	}

	// Record the CSV connection so users see it in /integrations with row
	// counts and the mapping they confirmed. Idempotent by (tenant, kind,
	// external_id) — repeated same-filename uploads overwrite.
	configMap := map[string]any{
		"mapping":   req.Mapping,
		"row_count": len(req.Rows),
	}
	integrationRepo := h.integrations.Repo()
	_, createErr := integrationRepo.Create(r.Context(), &domain.Integration{
		TenantID:      tenantID,
		Kind:          domain.IntegrationKindCSV,
		Status:        domain.IntegrationStatusSyncing,
		DisplayName:   filename,
		ExternalID:    filename,
		Config:        configMap,
		LastSyncJobID: job.ID,
	})
	if createErr != nil {
		h.log.FromContext(r.Context()).Error("csv_integration_record_failed", "error", createErr)
		// Don't fail the import — the job is already running.
	}

	writeJSON(w, http.StatusAccepted, map[string]any{
		"jobId":    job.ID,
		"fileName": job.FileName,
		"total":    job.TotalItems,
	})
}

