package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

func liveDoorToken() *ports.IngestToken {
	return &ports.IngestToken{
		Token:      "door-token-1",
		SessionID:  "sess-1",
		TenantID:   "t-1",
		TenantSlug: "acme-realty",
		Formats:    []string{"csv", "json"},
		ExpiresAt:  time.Now().Add(time.Hour),
	}
}

// uploadRequest builds a multipart POST with the token field FIRST (the
// documented wire order — the file streams through without buffering).
func uploadRequest(t *testing.T, token, fileName, content string, tokenFirst bool) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	writeToken := func() {
		if token != "" {
			if err := mw.WriteField("token", token); err != nil {
				t.Fatalf("write token: %v", err)
			}
		}
	}
	if tokenFirst {
		writeToken()
	}
	fw, err := mw.CreateFormFile("file", fileName)
	if err != nil {
		t.Fatalf("create file part: %v", err)
	}
	fw.Write([]byte(content))
	if !tokenFirst {
		writeToken()
	}
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/onboard/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: onboardCookieName, Value: expectedOnboardCookie("pw")})
	return req
}

// Happy path: a valid session-bound token streams the file to the admin
// gateway, records the job on the ingest step, and answers 202 {jobId}.
func TestOnboardUpload(t *testing.T) {
	t.Setenv("ONBOARDING_PASSWORD", "pw")
	state := &onboardStateFake{manifest: registeredManifest()}
	gw := &onboardGatewayFake{}
	tokens := &onboardTokensFake{token: liveDoorToken()}
	h := newOnboardTestHandler(state, gw, tokens)

	rec := httptest.NewRecorder()
	h.Upload(rec, uploadRequest(t, "door-token-1", "listings.csv", "name,price\nCasa,100\n", true))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d body %q", rec.Code, rec.Body.String())
	}
	var resp struct {
		JobID     string `json:"jobId"`
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.JobID != "job-1" || resp.SessionID != "sess-1" {
		t.Fatalf("resp = %+v", resp)
	}
	if gw.importSlug != "acme-realty" || gw.importedFile != "listings.csv" {
		t.Fatalf("gateway got slug=%q file=%q", gw.importSlug, gw.importedFile)
	}
	if gw.importedBody != "name,price\nCasa,100\n" {
		t.Fatalf("streamed content = %q", gw.importedBody)
	}
	// The job is stamped on the ingest step for the poll + next agent turn.
	m, _ := state.GetOnboarding(context.Background(), "sess-1")
	if m.Steps[1].Result["jobId"] != "job-1" {
		t.Fatalf("job not recorded on ingest step: %v", m.Steps[1].Result)
	}
}

// Upload guard matrix: cookie, token presence/order, validity, format.
func TestOnboardUploadGuards(t *testing.T) {
	t.Setenv("ONBOARDING_PASSWORD", "pw")

	newH := func(tok *ports.IngestToken) (*OnboardHandler, *onboardGatewayFake) {
		gw := &onboardGatewayFake{}
		return newOnboardTestHandler(&onboardStateFake{manifest: registeredManifest()}, gw, &onboardTokensFake{token: tok}), gw
	}

	// No cookie → 403.
	h, _ := newH(liveDoorToken())
	req := uploadRequest(t, "door-token-1", "l.csv", "x", true)
	req.Header.Del("Cookie")
	rec := httptest.NewRecorder()
	h.Upload(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("no cookie: %d, want 403", rec.Code)
	}

	// Unknown token → 403; nothing reaches the gateway.
	h, gw := newH(liveDoorToken())
	rec = httptest.NewRecorder()
	h.Upload(rec, uploadRequest(t, "who-dis", "l.csv", "x", true))
	if rec.Code != http.StatusForbidden || gw.importedFile != "" {
		t.Fatalf("unknown token: %d imported=%q, want 403 + no import", rec.Code, gw.importedFile)
	}

	// Token AFTER the file part → 400 (streaming requires token first).
	h, _ = newH(liveDoorToken())
	rec = httptest.NewRecorder()
	h.Upload(rec, uploadRequest(t, "door-token-1", "l.csv", "x", false))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("token after file: %d, want 400", rec.Code)
	}

	// Used token → 409.
	used := liveDoorToken()
	now := time.Now()
	used.UsedAt = &now
	h, _ = newH(used)
	rec = httptest.NewRecorder()
	h.Upload(rec, uploadRequest(t, "door-token-1", "l.csv", "x", true))
	if rec.Code != http.StatusConflict {
		t.Fatalf("used token: %d, want 409", rec.Code)
	}

	// Expired token → 403.
	expired := liveDoorToken()
	expired.ExpiresAt = time.Now().Add(-time.Minute)
	h, _ = newH(expired)
	rec = httptest.NewRecorder()
	h.Upload(rec, uploadRequest(t, "door-token-1", "l.csv", "x", true))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expired token: %d, want 403", rec.Code)
	}

	// Unsupported extension → 400.
	h, _ = newH(liveDoorToken())
	rec = httptest.NewRecorder()
	h.Upload(rec, uploadRequest(t, "door-token-1", "l.xlsx", "x", true))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad extension: %d, want 400", rec.Code)
	}

	// Format outside the token's scope → 400.
	csvOnly := liveDoorToken()
	csvOnly.Formats = []string{"csv"}
	h, _ = newH(csvOnly)
	rec = httptest.NewRecorder()
	h.Upload(rec, uploadRequest(t, "door-token-1", "l.json", `[{"name":"x"}]`, true))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("format outside token scope: %d, want 400", rec.Code)
	}

	// Unwired deps → 503, honest.
	bare := NewOnboardHandler(nil, nil, nil, onboardTestLog())
	rec = httptest.NewRecorder()
	bare.Upload(rec, uploadRequest(t, "door-token-1", "l.csv", "x", true))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unwired: %d, want 503", rec.Code)
	}
}

