package operations

import (
	"context"
	"fmt"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
	"keepstar_v5/internal/tools"
)

// CreateRecordExecutor creates one record of the configured entity
// (RUNTIME_SPEC.md §4.2). All writes go through EntityWrite (R10):
// validate → tx(record + v5_events) → inline automation dispatch. The
// session delta (RECORD_CREATE, Path "records") is written by the caller —
// the invoke handler on the widget path (handler_operations.go).
//
// Config: {"entity":"lead","defaults":{"status":"new"},"field_allowlist":[...]}
type CreateRecordExecutor struct {
	writer EntityWriter
	tmpl   domain.OperationTemplate
}

var _ ports.Executor = (*CreateRecordExecutor)(nil)

// NewCreateRecordExecutor wires the executor over the entity write path.
func NewCreateRecordExecutor(writer EntityWriter) *CreateRecordExecutor {
	return &CreateRecordExecutor{writer: writer, tmpl: seedRow("create_record")}
}

func (e *CreateRecordExecutor) Template() domain.OperationSpec        { return e.tmpl.Spec() }
func (e *CreateRecordExecutor) TemplateRow() domain.OperationTemplate { return e.tmpl }

// SpecForTenant derives the instance schema: EntityRecordSchema(definition,
// value sets) restricted to the configured field_allowlist. No config or a
// failed derivation keeps the static template (fail-open).
func (e *CreateRecordExecutor) SpecForTenant(ctx context.Context, t domain.Tenant, cfg map[string]any) (domain.OperationSpec, error) {
	spec := e.tmpl.Spec()
	entity := cfgString(cfg, "entity")
	if entity == "" {
		return spec, nil
	}
	def, sets, err := e.writer.LoadDefinition(ctx, t.ID, entity)
	if err != nil {
		return spec, nil
	}
	if schema := tools.EntityRecordSchema(def, sets); schema != nil {
		spec.InputSchema = restrictToAllowlist(schema, cfgStringSlice(cfg, "field_allowlist"))
		spec.Description = fmt.Sprintf("Create a %s record. %s", entityDisplayName(def, false), spec.Description)
	}
	return spec, nil
}

// Execute applies configured defaults and writes through EntityWrite; the
// catalog link derives from the definition's ref field inside the write
// path. Input arrives validated+coerced by the registry.
func (e *CreateRecordExecutor) Execute(ctx context.Context, octx domain.OperationContext, input map[string]any) (*domain.OperationResult, error) {
	entity := cfgString(octx.Config, "entity")
	if entity == "" {
		return failure(e.tmpl.Name, e.tmpl.Kind, domain.OutcomeError, "error: create_record instance config has no entity"), nil
	}
	data := applyDefaults(input, cfgMap(octx.Config, "defaults"))

	rec, err := e.writer.CreateRecord(ctx, octx.TenantID, octx.TenantSlug, entity, data, nil, actorOrSession(octx))
	if err != nil {
		return entityFailure(e.tmpl.Name, e.tmpl.Kind, err)
	}
	return &domain.OperationResult{
		Kind:       e.tmpl.Kind,
		Outcome:    domain.OutcomeOK,
		Count:      1,
		EntityKind: rec.EntitySlug,
		RecordID:   rec.ID,
		Summary:    fmt.Sprintf("ok: created %s %s", rec.EntitySlug, rec.ID),
		Output:     map[string]any{"recordId": rec.ID},
		Metadata:   map[string]any{"entity": rec.EntitySlug, "status": rec.Status},
	}, nil
}
