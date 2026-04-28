package ports

import (
	"context"

	"keepstar-admin/internal/domain"
)

// AuditPort writes append-only audit log entries. Spec §3.7:
// system bulk = one entry with aggregate_meta; human edits = per-field detail.
type AuditPort interface {
	// LogSystem writes one row for a bulk operation (importer batch, webhook
	// burst). aggregate_meta typically contains {batch_id, count, source}.
	LogSystem(ctx context.Context, tenantID string, actorID string, entityKind domain.EntityKind, entityID string, action domain.AuditAction, aggregateMeta map[string]interface{}) error

	// LogHuman writes one row per human action with field-level diff. Empty
	// fieldChanges is allowed (e.g. for create/delete actions).
	LogHuman(ctx context.Context, tenantID string, actorID string, entityKind domain.EntityKind, entityID string, action domain.AuditAction, fieldChanges map[string]domain.FieldChange) error

	// LogCurator writes one row for a curator action (catalog merge apply / revert).
	// Same shape as LogHuman but stamps actor_kind='curator' so the audit log can
	// distinguish operator (curator) edits from tenant-user edits when the same
	// person plays both roles.
	LogCurator(ctx context.Context, tenantID string, curatorID string, entityKind domain.EntityKind, entityID string, action domain.AuditAction, fieldChanges map[string]domain.FieldChange, aggregateMeta map[string]interface{}) error

	// ListAuditEntries returns recent audit entries for an entity. Limit
	// defaults to 100. Pagination via offset.
	ListAuditEntries(ctx context.Context, entityKind domain.EntityKind, entityID string, limit, offset int) ([]domain.AuditEntry, error)
}