// uploadSessionRequest builds a multipart POST carrying sessionId (and
// optionally a token) BEFORE the file part — the owner-2026-07-28 wire
// shape where the token is optional.
func uploadSessionRequest(t *testing.T, token, sessionID, fileName, content string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if token != "" {
		if err := mw.WriteField("token", token); err != nil {
			t.Fatalf("write token: %v", err)
		}
	}
	if sessionID != "" {
		if err := mw.WriteField("sessionId", sessionID); err != nil {
			t.Fatalf("write sessionId: %v", err)
		}
	}
	fw, err := mw.CreateFormFile("file", fileName)
	if err != nil {
		t.Fatalf("create file part: %v", err)
	}
	fw.Write([]byte(content))
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/onboard/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.AddCookie(&http.Cookie{Name: onboardCookieName, Value: expectedOnboardCookie("pw")})
	return req
}

// The user's upload IS the approval (owner 2026-07-28): NO token, only a
// sessionId, against a fully-PROPOSED manifest — the server auto-applies
// (provisioning the tenant, minting the door) and streams the file to
// admin. Never a 409 "apply the manifest first".
func TestOnboardUploadSessionFallbackAutoApplies(t *testing.T) {
	t.Setenv("ONBOARDING_PASSWORD", "pw")
	state := &onboardStateFake{manifest: proposedManifest()}
	gw := &onboardGatewayFake{}
	h := newOnboardTestHandler(state, gw, &onboardTokensFake{})

	rec := httptest.NewRecorder()
	h.Upload(rec, uploadSessionRequest(t, "", "sess-1", "listings.csv", "name,price\nCasa,100\n"))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d body %q — the tokenless upload must auto-apply, not bounce", rec.Code, rec.Body.String())
	}
	if gw.importSlug != "acme-realty" || gw.importedFile != "listings.csv" {
		t.Fatalf("gateway got slug=%q file=%q", gw.importSlug, gw.importedFile)
	}
	if gw.importedBody != "name,price\nCasa,100\n" {
		t.Fatalf("streamed content = %q", gw.importedBody)
	}

	m, _ := state.GetOnboarding(context.Background(), "sess-1")
	if m.Tenant.Slug != "acme-realty" {
		t.Fatalf("tenant not provisioned by the auto-apply: %+v", m.Tenant)
	}
	door := m.Steps[1]
	if tok, _ := door.Result["token"].(string); tok == "" {
		t.Fatalf("door not armed: %+v", door)
	}
	if door.Result["jobId"] != "job-1" {
		t.Fatalf("job not recorded on the door step: %v", door.Result)
	}
}

