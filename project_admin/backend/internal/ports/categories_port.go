package ports

import (
	"context"

	"keepstar-admin/internal/domain"
)

// CategoriesPort handles the M:N taxonomy: tenant_categories (their tree)
// + master_categories (our taxonomy) + mapping (N:1 with NULLable target).
type CategoriesPort interface {
	// --- Master categories (curator-only) ---

	UpsertMasterCategory(ctx context.Context, mc *domain.MasterCategory) (string, error)
	ListMasterCategories(ctx context.Context, vertical string) ([]domain.MasterCategory, error)

	// --- Tenant categories (per-tenant, populated by harvester from collections) ---

	UpsertTenantCategory(ctx context.Context, tc *domain.TenantCategory) (string, error)
	ListTenantCategories(ctx context.Context, tenantID string) ([]domain.TenantCategory, error)
	ListTenantCategoriesWithCounts(ctx context.Context, tenantID string) ([]domain.TenantCategoryWithCount, error)
	DeleteTenantCategory(ctx context.Context, tenantID, categoryID string) error

	// --- Category mapping (tenant_category -> master_category, N:1, NULLable) ---

	SetCategoryMapping(ctx context.Context, mapping *domain.CategoryMapping) error
	GetCategoryMapping(ctx context.Context, tenantCategoryID string) (*domain.CategoryMapping, error)

	// --- M:N joins ---

	// LinkMasterProductToCategories replaces all category links for a master.
	LinkMasterProductToCategories(ctx context.Context, masterProductID string, masterCategoryIDs []string) error

	// LinkListingToCategories replaces all tenant-category links for a listing.
	LinkListingToCategories(ctx context.Context, listingID string, tenantCategoryIDs []string) error

	// ListListingCategories returns the tenant_categories a listing belongs to.
	ListListingCategories(ctx context.Context, listingID string) ([]domain.TenantCategory, error)
}
