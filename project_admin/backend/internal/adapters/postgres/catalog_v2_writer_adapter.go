// Package postgres — CatalogV2WriterAdapter implements ports.CatalogV2WriterPort.
// All upserts are idempotent on natural keys so re-running apply_v2 over the
// same inbox state converges.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

type CatalogV2WriterAdapter struct {
	client *Client
	log    *logger.Logger
}

func NewCatalogV2WriterAdapter(client *Client, log *logger.Logger) *CatalogV2WriterAdapter {
	return &CatalogV2WriterAdapter{client: client, log: log}
}

var _ ports.CatalogV2WriterPort = (*CatalogV2WriterAdapter)(nil)

// MatchOrCreateMaster runs the deterministic 3-stage cascade and either
// returns an existing master_id (wasCreated=false) without modifying the row,
// or INSERTs a new master_products row (wasCreated=true) when none of the
// stages hits.
//
// Stages:
//
//  1. SKU exact (case-insensitive on input; sku is stored mixed-case).
//  2. GTIN exact (skipped when mp.GTIN is empty).
//  3. normalized_match_key exact (skipped when empty; populated for every
//     existing row by the migration backfill).
//
// On INSERT we rely on the master_products_sku_key UNIQUE constraint to be
// race-safe — if a concurrent process inserted the same SKU between our
// SELECTs and our INSERT, ON CONFLICT (sku) DO NOTHING returns no rows and
// we SELECT the racing row by SKU. wasCreated is false in that branch.
//
// images: when mp.ImageURL is set, a one-element JSONB array is written on
// INSERT only. On bind we never touch the existing images.
func (a *CatalogV2WriterAdapter) MatchOrCreateMaster(ctx context.Context, mp *ports.MasterProductUpsert) (string, bool, error) {
	if mp == nil {
		return "", false, fmt.Errorf("match or create master: nil input")
	}
	if mp.SKU == "" || mp.Name == "" {
		return "", false, fmt.Errorf("match or create master: sku and name required (sku=%q name=%q)", mp.SKU, mp.Name)
	}

	// Stage 1: SKU. master_products.sku is mixed-case but the legacy data
	// has clean values, so a case-insensitive compare is enough to absorb
	// merchants who upload "SKU-123" vs "sku-123" inconsistently.
	if id, found, err := a.findMasterBySKU(ctx, mp.SKU); err != nil {
		return "", false, err
	} else if found {
		return id, false, nil
	}

	// Stage 2: GTIN. Skipped when empty.
	if mp.GTIN != "" {
		if id, found, err := a.findMasterByGTIN(ctx, mp.GTIN); err != nil {
			return "", false, err
		} else if found {
			return id, false, nil
		}
	}

	// Stage 3: normalized match key. Skipped when empty.
	if mp.NormalizedMatchKey != "" {
		if id, found, err := a.findMasterByMatchKey(ctx, mp.NormalizedMatchKey); err != nil {
			return "", false, err
		} else if found {
			return id, false, nil
		}
	}

	// No match — INSERT new master. Pass NULLIF for empty optional fields so
	// the column gets NULL instead of '' (cleaner for downstream SQL).
	imagesJSON := []byte("[]")
	if mp.ImageURL != "" {
		b, _ := json.Marshal([]string{mp.ImageURL})
		imagesJSON = b
	}

	// approval_status: caller opts the row into staging by setting
	// CreateAsPending. Existing tenants that pre-date this flag keep the
	// default 'approved' so legacy seed paths don't suddenly disappear from
	// production. expires_at is only populated on the pending branch.
	approvalStatus := "approved"
	if mp.CreateAsPending {
		approvalStatus = "pending_approval"
	}

	var id string
	err := a.client.pool.QueryRow(ctx, `
		INSERT INTO catalog.master_products
			(sku, name, brand, description, vertical, images, owner_tenant_id,
			 gtin, normalized_match_key,
			 approval_status, pending_approval_expires_at,
			 created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, NULLIF($8,''), NULLIF($9,''),
			 $10,
			 CASE WHEN $10 = 'pending_approval' THEN NOW() + INTERVAL '30 days' ELSE NULL END,
			 NOW(), NOW())
		ON CONFLICT (sku) DO NOTHING
		RETURNING id::text
	`,
		mp.SKU, mp.Name, mp.Brand, mp.Description, mp.Vertical,
		string(imagesJSON), nullableUUID(mp.OwnerTenantID),
		mp.GTIN, mp.NormalizedMatchKey,
		approvalStatus,
	).Scan(&id)

	if err == nil {
		return id, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", false, fmt.Errorf("match or create master insert: %w", err)
	}

	// ON CONFLICT (sku) DO NOTHING returned no rows — concurrent insert won
	// the race. Re-fetch by SKU and return that id; we did not create it.
	raceID, found, raceErr := a.findMasterBySKU(ctx, mp.SKU)
	if raceErr != nil {
		return "", false, fmt.Errorf("match or create master race-recover: %w", raceErr)
	}
	if !found {
		return "", false, fmt.Errorf("match or create master: insert returned no rows AND post-race SELECT empty (sku=%q)", mp.SKU)
	}
	return raceID, false, nil
}

