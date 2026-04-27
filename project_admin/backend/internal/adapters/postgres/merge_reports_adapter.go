// Package postgres — merge_reports adapter (Phase D2, catalog completion 2026-04-28).
//
// Persists MergeReport rows. Per-proposal mutations rewrite the JSONB array
// rather than indexing in (cleaner atomicity for bulk-approve/reject UX).
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

type MergeReportsAdapter struct {
	client *Client
	log    *logger.Logger
}

func NewMergeReportsAdapter(client *Client, log *logger.Logger) *MergeReportsAdapter {
	return &MergeReportsAdapter{client: client, log: log}
}

var _ ports.MergeReportsPort = (*MergeReportsAdapter)(nil)

// Save inserts the new report and marks any prior pending/reviewed report
// for the same tenant as superseded. Done in one transaction so curators
// never see two "active" reports for one tenant simultaneously.
func (a *MergeReportsAdapter) Save(ctx context.Context, r *domain.MergeReport) (int64, error) {
	if r == nil || r.TenantID == "" {
		return 0, errors.New("merge_report: tenant_id required")
	}
	proposalsJSON, err := json.Marshal(r.Proposals)
	if err != nil {
		return 0, fmt.Errorf("marshal proposals: %w", err)
	}

	tx, err := a.client.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		UPDATE catalog.merge_reports
		SET status = 'superseded'
		WHERE tenant_id = $1 AND status IN ('pending', 'reviewed')`, r.TenantID); err != nil {
		return 0, fmt.Errorf("supersede prior: %w", err)
	}

	if r.Status == "" {
		r.Status = domain.MergeReportStatusPending
	}
	if r.SchemaVersion == 0 {
		r.SchemaVersion = 1
	}
	if r.GeneratedAt.IsZero() {
		r.GeneratedAt = time.Now().UTC()
	}

	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO catalog.merge_reports
			(tenant_id, generated_at, schema_version, status, agent_meta_summary,
			 total_listings, auto_link_count, new_master_count, needs_review_count, skip_count, already_linked_count,
			 proposals)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING id`,
		r.TenantID, r.GeneratedAt, r.SchemaVersion, r.Status, r.AgentMetaSummary,
		r.TotalListings, r.AutoLinkCount, r.NewMasterCount, r.NeedsReviewCount, r.SkipCount, r.AlreadyLinkedCount,
		proposalsJSON,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert merge_report: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	r.ID = id
	return id, nil
}

func (a *MergeReportsAdapter) GetByID(ctx context.Context, id int64) (*domain.MergeReport, error) {
	r, err := a.scanRow(ctx, `
		SELECT id, tenant_id, generated_at, schema_version, status, COALESCE(agent_meta_summary, ''),
			total_listings, auto_link_count, new_master_count, needs_review_count, skip_count, already_linked_count,
			proposals, applied_at, COALESCE(applied_by, '')
		FROM catalog.merge_reports WHERE id = $1`, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return r, err
}

func (a *MergeReportsAdapter) GetLatestForTenant(ctx context.Context, tenantID string) (*domain.MergeReport, error) {
	r, err := a.scanRow(ctx, `
		SELECT id, tenant_id, generated_at, schema_version, status, COALESCE(agent_meta_summary, ''),
			total_listings, auto_link_count, new_master_count, needs_review_count, skip_count, already_linked_count,
			proposals, applied_at, COALESCE(applied_by, '')
		FROM catalog.merge_reports
		WHERE tenant_id = $1
		ORDER BY generated_at DESC
		LIMIT 1`, tenantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return r, err
}

func (a *MergeReportsAdapter) ListForTenant(ctx context.Context, tenantID string, limit int) ([]domain.MergeReport, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := a.client.pool.Query(ctx, `
		SELECT id, tenant_id, generated_at, schema_version, status, COALESCE(agent_meta_summary, ''),
			total_listings, auto_link_count, new_master_count, needs_review_count, skip_count, already_linked_count,
			'[]'::jsonb, applied_at, COALESCE(applied_by, '')
		FROM catalog.merge_reports
		WHERE tenant_id = $1
		ORDER BY generated_at DESC
		LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, fmt.Errorf("list merge_reports: %w", err)
	}
	defer rows.Close()

	var out []domain.MergeReport
	for rows.Next() {
		var r domain.MergeReport
		var proposalsJSON []byte
		var appliedAt *time.Time
		if err := rows.Scan(&r.ID, &r.TenantID, &r.GeneratedAt, &r.SchemaVersion, &r.Status, &r.AgentMetaSummary,
			&r.TotalListings, &r.AutoLinkCount, &r.NewMasterCount, &r.NeedsReviewCount, &r.SkipCount, &r.AlreadyLinkedCount,
			&proposalsJSON, &appliedAt, &r.AppliedBy); err != nil {
			return nil, err
		}
		r.AppliedAt = appliedAt
		// Proposals intentionally empty in list view — caller fetches detail by id when needed.
		out = append(out, r)
	}
	return out, rows.Err()
}

func (a *MergeReportsAdapter) UpdateStatus(ctx context.Context, id int64, status domain.MergeReportStatus) error {
	_, err := a.client.pool.Exec(ctx,
		`UPDATE catalog.merge_reports SET status = $1 WHERE id = $2`, status, id)
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	return nil
}

func (a *MergeReportsAdapter) UpdateProposals(ctx context.Context, id int64, proposals []domain.MergeProposal, c ports.MergeReportCounters) error {
	js, err := json.Marshal(proposals)
	if err != nil {
		return fmt.Errorf("marshal proposals: %w", err)
	}
	_, err = a.client.pool.Exec(ctx, `
		UPDATE catalog.merge_reports
		SET proposals = $1,
			total_listings = $2,
			auto_link_count = $3,
			new_master_count = $4,
			needs_review_count = $5,
			skip_count = $6,
			already_linked_count = $7
		WHERE id = $8`,
		js, c.TotalListings, c.AutoLinkCount, c.NewMasterCount, c.NeedsReviewCount, c.SkipCount, c.AlreadyLinkedCount, id)
	if err != nil {
		return fmt.Errorf("update proposals: %w", err)
	}
	return nil
}

func (a *MergeReportsAdapter) MarkApplied(ctx context.Context, id int64, appliedBy string, finalStatus domain.MergeReportStatus) error {
	_, err := a.client.pool.Exec(ctx, `
		UPDATE catalog.merge_reports
		SET status = $1, applied_at = NOW(), applied_by = $2
		WHERE id = $3`, finalStatus, appliedBy, id)
	if err != nil {
		return fmt.Errorf("mark applied: %w", err)
	}
	return nil
}

func (a *MergeReportsAdapter) scanRow(ctx context.Context, sql string, args ...interface{}) (*domain.MergeReport, error) {
	var r domain.MergeReport
	var proposalsJSON []byte
	var appliedAt *time.Time
	err := a.client.pool.QueryRow(ctx, sql, args...).Scan(
		&r.ID, &r.TenantID, &r.GeneratedAt, &r.SchemaVersion, &r.Status, &r.AgentMetaSummary,
		&r.TotalListings, &r.AutoLinkCount, &r.NewMasterCount, &r.NeedsReviewCount, &r.SkipCount, &r.AlreadyLinkedCount,
		&proposalsJSON, &appliedAt, &r.AppliedBy,
	)
	if err != nil {
		return nil, err
	}
	r.AppliedAt = appliedAt
	if len(proposalsJSON) > 0 {
		if err := json.Unmarshal(proposalsJSON, &r.Proposals); err != nil {
			return nil, fmt.Errorf("unmarshal proposals: %w", err)
		}
	}
	return &r, nil
}
