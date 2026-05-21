// Package postgres — InboxAdapter implements ports.InboxPort against
// catalog.inbox_items (the universal raw-tenant-data buffer for the 6-step
// catalog flow). All discovery-agent tools live here as narrow SQL queries
// that return token-friendly aggregates, not raw payloads.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

type InboxAdapter struct {
	client *Client
	log    *logger.Logger
}

func NewInboxAdapter(client *Client, log *logger.Logger) *InboxAdapter {
	return &InboxAdapter{client: client, log: log}
}

// Compile-time interface conformance.
var _ ports.InboxPort = (*InboxAdapter)(nil)

// Upsert: idempotent on (tenant, source_kind, external_id). Uses a CTE to
// snapshot the OLD payload_hash before ON CONFLICT DO UPDATE rewrites it,
// so we can tell the caller whether the content actually changed.
//
// Behaviour on conflict:
//   - same payload_hash → only fetched_at advances; raw + applied_at untouched
//   - different payload_hash → raw replaced, applied_at reset to NULL so
//     apply_v2 reprocesses the item.
func (a *InboxAdapter) Upsert(ctx context.Context, item *domain.InboxItem) (bool, error) {
	if item == nil {
		return false, fmt.Errorf("inbox upsert: nil item")
	}
	if item.TenantID == "" || item.ExternalID == "" || item.SourceKind == "" {
		return false, fmt.Errorf("inbox upsert: tenant_id/external_id/source_kind required")
	}
	if len(item.Raw) == 0 {
		item.Raw = json.RawMessage("null")
	}

	var oldHash string
	err := a.client.pool.QueryRow(ctx, `
		WITH old AS (
			SELECT payload_hash AS h
			FROM catalog.inbox_items
			WHERE tenant_id = $1 AND source_kind = $2 AND external_id = $3
		)
		INSERT INTO catalog.inbox_items (tenant_id, source_kind, external_id, raw, payload_hash, fetched_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (tenant_id, source_kind, external_id) DO UPDATE
		SET fetched_at  = NOW(),
		    raw         = CASE WHEN inbox_items.payload_hash = EXCLUDED.payload_hash
		                       THEN inbox_items.raw ELSE EXCLUDED.raw END,
		    payload_hash = EXCLUDED.payload_hash,
		    applied_at   = CASE WHEN inbox_items.payload_hash = EXCLUDED.payload_hash
		                        THEN inbox_items.applied_at ELSE NULL END
		RETURNING COALESCE((SELECT h FROM old), '')
	`, item.TenantID, string(item.SourceKind), item.ExternalID, item.Raw, item.PayloadHash).
		Scan(&oldHash)
	if err != nil {
		return false, fmt.Errorf("inbox upsert exec: %w", err)
	}
	changed := oldHash == "" || oldHash != item.PayloadHash
	return changed, nil
}

// bulkUpsertBatchSize caps one server-side INSERT. ~500 inbox rows fit
// comfortably under Postgres' parameter limits and Neon's per-statement
// resource limits while still cutting the 8.5k-row Sephora seed from
// ~8500 roundtrips down to ~17. Tune up only if profiling shows the SQL
// CPU dominates the network savings.
const bulkUpsertBatchSize = 500

