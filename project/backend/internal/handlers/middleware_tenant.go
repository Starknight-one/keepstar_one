package handlers

import (
	"context"
	"net/http"
	"strings"

	"keepstar/internal/domain"
	"keepstar/internal/ports"
)

type contextKey string

const tenantContextKey contextKey = "tenant"

// TenantMiddleware handles tenant resolution from URL path
type TenantMiddleware struct {
	catalog ports.CatalogPort
}

// NewTenantMiddleware creates a new TenantMiddleware
func NewTenantMiddleware(catalog ports.CatalogPort) *TenantMiddleware {
	return &TenantMiddleware{catalog: catalog}
}

// ResolveTenant extracts tenant slug from URL and validates it
func (m *TenantMiddleware) ResolveTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract tenant slug from path: /api/v1/tenants/{slug}/...
		path := r.URL.Path
		parts := strings.Split(path, "/")

		// Find "tenants" in path and get the next segment
		var slug string
		for i, part := range parts {
			if part == "tenants" && i+1 < len(parts) {
				slug = parts[i+1]
				break
			}
		}

		if slug == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "tenant slug required",
			})
			return
		}

		// Validate tenant exists
		tenant, err := m.catalog.GetTenantBySlug(r.Context(), slug)
		if err != nil {
			if err == domain.ErrTenantNotFound {
				writeJSON(w, http.StatusNotFound, map[string]string{
					"error": "tenant not found",
				})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
			return
		}

		// Store tenant in context
		ctx := context.WithValue(r.Context(), tenantContextKey, tenant)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetTenantFromContext retrieves tenant from request context
func GetTenantFromContext(ctx context.Context) *domain.Tenant {
	tenant, ok := ctx.Value(tenantContextKey).(*domain.Tenant)
	if !ok {
		return nil
	}
	return tenant
}