// findMasterBySKU runs stage 1 of the cascade. Returns (id, true, nil) on hit,
// ("", false, nil) on miss, ("", false, err) on DB error.
func (a *CatalogV2WriterAdapter) findMasterBySKU(ctx context.Context, sku string) (string, bool, error) {
	var id string
	err := a.client.pool.QueryRow(ctx,
		`SELECT id::text FROM catalog.master_products WHERE LOWER(sku) = LOWER($1) LIMIT 1`,
		sku,
	).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return "", false, fmt.Errorf("find master by sku: %w", err)
}

func (a *CatalogV2WriterAdapter) findMasterByGTIN(ctx context.Context, gtin string) (string, bool, error) {
	var id string
	err := a.client.pool.QueryRow(ctx,
		`SELECT id::text FROM catalog.master_products WHERE gtin = $1 LIMIT 1`,
		gtin,
	).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return "", false, fmt.Errorf("find master by gtin: %w", err)
}

func (a *CatalogV2WriterAdapter) findMasterByMatchKey(ctx context.Context, key string) (string, bool, error) {
	var id string
	err := a.client.pool.QueryRow(ctx,
		`SELECT id::text FROM catalog.master_products WHERE normalized_match_key = $1 LIMIT 1`,
		key,
	).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return "", false, fmt.Errorf("find master by match key: %w", err)
}

