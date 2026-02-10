package handlers

import "net/http"

// SetupRoutes configures all HTTP routes
func SetupRoutes(mux *http.ServeMux, chat *ChatHandler, session *SessionHandler, health *HealthHandler, pipeline *PipelineHandler, tenantMw *TenantMiddleware, defaultTenant string) {
	// Health checks
	mux.HandleFunc("/health", health.HandleHealth)
	mux.HandleFunc("/ready", health.HandleReady)

	// API v1
	mux.HandleFunc("/api/v1/chat", chat.HandleChat)
	mux.HandleFunc("/api/v1/session/", session.HandleGetSession)

	// Session init (creates session + seeds tenant)
	if tenantMw != nil {
		mux.Handle("/api/v1/session/init", tenantMw.ResolveFromHeader(defaultTenant)(http.HandlerFunc(session.HandleInitSession)))
	} else {
		mux.HandleFunc("/api/v1/session/init", session.HandleInitSession)
	}

	// Pipeline API (Two-Agent system) with tenant from header
	if pipeline != nil {
		handler := http.HandlerFunc(pipeline.HandlePipeline)
		if tenantMw != nil {
			mux.Handle("/api/v1/pipeline", tenantMw.ResolveFromHeader(defaultTenant)(handler))
		} else {
			mux.HandleFunc("/api/v1/pipeline", pipeline.HandlePipeline)
		}
	}
}

// SetupNavigationRoutes configures navigation routes (expand/back)
func SetupNavigationRoutes(mux *http.ServeMux, nav *NavigationHandler) {
	mux.HandleFunc("/api/v1/navigation/expand", nav.HandleExpand)
	mux.HandleFunc("/api/v1/navigation/back", nav.HandleBack)
}

// SetupCatalogRoutes configures catalog routes with tenant middleware
func SetupCatalogRoutes(mux *http.ServeMux, catalog *CatalogHandler, tenantMw *TenantMiddleware) {
	// Catalog API - products
	mux.Handle("/api/v1/tenants/", tenantMw.ResolveTenant(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Route based on path pattern
		if isProductsListPath(r.URL.Path) {
			catalog.HandleListProducts(w, r)
			return
		}
		if isProductDetailPath(r.URL.Path) {
			catalog.HandleGetProduct(w, r)
			return
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	})))
}

// isProductsListPath checks if path matches /api/v1/tenants/{slug}/products
func isProductsListPath(path string) bool {
	// Match: /api/v1/tenants/{slug}/products (exactly)
	parts := splitPath(path)
	return len(parts) == 5 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "tenants" && parts[4] == "products"
}

// isProductDetailPath checks if path matches /api/v1/tenants/{slug}/products/{id}
func isProductDetailPath(path string) bool {
	// Match: /api/v1/tenants/{slug}/products/{id}
	parts := splitPath(path)
	return len(parts) == 6 && parts[0] == "api" && parts[1] == "v1" && parts[2] == "tenants" && parts[4] == "products"
}

// splitPath splits URL path into non-empty parts
func splitPath(path string) []string {
	var parts []string
	for _, p := range split(path, '/') {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

// split is a simple string split
func split(s string, sep rune) []string {
	var parts []string
	var current string
	for _, c := range s {
		if c == sep {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	parts = append(parts, current)
	return parts
}
