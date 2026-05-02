package ports

import (
	"context"

	"keepstar_v5/internal/domain"
)

// ProductFilter describes the read-side query parameters for ListProducts.
// Same shape as V4 — agents already speak this vocabulary.
type ProductFilter struct {
	CategoryID   string
	CategoryName string
	Brand        string
	MinPrice     int
	MaxPrice     int
	Search       string
	SortField    string // "price" | "rating" | "name" | "" (default created_at)
	SortOrder    string // "asc" | "desc" (default "desc")
	Limit        int
	Offset       int

	// Typed PIM filters
	ProductForm   string
	SkinType      string
	Concern       string
	KeyIngredient string
	TargetArea    string
	RoutineStep   string
	Texture       string
}

// CatalogPort exposes read access to the shared catalog.* schema (owned by
// project_admin). V5 only reads; admin/curator own the write side. Vector
// search and write methods deferred to later chunks.
type CatalogPort interface {
	GetTenantBySlug(ctx context.Context, slug string) (*domain.Tenant, error)
	ListProducts(ctx context.Context, tenantID string, filter ProductFilter) ([]domain.Product, int, error)
	GetProduct(ctx context.Context, tenantID string, productID string) (*domain.Product, error)
}
