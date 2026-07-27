package handlers

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- fakes -----------------------------------------------------------------

type recordingInvalidator struct{ calls []string }

func (r *recordingInvalidator) Invalidate(tenant string) { r.calls = append(r.calls, tenant) }

type recordingRegistry struct{ calls []string }

func (r *recordingRegistry) InvalidateTenant(tenant string) { r.calls = append(r.calls, tenant) }

func newCacheTestHandler() (*CacheHandler, *recordingInvalidator, *recordingInvalidator, *recordingRegistry) {
	a1 := &recordingInvalidator{}
	a2 := &recordingInvalidator{}
	reg := &recordingRegistry{}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewCacheHandler(a1, a2, reg, log), a1, a2, reg
}

func postInvalidate(h http.HandlerFunc, url, key string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, url, nil)
	if key != "" {
		req.Header.Set("X-Internal-Key", key)
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

// --- scope routing (R15) ---------------------------------------------------

// scope=agent1 must drop the Agent1 prompt cache AND the registry spec cache
// together; scope=agent2 only the Agent2 prompt cache; all = everything. This
// is the §6.1 cache matrix — a wrong scope silently serves stale prompts in
// production.
func TestCacheInvalidate_ScopeRouting(t *testing.T) {
	t.Setenv("V5_INTERNAL_KEY", "k")
	cases := []struct {
		scope                   string
		wantA1, wantA2, wantReg bool
	}{
		{"agent1", true, false, true},
		{"agent2", false, true, false},
		{"all", true, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.scope, func(t *testing.T) {
			h, a1, a2, reg := newCacheTestHandler()
			rec := postInvalidate(h.Invalidate, "/api/v1/internal/cache/invalidate?tenant=acme&scope="+tc.scope, "k")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
			}
			if got := len(a1.calls) == 1; got != tc.wantA1 {
				t.Errorf("agent1 invalidated = %v, want %v", got, tc.wantA1)
			}
			if got := len(a2.calls) == 1; got != tc.wantA2 {
				t.Errorf("agent2 invalidated = %v, want %v", got, tc.wantA2)
			}
			if got := len(reg.calls) == 1; got != tc.wantReg {
				t.Errorf("registry invalidated = %v, want %v", got, tc.wantReg)
			}
			if tc.wantA1 && a1.calls[0] != "acme" {
				t.Errorf("agent1 tenant = %q, want acme", a1.calls[0])
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("body not JSON: %v", err)
			}
			if body["invalidated"] != true || body["tenant"] != "acme" || body["scope"] != tc.scope {
				t.Errorf("body = %v, want {tenant:acme scope:%s invalidated:true}", body, tc.scope)
			}
		})
	}
}

// Bad input must not fire any invalidation — a typo'd scope silently doing
// nothing OR doing everything are both wrong.
func TestCacheInvalidate_Validation(t *testing.T) {
	t.Setenv("V5_INTERNAL_KEY", "k")
	for name, url := range map[string]string{
		"missing tenant": "/api/v1/internal/cache/invalidate?scope=all",
		"missing scope":  "/api/v1/internal/cache/invalidate?tenant=acme",
		"bad scope":      "/api/v1/internal/cache/invalidate?tenant=acme&scope=everything",
	} {
		t.Run(name, func(t *testing.T) {
			h, a1, a2, reg := newCacheTestHandler()
			rec := postInvalidate(h.Invalidate, url, "k")
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", rec.Code)
			}
			if len(a1.calls)+len(a2.calls)+len(reg.calls) != 0 {
				t.Errorf("invalidation fired on invalid input: a1=%v a2=%v reg=%v", a1.calls, a2.calls, reg.calls)
			}
		})
	}
}

// X-Internal-Key gate: unset env → 503 (disabled), mismatch → 403 — same
// contract as the kill switch (checkInternalKey).
func TestCacheInvalidate_KeyGate(t *testing.T) {
	h, a1, _, _ := newCacheTestHandler()

	t.Setenv("V5_INTERNAL_KEY", "")
	rec := postInvalidate(h.Invalidate, "/api/v1/internal/cache/invalidate?tenant=acme&scope=all", "anything")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unset env: status = %d, want 503", rec.Code)
	}

	t.Setenv("V5_INTERNAL_KEY", "right")
	rec = postInvalidate(h.Invalidate, "/api/v1/internal/cache/invalidate?tenant=acme&scope=all", "wrong")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("wrong key: status = %d, want 403", rec.Code)
	}
	if len(a1.calls) != 0 {
		t.Errorf("invalidation fired without valid key")
	}
}

// The legacy presets/cache-invalidate route is an alias ≡ scope=agent2
// (R15): agent2 only, and the response keeps the legacy {tenant,
// invalidated} keys the deployed admin binary expects.
func TestCacheInvalidate_LegacyAliasIsAgent2(t *testing.T) {
	t.Setenv("V5_INTERNAL_KEY", "k")
	h, a1, a2, reg := newCacheTestHandler()
	rec := postInvalidate(h.InvalidateLegacy, "/api/v1/internal/presets/cache-invalidate?tenant=acme", "k")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	if len(a2.calls) != 1 || a2.calls[0] != "acme" {
		t.Errorf("agent2 calls = %v, want [acme]", a2.calls)
	}
	if len(a1.calls) != 0 || len(reg.calls) != 0 {
		t.Errorf("legacy alias touched agent1/registry: a1=%v reg=%v", a1.calls, reg.calls)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if body["tenant"] != "acme" || body["invalidated"] != true {
		t.Errorf("legacy body = %v, want tenant=acme invalidated=true", body)
	}
}
