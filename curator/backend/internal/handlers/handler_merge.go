// Package handlers — merge-agent proxy to admin-backend (Phase D3,
// catalog completion 2026-04-28).
//
// The actual MergeApplyUseCase lives in admin-backend (project_admin/...) so
// it can share the catalog domain types and adapters with the agent /
// applier code. Curator-backend is a separate Go module — instead of
// duplicating the use case, it proxies HTTP requests on its `/curator/merge/*`
// surface to admin-backend's internal endpoints.
//
// Auth flow:
//   - Curator-frontend → curator-backend with the curator session cookie
//   - Session middleware authenticates the curator user (UserID in ctx)
//   - curator-backend → admin-backend with X-Internal-Key + X-Curator-User
//   - admin-backend's InternalKeyMiddleware accepts the header
//
// Configuration: ADMIN_BASE_URL (defaults to http://127.0.0.1:8081 — admin
// backend's local port) and CURATOR_INTERNAL_KEY (must match admin's env).
package handlers

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"keepstar-curator/internal/session"
)

// MergeProxy is a thin reverse-proxy keyed to a single backend.
type MergeProxy struct {
	adminBase string
	key       string
	client    *http.Client
}

func NewMergeProxy() *MergeProxy {
	base := firstNonEmptyEnv("ADMIN_BASE_URL", "http://127.0.0.1:8081")
	return &MergeProxy{
		adminBase: strings.TrimRight(base, "/"),
		key:       os.Getenv("CURATOR_INTERNAL_KEY"),
		client:    &http.Client{Timeout: 90 * time.Second},
	}
}

// HandleRun POST /curator/tenants/{id}/merge/run
func (p *MergeProxy) HandleRun(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromCuratorPath(r.URL.Path)
	if !ok {
		writeErr(w, http.StatusBadRequest, "tenant id missing")
		return
	}
	target := p.adminBase + "/admin/api/internal/curator/tenants/" + url.PathEscape(tenantID) + "/merge/run"
	p.proxy(w, r, http.MethodPost, target)
}

// HandleListReports GET /curator/tenants/{id}/merge-reports
func (p *MergeProxy) HandleListReports(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := tenantIDFromCuratorPath(r.URL.Path)
	if !ok {
		writeErr(w, http.StatusBadRequest, "tenant id missing")
		return
	}
	target := p.adminBase + "/admin/api/internal/curator/tenants/" + url.PathEscape(tenantID) + "/merge-reports"
	if q := r.URL.RawQuery; q != "" {
		target += "?" + q
	}
	p.proxy(w, r, http.MethodGet, target)
}

// HandleReport GET/POST /curator/merge-reports/{id}[/apply|/revert]
func (p *MergeProxy) HandleReport(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/curator/merge-reports/")
	if rest == "" {
		writeErr(w, http.StatusBadRequest, "report id missing")
		return
	}
	target := p.adminBase + "/admin/api/internal/curator/merge-reports/" + rest
	method := r.Method
	if method == "" {
		method = http.MethodGet
	}
	p.proxy(w, r, method, target)
}

func (p *MergeProxy) proxy(w http.ResponseWriter, r *http.Request, method, target string) {
	if p.key == "" {
		writeErr(w, http.StatusServiceUnavailable, "merge proxy not configured (CURATOR_INTERNAL_KEY missing)")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20)) // 8 MiB cap — proposal blobs aren't tiny
	if err != nil {
		writeErr(w, http.StatusBadRequest, "read body: "+err.Error())
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), method, target, bytes.NewReader(body))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "build request: "+err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Key", p.key)
	if uid := session.UserID(r.Context()); uid != "" {
		req.Header.Set("X-Curator-User", uid)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "admin backend unreachable: "+err.Error())
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func tenantIDFromCuratorPath(path string) (string, bool) {
	const prefix = "/curator/tenants/"
	if !strings.HasPrefix(path, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(path, prefix)
	idx := strings.Index(rest, "/")
	if idx < 0 {
		return "", false
	}
	return rest[:idx], true
}

func firstNonEmptyEnv(keys ...string) string {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}
