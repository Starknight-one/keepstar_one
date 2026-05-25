// Package adapters — pending approval read+write surface for curator UI.
// All queries operate on catalog.master_pending_changes and the new
// staging flags on catalog.master_products (approval_status,
// pending_approval_expires_at, approved_at, approved_by). See the
// 2026-05-20 migration in db/2026-05-20_pending_approval.sql.
//
// Scope:
//   - Read: tree of master_categories with pending counts, cards per
//     category, detail per master with its pending changes.
//   - Write: ApplyPendingChanges actually mutates target columns on
//     master_products / master_cosmetics / tier3 per field_name routing;
//     BulkApprove* / BulkReject* then flip the status flags. Approve
//     handler runs ApplyPendingChanges first inside one transaction.
package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// Column allowlists — these are the only fields a curator approve can write.
// Mirrors what apply_v2's buildEnrichScalars / buildEnrichArrays in the admin
// backend produces (the field_name on each master_pending_changes row comes
// from that path). Anything outside these maps is rejected as junk during
// apply so we never execute SQL with an unknown column name.
var (
	masterProductTextCols = map[string]bool{
		"name": true, "brand": true, "description": true, "image_url": true, "gtin": true,
	}
)

// PendingTreeNode is one node in the curator's master-category tree.
// Counts roll up the M:N master_product_categories join.
type PendingTreeNode struct {
	CategoryID         string `json:"category_id"`
	CategoryName       string `json:"category_name"`
	Slug               string `json:"slug"`
	ParentID           string `json:"parent_id,omitempty"`
	Vertical           string `json:"vertical"`
	PendingMasterCount int    `json:"pending_master_count"`
	PendingChangeCount int    `json:"pending_change_count"`
}

// PendingMasterCard is one row in the right-hand list for a category.
type PendingMasterCard struct {
	MasterID            string    `json:"master_id"`
	SKU                 string    `json:"sku"`
	Name                string    `json:"name"`
	Brand               string    `json:"brand,omitempty"`
	Vertical            string    `json:"vertical"`
	ApprovalStatus      string    `json:"approval_status"`       // 'pending_approval' | 'approved' | 'rejected' | 'expired'
	IsNewMaster         bool      `json:"is_new_master"`         // approval_status='pending_approval'
	PendingChangeCount  int       `json:"pending_change_count"`  // count of master_pending_changes WHERE status='pending'
	CreatedAt           time.Time `json:"created_at"`
	PendingExpiresAt    *time.Time `json:"pending_expires_at,omitempty"`
}