// A STALE explicit token accompanied by a sessionId is rescued server-side:
// the door re-mints and the upload proceeds (without a sessionId the same
// stale token keeps today's 403 — covered by TestOnboardUploadGuards).
func TestOnboardUploadStaleTokenWithSessionRescues(t *testing.T) {
	t.Setenv("ONBOARDING_PASSWORD", "pw")
	expired := liveDoorToken()
	expired.ExpiresAt = time.Now().Add(-time.Minute)
	state := &onboardStateFake{manifest: registeredManifest()}
	gw := &onboardGatewayFake{}
	h := newOnboardTestHandler(state, gw, &onboardTokensFake{token: expired})

	rec := httptest.NewRecorder()
	h.Upload(rec, uploadSessionRequest(t, "door-token-1", "sess-1", "listings.csv", "name\nCasa\n"))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d body %q — stale token + sessionId must resolve server-side", rec.Code, rec.Body.String())
	}
	if gw.importedFile != "listings.csv" {
		t.Fatalf("file did not reach the gateway: %q", gw.importedFile)
	}
	// The fresh token is stamped back onto the door step.
	m, _ := state.GetOnboarding(context.Background(), "sess-1")
	if tok, _ := m.Steps[1].Result["token"].(string); tok == "" || tok == "door-token-1" {
		t.Fatalf("door step still carries the stale token: %v", m.Steps[1].Result)
	}
}

func uploadStatusRequest(t *testing.T, jobID, sessionID string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/onboard/upload/"+jobID+"?sessionId="+sessionID, nil)
	req.SetPathValue("jobId", jobID)
	req.AddCookie(&http.Cookie{Name: onboardCookieName, Value: expectedOnboardCookie("pw")})
	return req
}

// The poll proxies admin's HONEST status and transitions the ingest step:
// completed → applied + token consumed; failed → stays accepted with the
// error recorded (R25 re-upload stays open).
func TestOnboardUploadStatus(t *testing.T) {
	t.Setenv("ONBOARDING_PASSWORD", "pw")
	state := &onboardStateFake{manifest: registeredManifest()}
	gw := &onboardGatewayFake{statusResp: &ports.AdminImportStatus{
		JobID: "job-1", Status: "failed", Errors: []string{"row 2: no name"},
	}}
	tokens := &onboardTokensFake{token: liveDoorToken()}
	h := newOnboardTestHandler(state, gw, tokens)

	// Failed import: step stays accepted, error recorded, token NOT consumed.
	rec := httptest.NewRecorder()
	h.UploadStatus(rec, uploadStatusRequest(t, "job-1", "sess-1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("failed poll: status = %d body %q", rec.Code, rec.Body.String())
	}
	m, _ := state.GetOnboarding(context.Background(), "sess-1")
	if m.Steps[1].Status != domain.ManifestStepAccepted || m.Steps[1].Result["lastError"] == "" {
		t.Fatalf("failed import handling wrong: %+v", m.Steps[1])
	}
	if len(tokens.used) != 0 {
		t.Fatalf("token consumed on a FAILED import — re-upload is now impossible")
	}

	// Completed import: step applied with the honest outcome, token consumed.
	gw.statusResp = &ports.AdminImportStatus{
		JobID: "job-1", Status: "completed", Processed: 20, TotalItems: 20,
		ProjectionRows: 20, Invalidated: true, Errors: []string{},
	}
	rec = httptest.NewRecorder()
	h.UploadStatus(rec, uploadStatusRequest(t, "job-1", "sess-1"))
	if rec.Code != http.StatusOK {
		t.Fatalf("completed poll: status = %d", rec.Code)
	}
	var resp struct {
		Status         string `json:"status"`
		ProjectionRows int    `json:"projectionRows"`
		Invalidated    bool   `json:"invalidated"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.Status != "completed" || resp.ProjectionRows != 20 || !resp.Invalidated {
		t.Fatalf("honest status lost in proxy: %+v", resp)
	}
	m, _ = state.GetOnboarding(context.Background(), "sess-1")
	if m.Steps[1].Status != domain.ManifestStepApplied {
		t.Fatalf("ingest step = %q, want applied", m.Steps[1].Status)
	}
	if len(tokens.used) != 1 || tokens.used[0] != "door-token-1" {
		t.Fatalf("token not consumed on completion: %v", tokens.used)
	}

	// Missing params → 400.
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/onboard/upload/job-1", nil)
	req.SetPathValue("jobId", "job-1")
	req.AddCookie(&http.Cookie{Name: onboardCookieName, Value: expectedOnboardCookie("pw")})
	h.UploadStatus(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing sessionId: %d, want 400", rec.Code)
	}
}
