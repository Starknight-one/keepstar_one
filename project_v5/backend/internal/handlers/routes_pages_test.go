package handlers

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Page hosts (RUNTIME_SPEC §5.1): GET /onboard, /s/{slug} and /crm/{slug}
// serve their built HTML files from staticDir ahead of the static
// catch-all, and are tenant-exempt — a request with NO X-Tenant-Slug and
// NO default slug must still get the page (the mounts resolve tenant
// client-side), never the middleware's 400/503.

func pagesTestRouter(t *testing.T, staticDir string) http.Handler {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return RegisterRoutes(
		log,
		&fakeCatalogPort{}, // empty: any tenant lookup would fail — pages must not look up
		nil,                // pool — page routes never touch it
		staticDir,
		"", // no default tenant slug — proves the tenant exemption
		NewSessionHandler(nil, nil),
		NewPipelineHandler(nil, nil, nil, nil, nil, log),
		NewActionHandler(nil),
		NewNavigationHandler(nil, nil, nil, nil, log),
		NewPresetHandler(nil, nil, nil, nil, log),
		NewThemeHandler(nil, log),
		NewOnboardHandler(nil, &fakeCatalogPort{}, nil, log),
		NewCacheHandler(&recordingInvalidator{}, &recordingInvalidator{}, &recordingRegistry{}, log),
		NewOperationsHandler(nil, nil, nil, nil, nil, nil, nil, log),
		nil, // cheap guard — nil allows everything
	)
}

func writePageFiles(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, name := range names {
		content := "<!DOCTYPE html><title>" + name + "</title>"
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}

func TestPageHosts_ServeBuiltHTML(t *testing.T) {
	dir := t.TempDir()
	writePageFiles(t, dir, "onboard.html", "storefront.html", "crm.html", "index.html")
	router := pagesTestRouter(t, dir)

	cases := []struct {
		path string
		want string // marker from the served file's <title>
	}{
		{"/onboard", "onboard.html"},
		{"/s/realtor-demo", "storefront.html"},
		{"/crm/realtor-demo?k=some-surface-token", "crm.html"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, tc.path, nil) // NO X-Tenant-Slug header
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s: status = %d, want 200 (body %q)", tc.path, rec.Code, rec.Body.String())
			continue
		}
		if !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("GET %s: body %q, want the %s content", tc.path, rec.Body.String(), tc.want)
		}
	}
}

func TestPageHosts_MissingFile404s(t *testing.T) {
	dir := t.TempDir() // no page files built at all
	router := pagesTestRouter(t, dir)

	for _, path := range []string{"/onboard", "/s/x", "/crm/x"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s with no built page: status = %d, want 404", path, rec.Code)
		}
	}
}
