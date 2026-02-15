package ports

import (
	"context"

	"keepstar-admin/internal/domain"
)

type EnrichmentPort interface {
	EnrichProducts(ctx context.Context, items []domain.EnrichmentInput) (*domain.EnrichmentResult, error)
	Model() string
}