// LookupVertical resolves a Shopify product_type into our internal vertical
// class. The alias table is lowercased on insert (see migration #94), and we
// lowercase the input here for case-insensitive lookup. Empty input returns
// (_, false, nil) so apply_v2 falls through to its rule-based classifier.
func (a *CatalogV2WriterAdapter) LookupVertical(ctx context.Context, productType string) (string, bool, error) {
	productType = strings.TrimSpace(productType)
	if productType == "" {
		return "", false, nil
	}
	var vertical string
	err := a.client.pool.QueryRow(ctx,
		`SELECT vertical FROM catalog.vertical_aliases WHERE alias = LOWER($1) LIMIT 1`,
		productType,
	).Scan(&vertical)
	if err == nil {
		return vertical, true, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return "", false, fmt.Errorf("lookup vertical: %w", err)
}

// SoftDeleteListing marks a catalog.products row deleted by setting
// deleted_at = NOW(). Idempotent: re-calling on an already-deleted row
// keeps the original deletion timestamp. Returns nil even when no row
// matches (delete-of-unknown — apply_v2 logs it, no error to surface).
func (a *CatalogV2WriterAdapter) SoftDeleteListing(ctx context.Context, tenantID, sourceSystem, sourceID string) error {
	if tenantID == "" || sourceSystem == "" || sourceID == "" {
		return fmt.Errorf("soft delete listing: tenant_id, source_system, source_id required")
	}
	_, err := a.client.pool.Exec(ctx, `
		UPDATE catalog.products
		SET deleted_at = COALESCE(deleted_at, NOW()),
		    updated_at = NOW()
		WHERE tenant_id = $1 AND source_system = $2 AND source_id = $3
	`, tenantID, sourceSystem, sourceID)
	if err != nil {
		return fmt.Errorf("soft delete listing exec: %w", err)
	}
	return nil
}

// enrichBulkBatchSize caps one INSERT-SELECT. Same number as inbox
// BulkUpsert — keeps batch behaviour predictable across the codebase.
const enrichBulkBatchSize = 500

// EnrichExistingMaster stages field-level changes against existing master
// rows. The caller (apply_v2) collects EnrichRequest entries on the bind
// path; this adapter unpacks each request into one row per scalar field
// and one row per proposed array element, then writes them all in a
// single UNNEST-driven INSERT per chunk.
//
// Dedup: a NOT EXISTS subquery on (master_product_id, field_name, op_kind,
// pending_value, status='pending') silently drops repeats so re-running
// apply over identical inbox rows is a no-op — no pending pile-up.
//
// Empty-target gating (the "fill only when empty scalars" / "set-union
// only new elements" rules) is enforced at APPROVE time, not write time:
// caller can stage everything safely and curator UI shows only the
// elements that would actually change the master on approval. This keeps
// the INSERT path one statement long and avoids per-row SELECTs.
func (a *CatalogV2WriterAdapter) EnrichExistingMaster(ctx context.Context, items []ports.EnrichRequest) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}

	// Flatten input into 6 parallel slices: one row per (master, field, value).
	masterIDs := make([]string, 0, len(items)*4)
	tenantIDs := make([]string, 0, len(items)*4)
	sourceItemIDs := make([]string, 0, len(items)*4)
	opKinds := make([]string, 0, len(items)*4)
	fieldNames := make([]string, 0, len(items)*4)
	pendingValues := make([]string, 0, len(items)*4)

	for _, req := range items {
		if req.MasterProductID == "" || req.TenantID == "" {
			continue
		}
		for field, val := range req.Scalars {
			if field == "" || isEmptyScalar(val) {
				continue
			}
			b, err := json.Marshal(val)
			if err != nil {
				continue
			}
			masterIDs = append(masterIDs, req.MasterProductID)
			tenantIDs = append(tenantIDs, req.TenantID)
			sourceItemIDs = append(sourceItemIDs, req.SourceInboxItemID)
			opKinds = append(opKinds, "enrich_scalar_fill")
			fieldNames = append(fieldNames, field)
			pendingValues = append(pendingValues, string(b))
		}
		for field, arr := range req.Arrays {
			if field == "" {
				continue
			}
			for _, elem := range arr {
				elem = strings.TrimSpace(elem)
				if elem == "" {
					continue
				}
				b, err := json.Marshal(elem)
				if err != nil {
					continue
				}
				masterIDs = append(masterIDs, req.MasterProductID)
				tenantIDs = append(tenantIDs, req.TenantID)
				sourceItemIDs = append(sourceItemIDs, req.SourceInboxItemID)
				opKinds = append(opKinds, "enrich_array_union")
				fieldNames = append(fieldNames, field)
				pendingValues = append(pendingValues, string(b))
			}
		}
	}

	if len(masterIDs) == 0 {
		return 0, nil
	}

	inserted := 0
	for start := 0; start < len(masterIDs); start += enrichBulkBatchSize {
		end := min(start+enrichBulkBatchSize, len(masterIDs))
		n, err := a.enrichInsertBatch(ctx,
			masterIDs[start:end], tenantIDs[start:end], sourceItemIDs[start:end],
			opKinds[start:end], fieldNames[start:end], pendingValues[start:end])
		if err != nil {
			return inserted, fmt.Errorf("enrich existing master batch [%d:%d]: %w", start, end, err)
		}
		inserted += n
	}
	return inserted, nil
}

func (a *CatalogV2WriterAdapter) enrichInsertBatch(
	ctx context.Context,
	masterIDs, tenantIDs, sourceItemIDs, opKinds, fieldNames, pendingValues []string,
) (int, error) {
	var inserted int
	err := a.client.pool.QueryRow(ctx, `
		WITH input AS (
			SELECT
				t.master_id::uuid                              AS master_id,
				t.tenant_id::uuid                              AS tenant_id,
				NULLIF(t.source_item_id, '')::uuid             AS source_item_id,
				t.op_kind                                       AS op_kind,
				t.field_name                                    AS field_name,
				t.pending_value::jsonb                          AS pending_value
			FROM unnest($1::text[], $2::text[], $3::text[], $4::text[], $5::text[], $6::text[])
				AS t(master_id, tenant_id, source_item_id, op_kind, field_name, pending_value)
		),
		ins AS (
			INSERT INTO catalog.master_pending_changes
				(master_product_id, tenant_id, source_inbox_item_id,
				 op_kind, field_name, pending_value)
			SELECT i.master_id, i.tenant_id, i.source_item_id,
			       i.op_kind, i.field_name, i.pending_value
			FROM input i
			WHERE NOT EXISTS (
				SELECT 1 FROM catalog.master_pending_changes p
				WHERE p.master_product_id = i.master_id
				  AND p.field_name        = i.field_name
				  AND p.op_kind           = i.op_kind
				  AND p.pending_value     = i.pending_value
				  AND p.status            = 'pending'
			)
			RETURNING 1
		)
		SELECT COUNT(*)::int FROM ins
	`, masterIDs, tenantIDs, sourceItemIDs, opKinds, fieldNames, pendingValues).Scan(&inserted)
	if err != nil {
		return 0, fmt.Errorf("enrich insert exec: %w", err)
	}
	return inserted, nil
}

