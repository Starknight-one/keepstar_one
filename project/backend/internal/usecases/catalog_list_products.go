package usecases

import (
	"context"
	"fmt"

	"keepstar/internal/domain"
	"keepstar/internal/ports"
)

// ListProductsUseCase handles product listing logic
type ListProductsUseCase struct {
	catalog ports.CatalogPort
}

// NewListProductsUseCase creates a new ListProductsUseCase
func NewListProductsUseCase(catalog ports.CatalogPort) *ListProductsUseCase {
	return &ListProductsUseCase{catalog: catalog}
}

// ListProductsRequest represents the input for listing products
type ListProductsRequest struct {
	TenantSlug string
	Filter     ports.ProductFilter
}

// ListProductsResponse represents the output of listing products
type ListProductsResponse struct {
	Products []domain.Product `json:"products"`
	Total    int              `json:"total"`
}

// Execute lists products for a tenant
func (uc *ListProductsUseCase) Execute(ctx context.Context, req ListProductsRequest) (*ListProductsResponse, error) {
	// Resolve tenant by slug
	tenant, err := uc.catalog.GetTenantBySlug(ctx, req.TenantSlug)
	if err != nil {
		return nil, fmt.Errorf("resolve tenant: %w", err)
	}

	// Get products with filtering and merging
	products, total, err := uc.catalog.ListProducts(ctx, tenant.ID, req.Filter)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}

	return &ListProductsResponse{
		Products: products,
		Total:    total,
	}, nil
}
