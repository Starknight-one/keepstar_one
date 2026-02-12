package ports

import (
	"context"

	"keepstar/internal/domain"
)

type ProductFilter struct {
	CategoryID   string
	CategoryName string            // category name/slug for ILIKE matching (agent passes name, not UUID)
	Brand        string
	MinPrice     int
	MaxPrice     int
	Search       string
	SortField    string            // "price", "rating", "name", "" (default: created_at)
	SortOrder    string            // "asc", "desc" (default: "desc")
	Limit        int
	Offset       int
	Attributes   map[string]string // JSONB attribute filters (key → ILIKE value)
}

// VectorFilter holds optional filters for VectorSearch to narrow results before ranking.
type VectorFilter struct {
	Brand        string
	CategoryName string
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

	// Stock operations
	GetStock(ctx context.Context, tenantID string, productID string) (*domain.Stock, error)

	// Service operations
	ListServices(ctx context.Context, tenantID string, filter ProductFilter) ([]domain.Service, int, error)
	GetService(ctx context.Context, tenantID string, serviceID string) (*domain.Service, error)
	VectorSearchServices(ctx context.Context, tenantID string, embedding []float32, limit int, filter *VectorFilter) ([]domain.Service, error)
	GetMasterServicesWithoutEmbedding(ctx context.Context) ([]domain.MasterService, error)

	// VectorSearch finds products by semantic similarity via pgvector.
	// filter may be nil for unfiltered search.
	VectorSearch(ctx context.Context, tenantID string, embedding []float32, limit int, filter *VectorFilter) ([]domain.Product, error)

	// SeedEmbedding saves embedding for a master product.
	SeedEmbedding(ctx context.Context, masterProductID string, embedding []float32) error

	// SeedServiceEmbedding saves embedding for a master service.
	SeedServiceEmbedding(ctx context.Context, masterServiceID string, embedding []float32) error

	// GetMasterProductsWithoutEmbedding returns master products that need embeddings.
	GetMasterProductsWithoutEmbedding(ctx context.Context) ([]domain.MasterProduct, error)

	// GenerateCatalogDigest computes a compact catalog meta-schema for a tenant.
	// Aggregates categories, brands, price ranges, attribute cardinality.
	GenerateCatalogDigest(ctx context.Context, tenantID string) (*domain.CatalogDigest, error)

	// GetCatalogDigest returns the pre-computed digest from tenants.catalog_digest.
	GetCatalogDigest(ctx context.Context, tenantID string) (*domain.CatalogDigest, error)

	// SaveCatalogDigest persists the computed digest to the tenants table.
	SaveCatalogDigest(ctx context.Context, tenantID string, digest *domain.CatalogDigest) error

	// GetAllTenants returns all tenants for batch operations (e.g. digest generation).
	GetAllTenants(ctx context.Context) ([]domain.Tenant, error)
}