// isEmptyScalar drops nil, "", and 0 from scalar fill proposals. We use a
// loose definition here: zero ints and empty strings are not interesting
// to stage. Callers that want to explicitly stage 0 should use Arrays or
// wrap in a pointer type that carries presence separately.
func isEmptyScalar(v any) bool {
	if v == nil {
		return true
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t) == ""
	case int:
		return t == 0
	case int64:
		return t == 0
	case float64:
		return t == 0
	}
	return false
}

// MergeTier3 merges (top-level overwrite) a JSON patch into
// master_products.tier3. Existing keys not in patch survive.
func (a *CatalogV2WriterAdapter) MergeTier3(ctx context.Context, masterID string, patch map[string]any) error {
	if masterID == "" {
		return fmt.Errorf("merge tier3: empty master_id")
	}
	if len(patch) == 0 {
		return nil
	}
	body, err := json.Marshal(patch)
	if err != nil {
		return fmt.Errorf("merge tier3 marshal: %w", err)
	}
	_, err = a.client.pool.Exec(ctx, `
		UPDATE catalog.master_products
		SET tier3 = COALESCE(tier3, '{}'::jsonb) || $2::jsonb,
		    updated_at = NOW()
		WHERE id = $1
	`, masterID, string(body))
	if err != nil {
		return fmt.Errorf("merge tier3 exec: %w", err)
	}
	return nil
}

// UpsertListing writes a slim catalog.products row. Idempotency key:
// (tenant_id, master_product_id) — one listing per tenant per master.
//
// price is in cents; currency defaults to USD when empty. Legacy columns
// (name, description, images) are populated from related fields for
// backwards compat with code paths still reading them; once those code
// paths are deleted (admin catalog UI, V5 chat fallback) the writes here
// can be trimmed.
func (a *CatalogV2WriterAdapter) UpsertListing(ctx context.Context, lst *ports.ListingUpsert) (string, error) {
	if lst == nil || lst.TenantID == "" || lst.MasterProductID == "" {
		return "", fmt.Errorf("upsert listing: tenant_id and master_product_id required")
	}
	currency := lst.Currency
	if currency == "" {
		currency = "USD"
	}
	var id string
	err := a.client.pool.QueryRow(ctx, `
		INSERT INTO catalog.products
			(tenant_id, master_product_id, name, price, currency, stock_quantity,
			 source_system, source_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		ON CONFLICT (tenant_id, master_product_id) DO UPDATE
		SET name           = COALESCE(NULLIF(EXCLUDED.name, ''), products.name),
		    price          = EXCLUDED.price,
		    currency       = COALESCE(NULLIF(EXCLUDED.currency, ''), products.currency),
		    stock_quantity = EXCLUDED.stock_quantity,
		    source_system  = COALESCE(NULLIF(EXCLUDED.source_system, ''), products.source_system),
		    source_id      = COALESCE(NULLIF(EXCLUDED.source_id, ''), products.source_id),
		    deleted_at     = NULL,
		    updated_at     = NOW()
		RETURNING id::text
	`,
		lst.TenantID, lst.MasterProductID, lst.CustomTitle,
		lst.Price, currency, lst.Stock,
		lst.SourceSystem, lst.SourceID,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("upsert listing exec: %w", err)
	}
	return id, nil
}

// listingBulkBatchSize caps one INSERT-SELECT for catalog.products. Matches
// inbox + enrich batch size so the bulk-write story is uniform across the
// admin postgres adapter.
const listingBulkBatchSize = 500

// masterBulkBatchSize caps one BulkMatchOrCreateMaster / MergeTier3
// transaction. Same as listing.
const masterBulkBatchSize = 500

