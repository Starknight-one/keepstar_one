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

// VectorFilter narrows VectorSearch before cosine ranking. Mirrors V4's
// shape so prompts and tool inputs translate one-for-one. Fields map to
// columns on master_products / categories — same predicates as ProductFilter
// minus the price range (vector search ranks by semantic distance, not price).
type VectorFilter struct {
	Brand         string
	CategoryName  string
	ProductForm   string
	SkinType      string
	Concern       string
	KeyIngredient string
	TargetArea    string
	RoutineStep   string
	Texture       string
}

// CatalogPort exposes read access to the shared catalog.* schema (owned by
// project_admin). V5 only reads; admin/curator own the write side.
type CatalogPort interface {
	GetTenantBySlug(ctx context.Context, slug string) (*domain.Tenant, error)
	ListProducts(ctx context.Context, tenantID string, filter ProductFilter) ([]domain.Product, int, error)
	GetProduct(ctx context.Context, tenantID string, productID string) (*domain.Product, error)

	// VectorSearch finds products by pgvector cosine distance against
	// master_products.embedding. filter may be nil for unfiltered search.
	// Caller supplies the embedding (typically via EmbeddingPort). Retained for
	// non-projection callers; catalog_search now uses SearchProjection.
	VectorSearch(ctx context.Context, tenantID string, embedding []float32, limit int, filter *VectorFilter) ([]domain.Product, error)

	// SearchProjection ranks catalog.tenant_search_projection by fused cosine +
	// full-text score (per-tenant slice) and hydrates display fields. embedding
	// may be nil (FTS-only). filter.Search is the FTS query; other fields narrow
	// via tier2/junction. This is the single read used by catalog_search.
	SearchProjection(ctx context.Context, tenantID string, embedding []float32, filter ProductFilter, limit int) ([]domain.Product, error)

	// BuildCatalogDigest assembles a per-tenant compact summary of the catalog
	// (categories, shared filters, top brands, top ingredients) for inlining
	// into Agent1's system prompt. V4 stored the result in tenants.catalog_digest;
	// V5 builds on demand and lets the use case cache in-process.
	BuildCatalogDigest(ctx context.Context, tenantID string) (*domain.CatalogDigest, error)
}
