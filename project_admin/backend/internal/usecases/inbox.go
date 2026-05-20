// Package usecases — InboxUseCase is the single entry point for writing
// tenant source data into catalog.inbox_items. Source ingestors (Shopify
// bulk harvester, CSV/Sheets importer, manual UI) call methods here
// instead of touching the adapter directly.
//
// Responsibilities:
//   - Compute payload_hash (SHA-256 of canonical JSON bytes).
//   - Compute external_id for sources that lack a natural ID (CSV/Sheets
//     without SKU → hash of significant fields).
//   - Dispatch to InboxPort.Upsert.
//   - Aggregate per-batch counts and write a single tenant_action_log row
//     summarising the pull (changed vs unchanged vs total).
package usecases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

type InboxUseCase struct {
	inbox ports.InboxPort
	log   ports.TenantActionLogPort
	llog  *logger.Logger
}

func NewInboxUseCase(inbox ports.InboxPort, actionLog ports.TenantActionLogPort, log *logger.Logger) *InboxUseCase {
	return &InboxUseCase{inbox: inbox, log: actionLog, llog: log}
}

// InboxWriteResult summarises one batch ingest.
type InboxWriteResult struct {
	Total     int `json:"total"`     // rows attempted
	Inserted  int `json:"inserted"`  // brand-new rows
	Updated   int `json:"updated"`   // existing rows with new payload (content changed)
	Unchanged int `json:"unchanged"` // existing rows, identical hash
	Errors    int `json:"errors"`
}

// ShopifyItem represents one product node coming out of a Shopify bulk
// operation. We don't model the entire Shopify schema here — the whole
// payload is stored verbatim as Raw.
type ShopifyItem struct {
	GID string          // shopify product GID — used as external_id
	Raw json.RawMessage // full bulk JSONL record
}

// WriteFromShopifyBulk ingests a slice of Shopify products. external_id is
// the Shopify GID (stable, globally unique inside Shopify). Rows with
// empty GID are counted as errors and skipped; everything else goes
// through one batched UNNEST upsert (see InboxPort.BulkUpsert).
func (uc *InboxUseCase) WriteFromShopifyBulk(ctx context.Context, tenantID string, items []ShopifyItem) (*InboxWriteResult, error) {
	res := &InboxWriteResult{Total: len(items)}
	rows := make([]*domain.InboxItem, 0, len(items))
	for _, it := range items {
		if it.GID == "" {
			res.Errors++
			continue
		}
		raw := it.Raw
		if len(raw) == 0 {
			raw = json.RawMessage("null")
		}
		rows = append(rows, &domain.InboxItem{
			TenantID:    tenantID,
			SourceKind:  domain.InboxSourceShopify,
			ExternalID:  it.GID,
			Raw:         raw,
			PayloadHash: hashContent(raw),
		})
	}
	changed, err := uc.inbox.BulkUpsert(ctx, rows)
	if err != nil {
		uc.llog.Warn("inbox_shopify_bulk_upsert_failed", "tenant", tenantID, "rows", len(rows), "error", err)
		res.Errors += len(rows)
		uc.recordPull(ctx, tenantID, "shopify", res)
		return res, err
	}
	res.Inserted = changed
	res.Unchanged = len(rows) - changed
	uc.recordPull(ctx, tenantID, "shopify", res)
	return res, nil
}

// CSVRow is one row from a CSV upload. Headers are the keys.
type CSVRow struct {
	NaturalID string            // SKU/handle if the CSV had one; empty otherwise
	Fields    map[string]string // column → value
}

// WriteFromCSV ingests CSV rows. When NaturalID is empty, external_id is a
// content hash of significant fields (best-effort dedup on re-upload).
func (uc *InboxUseCase) WriteFromCSV(ctx context.Context, tenantID string, rows []CSVRow) (*InboxWriteResult, error) {
	return uc.writeFromMapRows(ctx, tenantID, domain.InboxSourceCSV, rows)
}

