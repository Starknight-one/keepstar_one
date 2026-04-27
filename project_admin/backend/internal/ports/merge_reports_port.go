package ports

import (
	"context"

	"keepstar-admin/internal/domain"
)

// MergeReportsPort persists merge applier output. Phase D2 (catalog completion
// 2026-04-28). Implementations: postgres/merge_reports_adapter.go.
type MergeReportsPort interface {
	// Save inserts a fresh report and supersedes the prior pending/reviewed
	// report for the same tenant (one active report at a time keeps the curator
	// UX simple). Returns the new report's ID.
	Save(ctx context.Context, r *domain.MergeReport) (int64, error)

	// GetByID returns one report including all proposals, or nil if missing.
	GetByID(ctx context.Context, id int64) (*domain.MergeReport, error)

	// GetLatestForTenant returns the most recent report for a tenant.
	// Used by curator UI to show "current state" without listing them all.
	GetLatestForTenant(ctx context.Context, tenantID string) (*domain.MergeReport, error)

	// ListForTenant returns reports descending by generated_at, capped at limit.
	ListForTenant(ctx context.Context, tenantID string, limit int) ([]domain.MergeReport, error)

	// UpdateStatus flips the report-level status (after curator action / apply).
	UpdateStatus(ctx context.Context, id int64, status domain.MergeReportStatus) error

	// UpdateProposals overwrites the entire proposals JSONB blob. Used after
	// curator approves/rejects per-row — we re-write the whole array rather
	// than indexing into JSONB so mutations are atomic and the array shape
	// stays a single source of truth.
	UpdateProposals(ctx context.Context, id int64, proposals []domain.MergeProposal, counters MergeReportCounters) error

	// MarkApplied stamps applied_at + applied_by + status. Called once after
	// a successful apply pass (which may have flipped individual proposals to
	// applied/failed via UpdateProposals).
	MarkApplied(ctx context.Context, id int64, appliedBy string, finalStatus domain.MergeReportStatus) error
}

// MergeReportCounters mirrors the counter columns on the table — passed
// alongside proposal updates so we keep the per-status totals in sync without
// a re-aggregate query.
type MergeReportCounters struct {
	TotalListings      int
	AutoLinkCount      int
	NewMasterCount     int
	NeedsReviewCount   int
	SkipCount          int
	AlreadyLinkedCount int
}
