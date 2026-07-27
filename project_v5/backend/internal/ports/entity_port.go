package ports

import (
	"context"

	"keepstar_v5/internal/domain"
)

// EntityPort is the SOLE write surface for public.v5_entity_* /
// v5_value_sets / v5_automations / v5_events (RUNTIME_SPEC.md §4.4, R10).
// Operation executors never call it directly — they go through
// usecases.EntityWrite (validate → tx record+outbox → post-commit
// dispatch). There is no ports.EventPort: events are written in the SAME tx
// by this port (v5_events outbox, R10). No delete API — lifecycle = status.
type EntityPort interface {
	// Definitions. UpsertEntityDefinition is additive-only: rejects type
	// change, field removal and slug rename once records exist.
	UpsertEntityDefinition(ctx context.Context, def *domain.EntityDefinition) error
	GetEntityDefinition(ctx context.Context, tenantID, slug string) (*domain.EntityDefinition, error)
	ListEntityDefinitions(ctx context.Context, tenantID string) ([]domain.EntityDefinition, error)

	// Value sets (R27: ordered {value,label,color?} entries).
	UpsertValueSet(ctx context.Context, vs *domain.ValueSet) error
	GetValueSet(ctx context.Context, tenantID, slug string) (*domain.ValueSet, error)
	ListValueSets(ctx context.Context, tenantID string) ([]domain.ValueSet, error)

	// Records. Each mutation runs in ONE tx: the record write + its
	// v5_events outbox row; the created event is returned for post-commit
	// inline dispatch (R12: no sweeper v1).
	CreateRecord(ctx context.Context, rec *domain.EntityRecord) (*domain.EntityRecord, *domain.RuntimeEvent, error)
	UpdateRecord(ctx context.Context, tenantID, recordID string, patch map[string]any) (*domain.EntityRecord, *domain.RuntimeEvent, error)
	TransitionStatus(ctx context.Context, tenantID, recordID, toStatus string) (*domain.EntityRecord, *domain.RuntimeEvent, error)
	GetRecord(ctx context.Context, tenantID, recordID string) (*domain.EntityRecord, error)
	// QueryRecords is flat (no aggregation/group-by/FTS/vector); returns the
	// page plus the total count for the filter.
	QueryRecords(ctx context.Context, tenantID, entitySlug string, f domain.RecordFilter) ([]domain.EntityRecord, int, error)

	// Automations (flat v1: on event [if predicate] run notify; no chains).
	UpsertAutomation(ctx context.Context, a *domain.Automation) error
	ListAutomations(ctx context.Context, tenantID, eventType string) ([]domain.Automation, error)

	// MarkEventProcessed stamps v5_events.processed_at after inline dispatch.
	MarkEventProcessed(ctx context.Context, eventID int64) error
}
