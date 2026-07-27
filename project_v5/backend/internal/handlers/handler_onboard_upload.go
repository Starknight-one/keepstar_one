package handlers

import (
	"io"
	"net/http"
	"path"
	"strings"
	"time"
)

// Onboarding upload flow (RUNTIME_SPEC.md §5.4, R25) — browser → v5 → admin,
// no CORS, no key exposure:
//
//	POST /api/v1/onboard/upload            multipart {token, file} → 202 {jobId}
//	GET  /api/v1/onboard/upload/{jobId}?sessionId=  → honest admin job status
//
// The token is a v5_ingest_tokens row minted by the issue_ingest_door
// applier step: session-bound, format-scoped, expiring, consumed on the
// first COMPLETED import (a failed import leaves it usable — R25 re-upload).

// onboardUploadMaxBytes caps one upload at 20MB (§5.4; matches admin's
// MaxBytesReader on the service import route).
const onboardUploadMaxBytes = 20 << 20

// Upload handles POST /api/v1/onboard/upload. The multipart body MUST carry
// the "token" field BEFORE the file part — the file is streamed to admin
// without buffering, so the token has to be validated first (the widget's
// FormData appends token first; field order is preserved on the wire).
func (h *OnboardHandler) Upload(w http.ResponseWriter, r *http.Request) {
	if !checkOnboardCookie(w, r) {
		return
	}
	if !h.checkCheap(w, r) {
		return
	}
	if h.tokens == nil || h.gateway == nil || h.applier == nil {
		http.Error(w, "onboarding upload not configured", http.StatusServiceUnavailable)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, onboardUploadMaxBytes)
	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "multipart body required", http.StatusBadRequest)
		return
	}

	var tokenValue string
	for {
		part, perr := mr.NextPart()
		if perr != nil {
			// EOF without a file part, or a read error mid-stream.
			http.Error(w, "multipart body has no file part", http.StatusBadRequest)
			return
		}

		if part.FileName() == "" {
			if part.FormName() == "token" {
				tokenValue = readSmallPart(part, 128)
			}
			part.Close()
			continue
		}

		// File part reached — the token must already be on hand.
		defer part.Close()
		if tokenValue == "" {
			http.Error(w, "token field must precede the file part", http.StatusBadRequest)
			return
		}
		tok, err := h.tokens.GetIngestToken(r.Context(), tokenValue)
		if err != nil {
			http.Error(w, "invalid upload token", http.StatusForbidden)
			return
		}
		now := time.Now()
		if tok.UsedAt != nil {
			http.Error(w, "upload token already used", http.StatusConflict)
			return
		}
		if !tok.Valid(now) {
			http.Error(w, "upload token expired", http.StatusForbidden)
			return
		}

		fileName := path.Base(part.FileName())
		format := formatOfUpload(fileName)
		if format == "" {
			http.Error(w, "unsupported file format (want .csv or .json)", http.StatusBadRequest)
			return
		}
		if !tok.AllowsFormat(format) {
			http.Error(w, "format not allowed for this upload token", http.StatusBadRequest)
			return
		}
		if tok.TenantSlug == "" {
			// Token references a tenant the catalog cannot resolve — a broken
			// manifest state, not a client error.
			h.log.Error("onboard upload: tenant slug unresolved for token", "tenant_id", tok.TenantID)
			http.Error(w, "tenant unresolved for upload token", http.StatusInternalServerError)
			return
		}

		// Stream the file part straight through to admin (X-Service-Key on
		// the gateway side). MaxBytesReader keeps the 20MB cap on the pipe.
		job, err := h.gateway.StartImport(r.Context(), tok.TenantSlug, fileName, part)
		if err != nil {
			h.log.Error("onboard upload: admin import start failed",
				"tenant", tok.TenantSlug, "file", fileName, "err", err)
			http.Error(w, "import start failed", http.StatusBadGateway)
			return
		}

		// Stamp the job onto the issue_ingest_door step (best-effort: the
		// admin job is already running; the poll endpoint completes the step).
		if err := h.applier.RecordUploadJob(r.Context(), tok.SessionID, job.JobID, fileName); err != nil {
			h.log.Warn("onboard upload: record job on manifest failed",
				"session", tok.SessionID, "job", job.JobID, "err", err)
		}

		writeJSON(w, http.StatusAccepted, map[string]any{
			"jobId":      job.JobID,
			"status":     job.Status,
			"totalItems": job.TotalItems,
			"sessionId":  tok.SessionID,
		})
		return
	}
}

// UploadStatus handles GET /api/v1/onboard/upload/{jobId}?sessionId= —
// proxies the honest admin job status (R7) and, on a terminal status,
// transitions the issue_ingest_door step: completed → applied (+ token
// consumed); failed → step stays accepted with the error recorded so the
// agent can relay it and offer a re-upload (R25).
func (h *OnboardHandler) UploadStatus(w http.ResponseWriter, r *http.Request) {
	if !checkOnboardCookie(w, r) {
		return
	}
	if !h.checkCheap(w, r) {
		return
	}
	if h.gateway == nil || h.applier == nil || h.onboardState == nil {
		http.Error(w, "onboarding upload not configured", http.StatusServiceUnavailable)
		return
	}
	jobID := r.PathValue("jobId")
	sessionID := r.URL.Query().Get("sessionId")
	if jobID == "" || sessionID == "" {
		http.Error(w, "jobId and sessionId are required", http.StatusBadRequest)
		return
	}

	m, err := h.onboardState.GetOnboarding(r.Context(), sessionID)
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	if m == nil || m.Tenant.Slug == "" {
		http.Error(w, "manifest not applied", http.StatusConflict)
		return
	}

	status, err := h.gateway.ImportStatus(r.Context(), m.Tenant.Slug, jobID)
	if err != nil {
		h.log.Error("onboard upload: status proxy failed",
			"tenant", m.Tenant.Slug, "job", jobID, "err", err)
		http.Error(w, "import status unavailable", http.StatusBadGateway)
		return
	}

	switch status.Status {
	case "completed":
		if err := h.applier.CompleteIngestStep(r.Context(), sessionID, status); err != nil {
			h.log.Warn("onboard upload: ingest step completion failed",
				"session", sessionID, "job", jobID, "err", err)
		}
	case "failed":
		if err := h.applier.RecordIngestFailure(r.Context(), sessionID, status.Errors); err != nil {
			h.log.Warn("onboard upload: ingest failure record failed",
				"session", sessionID, "job", jobID, "err", err)
		}
	}

	errs := status.Errors
	if errs == nil {
		errs = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"jobId":          status.JobID,
		"status":         status.Status,
		"processed":      status.Processed,
		"totalItems":     status.TotalItems,
		"projectionRows": status.ProjectionRows,
		"invalidated":    status.Invalidated,
		"errors":         errs,
	})
}

// formatOfUpload maps a file name onto the ingest format vocabulary.
func formatOfUpload(fileName string) string {
	switch strings.ToLower(path.Ext(fileName)) {
	case ".csv":
		return "csv"
	case ".json":
		return "json"
	}
	return ""
}

// readSmallPart reads a small text field part with a hard byte limit.
func readSmallPart(part io.Reader, limit int64) string {
	raw, _ := io.ReadAll(io.LimitReader(part, limit))
	return strings.TrimSpace(string(raw))
}
