package handlers

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"keepstar_v5/internal/ports"

	"github.com/jackc/pgx/v5/pgxpool"
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
//	GET  /healthz                            — liveness probe (process alive)
//	GET  /readyz                             — readiness probe (DB ping)
//	POST /api/v1/session/init                — create session
//	GET  /api/v1/session/{id}                — read session state (debug)
//	POST /api/v1/pipeline                    — run Agent2 turn
//	POST /api/v1/actions                     — like / unlike / cart_add / cart_remove
//	POST /api/v1/navigation/expand           — drill into detail preset
//	POST /api/v1/navigation/back             — pop view stack, restore prior template
//	POST /api/v1/navigation/filter           — deterministic brand re-filter of the current grid
//	POST /api/v1/internal/presets/cache-invalidate — drop cached Agent2 prompt for ?tenant= (X-Internal-Key)
//	POST /api/v1/internal/presets/preview    — zero-LLM preset/draft render for ?tenant= (X-Internal-Key)
//
// Middleware chain (outermost to innermost):
//
//	WithLogging → WithCORS → WithTenant → mux
//
// Logging is outermost so we capture every request including those that
// the tenant middleware bounces. CORS pre-flight (OPTIONS) is short-
// circuited inside WithCORS so it doesn't hit tenant lookup. Tenant
// middleware applies to ALL routes so handlers can rely on
// TenantFromContext being populated; /healthz and /readyz are the lone
// exceptions (handled by ordering — they're registered first and the
// middleware applies to the entire mux uniformly, so we accept the
// redundant tenant lookup on probes as a tiny cost).
func RegisterRoutes(
	log *slog.Logger,
	catalog ports.CatalogPort,
	pool *pgxpool.Pool,
	staticDir string,
	defaultTenantSlug string,
	session *SessionHandler,
	pipeline *PipelineHandler,
	action *ActionHandler,
	navigation *NavigationHandler,
	preset *PresetHandler,
) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", HealthHandler)
	mux.HandleFunc("GET /readyz", ReadyzHandler(pool))
	mux.HandleFunc("POST /api/v1/session/init", session.Init)
	mux.HandleFunc("GET /api/v1/session/", session.Get)
	// Internal control (curator cockpit kill switch) — gated by X-Internal-Key
	// inside the handlers and exempt from WithTenant (middleware_tenant.go).
	mux.HandleFunc("POST /api/v1/internal/session/{id}/kill", session.Kill)
	mux.HandleFunc("POST /api/v1/internal/sessions/kill-all", session.KillAll)
	mux.HandleFunc("POST /api/v1/internal/presets/cache-invalidate", preset.CacheInvalidate)
	mux.HandleFunc("POST /api/v1/internal/presets/preview", preset.Preview)
	mux.HandleFunc("POST /api/v1/pipeline", pipeline.Pipeline)
	mux.HandleFunc("POST /api/v1/actions", action.Action)
	mux.HandleFunc("POST /api/v1/navigation/expand", navigation.Expand)
	mux.HandleFunc("POST /api/v1/navigation/back", navigation.Back)
	mux.HandleFunc("POST /api/v1/navigation/filter", navigation.Filter)

	// Static fileserver (V4 pattern, project_v4/backend/cmd/server/main.go:347-357).
	// Serves the V5 widget IIFE bundle (widget.js + widget.html) from same
	// origin so an embed `<script src=".../widget.js">` auto-resolves the
	// API base URL via script.src.origin (widget.jsx:30). When staticDir
	// is empty (e.g. local `go run` without a built frontend) the catch-
	// all is skipped and unknown paths get the default mux 404.
	if staticDir != "" {
		if info, err := os.Stat(staticDir); err == nil && info.IsDir() {
			fs := http.FileServer(http.Dir(staticDir))
			mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
				path := filepath.Join(staticDir, r.URL.Path)
				if st, err := os.Stat(path); err == nil && !st.IsDir() {
					fs.ServeHTTP(w, r)
					return
				}
				// SPA fallback — useful for any future HTML host page.
				if idx := filepath.Join(staticDir, "index.html"); fileExists(idx) {
					http.ServeFile(w, r, idx)
					return
				}
				http.NotFound(w, r)
			})
			log.Info("static_fileserver_enabled", "dir", staticDir)
		} else {
			log.Warn("static_fileserver_skipped", "dir", staticDir, "err", err)
		}
	}

	withTenant := WithTenant(catalog, defaultTenantSlug)
	withLogging := WithLogging(log)

	// Compose: logging(cors(tenant(mux)))
	return withLogging(WithCORS(withTenant(mux)))
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
