// Curator standalone backend (M11). Single binary; reads CURATOR_DATABASE_URL
// (defaults to DATABASE_URL — same Neon DB as admin/v4) and exposes /curator/*.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"

	"keepstar-curator/internal/adapters"
	"keepstar-curator/internal/handlers"
	"keepstar-curator/internal/session"
)

func main() {
	dsn := firstNonEmpty(os.Getenv("CURATOR_DATABASE_URL"), os.Getenv("DATABASE_URL"))
	if dsn == "" {
		log.Fatal("CURATOR_DATABASE_URL or DATABASE_URL required")
	}
	bind := firstNonEmpty(os.Getenv("CURATOR_BIND"), ":8082")
	secure := strings.EqualFold(os.Getenv("CURATOR_COOKIE_SECURE"), "true")

	ctx := context.Background()
	client, err := adapters.NewClient(ctx, dsn)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer client.Close()
	if err := client.RunMigrations(ctx); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	h := handlers.New(client, secure)
	mergeProxy := handlers.NewMergeProxy()
	auth := session.Middleware(client)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	// Public auth.
	mux.HandleFunc("/curator/auth/login", h.Login)
	mux.HandleFunc("/curator/auth/logout", h.Logout)

	// Authenticated routes — wrapped with session middleware.
	protected := http.NewServeMux()
	protected.HandleFunc("/curator/me", h.Me)
	protected.HandleFunc("/curator/candidates/attributes", h.ListAttributeCandidates)
	protected.HandleFunc("/curator/candidates/attributes/", func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/promote"):
			h.PromoteAttribute(w, r)
		case strings.HasSuffix(r.URL.Path, "/dismiss"):
			h.DismissAttribute(w, r)
		default:
			http.NotFound(w, r)
		}
	})
	protected.HandleFunc("/curator/candidates/categories", h.ListCategoryCandidates)
	protected.HandleFunc("/curator/junk", h.ListJunk)
	protected.HandleFunc("/curator/junk/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/classify") {
			h.ClassifyJunk(w, r)
			return
		}
		http.NotFound(w, r)
	})
	protected.HandleFunc("/curator/audit", h.ListAudit)

	// Tenants — operations dashboard (Этап 1, 2026-04-27 pivot).
	protected.HandleFunc("/curator/tenants", h.ListTenants)
	protected.HandleFunc("/curator/tenants/", func(w http.ResponseWriter, r *http.Request) {
		// /curator/tenants/{id}              → GetTenant
		// /curator/tenants/{id}/products     → ListTenantProducts
		// /curator/tenants/{id}/schema       → GetTenantSchema
		// /curator/tenants/{id}/merge/run    → mergeProxy.HandleRun
		// /curator/tenants/{id}/merge-reports → mergeProxy.HandleListReports
		path := r.URL.Path
		switch {
		case strings.HasSuffix(path, "/products"):
			h.ListTenantProducts(w, r)
		case strings.HasSuffix(path, "/schema"):
			h.GetTenantSchema(w, r)
		case strings.HasSuffix(path, "/merge/run"):
			mergeProxy.HandleRun(w, r)
		case strings.HasSuffix(path, "/merge-reports"):
			mergeProxy.HandleListReports(w, r)
		default:
			h.GetTenant(w, r)
		}
	})

	// Merge reports (Phase D3, 2026-04-28). All proxied to admin-backend.
	protected.HandleFunc("/curator/merge-reports/", mergeProxy.HandleReport)

	// Master catalog browse.
	protected.HandleFunc("/curator/master/products", h.ListMasterProducts)
	protected.HandleFunc("/curator/master/products/", h.GetMasterProduct)

	mux.Handle("/curator/me", auth(protected))
	mux.Handle("/curator/candidates/", auth(protected))
	mux.Handle("/curator/candidates/attributes", auth(protected))
	mux.Handle("/curator/candidates/categories", auth(protected))
	mux.Handle("/curator/junk", auth(protected))
	mux.Handle("/curator/junk/", auth(protected))
	mux.Handle("/curator/audit", auth(protected))
	mux.Handle("/curator/tenants", auth(protected))
	mux.Handle("/curator/tenants/", auth(protected))
	mux.Handle("/curator/master/products", auth(protected))
	mux.Handle("/curator/master/products/", auth(protected))
	mux.Handle("/curator/merge-reports/", auth(protected))

	// SPA fallback for the curator frontend (./static).
	staticDir := firstNonEmpty(os.Getenv("CURATOR_STATIC_DIR"), "./static")
	if _, err := os.Stat(staticDir); err == nil {
		fs := http.FileServer(http.Dir(staticDir))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			path := staticDir + r.URL.Path
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				fs.ServeHTTP(w, r)
				return
			}
			http.ServeFile(w, r, staticDir+"/index.html")
		})
	}

	log.Printf("curator listening on %s", bind)
	if err := http.ListenAndServe(bind, withCORS(mux)); err != nil {
		log.Fatal(err)
	}
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}

// withCORS lets the dev frontend (Vite on :5175) talk to :8082 in development.
// Production deploys behind a single domain don't need this — but the env-gated
// allow-list is intentional, no `*` wildcard.
func withCORS(next http.Handler) http.Handler {
	allow := os.Getenv("CURATOR_CORS_ORIGIN")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if allow != "" && r.Header.Get("Origin") == allow {
			w.Header().Set("Access-Control-Allow-Origin", allow)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
