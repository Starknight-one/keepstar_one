// Package postgres — staging buffer for Shopify bulk-pull payloads.
// Spec: docs/New features/admin_catalog_design_2026-04-23.md §4.1 (metadata-first import).
package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

type ShopifyStagingAdapter struct {
	client *Client
	log    *logger.Logger
}

func NewShopifyStagingAdapter(client *Client, log *logger.Logger) *ShopifyStagingAdapter {
	return &ShopifyStagingAdapter{client: client, log: log}
}

// Compile-time interface conformance.
var _ ports.ShopifyStagingPort = (*ShopifyStagingAdapter)(nil)

// UpsertRaw replaces any existing payload for (tenant, kind, source_id).
// The composite primary key on the table makes this idempotent — re-pulls
// after a webhook event won't grow the row count.
func (a *ShopifyStagingAdapter) UpsertRaw(ctx context.Context, tenantID, sourceKind, sourceID string, payload json.RawMessage) error {
	if len(payload) == 0 {
		payload = json.RawMessage(`null`)
	}
	_, err := a.client.pool.Exec(ctx, `
		INSERT INTO catalog.shopify_raw_imports (tenant_id, source_kind, source_id, payload, fetched_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (tenant_id, source_kind, source_id)
		DO UPDATE SET payload = EXCLUDED.payload, fetched_at = NOW()
	`, tenantID, sourceKind, sourceID, payload)
	if err != nil {
		return fmt.Errorf("staging upsert: %w", err)
	}
	return nil
}

// CountByKind returns a small map of {source_kind: row_count}. Cheap aggregate;
// safe to call during long-running progress polling.
func (a *ShopifyStagingAdapter) CountByKind(ctx context.Context, tenantID string) (map[string]int, error) {
	rows, err := a.client.pool.Query(ctx, `
		SELECT source_kind, COUNT(*)::INT
		FROM catalog.shopify_raw_imports
		WHERE tenant_id = $1
		GROUP BY source_kind
	`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("staging count: %w", err)
	}
	defer rows.Close()
	out := make(map[string]int)
	for rows.Next() {
		var kind string
		var n int
		if err := rows.Scan(&kind, &n); err != nil {
			return nil, fmt.Errorf("staging count scan: %w", err)
		}
		out[kind] = n
	}
	return out, rows.Err()
}

// IterateProducts streams in source_id order so a caller's downstream batching
// stays deterministic across runs. JSONB is returned as-is without unmarshal —
// the harvester does its own typed unmarshal once per row.
func (a *ShopifyStagingAdapter) IterateProducts(ctx context.Context, tenantID string, fn func(sourceID string, payload json.RawMessage, fetchedAt time.Time) error) error {
	rows, err := a.client.pool.Query(ctx, `
		SELECT source_id, payload, fetched_at
		FROM catalog.shopify_raw_imports
		WHERE tenant_id = $1 AND source_kind = 'product'
		ORDER BY source_id
	`, tenantID)
	if err != nil {
		return fmt.Errorf("staging iterate: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			sourceID  string
			payload   json.RawMessage
			fetchedAt time.Time
		)
		if err := rows.Scan(&sourceID, &payload, &fetchedAt); err != nil {
			return fmt.Errorf("staging iterate scan: %w", err)
		}
		if err := fn(sourceID, payload, fetchedAt); err != nil {
			return err
		}
	}
	return rows.Err()
}

// DeleteByTenant clears the staging buffer entirely. Reinstall flow uses this
// before a fresh bulk pull so we don't keep stale source_ids around forever.
func (a *ShopifyStagingAdapter) DeleteByTenant(ctx context.Context, tenantID string) error {
	_, err := a.client.pool.Exec(ctx, `
		DELETE FROM catalog.shopify_raw_imports WHERE tenant_id = $1
	`, tenantID)
	if err != nil {
		return fmt.Errorf("staging delete tenant: %w", err)
	}
	return nil
}
