package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

// The gateway must send X-Service-Key on EVERY route (R7) and speak the
// admin wire contract byte-for-byte (paths + field names verified against
// keepstar-admin handler_service_v1.go).
func TestAdminGatewayContract(t *testing.T) {
	var gotKey, gotPath, gotMethod string
	var gotBody map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Service-Key")
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotBody = nil
		if strings.Contains(r.Header.Get("Content-Type"), "application/json") {
			json.NewDecoder(r.Body).Decode(&gotBody)
		}
		switch {
		case r.URL.Path == "/admin/api/service/v1/tenants":
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]any{"tenantId": "t-1", "slug": "acme-realty"})
		case strings.HasSuffix(r.URL.Path, "/users"):
			w.WriteHeader(http.StatusOK) // idempotent re-provision path
			json.NewEncoder(w).Encode(map[string]any{"userId": "u-1"})
		case strings.HasSuffix(r.URL.Path, "/presets/adopt"):
			json.NewEncoder(w).Encode(map[string]any{
				"adopted": []string{"lead_table"}, "bindingReports": []any{}, "invalidated": true,
			})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	g := NewAdminGateway(srv.URL, "service-key-1", discardLog())
	ctx := context.Background()

	tenant, err := g.CreateTenant(ctx, "Acme Realty", "real estate agency")
	if err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	if gotKey != "service-key-1" || gotMethod != http.MethodPost {
		t.Fatalf("auth/method wrong: key=%q method=%q", gotKey, gotMethod)
	}
	if gotBody["name"] != "Acme Realty" || gotBody["vertical"] != "real estate agency" {
		t.Fatalf("create body = %v", gotBody)
	}
	if tenant.TenantID != "t-1" || tenant.Slug != "acme-realty" {
		t.Fatalf("tenant = %+v", tenant)
	}

	userID, err := g.ProvisionUser(ctx, "acme-realty", "o@a.test", "pw-1", "owner")
	if err != nil {
		t.Fatalf("ProvisionUser: %v", err)
	}
	if gotPath != "/admin/api/service/v1/tenants/acme-realty/users" {
		t.Fatalf("users path = %q", gotPath)
	}
	if gotBody["email"] != "o@a.test" || gotBody["password"] != "pw-1" || gotBody["role"] != "owner" {
		t.Fatalf("users body = %v", gotBody)
	}
	if userID != "u-1" {
		t.Fatalf("userID = %q", userID)
	}

	adopt, err := g.AdoptPresets(ctx, "acme-realty", []string{"lead_table"})
	if err != nil {
		t.Fatalf("AdoptPresets: %v", err)
	}
	if gotPath != "/admin/api/service/v1/tenants/acme-realty/presets/adopt" {
		t.Fatalf("adopt path = %q", gotPath)
	}
	if names, _ := gotBody["names"].([]any); len(names) != 1 || names[0] != "lead_table" {
		t.Fatalf("adopt body = %v", gotBody)
	}
	if !adopt.Invalidated || len(adopt.Adopted) != 1 {
		t.Fatalf("adopt result = %+v", adopt)
	}
}

// StartImport must stream a real multipart file part named "file" and parse
// the 202 job handle; ImportStatus proxies the honest status fields.
func TestAdminGatewayImport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/import"):
			mr, err := r.MultipartReader()
			if err != nil {
				t.Errorf("multipart reader: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			part, err := mr.NextPart()
			if err != nil || part.FormName() != "file" || part.FileName() != "listings.csv" {
				t.Errorf("file part wrong: %v %q %q", err, part.FormName(), part.FileName())
			}
			content, _ := io.ReadAll(part)
			if string(content) != "name,price\nCasa,100\n" {
				t.Errorf("file content = %q", content)
			}
			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(map[string]any{"jobId": "job-9", "status": "pending", "totalItems": 1})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/import/"):
			json.NewEncoder(w).Encode(map[string]any{
				"jobId": "job-9", "status": "completed", "processed": 1, "totalItems": 1,
				"projectionRows": 1, "invalidated": true, "errors": []string{},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	g := NewAdminGateway(srv.URL, "k", discardLog())
	job, err := g.StartImport(context.Background(), "acme-realty", "listings.csv",
		strings.NewReader("name,price\nCasa,100\n"))
	if err != nil {
		t.Fatalf("StartImport: %v", err)
	}
	if job.JobID != "job-9" || job.Status != "pending" {
		t.Fatalf("job = %+v", job)
	}

	st, err := g.ImportStatus(context.Background(), "acme-realty", "job-9")
	if err != nil {
		t.Fatalf("ImportStatus: %v", err)
	}
	if st.Status != "completed" || st.ProjectionRows != 1 || !st.Invalidated {
		t.Fatalf("status = %+v", st)
	}
}

// Non-2xx responses surface admin's error message + status — and NEVER echo
// the request (a ProvisionUser failure must not leak the password into the
// error string, R6).
func TestAdminGatewayErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"error": "email is invalid"})
	}))
	defer srv.Close()

	g := NewAdminGateway(srv.URL, "k", discardLog())
	_, err := g.ProvisionUser(context.Background(), "acme", "bad", "MY-secret-PW", "owner")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "email is invalid") || !strings.Contains(err.Error(), "400") {
		t.Fatalf("error lost admin context: %v", err)
	}
	if strings.Contains(err.Error(), "MY-secret-PW") {
		t.Fatalf("PASSWORD LEAKED into gateway error (R6 violation): %v", err)
	}
}

// An unconfigured gateway (env unset) fails every call honestly.
func TestAdminGatewayNotConfigured(t *testing.T) {
	g := NewAdminGateway("", "", discardLog())
	if _, err := g.CreateTenant(context.Background(), "x", "y"); !errors.Is(err, ErrAdminGatewayNotConfigured) {
		t.Fatalf("err = %v, want ErrAdminGatewayNotConfigured", err)
	}
	if _, err := g.StartImport(context.Background(), "s", "f.csv", strings.NewReader("")); !errors.Is(err, ErrAdminGatewayNotConfigured) {
		t.Fatalf("import err = %v, want ErrAdminGatewayNotConfigured", err)
	}
}
