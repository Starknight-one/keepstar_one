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
// Idempotency: all writes are keyed on natural identifiers (master.sku,
// listing's tenant_id+source_system+source_id) so re-running apply_v2 over
// the same inbox rows converges to the same state.
//
// **No master overwrites on bind**: on a successful match in
// MatchOrCreateMaster the existing master_products row is returned as-is.
// Apply_v2 never touches its columns directly. New data from the inbox flows
// instead to catalog.master_pending_changes via EnrichExistingMaster — that
// is the staging path the curator approves before any master row is mutated.
type CatalogV2WriterPort interface {
	// MatchOrCreateMaster runs the deterministic 3-stage match cascade
	// (SKU → GTIN → normalized_match_key) and returns the matched master_id
	// without touching it. On a miss, INSERTs a new master_products row.
	//
	// wasCreated is true when this call inserted the row, false when the row
	// existed before. Apply_v2 uses this to decide whether to also write
	// Tier 2 (typed) data — we only write Tier 2 on creation; on bind we
	// only write the listing and stage enrichments via EnrichExistingMaster.
	//
	// When mp.CreateAsPending is true and this call ends up inserting a new
	// row, the adapter sets approval_status='pending_approval' and
	// pending_approval_expires_at=NOW()+30d. Otherwise approval_status
	// defaults to 'approved' (back-compat for callers that pre-date the
	// staging flow).
	MatchOrCreateMaster(ctx context.Context, mp *MasterProductUpsert) (id string, wasCreated bool, err error)

	// EnrichExistingMaster stages field-level changes against an existing
	// master_products row, one batch call per applyOne loop. Two op kinds:
	//
	//   enrich_array_union  — proposed array element NOT already present in
	//                          the master's current value; one pending row
	//                          per new element.
	//   enrich_scalar_fill  — proposed scalar where the master's current
	//                          value is NULL or ''. Non-empty scalars are
	//                          never proposed (curator-only edit space).
	//
	// The adapter performs the empty/diff checks server-side via SQL — the
	// caller passes the FULL proposed value set, the adapter decides what to
	// stage. Returns the count of pending rows actually inserted.
	//
	// Idempotency: the same (master, field, value) tuple is dedup'd by the
	// adapter so re-running apply_v2 over identical inbox rows does not
	// pile up duplicate pending entries.
	EnrichExistingMaster(ctx context.Context, items []EnrichRequest) (int, error)

	// UpsertCosmetics writes per-vertical cosmetics data keyed by
	// master_product_id. Returns ErrCosmeticsSchemaNotReady when the
	// master_cosmetics table still has the legacy master_variant_id PK shape
	// — apply_v2 reacts by routing cosmetics.* fields into tier3 instead.
	UpsertCosmetics(ctx context.Context, masterID string, fields *MasterCosmeticsUpsert) error

	// MergeTier3 merges (top-level overwrite) the given JSON object into
	// master_products.tier3. Apply_v2 calls this for the unknown-vertical
	// path and for cosmetics fallback when cosmetics schema isn't ready.
	MergeTier3(ctx context.Context, masterID string, patch map[string]any) error

	// UpsertListing writes/updates one catalog.products row keyed by
	// (tenant_id, source_system, source_id).
	UpsertListing(ctx context.Context, lst *ListingUpsert) (string, error)

	// BulkUpsertListing writes/updates many listings in one statement per
	// batch. Same upsert semantics as UpsertListing — idempotent on
	// (tenant_id, master_product_id) and revives soft-deleted rows.
	// Returns the count of rows actually inserted-or-updated. The adapter
	// chunks input into batches (~500) under the hood; callers can pass an
	// arbitrarily large slice. Apply path will move to this after the
	// first milestone — per-row UpsertListing stays as the per-applyOne
	// fallback until then.
	BulkUpsertListing(ctx context.Context, items []ListingUpsert) (int, error)

	// LookupVertical resolves a shopify-style product_type string into the
	// internal vertical class via catalog.vertical_aliases. The alias table
	// is lowercased on insert; the lookup lowercases its input. Returns
	// found=false when no alias row matches (apply_v2 falls back to the
	// artifact's classify_rules, then to "unknown").
	LookupVertical(ctx context.Context, productType string) (vertical string, found bool, err error)

	// SoftDeleteListing marks a catalog.products row as deleted by setting
	// deleted_at = NOW(). The master_product row is untouched (other
	// tenants may still link to it). Idempotent: re-calling on an already
	// soft-deleted listing is a no-op. Returns nil even when no row matches
	// (delete-of-unknown is harmless and surfaces in callers' logs only).
	SoftDeleteListing(ctx context.Context, tenantID, sourceSystem, sourceID string) error
}

// MasterProductUpsert is the Tier-1 master row payload. Apply_v2 fills these
// fields from the active artifact branch's rules; the writer adapter applies
// the cascade against (SKU, GTIN, NormalizedMatchKey) before considering an
// INSERT.
type MasterProductUpsert struct {
	SKU                string // natural key; required (apply_v2 synthesizes "auto-..." when source has no SKU but has name)
	Name               string // required
	Brand              string
	Description        string
	Vertical           string // 'cosmetics' | 'electronics' | … | 'unknown'
	ImageURL           string // first image; full media array goes in tier3.images
	OwnerTenantID      string // tenant that "introduced" this master (FK; never reassigned on bind)
	GTIN               string // optional secondary key for stage 2 of the cascade
	NormalizedMatchKey string // computed via NormalizeMatchKey(brand, name); empty = skip stage 3
	CreateAsPending    bool   // when true and this call inserts, the row lands with approval_status='pending_approval' (30-day TTL)
}

// EnrichRequest is one master + a bag of proposed enrichments. The adapter
// decides which actually stage (only empty scalars; only set-diff array
// elements). One request per inbox row on the bind path.
type EnrichRequest struct {
	MasterProductID   string              // FK to catalog.master_products.id
	TenantID          string              // FK to catalog.tenants.id (the tenant whose inbox row produced this)
	SourceInboxItemID string              // FK to catalog.inbox_items.id, audit trail
	Scalars           map[string]any      // master_products column name → proposed value; nil/empty values are ignored
	Arrays            map[string][]string // master_products array column name → proposed elements; only elements absent from current value land in pending
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