// PendingChangeRow is one row in the master detail drawer.
type PendingChangeRow struct {
	ID              string    `json:"id"`
	OpKind          string    `json:"op_kind"`            // 'enrich_scalar_fill' | 'enrich_array_union'
	FieldName       string    `json:"field_name"`
	PendingValue    string    `json:"pending_value"`      // jsonb-stringified for display
	Status          string    `json:"status"`             // 'pending' | 'approved' | 'rejected' | 'expired'
	SourceInboxItem string    `json:"source_inbox_item,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	ExpiresAt       time.Time `json:"expires_at"`
}

// ListPendingTree returns the master_categories tree filtered by
// vertical (empty = all), each annotated with two counts:
//   - pending_master_count: master_products with approval_status='pending_approval' linked via M:N
//   - pending_change_count: master_pending_changes with status='pending' against masters in this category
//
// Counts are cheap (covered by partial indexes from the 2026-05-20 migration).
func (c *Client) ListPendingTree(ctx context.Context, vertical string) ([]PendingTreeNode, error) {
	rows, err := c.Pool.Query(ctx, `
		SELECT
			mc.id::text,
			mc.name,
			mc.slug,
			COALESCE(mc.parent_id::text, ''),
			mc.vertical,
			COALESCE((
				SELECT COUNT(DISTINCT mp.id)::int
				FROM catalog.master_products mp
				JOIN catalog.master_product_categories mpc ON mpc.master_product_id = mp.id
				WHERE mpc.master_category_id = mc.id
				  AND mp.approval_status = 'pending_approval'
			), 0) AS pending_master_count,
			COALESCE((
				SELECT COUNT(*)::int
				FROM catalog.master_pending_changes pc
				JOIN catalog.master_product_categories mpc
				  ON mpc.master_product_id = pc.master_product_id
				WHERE mpc.master_category_id = mc.id
				  AND pc.status = 'pending'
			), 0) AS pending_change_count
		FROM catalog.master_categories mc
		WHERE ($1 = '' OR mc.vertical = $1)
		ORDER BY mc.vertical, COALESCE(mc.parent_id::text, ''), mc.name
	`, vertical)
	if err != nil {
		return nil, fmt.Errorf("list pending tree: %w", err)
	}
	defer rows.Close()
	var out []PendingTreeNode
	for rows.Next() {
		var n PendingTreeNode
		if err := rows.Scan(&n.CategoryID, &n.CategoryName, &n.Slug, &n.ParentID, &n.Vertical, &n.PendingMasterCount, &n.PendingChangeCount); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// ListPendingMastersInCategory returns cards for the right-hand pane.
// Includes masters in either of two states: approval_status='pending_approval'
// OR masters that have at least one pending change. Sorted newest first.
func (c *Client) ListPendingMastersInCategory(ctx context.Context, categoryID string, limit, offset int) ([]PendingMasterCard, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	// Total count first for pagination UX.
	var total int
	err := c.Pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT mp.id)
		FROM catalog.master_products mp
		JOIN catalog.master_product_categories mpc ON mpc.master_product_id = mp.id
		WHERE mpc.master_category_id = $1
		  AND (
		    mp.approval_status = 'pending_approval'
		    OR EXISTS (
		      SELECT 1 FROM catalog.master_pending_changes pc
		      WHERE pc.master_product_id = mp.id AND pc.status='pending'
		    )
		  )
	`, categoryID).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count pending masters: %w", err)
	}

	rows, err := c.Pool.Query(ctx, `
		SELECT
			mp.id::text,
			COALESCE(mp.sku, ''),
			mp.name,
			COALESCE(mp.brand, ''),
			COALESCE(mp.vertical, ''),
			mp.approval_status,
			mp.created_at,
			mp.pending_approval_expires_at,
			COALESCE((
				SELECT COUNT(*)::int FROM catalog.master_pending_changes pc
				WHERE pc.master_product_id = mp.id AND pc.status='pending'
			), 0) AS pending_change_count
		FROM catalog.master_products mp
		JOIN catalog.master_product_categories mpc ON mpc.master_product_id = mp.id
		WHERE mpc.master_category_id = $1
		  AND (
		    mp.approval_status = 'pending_approval'
		    OR EXISTS (
		      SELECT 1 FROM catalog.master_pending_changes pc
		      WHERE pc.master_product_id = mp.id AND pc.status='pending'
		    )
		  )
		ORDER BY mp.created_at DESC
		LIMIT $2 OFFSET $3
	`, categoryID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list pending masters: %w", err)
	}
	defer rows.Close()

	var out []PendingMasterCard
	for rows.Next() {
		var c PendingMasterCard
		if err := rows.Scan(&c.MasterID, &c.SKU, &c.Name, &c.Brand, &c.Vertical, &c.ApprovalStatus, &c.CreatedAt, &c.PendingExpiresAt, &c.PendingChangeCount); err != nil {
			return nil, 0, err
		}
		c.IsNewMaster = c.ApprovalStatus == "pending_approval"
		out = append(out, c)
	}
	return out, total, rows.Err()
}

// PendingMasterDetail is the response of GetPendingMaster.
type PendingMasterDetail struct {
	Card    PendingMasterCard  `json:"card"`
	Changes []PendingChangeRow `json:"changes"`
}

