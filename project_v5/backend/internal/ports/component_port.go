package ports

import (
	"context"

	"keepstar_v5/internal/domain"
)

// ComponentPort is the read surface for tenant-edited reusable v9
// components. The future v9-canvas microservice (Stream B) will be the
// write side; this stays read-only until that lands.
//
// Tenant identifier accepts slug or UUID — same contract as PresetPort.
type ComponentPort interface {
	GetPublishedComponent(ctx context.Context, tenantSlugOrID string, name string) (*domain.Component, error)
	ListPublishedComponents(ctx context.Context, tenantSlugOrID string) ([]domain.Component, error)
}
