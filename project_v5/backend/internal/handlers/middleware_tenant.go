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
