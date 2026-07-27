package handlers

import (
	"context"
	"net/http"
	"strings"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

type ctxKeyTenant struct{}

// TenantFromContext pulls the resolved Tenant from request context.
// Returns nil when middleware wasn't applied or resolution failed —
// callers should bail with 503 in that case.
func TenantFromContext(ctx context.Context) *domain.Tenant {
	v, _ := ctx.Value(ctxKeyTenant{}).(*domain.Tenant)
	return v
}

// WithTenant resolves the tenant from the X-Tenant-Slug header (falling
// back to defaultSlug — set from TENANT_SLUG env). The resolved
// *domain.Tenant lands in request context for downstream handlers.
//
// On lookup failure: 503 Service Unavailable. The chat widget can retry;
// a missing tenant typically means a misconfigured embed or DB outage —
// both transient.
func WithTenant(catalog ports.CatalogPort, defaultSlug string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Internal control endpoints (curator kill switch) are tenant-agnostic
			// and gated by X-Internal-Key inside the handler — skip tenant resolution.
			if strings.HasPrefix(r.URL.Path, "/api/v1/internal/") {
				next.ServeHTTP(w, r)
				return
			}
			// Onboarding routes (R5) are cookie-gated and tenant-agnostic to the
			// caller: sessions live under the keepstar-onboarding system tenant,
			// resolved server-side (handler_onboard.go) — never from X-Tenant-Slug.
			if strings.HasPrefix(r.URL.Path, "/api/v1/onboard/") {
				next.ServeHTTP(w, r)
				return
			}
			// Static page hosts (§5.1) serve plain HTML — no tenant resolution
			// here (a missing default slug or a DB hiccup must not 400/503 the
			// page itself). The page mounts parse the slug client-side and every
			// API call they make carries X-Tenant-Slug through this middleware.
			if r.URL.Path == "/onboard" || strings.HasPrefix(r.URL.Path, "/s/") ||
				strings.HasPrefix(r.URL.Path, "/crm/") {
				next.ServeHTTP(w, r)
				return
			}
			slug := r.Header.Get("X-Tenant-Slug")
			if slug == "" {
				slug = defaultSlug
			}
			if slug == "" {
				http.Error(w, "tenant slug missing", http.StatusBadRequest)
				return
			}
			tenant, err := catalog.GetTenantBySlug(r.Context(), slug)
			if err != nil {
				http.Error(w, "tenant unavailable", http.StatusServiceUnavailable)
				return
			}
			ctx := context.WithValue(r.Context(), ctxKeyTenant{}, tenant)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