// BulkUpsert: same per-row semantics as Upsert, but one SQL per batch of
// rows. Implementation strategy:
//
//   - Group input by batches of bulkUpsertBatchSize.
//   - Per batch: pass 5 parallel text[] arrays through UNNEST. A CTE
//     `prior` LEFT JOINs the input keys against the existing table so we
//     can compute changed-vs-unchanged from the OLD payload_hash before
//     the upsert rewrites it. The INSERT … ON CONFLICT DO UPDATE block
//     is the same one Upsert uses.
//
// Returns the number of rows that were either brand-new or whose
// payload_hash differed from the stored value — i.e. the number that
// callers should report as "Inserted" in InboxWriteResult. Rows missing
// any of tenant_id/source_kind/external_id are silently skipped (caller
// is expected to pre-validate; Upsert is the path for per-row error
// attribution).
func (a *InboxAdapter) BulkUpsert(ctx context.Context, items []*domain.InboxItem) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}
	totalChanged := 0
	for start := 0; start < len(items); start += bulkUpsertBatchSize {
		end := min(start+bulkUpsertBatchSize, len(items))
		c, err := a.bulkUpsertBatch(ctx, items[start:end])
		if err != nil {
			return totalChanged, fmt.Errorf("inbox bulk upsert batch [%d:%d]: %w", start, end, err)
		}
		totalChanged += c
	}
	return totalChanged, nil
}

func (a *InboxAdapter) bulkUpsertBatch(ctx context.Context, batch []*domain.InboxItem) (int, error) {
	tids := make([]string, 0, len(batch))
	sks := make([]string, 0, len(batch))
	eids := make([]string, 0, len(batch))
	raws := make([]string, 0, len(batch))
	phs := make([]string, 0, len(batch))
	for _, it := range batch {
		if it == nil || it.TenantID == "" || it.ExternalID == "" || it.SourceKind == "" {
			continue
		}
		raw := it.Raw
		if len(raw) == 0 {
			raw = json.RawMessage("null")
		}
		tids = append(tids, it.TenantID)
		sks = append(sks, string(it.SourceKind))
		eids = append(eids, it.ExternalID)
		raws = append(raws, string(raw))
		phs = append(phs, it.PayloadHash)
	}
	if len(tids) == 0 {
		return 0, nil
	}

	var changed int
	err := a.client.pool.QueryRow(ctx, `
		WITH input AS (
			SELECT
				t.tid::uuid AS tid,
				t.sk        AS sk,
				t.eid       AS eid,
				t.raw::jsonb AS raw,
				t.ph        AS ph,
				t.ord       AS ord
			FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::text[])
				WITH ORDINALITY AS t(tid, sk, eid, raw, ph, ord)
		),
		prior AS (
			SELECT i.ord, ii.payload_hash AS old_hash
			FROM input i
			LEFT JOIN catalog.inbox_items ii
				ON ii.tenant_id   = i.tid
				AND ii.source_kind = i.sk
				AND ii.external_id = i.eid
		),
		ins AS (
			INSERT INTO catalog.inbox_items (tenant_id, source_kind, external_id, raw, payload_hash, fetched_at)
			SELECT tid, sk, eid, raw, ph, NOW() FROM input
			ON CONFLICT (tenant_id, source_kind, external_id) DO UPDATE
			SET fetched_at  = NOW(),
			    raw         = CASE WHEN inbox_items.payload_hash = EXCLUDED.payload_hash
			                       THEN inbox_items.raw ELSE EXCLUDED.raw END,
			    payload_hash = EXCLUDED.payload_hash,
			    applied_at  = CASE WHEN inbox_items.payload_hash = EXCLUDED.payload_hash
			                       THEN inbox_items.applied_at ELSE NULL END
		)
		-- Per Postgres docs: a data-modifying CTE without RETURNING still
		-- executes exactly once. The prior CTE was snapshotted BEFORE the
		-- upsert, so this count reflects pre-upsert state: brand-new keys
		-- plus existing keys whose payload_hash differs from input.
		SELECT COUNT(*)::int FROM input i
		JOIN prior p ON p.ord = i.ord
		WHERE p.old_hash IS NULL OR p.old_hash <> i.ph
	`, tids, sks, eids, raws, phs).Scan(&changed)
	if err != nil {
		return 0, fmt.Errorf("inbox bulk upsert exec: %w", err)
	}
	return changed, nil
}

