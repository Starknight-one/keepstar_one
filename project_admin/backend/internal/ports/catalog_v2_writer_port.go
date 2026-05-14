package ports

import (
	"context"
	"errors"
)

// ErrCosmeticsSchemaNotReady is returned by UpsertCosmetics when the
// master_cosmetics table still uses the legacy master_variant_id PK. Apply
// reacts by routing the cosmetics fields into tier3 JSONB instead.
var ErrCosmeticsSchemaNotReady = errors.New("master_cosmetics schema not yet reshaped (legacy master_variant_id PK)")

// CatalogV2WriterPort is the write surface apply_v2 uses to land mapped data
// into master_products + per-vertical tables + slim catalog.products. Kept
// narrow on purpose — each method is one logical row write that the adapter
// implements with one or two SQL statements.
//
// Idempotency: all upsert methods are keyed on natural identifiers
// (master.sku, listing's tenant_id+source_system+source_id) so re-running
// apply_v2 on the same inbox rows converges to the same state.
type CatalogV2WriterPort interface {
	// UpsertMaster writes/updates one master_products row by SKU. Returns the
	// resulting master_product_id.
	UpsertMaster(ctx context.Context, mp *MasterProductUpsert) (string, error)

	// UpsertCosmetics writes per-vertical cosmetics data keyed by
	// master_product_id. Returns ErrCosmeticsSchemaNotReady when the
	// master_cosmetics table still has the legacy master_variant_id PK shape
	// — apply_v2 reacts by routing cosmetics.* fields into tier3 instead.
	UpsertCosmetics(ctx context.Context, masterID string, fields *MasterCosmeticsUpsert) error

	// MergeTier3 merges (deep-overwrite at top-level keys) the given JSON
	// object into master_products.tier3. Apply_v2 calls this for the
	// "unknown vertical" path and for cosmetics fallback when cosmetics
	// schema isn't ready.
	MergeTier3(ctx context.Context, masterID string, patch map[string]any) error

	// UpsertListing writes/updates one catalog.products row keyed by
	// (tenant_id, source_system, source_id).
	UpsertListing(ctx context.Context, lst *ListingUpsert) (string, error)
}

// MasterProductUpsert is the Tier-1 master row payload.
type MasterProductUpsert struct {
	SKU           string // natural key, required
	Name          string // required
	Brand         string
	Description   string
	Vertical      string // 'cosmetics' | 'electronics' | … | 'unknown'
	ImageURL      string // first image; full array of images stays in tier3.images if needed
	OwnerTenantID string // the tenant that "introduced" this master (FK)
}

// MasterCosmeticsUpsert mirrors the (reshaped) master_cosmetics column set.
// Pointer types for nullable scalars; nil means "leave existing value".
type MasterCosmeticsUpsert struct {
	SkinType          []string
	Concern           []string
	KeyIngredients    []string
	TargetArea        []string
	ProductForm       *string
	Texture           *string
	RoutineStep       *string
	RoutineTime       *string
	ApplicationMethod *string
	FreeFrom          []string
	Scent             *string
	SPF               *int
	MarketingClaim    *string
	Benefits          []string
	HowToUse          *string
	VolumeML          *int
	WeightG           *int
	UnitCount         *int
	Extra             map[string]any // misc cosmetic attrs that didn't earn a column
}

// ListingUpsert is the slim tenant overlay row in catalog.products.
type ListingUpsert struct {
	TenantID        string
	MasterProductID string // FK; required
	Price           int    // cents
	Currency        string
	Stock           int
	CustomTitle     string // optional tenant-side rename
	SourceSystem    string // inbox.source_kind, e.g. 'shopify'
	SourceID        string // inbox.external_id
}
