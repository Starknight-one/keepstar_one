package ports

import (
	"context"

	"keepstar-admin/internal/domain"
)

// IntegrationsPort is the persistence contract for tenant integrations and
// their short-lived OAuth state rows. All methods are tenant-scoped except
// OAuth state ops (which are keyed by the signed state string).
type IntegrationsPort interface {
	// Integration CRUD
	Create(ctx context.Context, integration *domain.Integration) (*domain.Integration, error)
	Get(ctx context.Context, tenantID, id string) (*domain.Integration, error)
	GetByKindAndExternalID(ctx context.Context, tenantID string, kind domain.IntegrationKind, externalID string) (*domain.Integration, error)
	GetByShopDomain(ctx context.Context, shopDomain string) (*domain.Integration, error)
	ListByTenant(ctx context.Context, tenantID string) ([]domain.Integration, error)
	Update(ctx context.Context, integration *domain.Integration) error
	UpdateStatus(ctx context.Context, id string, status domain.IntegrationStatus, lastError string) error
	UpdateLastSync(ctx context.Context, id string, jobID string) error
	Delete(ctx context.Context, tenantID, id string) error

	// OAuth state
	CreateOAuthState(ctx context.Context, state *domain.OAuthState) error
	ConsumeOAuthState(ctx context.Context, state string) (*domain.OAuthState, error)
	CleanupExpiredOAuthStates(ctx context.Context) (int64, error)
}
