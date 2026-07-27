package handlers

import (
	"context"
	"errors"
	"io"
	"net/http"
	"path"
	"strings"
	"time"

	"keepstar_v5/internal/ports"
	"keepstar_v5/internal/usecases"
)

// Onboarding upload flow (RUNTIME_SPEC.md §5.4, R25) — browser → v5 → admin,
// no CORS, no key exposure:
//
//	POST /api/v1/onboard/upload            multipart {token?, sessionId?, file} → 202 {jobId}
//	GET  /api/v1/onboard/upload/{jobId}?sessionId=  → honest admin job status
//
// The token is a v5_ingest_tokens row minted by the issue_ingest_door
// applier step: session-bound, format-scoped, expiring, consumed on the
// first COMPLETED import (a failed import leaves it usable — R25 re-upload).
//
// The token is OPTIONAL (owner decision 2026-07-28): with the valid
// onboarding cookie + a sessionId field, a missing or stale token resolves
// server-side — ManifestApplier.ResolveIngestToken auto-applies the staged
// manifest when needed (minting the ingest door) and returns the session's
// live token. The user's upload IS the approval; it is never bounced with
// "apply the manifest first". The explicit-token fast path is kept.

// onboardUploadMaxBytes caps one upload at 20MB (§5.4; matches admin's
// MaxBytesReader on the service import route).
const onboardUploadMaxBytes = 20 << 20

// Upload handles POST /api/v1/onboard/upload. The multipart body MUST carry
// the "token" and/or "sessionId" fields BEFORE the file part — the file is
// streamed to admin without buffering, so the door has to be resolved first
// (the widget's FormData appends them first; field order is preserved on
// the wire).
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

	var tokenValue, sessionID string
	for {
		part, perr := mr.NextPart()
		if perr != nil {
			// EOF without a file part, or a read error mid-stream.
			http.Error(w, "multipart body has no file part", http.StatusBadRequest)
			return
		}

		if part.FileName() == "" {
			switch part.FormName() {
			case "token":
				tokenValue = readSmallPart(part, 128)
			case "sessionId":
				sessionID = readSmallPart(part, 128)
			}
			part.Close()
			continue
		}

		// File part reached — the door must be resolvable from what is on
		// hand: explicit token fast path, session fallback otherwise.
		defer part.Close()
		tok, status, msg := h.resolveUploadDoor(r.Context(), tokenValue, sessionID)
		if tok == nil {
			http.Error(w, msg, status)
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

// resolveUploadDoor turns whatever the multipart body carried (explicit
// token, sessionId, or both) into a live ingest token. Non-nil token means
// go; otherwise status+msg are the HTTP answer. Precedence:
//
//  1. a VALID explicit token wins (today's fast path, byte-identical);
//  2. a missing/stale/consumed token with a sessionId at hand resolves
//     server-side (ResolveIngestToken: auto-apply if needed, re-mint stale);
//  3. a bad token WITHOUT a sessionId keeps today's exact statuses;
//  4. neither field → 400.
func (h *OnboardHandler) resolveUploadDoor(ctx context.Context, tokenValue, sessionID string) (*ports.IngestToken, int, string) {
	if tokenValue != "" {
		tok, err := h.tokens.GetIngestToken(ctx, tokenValue)
		switch {
		case err == nil && tok.Valid(time.Now()):
			return tok, 0, ""
		case sessionID != "":
			// Stale/unknown token but the session is known — fall through to
			// the server-side resolution (owner 2026-07-28: the upload is
			// never bounced on a token the client couldn't refresh).
		case err != nil:
			return nil, http.StatusForbidden, "invalid upload token"
		case tok.UsedAt != nil:
			return nil, http.StatusConflict, "upload token already used"
		default:
			return nil, http.StatusForbidden, "upload token expired"
		}
	}
	if sessionID == "" {
		return nil, http.StatusBadRequest, "token or sessionId field must precede the file part"
	}
	tok, err := h.applier.ResolveIngestToken(ctx, sessionID)
	if err != nil {
		switch {
		case errors.Is(err, usecases.ErrNoManifest), errors.Is(err, usecases.ErrStepNotFound):
			return nil, http.StatusConflict, "no uploader staged for this session yet — ask the assistant to add a data upload to the plan"
		case errors.Is(err, usecases.ErrTenantNotProvisioned), errors.Is(err, usecases.ErrStepNotReady):
			// Carries the failed step's reason (owner 2026-07-28: say WHY).
			return nil, http.StatusConflict, err.Error()
		default:
			h.log.Error("onboard upload: session door resolution failed", "session", sessionID, "err", err)
			return nil, http.StatusInternalServerError, "upload door resolution failed"
		}
	}
	return tok, 0, ""
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
