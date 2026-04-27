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

	// FindByVendorAndFuzzyName scans variants of products whose brand matches
	// vendor (case-insensitive ILIKE) and whose name has trigram similarity
	// to the given title above the threshold (typically 0.85). Results are
	// returned with the similarity score in MatchedScore for the cascade to
	// pick the top hit. Limit caps result count (10 is plenty).
	FindByVendorAndFuzzyName(ctx context.Context, vendor, title string, threshold float64, limit int) ([]MasterVariantWithScore, error)

	// FindByEmbedding does a vector-cosine search against master_variants.embedding.
	// Returns variants ordered by similarity DESC, capped at limit. Threshold
	// is a cosine distance bound — typical cascade value is 0.92 (i.e. distance
	// < 0.08). Implementations should rely on the hnsw index for performance.
	FindByEmbedding(ctx context.Context, embedding []float32, threshold float64, limit int) ([]MasterVariantWithScore, error)

	// UpsertMasterCosmetics writes/updates the cosmetics Tier 2 row for a variant.
	UpsertMasterCosmetics(ctx context.Context, mc *domain.MasterCosmetics) error

	// GetMasterCosmetics returns the Tier 2 cosmetics row for a variant, or nil.
	GetMasterCosmetics(ctx context.Context, masterVariantID string) (*domain.MasterCosmetics, error)

	// FindMasterProductsByEmbedding searches master_products (not variants) by
	// cosine similarity. Used by harvester (M4d) match cascade step 4 — the
	// expensive long-tail catch-all. Discovery agent uses the cheaper
	// FindMasterProductsByName instead (no embedding key required).
	FindMasterProductsByEmbedding(ctx context.Context, embedding []float32, vertical string, limit int) ([]MasterProductSummary, error)

	// FindMasterProductsByName runs a pg_trgm fuzzy match over name + brand.
	// Used by the discovery agent's find_similar_masters tool — no external
	// dependency, no API costs. Optional vertical filter narrows scope.
	// Empty vertical = search all. Score is the trigram similarity [0, 1].
	FindMasterProductsByName(ctx context.Context, query, vertical string, limit int) ([]MasterProductSummary, error)

	// GetMasterProductSummary returns a compact view of one master_product
	// + variant summaries for the discovery agent's peek_master tool.
	// Returns nil if not found. Tier 2 columns aren't included — they're
	// vertical-specific and the agent reads them via candidate-tools instead.
	GetMasterProductSummary(ctx context.Context, masterProductID string) (*MasterProductSummary, error)

	// GetMasterOverview returns a compact catalog-wide overview of master_products
	// grouped by vertical. Used to inject ~500 tokens of "what's in master?" into
	// the discovery agent's system prompt so it can make merge decisions without
	// per-listing tool calls. Computed on-demand (not cached) — the SQL is small
	// and runs once per agent session.
	GetMasterOverview(ctx context.Context) (*MasterOverview, error)
}

// MasterOverview is the master-side digest fed into the discovery agent's
// system prompt. Format mirrors V4's CatalogDigest (tenant-side digest) so
// the agent gets a consistent "soup" view of both sides.
type MasterOverview struct {
	TotalProducts   int                       `json:"totalProducts"`
	Verticals       []MasterVerticalSummary   `json:"verticals"`
	RegisteredFields []MasterFieldDefSummary  `json:"registeredFields,omitempty"` // from master_field_definitions
}

// MasterVerticalSummary is one vertical's slice of the master overview.
type MasterVerticalSummary struct {
	Vertical    string   `json:"vertical"`
	ProductCount int     `json:"productCount"`
	TopBrands   []string `json:"topBrands,omitempty"`   // ≤10
	Categories  []string `json:"categories,omitempty"`  // master_categories slugs ≤20
}

// MasterFieldDefSummary is a tier-2 field already promoted in master_field_definitions.
// The agent uses this to know which keys it should map TO existing typed fields
// vs. propose as new candidates.
type MasterFieldDefSummary struct {
	Vertical string   `json:"vertical"`
	Key      string   `json:"key"`
	Type     string   `json:"type"`
	EnumValues []string `json:"enumValues,omitempty"`
}

// MasterProductSummary is the discovery-agent-facing snapshot of a master.
// Trimmed to what the agent needs to make a linkage call: enough identity
// (name, brand, vertical) and shape (variant count, axis names, GTIN
// presence) to decide whether incoming data should link or not.
type MasterProductSummary struct {
	ID            string                  `json:"id"`
	Name          string                  `json:"name"`
	Brand         string                  `json:"brand"`
	Vertical      string                  `json:"vertical"`
	Confidence    string                  `json:"confidence"`
	OwnerTenantID string                  `json:"ownerTenantId,omitempty"`
	Score         float64                 `json:"score,omitempty"` // similarity, when from FindMasterProductsByEmbedding
	Variants      []MasterVariantSnippet  `json:"variants,omitempty"`
}

// MasterVariantSnippet is a tiny per-variant view embedded in
// MasterProductSummary. Picks the absolute minimum the agent needs to
// recognize a variant — sku + axes + presence flags for GTIN/dimensions.
type MasterVariantSnippet struct {
	ID          string            `json:"id"`
	SKU         string            `json:"sku,omitempty"`
	Axes        map[string]string `json:"axes,omitempty"`
	HasGTIN     bool              `json:"hasGtin"`
	VariantKind string            `json:"variantKind"`
}

// MasterVariantWithScore is a match-cascade result row that carries the
// similarity score the SQL query computed (trigram or cosine, depending on
// step). Score range: [0, 1] for trigram, [0, 2] for cosine distance —
// adapters normalize to "higher is better" so callers can compare across
// steps without remembering which is which.
type MasterVariantWithScore struct {
	domain.MasterVariant
	MatchedScore float64
}
