// Package adapters holds cross-service clients that are not tied to one
// storage backend. AdminGateway is v5's client for the admin service-route
// family (RUNTIME_SPEC.md §5.5, R7).
package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"keepstar_v5/internal/ports"
)

// ErrAdminGatewayNotConfigured is returned by every method when the gateway
// was constructed without a base URL or service key (env ADMIN_BASE_URL /
// ADMIN_SERVICE_KEY unset). Callers surface it honestly (503-class), never
// silently skip the step.
var ErrAdminGatewayNotConfigured = errors.New("admin gateway not configured (ADMIN_BASE_URL / ADMIN_SERVICE_KEY)")

// Compile-time port conformance.
var _ ports.AdminGatewayPort = (*AdminGateway)(nil)

// serviceKeyHeader is the R7 service-to-service auth header; the admin side
// compares it constant-time against the SAME env name.
const serviceKeyHeader = "X-Service-Key"

// AdminGateway implements ports.AdminGatewayPort over HTTP. All requests
// carry X-Service-Key; bodies are JSON except StartImport (streamed
// multipart — browser→v5→admin without buffering the file, §5.4).
//
// R6: ProvisionUser sends the password in the request body ONLY — it is
// never logged, never included in errors, never stored on the struct.
type AdminGateway struct {
	baseURL    string
	serviceKey string
	httpc      *http.Client
	log        *slog.Logger
}

// NewAdminGateway constructs the client. baseURL is the admin origin (e.g.
// https://admin-dev-85d4.up.railway.app) — trailing slash tolerated. Empty
// baseURL or serviceKey yields a client whose every call returns
// ErrAdminGatewayNotConfigured (honest degraded mode for unwired deploys).
func NewAdminGateway(baseURL, serviceKey string, log *slog.Logger) *AdminGateway {
	return &AdminGateway{
		baseURL:    strings.TrimRight(baseURL, "/"),
		serviceKey: serviceKey,
		// One generous timeout covers the 20MB import upload on a slow link;
		// JSON calls finish far inside it.
		httpc: &http.Client{Timeout: 60 * time.Second},
		log:   log,
	}
}

func (g *AdminGateway) configured() error {
	if g.baseURL == "" || g.serviceKey == "" {
		return ErrAdminGatewayNotConfigured
	}
	return nil
}

// CreateTenant implements ports.AdminGatewayPort.
func (g *AdminGateway) CreateTenant(ctx context.Context, name, vertical string) (*ports.AdminTenant, error) {
	var out ports.AdminTenant
	err := g.postJSON(ctx, "/admin/api/service/v1/tenants",
		map[string]string{"name": name, "vertical": vertical}, &out)
	if err != nil {
		return nil, fmt.Errorf("create tenant: %w", err)
	}
	if out.TenantID == "" || out.Slug == "" {
		return nil, fmt.Errorf("create tenant: admin returned empty tenantId/slug")
	}
	return &out, nil
}

// ProvisionUser implements ports.AdminGatewayPort. The password crosses this
// method body and nothing else (R6).
func (g *AdminGateway) ProvisionUser(ctx context.Context, tenantSlug, email, password, role string) (string, error) {
	var out struct {
		UserID string `json:"userId"`
	}
	err := g.postJSON(ctx, "/admin/api/service/v1/tenants/"+url.PathEscape(tenantSlug)+"/users",
		map[string]string{"email": email, "password": password, "role": role}, &out)
	if err != nil {
		return "", fmt.Errorf("provision user: %w", err)
	}
	if out.UserID == "" {
		return "", fmt.Errorf("provision user: admin returned empty userId")
	}
	return out.UserID, nil
}

// AdoptPresets implements ports.AdminGatewayPort.
func (g *AdminGateway) AdoptPresets(ctx context.Context, tenantSlug string, names []string) (*ports.AdminAdoptResult, error) {
	var out ports.AdminAdoptResult
	err := g.postJSON(ctx, "/admin/api/service/v1/tenants/"+url.PathEscape(tenantSlug)+"/presets/adopt",
		map[string]any{"names": names}, &out)
	if err != nil {
		return nil, fmt.Errorf("adopt presets: %w", err)
	}
	return &out, nil
}

// StartImport implements ports.AdminGatewayPort: streams the file through a
// pipe so the 20MB body never sits in v5 memory twice (browser→v5→admin).
func (g *AdminGateway) StartImport(ctx context.Context, tenantSlug, fileName string, file io.Reader) (*ports.AdminImportJob, error) {
	if err := g.configured(); err != nil {
		return nil, err
	}
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	go func() {
		part, err := mw.CreateFormFile("file", fileName)
		if err == nil {
			_, err = io.Copy(part, file)
		}
		if cerr := mw.Close(); err == nil {
			err = cerr
		}
		pw.CloseWithError(err) // nil err closes clean
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		g.baseURL+"/admin/api/service/v1/tenants/"+url.PathEscape(tenantSlug)+"/import", pr)
	if err != nil {
		return nil, fmt.Errorf("start import: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set(serviceKeyHeader, g.serviceKey)

	var out ports.AdminImportJob
	if err := g.do(req, &out); err != nil {
		return nil, fmt.Errorf("start import: %w", err)
	}
	if out.JobID == "" {
		return nil, fmt.Errorf("start import: admin returned empty jobId")
	}
	return &out, nil
}

// ImportStatus implements ports.AdminGatewayPort.
func (g *AdminGateway) ImportStatus(ctx context.Context, tenantSlug, jobID string) (*ports.AdminImportStatus, error) {
	if err := g.configured(); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		g.baseURL+"/admin/api/service/v1/tenants/"+url.PathEscape(tenantSlug)+"/import/"+url.PathEscape(jobID), nil)
	if err != nil {
		return nil, fmt.Errorf("import status: %w", err)
	}
	req.Header.Set(serviceKeyHeader, g.serviceKey)

	var out ports.AdminImportStatus
	if err := g.do(req, &out); err != nil {
		return nil, fmt.Errorf("import status: %w", err)
	}
	return &out, nil
}

// --- transport helpers ---

// postJSON marshals body, POSTs it with the service key, and decodes a 2xx
// response into out.
func (g *AdminGateway) postJSON(ctx context.Context, path string, body any, out any) error {
	if err := g.configured(); err != nil {
		return err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(serviceKeyHeader, g.serviceKey)
	return g.do(req, out)
}

// do executes the request and decodes a 2xx JSON body into out. Non-2xx →
// error carrying admin's {"error": …} message (best-effort) + status code.
// Only the RESPONSE body is ever echoed into errors — request bodies (which
// may carry a password) never are.
func (g *AdminGateway) do(req *http.Request, out any) error {
	resp, err := g.httpc.Do(req)
	if err != nil {
		return fmt.Errorf("admin request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("admin responded %d: %s", resp.StatusCode, readAdminError(resp.Body))
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
		return fmt.Errorf("decode admin response: %w", err)
	}
	return nil
}

// readAdminError extracts admin's error message ({"error": "..."} or raw
// text), capped small so a misbehaving upstream can't bloat logs.
func readAdminError(body io.Reader) string {
	raw, _ := io.ReadAll(io.LimitReader(body, 8<<10))
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(raw, &e) == nil && e.Error != "" {
		return e.Error
	}
	s := strings.TrimSpace(string(raw))
	if s == "" {
		return "(no body)"
	}
	return s
}
