package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"keepstar_v5/internal/domain"
)

// initRequest builds a POST /api/v1/session/init request with a resolved
// tenant already in context (what WithTenant does in production).
func initRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/session/init", strings.NewReader(body))
	ctx := context.WithValue(req.Context(), ctxKeyTenant{},
		&domain.Tenant{ID: "11111111-1111-1111-1111-111111111111", Slug: "acme", Name: "Acme"})
	return req.WithContext(ctx)
}

// R17 gate paths that must reject BEFORE any DB access (the handler under
// test carries a nil pool — reaching the pool would panic, which doubles as
// the ordering assertion: validation strictly precedes persistence).
func TestSessionInit_ModeValidation(t *testing.T) {
	h := &SessionHandler{} // nil state, nil pool on purpose

	cases := []struct {
		name     string
		body     string
		wantCode int
	}{
		// onboarding sessions only via POST /api/v1/onboard/session (R17).
		{"onboarding rejected", `{"mode":"onboarding"}`, http.StatusForbidden},
		// crm without a surface token never reaches the token lookup (R13).
		{"crm without token", `{"mode":"crm"}`, http.StatusForbidden},
		// forms are data, but session/init accepts storefront|crm only (R17).
		{"unknown mode", `{"mode":"kanban"}`, http.StatusBadRequest},
		{"malformed JSON", `{"mode":`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.Init(rec, initRequest(tc.body))
			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d (body %q)", rec.Code, tc.wantCode, rec.Body.String())
			}
		})
	}
}
