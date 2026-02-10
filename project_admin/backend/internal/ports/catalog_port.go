package ports

import (
	"context"

	"keepstar-admin/internal/domain"
)

type AdminCatalogPort interface {
	// Tenant
	GetTenantByID(ctx context.Context, id string) (*domain.Tenant, error)
	CreateTenant(ctx context.Context, tenant *domain.Tenant) (*domain.Tenant, error)
	UpdateTenantSettings(ctx context.Context, tenantID string, settings domain.TenantSettings) error

	// Products (tenant-scoped)
	ListProducts(ctx context.Context, tenantID string, filter domain.AdminProductFilter) ([]domain.Product, int, error)
	GetProduct(ctx context.Context, tenantID string, productID string) (*domain.Product, error)
	UpdateProduct(ctx context.Context, tenantID string, productID string, update domain.ProductUpdate) error

	// Categories
	GetCategories(ctx context.Context) ([]domain.Category, error)

	// Import operations
	UpsertMasterProduct(ctx context.Context, mp *domain.MasterProduct) (string, error)
	UpsertProductListing(ctx context.Context, p *domain.Product) (string, error)
	GetOrCreateCategory(ctx context.Context, name string, slug string) (string, error)

	// Post-import
	GetMasterProductsWithoutEmbedding(ctx context.Context, tenantID string) ([]domain.MasterProduct, error)
	SeedEmbedding(ctx context.Context, masterProductID string, embedding []float32) error
	GenerateCatalogDigest(ctx context.Context, tenantID string) error
}
