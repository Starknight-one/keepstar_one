package ports

import (
	"context"

	"keepstar-admin/internal/domain"
)

// InboxPort is the persistence boundary for catalog.inbox_items. It carries
// both the write surface (called by source ingestors — Shopify bulk,
// CSV/Sheets upload, manual entry) and the narrow read surface used by:
//   - discovery_v2 agent tools (ListFields, SampleValues, CountBy, …)
//   - apply_v2 (ListUnapplied, MarkApplied)
//   - curator UI (Inbox tab → GetByID, PeekFullRows)
//
// All write calls go through Upsert which is idempotent on
// (tenant_id, source_kind, external_id) and uses payload_hash to detect
// real content changes (re-fetch of unchanged item is a no-op).
type InboxPort interface {
	// --- write ---

	// Upsert stores or replaces one inbox row. Returns changed=true when
	// either the row was new OR payload_hash differs from what was stored
	// (i.e. real content change). When changed=false, only fetched_at
	// advances. The caller is responsible for computing payload_hash.
	//
	// When content changes, applied_at is reset to NULL so apply_v2 picks
	// it up again.
	Upsert(ctx context.Context, item *domain.InboxItem) (changed bool, err error)

	// MarkApplied stamps applied_at=NOW() on a specific item. Called by
	// apply_v2 after successfully writing the master/listing rows derived
	// from this inbox item.
	MarkApplied(ctx context.Context, itemID string) error

	// DeleteByTenant removes all inbox rows for one tenant. Used on
	// disconnect or full re-import. Cascades via FK from tenants.
	DeleteByTenant(ctx context.Context, tenantID string) error

	// --- basic CRUD ---

	// GetByID returns one full inbox item. Used by curator detail view.
	GetByID(ctx context.Context, itemID string) (*domain.InboxItem, error)

	// ListUnapplied streams rows where applied_at IS NULL. Consumed by
	// apply_v2 in chunks (limit/offset). Order: fetched_at ASC for
	// FIFO-ish processing.
	ListUnapplied(ctx context.Context, tenantID string, limit, offset int) ([]*domain.InboxItem, error)

	// CountTotal returns the row count for a tenant. Cheap aggregate.
	CountTotal(ctx context.Context, tenantID string) (int, error)

	// --- discovery agent tools (read-only JSONB introspection) ---
	//
	// Each method maps 1:1 to one tool exposed to the agent. They take
	// minimal arguments and return compact, token-friendly summaries —
	// agents do NOT receive raw payloads from these tools.

	// ListFields returns up to `limit` distinct top-level JSONB keys across
	// all inbox rows for a tenant. Used by agent's `list_fields` tool.
	ListFields(ctx context.Context, tenantID string, limit int) ([]string, error)

	// SampleValues returns up to `limit` distinct stringified values for
	// the given top-level field. Used by agent's `sample_values` tool.
	SampleValues(ctx context.Context, tenantID, field string, limit int) ([]string, error)

	// CountBy returns the distribution of values for one field:
	// map value→count. Capped at maxBuckets entries (top-N by frequency).
	// Used by agent's `count_by` tool.
	CountBy(ctx context.Context, tenantID, field string, maxBuckets int) (map[string]int, error)

	// FieldStats returns the compact stats summary for one field
	// (non-null count, distinct count, samples, numeric range, length
	// range). Used by agent's `field_stats` tool.
	FieldStats(ctx context.Context, tenantID, field string, sampleN int) (*domain.InboxFieldStats, error)

	// PeekFullRows returns up to `limit` full inbox rows for the tenant.
	// This is a HEAVY tool (whole raw payload per row, blows context). The
	// usecase layer must gate it with a per-run cap (~10 calls). Used by
	// agent's `peek_full_rows` tool.
	PeekFullRows(ctx context.Context, tenantID string, limit int) ([]*domain.InboxItem, error)
}
