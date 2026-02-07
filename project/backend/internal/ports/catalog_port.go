package ports

import (
	"context"

	"keepstar/internal/domain"
)

type ProductFilter struct {
	CategoryID string
	Brand      string
	MinPrice   int
	MaxPrice   int
	Search     string
	SortField  string // "price", "rating", "name", "" (default: created_at)
	SortOrder  string // "asc", "desc" (default: "desc")
	Limit      int
	Offset     int
}

type CatalogPort interface {
	// Tenant operations
	GetTenantBySlug(ctx context.Context, slug string) (*domain.Tenant, error)

	// Category operations
	GetCategories(ctx context.Context) ([]domain.Category, error)

	// Master product operations
	GetMasterProduct(ctx context.Context, id string) (*domain.MasterProduct, error)

	// Tenant product operations
	ListProducts(ctx context.Context, tenantID string, filter ProductFilter) ([]domain.Product, int, error)
	GetProduct(ctx context.Context, tenantID string, productID string) (*domain.Product, error)
}
