// Package adapters — pending approval read+write surface for curator UI.
// All queries operate on catalog.master_pending_changes and the new
// staging flags on catalog.master_products (approval_status,
// pending_approval_expires_at, approved_at, approved_by). See the
// 2026-05-20 migration in db/2026-05-20_pending_approval.sql.
//
// MVP scope:
//   - Read: tree of master_categories with pending counts, cards per
//     category, detail per master with its pending changes.
//   - Write: bulk approve/reject — flips status fields only. The
//     ACTUAL mutation of master_products / master_cosmetics columns on
//     approve is a followup; for v1 the flag itself is the source of
//     truth for "visible in production". That keeps the approve loop
//     atomic without per-field schema routing yet.
package adapters

import (
	"context"
	"fmt"
	"time"
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

// BulkApprovePendingChanges flips status from 'pending' to 'approved'.
// Actual mutation of master_products columns is followup (per-field
// schema routing); v1 surfaces this as a UI signal only.
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