// BulkMatchOrCreateMaster runs the same 3-stage cascade as per-row
// MatchOrCreateMaster but for a whole batch in one statement per stage:
//
//	Stage A. Bulk match: LEFT JOIN master_products on SKU (case-insensitive),
//	         GTIN (when non-empty), normalized_match_key (when non-empty).
//	         One row per input ord — existing_id set when ANY match hit.
//	Stage B. Bulk INSERT: items where existing_id IS NULL. ON CONFLICT (sku)
//	         DO NOTHING. RETURNING id, sku — maps back to ords by sku.
//	Stage C. Race recovery: items that neither matched in A nor RETURNED
//	         in B (concurrent insert won the sku race) — SELECT by sku.
//
// Total: 3 SQL roundtrips per batch (500 items). Compared to per-row
// 4 roundtrips × 500 items = 2000, that is ~700× faster on wall-clock.
//
// CreateAsPending: when item.CreateAsPending is true AND this call ends
// up inserting it (Stage B), approval_status='pending_approval' and
// pending_approval_expires_at=NOW()+30d are set; otherwise 'approved'.
func (a *CatalogV2WriterAdapter) BulkMatchOrCreateMaster(ctx context.Context, items []ports.MasterProductUpsert) ([]ports.MatchResult, error) {
	if len(items) == 0 {
		return nil, nil
	}
	results := make([]ports.MatchResult, len(items))
	for start := 0; start < len(items); start += masterBulkBatchSize {
		end := min(start+masterBulkBatchSize, len(items))
		if err := a.bulkMatchOrCreateBatch(ctx, items[start:end], results[start:end]); err != nil {
			return results, fmt.Errorf("bulk match-or-create batch [%d:%d]: %w", start, end, err)
		}
	}
	return results, nil
}

