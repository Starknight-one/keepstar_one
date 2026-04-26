package ports

import (
	"context"
	"encoding/json"
	"time"
)

// ShopifyStagingPort persists raw bulk-pull payloads from Shopify before any
// transformation. The staging buffer (catalog.shopify_raw_imports) lets the
// discovery agent and harvester read the same canonical bytes the merchant
// sent us — no early lossy mapping. Re-pulls overwrite (ON CONFLICT) so the
// table always reflects the latest known state of the source catalog.
//
// SourceKind values currently in use:
//   - "product"    → top-level Shopify product node
//   - "collection" → standalone collection node (when not embedded under a product)
//   - "menu"       → navigation menu snapshot (one row per handle)
//   - "metadata"   → discovery context (vendors, types, tags, metafield defs)
type ShopifyStagingPort interface {
	// UpsertRaw stores one row keyed by (tenant_id, source_kind, source_id).
	// payload is stored as-is (no normalization). Existing rows are replaced
	// and fetched_at advances to NOW().
	UpsertRaw(ctx context.Context, tenantID, sourceKind, sourceID string, payload json.RawMessage) error

	// CountByKind returns row counts grouped by source_kind for a tenant.
	// Used by the progress UI ("Pulled 17 products, 5 collections...").
	CountByKind(ctx context.Context, tenantID string) (map[string]int, error)

	// IterateProducts streams every staged product row for a tenant in id order.
	// Callback receives (sourceID, payload, fetchedAt). Returning a non-nil error
	// from the callback aborts iteration. Implementation must NOT load the entire
	// table into memory — staged catalogs can be ≥1GB JSON.
	IterateProducts(ctx context.Context, tenantID string, fn func(sourceID string, payload json.RawMessage, fetchedAt time.Time) error) error

	// DeleteByTenant removes all staged rows for a tenant. Used during a
	// reinstall or on integration disconnect.
	DeleteByTenant(ctx context.Context, tenantID string) error
}