func (a *InboxAdapter) MarkApplied(ctx context.Context, itemID string) error {
	tag, err := a.client.pool.Exec(ctx, `
		UPDATE catalog.inbox_items
		SET applied_at = NOW()
		WHERE id = $1
	`, itemID)
	if err != nil {
		return fmt.Errorf("inbox mark applied: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("inbox mark applied: item %s not found", itemID)
	}
	return nil
}

func (a *InboxAdapter) BulkMarkApplied(ctx context.Context, itemIDs []string) error {
	if len(itemIDs) == 0 {
		return nil
	}
	_, err := a.client.pool.Exec(ctx, `
		UPDATE catalog.inbox_items
		SET applied_at = NOW()
		WHERE id::text = ANY($1::text[])
	`, itemIDs)
	if err != nil {
		return fmt.Errorf("inbox bulk mark applied: %w", err)
	}
	return nil
}

func (a *InboxAdapter) DeleteByTenant(ctx context.Context, tenantID string) error {
	_, err := a.client.pool.Exec(ctx, `
		DELETE FROM catalog.inbox_items WHERE tenant_id = $1
	`, tenantID)
	if err != nil {
		return fmt.Errorf("inbox delete by tenant: %w", err)
	}
	return nil
}

func (a *InboxAdapter) GetByID(ctx context.Context, itemID string) (*domain.InboxItem, error) {
	row := a.client.pool.QueryRow(ctx, `
		SELECT id, tenant_id, source_kind, external_id, raw, payload_hash, fetched_at, applied_at
		FROM catalog.inbox_items
		WHERE id = $1
	`, itemID)
	return scanInboxItem(row)
}

func (a *InboxAdapter) ListUnapplied(ctx context.Context, tenantID string, limit, offset int) ([]*domain.InboxItem, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := a.client.pool.Query(ctx, `
		SELECT id, tenant_id, source_kind, external_id, raw, payload_hash, fetched_at, applied_at
		FROM catalog.inbox_items
		WHERE tenant_id = $1 AND applied_at IS NULL
		ORDER BY fetched_at ASC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("inbox list unapplied: %w", err)
	}
	defer rows.Close()

	var out []*domain.InboxItem
	for rows.Next() {
		it, err := scanInboxItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (a *InboxAdapter) CountTotal(ctx context.Context, tenantID string) (int, error) {
	var n int
	err := a.client.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM catalog.inbox_items WHERE tenant_id = $1
	`, tenantID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("inbox count total: %w", err)
	}
	return n, nil
}

// ----- discovery tools -----

func (a *InboxAdapter) ListFields(ctx context.Context, tenantID string, limit int) ([]string, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := a.client.pool.Query(ctx, `
		SELECT key
		FROM (
			SELECT DISTINCT jsonb_object_keys(raw) AS key
			FROM catalog.inbox_items
			WHERE tenant_id = $1
		) k
		ORDER BY key
		LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("inbox list fields: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (a *InboxAdapter) SampleValues(ctx context.Context, tenantID, field string, limit int) ([]string, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := a.client.pool.Query(ctx, `
		SELECT DISTINCT raw->>$2 AS v
		FROM catalog.inbox_items
		WHERE tenant_id = $1 AND raw ? $2 AND raw->>$2 IS NOT NULL
		LIMIT $3
	`, tenantID, field, limit)
	if err != nil {
		return nil, fmt.Errorf("inbox sample values: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (a *InboxAdapter) CountBy(ctx context.Context, tenantID, field string, maxBuckets int) (map[string]int, error) {
	if maxBuckets <= 0 || maxBuckets > 500 {
		maxBuckets = 100
	}
	rows, err := a.client.pool.Query(ctx, `
		SELECT raw->>$2 AS v, COUNT(*)::int AS n
		FROM catalog.inbox_items
		WHERE tenant_id = $1 AND raw ? $2
		GROUP BY 1
		ORDER BY n DESC
		LIMIT $3
	`, tenantID, field, maxBuckets)
	if err != nil {
		return nil, fmt.Errorf("inbox count by: %w", err)
	}
	defer rows.Close()

	out := make(map[string]int, maxBuckets)
	for rows.Next() {
		var v string
		var n int
		if err := rows.Scan(&v, &n); err != nil {
			return nil, err
		}
		out[v] = n
	}
	return out, rows.Err()
}

func (a *InboxAdapter) FieldStats(ctx context.Context, tenantID, field string, sampleN int) (*domain.InboxFieldStats, error) {
	if sampleN <= 0 || sampleN > 100 {
		sampleN = 20
	}
	// Single query collecting all stats. Numeric range uses a regex gate so
	// non-numeric strings don't trip ::numeric cast.
	var nonNull, distinctCount int
	var samples []string
	var minN, maxN *float64
	var minLen, maxLen *int

	err := a.client.pool.QueryRow(ctx, `
		WITH vals AS (
			SELECT raw->>$2 AS v
			FROM catalog.inbox_items
			WHERE tenant_id = $1 AND raw ? $2 AND raw->>$2 IS NOT NULL
		),
		sample AS (
			SELECT array_agg(v) AS s FROM (SELECT DISTINCT v FROM vals LIMIT $3) sub
		)
		SELECT
			(SELECT COUNT(*)        FROM vals)::int,
			(SELECT COUNT(DISTINCT v) FROM vals)::int,
			COALESCE((SELECT s FROM sample), ARRAY[]::text[]),
			(SELECT MIN(CASE WHEN v ~ '^-?[0-9]+(\.[0-9]+)?$' THEN v::numeric END) FROM vals),
			(SELECT MAX(CASE WHEN v ~ '^-?[0-9]+(\.[0-9]+)?$' THEN v::numeric END) FROM vals),
			(SELECT MIN(LENGTH(v)) FROM vals)::int,
			(SELECT MAX(LENGTH(v)) FROM vals)::int
	`, tenantID, field, sampleN).
		Scan(&nonNull, &distinctCount, &samples, &minN, &maxN, &minLen, &maxLen)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &domain.InboxFieldStats{Field: field}, nil
		}
		return nil, fmt.Errorf("inbox field stats: %w", err)
	}

	return &domain.InboxFieldStats{
		Field:         field,
		NonNullCount:  nonNull,
		DistinctCount: distinctCount,
		SampleValues:  samples,
		MinNumeric:    minN,
		MaxNumeric:    maxN,
		MinLength:     minLen,
		MaxLength:     maxLen,
	}, nil
}

func (a *InboxAdapter) PeekFullRows(ctx context.Context, tenantID string, limit int) ([]*domain.InboxItem, error) {
	if limit <= 0 || limit > 20 {
		// hard cap — peek is the expensive tool. Discovery_v2 usecase
		// enforces a per-run call cap on top of this.
		limit = 5
	}
	rows, err := a.client.pool.Query(ctx, `
		SELECT id, tenant_id, source_kind, external_id, raw, payload_hash, fetched_at, applied_at
		FROM catalog.inbox_items
		WHERE tenant_id = $1
		ORDER BY fetched_at DESC
		LIMIT $2
	`, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("inbox peek full rows: %w", err)
	}
	defer rows.Close()

	var out []*domain.InboxItem
	for rows.Next() {
		it, err := scanInboxItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// scanInboxItem accepts any *pgx.Row or pgx.Rows-like interface that has Scan.
// (rowScanner is declared in integrations_adapter.go and shared package-wide.)
func scanInboxItem(s rowScanner) (*domain.InboxItem, error) {
	var it domain.InboxItem
	var sourceKind string
	if err := s.Scan(&it.ID, &it.TenantID, &sourceKind, &it.ExternalID, &it.Raw, &it.PayloadHash, &it.FetchedAt, &it.AppliedAt); err != nil {
		return nil, err
	}
	it.SourceKind = domain.InboxSourceKind(sourceKind)
	return &it, nil
}