func (a *CatalogV2WriterAdapter) bulkMatchOrCreateBatch(ctx context.Context, batch []ports.MasterProductUpsert, out []ports.MatchResult) error {
	if len(batch) == 0 {
		return nil
	}
	// Parallel slices for UNNEST. ord is the row's position within the
	// batch (used to map results back to input order). Skip empties at
	// the very top — invalid input would fail at INSERT anyway, but the
	// caller (apply) validates before calling.
	skus := make([]string, len(batch))
	gtins := make([]string, len(batch))
	matchKeys := make([]string, len(batch))
	for i, item := range batch {
		skus[i] = item.SKU
		gtins[i] = item.GTIN
		matchKeys[i] = item.NormalizedMatchKey
	}

	// Stage A: bulk match. Returns one row per existing master that
	// matches any input's keys. We pick the lowest-ord input that matches
	// (deterministic) when multiple inputs hit the same existing master.
	existingByOrd := make(map[int]string, len(batch))
	rows, err := a.client.pool.Query(ctx, `
		WITH input AS (
			SELECT t.ord, t.sku, t.gtin, t.mkey
			FROM unnest($1::text[], $2::text[], $3::text[])
				WITH ORDINALITY AS t(sku, gtin, mkey, ord)
		),
		matches AS (
			SELECT i.ord, mp.id::text AS master_id
			FROM input i
			JOIN catalog.master_products mp ON
				LOWER(mp.sku) = LOWER(i.sku)
				OR (i.gtin <> '' AND mp.gtin = i.gtin)
				OR (i.mkey <> '' AND mp.normalized_match_key = i.mkey)
		)
		SELECT DISTINCT ON (ord) ord, master_id FROM matches ORDER BY ord
	`, skus, gtins, matchKeys)
	if err != nil {
		return fmt.Errorf("stage A bulk match: %w", err)
	}
	for rows.Next() {
		var ord int
		var mid string
		if err := rows.Scan(&ord, &mid); err != nil {
			rows.Close()
			return err
		}
		// ord is 1-based from WITH ORDINALITY; convert to 0-based index.
		existingByOrd[ord-1] = mid
	}
	rows.Close()

	// Mark matched results, collect items to INSERT.
	var insertItems []ports.MasterProductUpsert
	var insertOrds []int
	for i, item := range batch {
		if id, ok := existingByOrd[i]; ok {
			out[i] = ports.MatchResult{ID: id, WasCreated: false}
			continue
		}
		// Need to INSERT. Validate same as per-row path.
		if item.SKU == "" || item.Name == "" {
			return fmt.Errorf("stage B prep: row %d missing sku or name (sku=%q name=%q)", i, item.SKU, item.Name)
		}
		insertItems = append(insertItems, item)
		insertOrds = append(insertOrds, i)
	}

	if len(insertItems) == 0 {
		return nil
	}

	// Stage B: bulk INSERT. ON CONFLICT (sku) DO NOTHING to absorb
	// concurrent-insert races; we recover survivors in stage C.
	skuCol := make([]string, len(insertItems))
	nameCol := make([]string, len(insertItems))
	brandCol := make([]string, len(insertItems))
	descCol := make([]string, len(insertItems))
	verticalCol := make([]string, len(insertItems))
	imagesCol := make([]string, len(insertItems))
	ownerCol := make([]string, len(insertItems))
	gtinCol := make([]string, len(insertItems))
	mkeyCol := make([]string, len(insertItems))
	approvalCol := make([]string, len(insertItems))
	for i, it := range insertItems {
		skuCol[i] = it.SKU
		nameCol[i] = it.Name
		brandCol[i] = it.Brand
		descCol[i] = it.Description
		verticalCol[i] = it.Vertical
		imgs := "[]"
		if it.ImageURL != "" {
			b, _ := json.Marshal([]string{it.ImageURL})
			imgs = string(b)
		}
		imagesCol[i] = imgs
		ownerCol[i] = it.OwnerTenantID
		gtinCol[i] = it.GTIN
		mkeyCol[i] = it.NormalizedMatchKey
		if it.CreateAsPending {
			approvalCol[i] = "pending_approval"
		} else {
			approvalCol[i] = "approved"
		}
	}

	insertedBySKU := make(map[string]string, len(insertItems))
	insRows, err := a.client.pool.Query(ctx, `
		WITH input AS (
			SELECT
				t.sku, t.name, t.brand, t.description, t.vertical,
				t.images, NULLIF(t.owner,'') AS owner, NULLIF(t.gtin,'') AS gtin,
				NULLIF(t.mkey,'') AS mkey, t.approval
			FROM unnest(
				$1::text[], $2::text[], $3::text[], $4::text[],
				$5::text[], $6::text[], $7::text[], $8::text[],
				$9::text[], $10::text[]
			) AS t(sku, name, brand, description, vertical, images, owner, gtin, mkey, approval)
		)
		INSERT INTO catalog.master_products
			(sku, name, brand, description, vertical, images, owner_tenant_id,
			 gtin, normalized_match_key,
			 approval_status, pending_approval_expires_at,
			 created_at, updated_at)
		SELECT
			i.sku, i.name, i.brand, i.description, i.vertical,
			i.images::jsonb, i.owner::uuid, i.gtin, i.mkey,
			i.approval,
			CASE WHEN i.approval = 'pending_approval'
				THEN NOW() + INTERVAL '30 days' ELSE NULL END,
			NOW(), NOW()
		FROM input i
		ON CONFLICT (sku) DO NOTHING
		RETURNING id::text, sku
	`, skuCol, nameCol, brandCol, descCol, verticalCol, imagesCol, ownerCol, gtinCol, mkeyCol, approvalCol)
	if err != nil {
		return fmt.Errorf("stage B bulk insert: %w", err)
	}
	for insRows.Next() {
		var id, sku string
		if err := insRows.Scan(&id, &sku); err != nil {
			insRows.Close()
			return err
		}
		insertedBySKU[sku] = id
	}
	insRows.Close()

	// Stage C: race recovery. Anyone in insertItems whose SKU is NOT in
	// insertedBySKU lost the ON CONFLICT race — re-SELECT by SKU.
	var raceSKUs []string
	var raceOrds []int
	for i, it := range insertItems {
		if _, ok := insertedBySKU[it.SKU]; ok {
			out[insertOrds[i]] = ports.MatchResult{ID: insertedBySKU[it.SKU], WasCreated: true}
			continue
		}
		raceSKUs = append(raceSKUs, it.SKU)
		raceOrds = append(raceOrds, insertOrds[i])
	}

	if len(raceSKUs) > 0 {
		raceRows, err := a.client.pool.Query(ctx, `
			SELECT id::text, sku FROM catalog.master_products
			WHERE LOWER(sku) = ANY(SELECT LOWER(s) FROM unnest($1::text[]) AS s)
		`, raceSKUs)
		if err != nil {
			return fmt.Errorf("stage C race recovery: %w", err)
		}
		recoveredBySKU := make(map[string]string, len(raceSKUs))
		for raceRows.Next() {
			var id, sku string
			if err := raceRows.Scan(&id, &sku); err != nil {
				raceRows.Close()
				return err
			}
			recoveredBySKU[strings.ToLower(sku)] = id
		}
		raceRows.Close()
		for i, sku := range raceSKUs {
			if id, ok := recoveredBySKU[strings.ToLower(sku)]; ok {
				out[raceOrds[i]] = ports.MatchResult{ID: id, WasCreated: false}
			} else {
				return fmt.Errorf("stage C: race recovery for sku=%q returned no row", sku)
			}
		}
	}

	return nil
}

