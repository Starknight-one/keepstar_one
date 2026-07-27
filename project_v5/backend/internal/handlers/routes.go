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
//	POST /api/v1/session/init                — create session (optional {mode, k} body, R17)
//	GET  /api/v1/session/{id}                — read session state (debug)
//	POST /api/v1/onboard/session             — create mode=onboarding session (ks_onboard cookie, R5/R17)
//	POST /api/v1/pipeline                    — run Agent2 turn
//	POST /api/v1/pipeline/stream             — same turn with SSE progress frames
//	POST /api/v1/actions                     — like / unlike / cart_add / cart_remove
//	POST /api/v1/operations/invoke           — public operation endpoint (R1; widget operation_invoke)
//	POST /api/v1/navigation/expand           — drill into detail preset
//	POST /api/v1/navigation/back             — pop view stack, restore prior template
//	POST /api/v1/navigation/filter           — deterministic brand re-filter of the current grid
//	POST /api/v1/internal/cache/invalidate   — scoped cache invalidation for ?tenant=&scope=agent1|agent2|all (X-Internal-Key, R15)
//	POST /api/v1/internal/presets/cache-invalidate — legacy alias ≡ scope=agent2 (X-Internal-Key)
//	POST /api/v1/internal/presets/preview    — zero-LLM preset/draft render for ?tenant= (X-Internal-Key)
//	GET  /api/v1/internal/presets/system     — system preset library listing (X-Internal-Key)
//	GET  /api/v1/internal/presets/system/doc — compiled doc_json for one system preset by ?name= (X-Internal-Key)
//	GET  /api/v1/internal/theme              — read resolved theme for ?tenant= (X-Internal-Key)
//	PUT  /api/v1/internal/theme              — upsert tenant theme override for ?tenant= (X-Internal-Key)
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
	theme *ThemeHandler,
	onboard *OnboardHandler,
	cache *CacheHandler,
	operations *OperationsHandler,
	cheapGuard *PipelineGuard,
) http.Handler {
	// §6.3 cheap bucket: the SAME guard instance fronts every no-LLM-spend
	// route. /operations/invoke checks it inside the handler (it owns its
	// own 429 there) so it is mounted bare; the rest are wrapped here. A
	// nil guard allows everything (PipelineGuard.Allow nil-receiver).
	withCheap := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if ok, _ := cheapGuard.Allow(clientIP(r)); !ok {
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next(w, r)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", HealthHandler)
	mux.HandleFunc("GET /readyz", ReadyzHandler(pool))
	mux.HandleFunc("POST /api/v1/session/init", withCheap(session.Init))
	mux.HandleFunc("GET /api/v1/session/", session.Get)
	// Onboarding (R5/R17) — ks_onboard cookie gated inside the handler,
	// exempt from WithTenant (system tenant resolved server-side).
	mux.HandleFunc("POST /api/v1/onboard/session", onboard.CreateSession)
	// Internal control (curator cockpit kill switch) — gated by X-Internal-Key
	// inside the handlers and exempt from WithTenant (middleware_tenant.go).
	mux.HandleFunc("POST /api/v1/internal/session/{id}/kill", session.Kill)
	mux.HandleFunc("POST /api/v1/internal/sessions/kill-all", session.KillAll)
	// Scoped cache invalidation (R15) + the legacy agent2-only alias.
	mux.HandleFunc("POST /api/v1/internal/cache/invalidate", cache.Invalidate)
	mux.HandleFunc("POST /api/v1/internal/presets/cache-invalidate", cache.InvalidateLegacy)
	mux.HandleFunc("POST /api/v1/internal/presets/preview", preset.Preview)
	mux.HandleFunc("GET /api/v1/internal/presets/system", preset.ListSystem)
	mux.HandleFunc("GET /api/v1/internal/presets/system/doc", preset.SystemDoc)
	// Admin-facing theme read/write for the named tenant (X-Internal-Key,
	// exempt from WithTenant). GET + PUT on the same path.
	mux.HandleFunc("GET /api/v1/internal/theme", theme.Theme)
	mux.HandleFunc("PUT /api/v1/internal/theme", theme.Theme)
	mux.HandleFunc("POST /api/v1/pipeline", pipeline.Pipeline)
	mux.HandleFunc("POST /api/v1/pipeline/stream", pipeline.PipelineStream)
	mux.HandleFunc("POST /api/v1/actions", withCheap(action.Action))
	// Public operation endpoint (R1) — inside the WithTenant chain, NOT
	// under /api/v1/internal/. The handler applies the cheap bucket itself.
	mux.HandleFunc("POST /api/v1/operations/invoke", operations.Invoke)
	mux.HandleFunc("POST /api/v1/navigation/expand", withCheap(navigation.Expand))
	mux.HandleFunc("POST /api/v1/navigation/back", withCheap(navigation.Back))
	mux.HandleFunc("POST /api/v1/navigation/filter", withCheap(navigation.Filter))

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
