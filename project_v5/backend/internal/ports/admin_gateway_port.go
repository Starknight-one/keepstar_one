package ports

import (
	"context"
	"io"
)

// AdminGatewayPort is v5's client surface over the admin service-route
// family `/admin/api/service/v1/*` (RUNTIME_SPEC.md §5.5, R7) — consumed by
// the ManifestApplier and the onboarding upload proxy, gated on the admin
// side by X-Service-Key (env ADMIN_SERVICE_KEY, same name both services).
//
// R6 discipline: ProvisionUser is the ONLY place a registration password
// crosses this port; implementations must never log or persist it.
type AdminGatewayPort interface {
	// CreateTenant creates the business workspace: {name, vertical} →
	// {tenantId, slug}. Admin suffixes the slug on collision (-2, …), so the
	// returned slug is the truth.
	CreateTenant(ctx context.Context, name, vertical string) (*AdminTenant, error)
	// ProvisionUser registers an admin account for the tenant. Idempotent on
	// the admin side (existing email → 200 + existing user id).
	ProvisionUser(ctx context.Context, tenantSlug, email, password, role string) (userID string, err error)
	// AdoptPresets copies named system presets into the tenant.
	AdoptPresets(ctx context.Context, tenantSlug string, names []string) (*AdminAdoptResult, error)
	// StartImport streams a catalog file (.csv/.json, ≤20MB) to the admin
	// multipart import route and returns the async job handle.
	StartImport(ctx context.Context, tenantSlug, fileName string, file io.Reader) (*AdminImportJob, error)
	// ImportStatus reads the honest job status: projectionRows/invalidated
	// report what actually happened, never claimed (R7).
	ImportStatus(ctx context.Context, tenantSlug, jobID string) (*AdminImportStatus, error)
}

// AdminTenant is the create-tenant response.
type AdminTenant struct {
	TenantID string `json:"tenantId"`
	Slug     string `json:"slug"`
}

// AdminAdoptResult is the presets/adopt response. BindingReports carries the
// admin-side wire field verbatim (empty until the §6.2 extended preview
// lands — the applier runs its own binding verification, nothing here
// pretends otherwise). Invalidated reports whether the admin→v5 agent2
// cache invalidation was confirmed.
type AdminAdoptResult struct {
	Adopted        []string         `json:"adopted"`
	BindingReports []map[string]any `json:"bindingReports"`
	Invalidated    bool             `json:"invalidated"`
}

// AdminImportJob is the accepted-upload response.
type AdminImportJob struct {
	JobID      string `json:"jobId"`
	Status     string `json:"status"`
	TotalItems int    `json:"totalItems"`
}

// AdminImportStatus is the honest import status contract (R7):
// {status, processed, projectionRows, invalidated, errors[]}.
// Status values mirror admin's job lifecycle:
// pending | processing | completed | failed.
type AdminImportStatus struct {
	JobID          string   `json:"jobId"`
	Status         string   `json:"status"`
	Processed      int      `json:"processed"`
	TotalItems     int      `json:"totalItems"`
	ProjectionRows int      `json:"projectionRows"`
	Invalidated    bool     `json:"invalidated"`
	Errors         []string `json:"errors"`
}
