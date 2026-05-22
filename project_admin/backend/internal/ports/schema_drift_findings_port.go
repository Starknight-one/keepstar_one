package ports

import (
	"context"
	"encoding/json"

	"keepstar-admin/internal/domain"
)

// SchemaDriftFindingsPort is the persistence boundary for
// catalog.schema_drift_findings. Producers (the worker, after LLM
// classification) call Upsert; the admin UI handler calls List /
// MarkApplied / MarkDismissed.
type SchemaDriftFindingsPort interface {
	// Upsert writes a classification result. Idempotent on
	// (tenant_id, apply_run_id, field_name) so re-delivery from broker
	// produces the same row, just updated with the latest values.
	Upsert(ctx context.Context, in *DriftFindingUpsert) error

	// List returns findings for a tenant filtered by status. Empty
	// status returns all non-dismissed.
	List(ctx context.Context, tenantID string, status domain.SchemaDriftStatus, limit int) ([]*domain.SchemaDriftFinding, error)

	// Get returns one finding by ID.
	Get(ctx context.Context, id string) (*domain.SchemaDriftFinding, error)

	// MarkApplied flips status to 'applied'. Called after owner
	// triggers patch-discovery for this finding.
	MarkApplied(ctx context.Context, id string) error

	// MarkDismissed flips status to 'dismissed'. Owner rejects the
	// suggestion.
	MarkDismissed(ctx context.Context, id string) error
}

// DriftFindingUpsert is the write payload. SuggestedAction is opaque
// JSON; the adapter does not interpret it.
type DriftFindingUpsert struct {
	TenantID        string
	ApplyRunID      string
	FieldName       string
	TypeGuess       string
	Stats           json.RawMessage
	Decision        domain.SchemaDriftDecision
	Confidence      float64
	SuggestedAction json.RawMessage
}
