package ports

import (
	"context"

	"keepstar-admin/internal/domain"
)

type EnrichmentPort interface {
	EnrichProducts(ctx context.Context, items []domain.EnrichmentInput) (*domain.EnrichmentResult, error)
	EnrichProductsV2(ctx context.Context, items []domain.EnrichmentInput) (*domain.EnrichmentResultV2, error)
	Model() string
}