// WriteFromSheets is functionally identical to WriteFromCSV — same shape.
func (uc *InboxUseCase) WriteFromSheets(ctx context.Context, tenantID string, rows []CSVRow) (*InboxWriteResult, error) {
	return uc.writeFromMapRows(ctx, tenantID, domain.InboxSourceSheets, rows)
}

// WriteManual is for the in-admin "add product manually" form. external_id
// must be supplied by the caller (UI generates a UUID for new items).
func (uc *InboxUseCase) WriteManual(ctx context.Context, tenantID, externalID string, fields map[string]any) (*InboxWriteResult, error) {
	raw, err := json.Marshal(fields)
	if err != nil {
		return nil, fmt.Errorf("manual marshal: %w", err)
	}
	changed, err := uc.upsert(ctx, tenantID, domain.InboxSourceManual, externalID, raw)
	res := &InboxWriteResult{Total: 1}
	switch {
	case err != nil:
		res.Errors = 1
		return res, err
	case changed:
		res.Inserted = 1 // treat new-or-changed as inserted for manual flow
	default:
		res.Unchanged = 1
	}
	uc.recordPull(ctx, tenantID, "manual", res)
	return res, nil
}

func (uc *InboxUseCase) writeFromMapRows(ctx context.Context, tenantID string, source domain.InboxSourceKind, rows []CSVRow) (*InboxWriteResult, error) {
	res := &InboxWriteResult{Total: len(rows)}
	items := make([]*domain.InboxItem, 0, len(rows))
	for _, r := range rows {
		raw, err := json.Marshal(r.Fields)
		if err != nil {
			res.Errors++
			continue
		}
		extID := r.NaturalID
		if extID == "" {
			extID = hashContent(raw)
		}
		items = append(items, &domain.InboxItem{
			TenantID:    tenantID,
			SourceKind:  source,
			ExternalID:  extID,
			Raw:         raw,
			PayloadHash: hashContent(raw),
		})
	}
	changed, err := uc.inbox.BulkUpsert(ctx, items)
	if err != nil {
		uc.llog.Warn("inbox_csv_bulk_upsert_failed", "tenant", tenantID, "rows", len(items), "error", err)
		res.Errors += len(items)
		uc.recordPull(ctx, tenantID, string(source), res)
		return res, err
	}
	res.Inserted = changed
	res.Unchanged = len(items) - changed
	uc.recordPull(ctx, tenantID, string(source), res)
	return res, nil
}

// upsert is the shared core: hashes the payload, calls the port.
func (uc *InboxUseCase) upsert(ctx context.Context, tenantID string, source domain.InboxSourceKind, externalID string, raw json.RawMessage) (bool, error) {
	if len(raw) == 0 {
		raw = json.RawMessage("null")
	}
	item := &domain.InboxItem{
		TenantID:    tenantID,
		SourceKind:  source,
		ExternalID:  externalID,
		Raw:         raw,
		PayloadHash: hashContent(raw),
	}
	return uc.inbox.Upsert(ctx, item)
}

// recordPull writes one tenant_action_log row per ingest batch. Best-effort:
// log failure does not abort the caller.
func (uc *InboxUseCase) recordPull(ctx context.Context, tenantID, source string, res *InboxWriteResult) {
	payload, _ := json.Marshal(map[string]any{
		"source":    source,
		"total":     res.Total,
		"inserted":  res.Inserted,
		"updated":   res.Updated,
		"unchanged": res.Unchanged,
		"errors":    res.Errors,
	})
	status := "ok"
	if res.Errors > 0 && res.Errors == res.Total {
		status = "error"
	} else if res.Errors > 0 {
		status = "warning"
	}
	if err := uc.log.Log(ctx, &ports.TenantActionLogEntry{
		TenantID: tenantID,
		Action:   "inbox_pull",
		Status:   status,
		Payload:  payload,
	}); err != nil {
		uc.llog.Warn("action_log_write_failed", "tenant", tenantID, "action", "inbox_pull", "error", err)
	}
}

// hashContent returns a hex-encoded SHA-256 of the input bytes. Used both
// for payload_hash (change detection) and as fallback external_id for
// sources without a natural identifier.
func hashContent(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
