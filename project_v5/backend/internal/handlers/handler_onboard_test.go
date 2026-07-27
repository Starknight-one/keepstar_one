package handlers

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestExpectedOnboardCookie pins the R5 cookie derivation:
// hex(HMAC-SHA256("keepstar-onboard-v1", key=ONBOARDING_PASSWORD)) —
// deterministic per password, different across passwords (rotation must
// invalidate every outstanding cookie by construction).
func TestExpectedOnboardCookie(t *testing.T) {
	a := expectedOnboardCookie("pw-one")
	if len(a) != 64 { // hex(SHA-256) = 64 chars
		t.Fatalf("cookie length = %d, want 64 hex chars", len(a))
	}
	if a != expectedOnboardCookie("pw-one") {
		t.Errorf("cookie not deterministic for the same password")
	}
	if a == expectedOnboardCookie("pw-two") {
		t.Errorf("cookie identical across different passwords — rotation would not kick cookies")
	}
}

func onboardSessionRequest(cookie string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/onboard/session", nil)
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: onboardCookieName, Value: cookie})
	}
	return req
}

// The R5 gate on session creation: unset password → 503 (disabled, never
// accidentally open); absent/wrong cookie → 403; the one valid cookie value
// passes the gate (proven by reaching the system-tenant resolution, which
// fails honest 503 with the fake empty catalog — no DB in unit tests).
func TestOnboardCreateSession_CookieGate(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewOnboardHandler(nil, &fakeCatalogPort{}, nil, log)

	t.Setenv("ONBOARDING_PASSWORD", "")
	rec := httptest.NewRecorder()
	h.CreateSession(rec, onboardSessionRequest("anything"))
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "onboarding disabled") {
		t.Fatalf("unset password: got %d %q, want 503 onboarding disabled", rec.Code, rec.Body.String())
	}

	t.Setenv("ONBOARDING_PASSWORD", "s3cret")
	rec = httptest.NewRecorder()
	h.CreateSession(rec, onboardSessionRequest(""))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("no cookie: status = %d, want 403", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.CreateSession(rec, onboardSessionRequest("deadbeef"))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("wrong cookie: status = %d, want 403", rec.Code)
	}

	// Valid cookie → past the gate; the fake catalog has no
	// keepstar-onboarding tenant, so the handler fails honest (503 tenant
	// unavailable), which is exactly the pre-seed deploy-order behavior.
	rec = httptest.NewRecorder()
	h.CreateSession(rec, onboardSessionRequest(expectedOnboardCookie("s3cret")))
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "onboarding tenant unavailable") {
		t.Fatalf("valid cookie: got %d %q, want 503 onboarding tenant unavailable", rec.Code, rec.Body.String())
	}
}
