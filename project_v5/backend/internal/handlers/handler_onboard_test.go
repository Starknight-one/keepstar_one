package handlers

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"keepstar_v5/internal/domain"
)

func onboardTestLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

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
	h := NewOnboardHandler(nil, &fakeCatalogPort{}, nil, onboardTestLog())

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

func gateRequest(body string) *http.Request {
	return httptest.NewRequest(http.MethodPost, "/api/v1/onboard/gate", strings.NewReader(body))
}

// R5 gate endpoint: unset env → 503; wrong password → 403; right password →
// the ks_onboard HMAC cookie with the exact attribute set (HttpOnly,
// SameSite=Lax, 7d, Path=/).
func TestOnboardGate(t *testing.T) {
	t.Setenv("ONBOARDING_PASSWORD", "")
	h := NewOnboardHandler(nil, nil, nil, onboardTestLog())
	rec := httptest.NewRecorder()
	h.Gate(rec, gateRequest(`{"password":"x"}`))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unset env: status = %d, want 503", rec.Code)
	}

	t.Setenv("ONBOARDING_PASSWORD", "Bb8tj13k")
	h = NewOnboardHandler(nil, nil, nil, onboardTestLog())

	rec = httptest.NewRecorder()
	h.Gate(rec, gateRequest(`{"password":"wrong"}`))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("wrong password: status = %d, want 403", rec.Code)
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatalf("wrong password must not set a cookie")
	}

	rec = httptest.NewRecorder()
	h.Gate(rec, gateRequest(`not json`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed body: status = %d, want 400", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.Gate(rec, gateRequest(`{"password":"Bb8tj13k"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("right password: status = %d body %q", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	ck := cookies[0]
	if ck.Name != onboardCookieName || ck.Value != expectedOnboardCookie("Bb8tj13k") {
		t.Fatalf("cookie = %s=%s, want the HMAC value", ck.Name, ck.Value)
	}
	if !ck.HttpOnly || ck.SameSite != http.SameSiteLaxMode || ck.MaxAge != onboardCookieMaxAge || ck.Path != "/" {
		t.Fatalf("cookie attributes wrong: HttpOnly=%v SameSite=%v MaxAge=%d Path=%q", ck.HttpOnly, ck.SameSite, ck.MaxAge, ck.Path)
	}
}

// The gate is rate-limited 5/min/IP (R5) — the 6th attempt from one IP is
// bounced BEFORE the password compare, so brute-forcing burns the bucket
// whether guesses are right or wrong.
func TestOnboardGateRateLimit(t *testing.T) {
	t.Setenv("ONBOARDING_PASSWORD", "pw")
	h := NewOnboardHandler(nil, nil, nil, onboardTestLog())

	for i := 0; i < onboardGateRatePerMin; i++ {
		rec := httptest.NewRecorder()
		h.Gate(rec, gateRequest(`{"password":"wrong"}`))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("attempt %d: status = %d, want 403", i+1, rec.Code)
		}
	}
	rec := httptest.NewRecorder()
	h.Gate(rec, gateRequest(`{"password":"pw"}`))
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("6th attempt: status = %d, want 429 (rate limit before compare)", rec.Code)
	}
}

// The 60-turn cap counts USER turns only — tool traffic and assistant
// messages must not eat the budget (§6.3).
func TestOnboardingTurnCap(t *testing.T) {
	var history []domain.LLMMessage
	for i := 0; i < OnboardingTurnCap-1; i++ {
		history = append(history,
			domain.LLMMessage{Role: "user", Content: "turn"},
			domain.LLMMessage{Role: "assistant", ToolCalls: []domain.ToolCall{{ID: "t", Name: "search_library"}}},
			domain.LLMMessage{Role: "user", ToolResult: &domain.ToolResult{ToolUseID: "t", Content: "ok"}},
			domain.LLMMessage{Role: "assistant", Content: "reply"},
		)
	}
	if got := OnboardingTurnsUsed(history); got != OnboardingTurnCap-1 {
		t.Fatalf("turns used = %d, want %d (tool results must not count)", got, OnboardingTurnCap-1)
	}
	if OnboardingTurnCapReached(history) {
		t.Fatalf("cap reported reached one turn early")
	}
	history = append(history, domain.LLMMessage{Role: "user", Content: "one more"})
	if !OnboardingTurnCapReached(history) {
		t.Fatalf("cap not reached at %d user turns", OnboardingTurnCap)
	}
}

// Resume transcript carries plain user/assistant text only — tool calls and
// tool results never reach the client.
func TestTranscriptOf(t *testing.T) {
	history := []domain.LLMMessage{
		{Role: "user", Content: "I run a realtor agency"},
		{Role: "assistant", ToolCalls: []domain.ToolCall{{ID: "1", Name: "search_library"}}},
		{Role: "user", ToolResult: &domain.ToolResult{ToolUseID: "1", Content: "internal"}},
		{Role: "assistant", Content: "Here is the plan"},
	}
	tr := transcriptOf(history)
	if len(tr) != 2 {
		t.Fatalf("transcript entries = %d, want 2: %v", len(tr), tr)
	}
	if tr[0]["role"] != "user" || tr[0]["text"] != "I run a realtor agency" {
		t.Errorf("entry 0 = %v", tr[0])
	}
	if tr[1]["role"] != "assistant" || tr[1]["text"] != "Here is the plan" {
		t.Errorf("entry 1 = %v", tr[1])
	}
	for _, e := range tr {
		if strings.Contains(e["text"], "internal") {
			t.Errorf("tool traffic leaked into transcript: %v", e)
		}
	}
}