// GetPendingMaster returns the card + every pending change attached to it.
func (c *Client) GetPendingMaster(ctx context.Context, masterID string) (*PendingMasterDetail, error) {
	if masterID == "" {
		return nil, fmt.Errorf("get pending master: empty id")
	}
	var d PendingMasterDetail
	err := c.Pool.QueryRow(ctx, `
		SELECT
			mp.id::text,
			COALESCE(mp.sku, ''),
			mp.name,
			COALESCE(mp.brand, ''),
			COALESCE(mp.vertical, ''),
			mp.approval_status,
			mp.created_at,
			mp.pending_approval_expires_at,
			COALESCE((
				SELECT COUNT(*)::int FROM catalog.master_pending_changes pc
				WHERE pc.master_product_id = mp.id AND pc.status='pending'
			), 0)
		FROM catalog.master_products mp
		WHERE mp.id = $1
	`, masterID).Scan(&d.Card.MasterID, &d.Card.SKU, &d.Card.Name, &d.Card.Brand, &d.Card.Vertical, &d.Card.ApprovalStatus, &d.Card.CreatedAt, &d.Card.PendingExpiresAt, &d.Card.PendingChangeCount)
	if err != nil {
		return nil, fmt.Errorf("get pending master: %w", err)
	}
	d.Card.IsNewMaster = d.Card.ApprovalStatus == "pending_approval"

	rows, err := c.Pool.Query(ctx, `
		SELECT
			id::text,
			op_kind,
			field_name,
			pending_value::text,
			status,
			COALESCE(source_inbox_item_id::text, ''),
			created_at,
			expires_at
		FROM catalog.master_pending_changes
		WHERE master_product_id = $1
		ORDER BY created_at DESC
	`, masterID)
	if err != nil {
		return nil, fmt.Errorf("get pending master changes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var r PendingChangeRow
		if err := rows.Scan(&r.ID, &r.OpKind, &r.FieldName, &r.PendingValue, &r.Status, &r.SourceInboxItem, &r.CreatedAt, &r.ExpiresAt); err != nil {
			return nil, err
		}
		d.Changes = append(d.Changes, r)
	}
	return &d, rows.Err()
}

// ApproveResult is returned by Bulk*Approve / Bulk*Reject so the UI can
// reflect how many rows actually moved (idempotent: already-decided rows
// are skipped silently).
type ApproveResult struct {
	Updated int `json:"updated"`
}

// BulkApproveMasters flips approval_status from 'pending_approval' to
// 'approved' for the given master IDs. Idempotent — already-approved
// masters are not touched and not counted.
func (c *Client) BulkApproveMasters(ctx context.Context, masterIDs []string, decidedBy string) (ApproveResult, error) {
	if len(masterIDs) == 0 {
		return ApproveResult{}, nil
	}
	tag, err := c.Pool.Exec(ctx, `
		UPDATE catalog.master_products
		SET approval_status='approved',
		    approved_at=NOW(),
		    approved_by=$2,
		    pending_approval_expires_at=NULL
		WHERE id::text = ANY($1::text[])
		  AND approval_status='pending_approval'
	`, masterIDs, decidedBy)
	if err != nil {
		return ApproveResult{}, fmt.Errorf("bulk approve masters: %w", err)
	}
	return ApproveResult{Updated: int(tag.RowsAffected())}, nil
}

// BulkRejectMasters flips approval_status to 'rejected'. Idempotent.
func (c *Client) BulkRejectMasters(ctx context.Context, masterIDs []string, decidedBy, reason string) (ApproveResult, error) {
	if len(masterIDs) == 0 {
		return ApproveResult{}, nil
	}
	tag, err := c.Pool.Exec(ctx, `
		UPDATE catalog.master_products
		SET approval_status='rejected',
		    approved_at=NOW(),
		    approved_by=$2,
		    pending_approval_expires_at=NULL
		WHERE id::text = ANY($1::text[])
		  AND approval_status='pending_approval'
	`, masterIDs, decidedBy)
	if err != nil {
		return ApproveResult{}, fmt.Errorf("bulk reject masters: %w", err)
	}
	_ = reason // reserved: extend schema with rejected_reason on master_products in a future migration.
	return ApproveResult{Updated: int(tag.RowsAffected())}, nil
}

// ApplyPendingChanges mutates the target columns on master_products /
// master_cosmetics / tier3 for each pending change, then leaves the row in
// 'pending' for BulkApprovePendingChanges to flip. Caller wraps both in a
// transaction via the handler.
//
// Routing rules (field_name format mirrors apply_v2.buildEnrichScalars):
//
//   - "tier3.<key>"      → master_products.tier3 || jsonb_build_object(<key>, value)
//   - "tier2.<key>"      → master_products.tier2 (typed per-vertical attrs;
//                          scalar fill-when-empty or array set-union by op_kind)
//   - bare master col    → master_products column
//
// op_kind controls scalar-vs-array semantics:
//
//   - "enrich_scalar_fill"  — only fill when current value is NULL/empty
//   - "enrich_array_union"  — merge as set (preserves existing elements)
//
// Unknown field names are skipped with a count in result.Skipped (not an
// error — the curator can investigate later). Idempotent: already-approved
// changes are skipped.
type ApplyChangesResult struct {
	Applied int `json:"applied"`
	Skipped int `json:"skipped"`
}

func (c *Client) ApplyPendingChanges(ctx context.Context, changeIDs []string) (ApplyChangesResult, error) {
	if len(changeIDs) == 0 {
		return ApplyChangesResult{}, nil
	}

	tx, err := c.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ApplyChangesResult{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		SELECT id::text, master_product_id::text, op_kind, field_name, pending_value
		FROM catalog.master_pending_changes
		WHERE id::text = ANY($1::text[])
		  AND status = 'pending'
		FOR UPDATE
	`, changeIDs)
	if err != nil {
		return ApplyChangesResult{}, fmt.Errorf("load pending changes: %w", err)
	}
	type loaded struct {
		id, masterID, opKind, fieldName string
		raw                             []byte
	}
	var items []loaded
	for rows.Next() {
		var l loaded
		if err := rows.Scan(&l.id, &l.masterID, &l.opKind, &l.fieldName, &l.raw); err != nil {
			rows.Close()
			return ApplyChangesResult{}, fmt.Errorf("scan pending change: %w", err)
		}
		items = append(items, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return ApplyChangesResult{}, fmt.Errorf("iterate pending changes: %w", err)
	}

	var res ApplyChangesResult
	for _, it := range items {
		applied, aerr := applyOneChange(ctx, tx, it.masterID, it.opKind, it.fieldName, it.raw)
		if aerr != nil {
			return res, fmt.Errorf("apply change %s (field=%s): %w", it.id, it.fieldName, aerr)
		}
		if applied {
			res.Applied++
		} else {
			res.Skipped++
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return res, fmt.Errorf("commit apply: %w", err)
	}
	return res, nil
}

// applyOneChange routes one pending change to the right table+column.
// Returns applied=true when the change targeted a known column.
func applyOneChange(ctx context.Context, tx pgx.Tx, masterID, opKind, fieldName string, rawValue []byte) (bool, error) {
	// tier3.<key> — JSONB pocket on master_products
	if len(fieldName) > 6 && fieldName[:6] == "tier3." {
		key := fieldName[6:]
		if key == "" {
			return false, nil
		}
		_, err := tx.Exec(ctx, `
			UPDATE catalog.master_products
			SET tier3 = COALESCE(tier3, '{}'::jsonb) || jsonb_build_object($2::text, $3::jsonb),
			    updated_at = NOW()
			WHERE id::text = $1
		`, masterID, key, string(rawValue))
		if err != nil {
			return false, fmt.Errorf("tier3 update: %w", err)
		}
		return true, nil
	}

	// tier2.<key> — typed per-vertical attributes pocket on master_products
	// (Group D: replaced the master_cosmetics typed table). op_kind selects
	// scalar fill-when-empty vs array set-union, both into the tier2 jsonb.
	if len(fieldName) > 6 && fieldName[:6] == "tier2." {
		key := fieldName[6:]
		if key == "" {
			return false, nil
		}
		if opKind == "enrich_array_union" {
			// EnrichExistingMaster stages one row per element (pending_value is a
			// single JSON string); tolerate both a single string and an array.
			var arr []string
			if err := json.Unmarshal(rawValue, &arr); err != nil {
				var one string
				if err2 := json.Unmarshal(rawValue, &one); err2 != nil {
					return false, fmt.Errorf("decode tier2 array value: %w", err)
				}
				arr = []string{one}
			}
			if len(arr) == 0 {
				return true, nil
			}
			_, err := tx.Exec(ctx, `
				UPDATE catalog.master_products
				SET tier2 = jsonb_set(
						COALESCE(tier2, '{}'::jsonb),
						ARRAY[$2::text],
						(
							SELECT COALESCE(jsonb_agg(DISTINCT e), '[]'::jsonb)
							FROM (
								SELECT jsonb_array_elements_text(
									CASE WHEN jsonb_typeof(tier2->$2) = 'array'
									     THEN tier2->$2 ELSE '[]'::jsonb END) AS e
								UNION
								SELECT unnest($3::text[]) AS e
							) u
						),
						true),
				    updated_at = NOW()
				WHERE id::text = $1
			`, masterID, key, arr)
			if err != nil {
				return false, fmt.Errorf("tier2 array union (%s): %w", key, err)
			}
			return true, nil
		}
		// scalar fill-when-empty: only set when the key is absent or empty.
		_, err := tx.Exec(ctx, `
			UPDATE catalog.master_products
			SET tier2 = COALESCE(tier2, '{}'::jsonb) || jsonb_build_object($2::text, $3::jsonb),
			    updated_at = NOW()
			WHERE id::text = $1
			  AND (tier2 -> $2 IS NULL OR tier2 ->> $2 = '')
		`, masterID, key, string(rawValue))
		if err != nil {
			return false, fmt.Errorf("tier2 scalar fill (%s): %w", key, err)
		}
		return true, nil
	}

	// master_products text column
	if masterProductTextCols[fieldName] {
		var s string
		if err := json.Unmarshal(rawValue, &s); err != nil {
			return false, fmt.Errorf("decode text value: %w", err)
		}
		if s == "" {
			return true, nil
		}
		stmt := fmt.Sprintf(`
			UPDATE catalog.master_products
			SET %s = $2::text, updated_at = NOW()
			WHERE id::text = $1
			  AND (%s IS NULL OR %s = '')
		`, fieldName, fieldName, fieldName)
		if _, err := tx.Exec(ctx, stmt, masterID, s); err != nil {
			return false, fmt.Errorf("master text fill (%s): %w", fieldName, err)
		}
		return true, nil
	}

	// Unknown field — skip with a count.
	return false, nil
}

// BulkApprovePendingChanges flips status from 'pending' to 'approved'.
// Should be called AFTER ApplyPendingChanges in the same transaction so
// "approved" implies "actually written".
func (c *Client) BulkApprovePendingChanges(ctx context.Context, changeIDs []string, decidedBy string) (ApproveResult, error) {
	if len(changeIDs) == 0 {
		return ApproveResult{}, nil
	}
	tag, err := c.Pool.Exec(ctx, `
		UPDATE catalog.master_pending_changes
		SET status='approved',
		    decided_at=NOW(),
		    decided_by=$2
		WHERE id::text = ANY($1::text[])
		  AND status='pending'
	`, changeIDs, decidedBy)
	if err != nil {
		return ApproveResult{}, fmt.Errorf("bulk approve pending changes: %w", err)
	}
	return ApproveResult{Updated: int(tag.RowsAffected())}, nil
}

// BulkRejectPendingChanges flips status from 'pending' to 'rejected'.
func (c *Client) BulkRejectPendingChanges(ctx context.Context, changeIDs []string, decidedBy, reason string) (ApproveResult, error) {
	if len(changeIDs) == 0 {
		return ApproveResult{}, nil
	}
	tag, err := c.Pool.Exec(ctx, `
		UPDATE catalog.master_pending_changes
		SET status='rejected',
		    decided_at=NOW(),
		    decided_by=$2,
		    decided_reason=$3
		WHERE id::text = ANY($1::text[])
		  AND status='pending'
	`, changeIDs, decidedBy, reason)
	if err != nil {
		return ApproveResult{}, fmt.Errorf("bulk reject pending changes: %w", err)
	}
	return ApproveResult{Updated: int(tag.RowsAffected())}, nil
}
