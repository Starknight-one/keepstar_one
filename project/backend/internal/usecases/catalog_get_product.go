package usecases

import (
	"context"
	"fmt"

	"keepstar/internal/domain"
	"keepstar/internal/ports"
)

// GetProductUseCase handles single product retrieval logic
type GetProductUseCase struct {
	catalog ports.CatalogPort
}

// NewGetProductUseCase creates a new GetProductUseCase
func NewGetProductUseCase(catalog ports.CatalogPort) *GetProductUseCase {
	return &GetProductUseCase{catalog: catalog}
}

// GetProductRequest represents the input for getting a product
type GetProductRequest struct {
	TenantSlug string
	ProductID  string
}

// Execute retrieves a single product
func (uc *GetProductUseCase) Execute(ctx context.Context, req GetProductRequest) (*domain.Product, error) {
	// Resolve tenant by slug
	tenant, err := uc.catalog.GetTenantBySlug(ctx, req.TenantSlug)
	if err != nil {
		return nil, fmt.Errorf("resolve tenant: %w", err)
	}

	// Get product with merging
	product, err := uc.catalog.GetProduct(ctx, tenant.ID, req.ProductID)
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}

	return product, nil
}
