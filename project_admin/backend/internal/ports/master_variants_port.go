package ports

import (
	"context"

	"keepstar-admin/internal/domain"
)

// MasterVariantsPort handles parent-child variant model (spec §3.1).
// Implementations live in adapters/postgres/master_variants_adapter.go.
type MasterVariantsPort interface {
	// UpsertMasterVariant inserts or updates by (master_product_id, sku).
	// If sku is empty, treats as new variant. Returns the variant ID.
	UpsertMasterVariant(ctx context.Context, mv *domain.MasterVariant) (string, error)

	// GetMasterVariant returns a single variant by ID, or nil if not found.
	GetMasterVariant(ctx context.Context, id string) (*domain.MasterVariant, error)

	// ListMasterVariants returns all variants for a given master product.
	ListMasterVariants(ctx context.Context, masterProductID string) ([]domain.MasterVariant, error)

	// FindByGTIN returns variants matching any of the given GTINs (Postgres
	// `gtins && $1`). Returns empty slice if none. Used by match cascade.
	FindByGTIN(ctx context.Context, gtins []string) ([]domain.MasterVariant, error)

	// FindByVendorAndSKU narrows by master_product.brand + variant.sku.
	// Used as fallback in match cascade when GTIN absent.
	FindByVendorAndSKU(ctx context.Context, vendor, sku string) ([]domain.MasterVariant, error)

	// UpsertMasterCosmetics writes/updates the cosmetics Tier 2 row for a variant.
	UpsertMasterCosmetics(ctx context.Context, mc *domain.MasterCosmetics) error

	// GetMasterCosmetics returns the Tier 2 cosmetics row for a variant, or nil.
	GetMasterCosmetics(ctx context.Context, masterVariantID string) (*domain.MasterCosmetics, error)
}
