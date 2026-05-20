package ports

import (
	"context"

	"keepstar-admin/internal/domain"
)

// MasterDigestPort is the read-only view of the master catalog exposed to
// discovery_v2's LLM agent. It is INTENTIONALLY narrow:
//
//   - The agent must be able to see what categories and brands we already
//     have for a vertical before proposing a mapping artifact, so it can
//     mirror our taxonomy instead of inventing parallel structures.
//   - The agent must be able to look up specific masters by SKU/GTIN so it
//     can write classify_rules that route inbox rows to bind paths rather
//     than create-new-master paths.
//
// All methods are global (not tenant-scoped) — the master catalog is the
// shared substrate every tenant binds against.
type MasterDigestPort interface {
	// ListMasterCategories returns every master category for a vertical.
	// Empty vertical returns all categories (rarely useful for the agent —
	// it should always pass the tenant's vertical hint). ProductCount is
	// the number of master_products linked via master_product_categories
	// M:N, capped at limit.
	ListMasterCategories(ctx context.Context, vertical string, limit int) ([]domain.MasterCategoryNode, error)

	// DigestMasterCategory returns the compact roll-up for one category:
	// total product count, top brands by frequency, and recent product
	// names. Used by the agent to understand what data shape already
	// lives there.
	DigestMasterCategory(ctx context.Context, categoryID string) (*domain.MasterCategoryDigest, error)

	// FindMasterBySKU resolves a SKU to a MasterRef. Case-insensitive.
	// Returns nil when no master matches (not an error — common case).
	FindMasterBySKU(ctx context.Context, sku string) (*domain.MasterRef, error)

	// FindMasterByGTIN resolves a GTIN to a MasterRef. Exact match.
	// Returns nil when no master matches.
	FindMasterByGTIN(ctx context.Context, gtin string) (*domain.MasterRef, error)

	// ListMasterBrandsInCategory returns the top-N brands present in the
	// given master category, ordered by product count desc. limit caps the
	// number of brands; pass 0 for the default (30).
	ListMasterBrandsInCategory(ctx context.Context, categoryID string, limit int) ([]string, error)
}
