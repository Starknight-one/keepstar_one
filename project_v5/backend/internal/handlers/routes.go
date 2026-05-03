package handlers

import (
	"log/slog"
	"net/http"

	"keepstar_v5/internal/ports"
)

// HealthHandler returns 200 OK with a one-liner JSON body. Used by
// uptime checks and Railway health probes.
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// RegisterRoutes wires up the V5 endpoint catalog and returns the chained
// http.Handler ready for http.Server.
//
// Endpoint catalog:
//
//	GET  /healthz                            — health probe
//	POST /api/v1/session/init                — create session
//	GET  /api/v1/session/{id}                — read session state (debug)
//	POST /api/v1/pipeline                    — run Agent2 turn
//	POST /api/v1/actions                     — like / unlike / cart_add / cart_remove
//	POST /api/v1/navigation/expand           — drill into detail preset
//	POST /api/v1/navigation/back             — pop view stack, restore prior template
//
// Middleware chain (outermost to innermost):
//
//	WithLogging → WithCORS → WithTenant → mux
//
// Logging is outermost so we capture every request including those that
// the tenant middleware bounces. CORS pre-flight (OPTIONS) is short-
// circuited inside WithCORS so it doesn't hit tenant lookup. Tenant
// middleware applies to ALL routes so handlers can rely on
// TenantFromContext being populated; /healthz is the lone exception
// (handled by ordering — health is registered first and middleware
// applies to the entire mux uniformly, so we accept the redundant
// tenant lookup on /healthz as a tiny cost).
func RegisterRoutes(
	log *slog.Logger,
	catalog ports.CatalogPort,
	defaultTenantSlug string,
	session *SessionHandler,
	pipeline *PipelineHandler,
	action *ActionHandler,
	navigation *NavigationHandler,
) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", HealthHandler)
	mux.HandleFunc("POST /api/v1/session/init", session.Init)
	mux.HandleFunc("GET /api/v1/session/", session.Get)
	mux.HandleFunc("POST /api/v1/pipeline", pipeline.Pipeline)
	mux.HandleFunc("POST /api/v1/actions", action.Action)
	mux.HandleFunc("POST /api/v1/navigation/expand", navigation.Expand)
	mux.HandleFunc("POST /api/v1/navigation/back", navigation.Back)

	withTenant := WithTenant(catalog, defaultTenantSlug)
	withLogging := WithLogging(log)

	// Compose: logging(cors(tenant(mux)))
	return withLogging(WithCORS(withTenant(mux)))
}
