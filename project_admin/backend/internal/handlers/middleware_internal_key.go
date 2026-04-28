// Package handlers — internal API key middleware (Phase D3, 2026-04-28).
//
// Used for /admin/api/internal/* endpoints called by sibling services
// (curator-backend) over HTTP. NOT for tenant-facing API keys (those use
// middleware_apikey.go); the keys are different scopes.
//
// The expected key is set via env CURATOR_INTERNAL_KEY at admin-server boot
// and shared with curator-backend's CURATOR_INTERNAL_KEY. Constant-time
// comparison prevents timing oracles. Empty server-side key disables the
// route entirely (defense-in-depth — never accidentally expose because of
// missing config).
package handlers

import (
	"crypto/subtle"
	"net/http"
)

// InternalKeyMiddleware wraps the inner handler with an X-Internal-Key check.
// Returns 503 when the server isn't configured (no key in env), 401 on bad key.
func InternalKeyMiddleware(expected string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if expected == "" {
				writeError(w, http.StatusServiceUnavailable, "internal endpoint not configured")
				return
			}
			got := r.Header.Get("X-Internal-Key")
			if subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
				writeError(w, http.StatusUnauthorized, "invalid internal key")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