// BulkMergeTier3 merges JSON patches into master_products.tier3 for many
// rows in one UPDATE per batch. Same `tier3 = COALESCE(tier3,'{}') || patch`
// semantics as the per-row MergeTier3.
func (a *CatalogV2WriterAdapter) BulkMergeTier3(ctx context.Context, items []ports.BulkTier3Item) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}
	written := 0
	for start := 0; start < len(items); start += masterBulkBatchSize {
		end := min(start+masterBulkBatchSize, len(items))
		n, err := a.bulkMergeTier3Batch(ctx, items[start:end])
		if err != nil {
			return written, fmt.Errorf("bulk merge tier3 batch [%d:%d]: %w", start, end, err)
		}
		written += n
	}
	return written, nil
}

func (a *CatalogV2WriterAdapter) bulkMergeTier3Batch(ctx context.Context, batch []ports.BulkTier3Item) (int, error) {
	masterIDs := make([]string, 0, len(batch))
	patches := make([]string, 0, len(batch))
	for _, it := range batch {
		if it.MasterProductID == "" || len(it.Patch) == 0 {
			continue
		}
		b, err := json.Marshal(it.Patch)
		if err != nil {
			continue
		}
		masterIDs = append(masterIDs, it.MasterProductID)
		patches = append(patches, string(b))
	}
	if len(masterIDs) == 0 {
		return 0, nil
	}
	tag, err := a.client.pool.Exec(ctx, `
		WITH input AS (
			SELECT t.mid::uuid AS mid, t.patch::jsonb AS patch
			FROM unnest($1::text[], $2::text[]) AS t(mid, patch)
		)
		UPDATE catalog.master_products mp
		SET tier3 = COALESCE(mp.tier3, '{}'::jsonb) || i.patch,
		    updated_at = NOW()
		FROM input i
		WHERE mp.id = i.mid
	`, masterIDs, patches)
	if err != nil {
		return 0, fmt.Errorf("bulk merge tier3 exec: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// BulkMergeTier2 merges JSON patches into master_products.tier2 for many rows
// in one UPDATE per batch. tier2 is the canonical home for typed per-vertical
// attributes (Group D: replaced the master_cosmetics typed table). Same
// `tier2 = COALESCE(tier2,'{}') || patch` semantics as BulkMergeTier3.
func (a *CatalogV2WriterAdapter) BulkMergeTier2(ctx context.Context, items []ports.BulkTier2Item) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}
	written := 0
	for start := 0; start < len(items); start += masterBulkBatchSize {
		end := min(start+masterBulkBatchSize, len(items))
		n, err := a.bulkMergeTier2Batch(ctx, items[start:end])
		if err != nil {
			return written, fmt.Errorf("bulk merge tier2 batch [%d:%d]: %w", start, end, err)
		}
		written += n
	}
	return written, nil
}

func (a *CatalogV2WriterAdapter) bulkMergeTier2Batch(ctx context.Context, batch []ports.BulkTier2Item) (int, error) {
	masterIDs := make([]string, 0, len(batch))
	patches := make([]string, 0, len(batch))
	for _, it := range batch {
		if it.MasterProductID == "" || len(it.Patch) == 0 {
			continue
		}
		b, err := json.Marshal(it.Patch)
		if err != nil {
			continue
		}
		masterIDs = append(masterIDs, it.MasterProductID)
		patches = append(patches, string(b))
	}
	if len(masterIDs) == 0 {
		return 0, nil
	}
	tag, err := a.client.pool.Exec(ctx, `
		WITH input AS (
			SELECT t.mid::uuid AS mid, t.patch::jsonb AS patch
			FROM unnest($1::text[], $2::text[]) AS t(mid, patch)
		)
		UPDATE catalog.master_products mp
		SET tier2 = COALESCE(mp.tier2, '{}'::jsonb) || i.patch,
		    updated_at = NOW()
		FROM input i
		WHERE mp.id = i.mid
	`, masterIDs, patches)
	if err != nil {
		return 0, fmt.Errorf("bulk merge tier2 exec: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// BulkUpsertListing batches the same INSERT … ON CONFLICT semantics as
// UpsertListing but issues one statement per ~500 rows via UNNEST. Caller
// passes the raw slice; the adapter chunks. Returns the total RETURNING
// row count — every row in catalog.products that was inserted or updated
// by the call.
func (a *CatalogV2WriterAdapter) BulkUpsertListing(ctx context.Context, items []ports.ListingUpsert) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}
	written := 0
	for start := 0; start < len(items); start += listingBulkBatchSize {
		end := min(start+listingBulkBatchSize, len(items))
		n, err := a.bulkUpsertListingBatch(ctx, items[start:end])
		if err != nil {
			return written, fmt.Errorf("bulk upsert listing batch [%d:%d]: %w", start, end, err)
		}
		written += n
	}
	return written, nil
}

func (a *CatalogV2WriterAdapter) bulkUpsertListingBatch(ctx context.Context, batch []ports.ListingUpsert) (int, error) {
	tenantIDs := make([]string, 0, len(batch))
	masterIDs := make([]string, 0, len(batch))
	titles := make([]string, 0, len(batch))
	prices := make([]int, 0, len(batch))
	currencies := make([]string, 0, len(batch))
	stocks := make([]int, 0, len(batch))
	sourceSystems := make([]string, 0, len(batch))
	sourceIDs := make([]string, 0, len(batch))
	for _, lst := range batch {
		if lst.TenantID == "" || lst.MasterProductID == "" {
			continue
		}
		currency := lst.Currency
		if currency == "" {
			currency = "USD"
		}
		tenantIDs = append(tenantIDs, lst.TenantID)
		masterIDs = append(masterIDs, lst.MasterProductID)
		titles = append(titles, lst.CustomTitle)
		prices = append(prices, lst.Price)
		currencies = append(currencies, currency)
		stocks = append(stocks, lst.Stock)
		sourceSystems = append(sourceSystems, lst.SourceSystem)
		sourceIDs = append(sourceIDs, lst.SourceID)
	}
	if len(tenantIDs) == 0 {
		return 0, nil
	}
	var written int
	err := a.client.pool.QueryRow(ctx, `
		WITH input AS (
			SELECT
				t.tenant_id::uuid          AS tenant_id,
				t.master_id::uuid          AS master_id,
				t.title                     AS title,
				t.price                     AS price,
				t.currency                  AS currency,
				t.stock                     AS stock,
				t.source_system             AS source_system,
				t.source_id                 AS source_id
			FROM unnest($1::text[], $2::text[], $3::text[], $4::int[], $5::text[], $6::int[], $7::text[], $8::text[])
				AS t(tenant_id, master_id, title, price, currency, stock, source_system, source_id)
		),
		ins AS (
			INSERT INTO catalog.products
				(tenant_id, master_product_id, name, price, currency, stock_quantity,
				 source_system, source_id, created_at, updated_at)
			SELECT tenant_id, master_id, title, price, currency, stock,
			       source_system, source_id, NOW(), NOW()
			FROM input
			ON CONFLICT (tenant_id, master_product_id) DO UPDATE
			SET name           = COALESCE(NULLIF(EXCLUDED.name, ''), products.name),
			    price          = EXCLUDED.price,
			    currency       = COALESCE(NULLIF(EXCLUDED.currency, ''), products.currency),
			    stock_quantity = EXCLUDED.stock_quantity,
			    source_system  = COALESCE(NULLIF(EXCLUDED.source_system, ''), products.source_system),
			    source_id      = COALESCE(NULLIF(EXCLUDED.source_id, ''), products.source_id),
			    deleted_at     = NULL,
			    updated_at     = NOW()
			RETURNING 1
		)
		SELECT COUNT(*)::int FROM ins
	`, tenantIDs, masterIDs, titles, prices, currencies, stocks, sourceSystems, sourceIDs).Scan(&written)
	if err != nil {
		return 0, fmt.Errorf("bulk upsert listing exec: %w", err)
	}
	return written, nil
}

// ----- internal helpers -----

func nullableUUID(s string) any {
	if s == "" {
		return nil
	}
	return s
}
