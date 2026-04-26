// Package handlers — X-API-Key middleware for /api/v1/* (M10).
//
// Different from AuthMiddleware (JWT bearer). Public REST clients send a
// long-lived plain token in the X-API-Key header; the middleware resolves it
// to a tenant + key id and stamps both into the request context.
package handlers

import (
	"context"
	"net/http"

	"keepstar-admin/internal/ports"
)

const ctxAPIKeyID contextKey = "akid"

// APIKeyID returns the api_key id stored in context (set by APIKeyMiddleware).
// Empty for non-public-API requests.
func APIKeyID(ctx context.Context) string { v, _ := ctx.Value(ctxAPIKeyID).(string); return v }

func APIKeyMiddleware(keys ports.APIKeysPort) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			plain := r.Header.Get("X-API-Key")
			if plain == "" {
				writeError(w, http.StatusUnauthorized, "missing X-API-Key header")
				return
			}
			tenantID, apiKeyID, err := keys.Verify(r.Context(), plain)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid api key")
				return
			}
			// Touch best-effort, doesn't block the request on failure.
			go func() { _ = keys.TouchLastUsed(context.Background(), apiKeyID) }()

			ctx := context.WithValue(r.Context(), ctxTenantID, tenantID)
			ctx = context.WithValue(ctx, ctxAPIKeyID, apiKeyID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
