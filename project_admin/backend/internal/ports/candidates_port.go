package ports

import (
	"context"

	"keepstar-admin/internal/domain"
)

// CandidatesPort handles staging tables for attributes/categories awaiting
// curator promotion, and the junk-variant detection queue.
type CandidatesPort interface {
	// --- Attribute candidates ---

	// UpsertAttributeCandidate inserts a new candidate or increments
	// seen_in_tenants on an existing one (UNIQUE on key+vertical).
	UpsertAttributeCandidate(ctx context.Context, key, vertical, sampleValue, agentMeta string) error

	// ListAttributeCandidates returns candidates filtered by status/vertical.
	// Curator UI shows pending + seen_in_tenants >= 2.
	ListAttributeCandidates(ctx context.Context, vertical string, status domain.CandidateStatus, minSeen int) ([]domain.AttributeCandidate, error)

	// MarkAttributeCandidatePromoted updates status + promoted_to_column atomically.
	// Called inside the same transaction as the ALTER TABLE.
	MarkAttributeCandidatePromoted(ctx context.Context, id, promotedToColumn string) error

	// --- Category candidates ---

	// UpsertCategoryCandidate inserts new or increments counter.
	UpsertCategoryCandidate(ctx context.Context, name, proposedParent, vertical string) error

	// ListCategoryCandidates with same filters as attribute candidates.
	ListCategoryCandidates(ctx context.Context, vertical string, status domain.CandidateStatus, minSeen int) ([]domain.CategoryCandidate, error)

	MarkCategoryCandidatePromoted(ctx context.Context, id, promotedToID string) error

	// --- Junk candidates ---

	// InsertJunkCandidate is called by the harvester when junk heuristic
	// signals >= 2. Idempotent on (tenant_id, listing_id).
	InsertJunkCandidate(ctx context.Context, jc *domain.JunkCandidate) error

	// ListJunkCandidates for a tenant by classification status.
	ListJunkCandidates(ctx context.Context, tenantID string, status domain.JunkClassification) ([]domain.JunkCandidate, error)

	// ClassifyJunkCandidate updates classification + classified_at/by atomically.
	ClassifyJunkCandidate(ctx context.Context, id string, classification domain.JunkClassification, classifiedBy string) error

	// CountJunkPending tells the admin sidebar whether to render the link.
	CountJunkPending(ctx context.Context, tenantID string) (int, error)
}
