package operations

import (
	"context"
	"fmt"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
	"keepstar_v5/internal/tools"
)

// UpdateRecordExecutor patches an existing record of the configured entity
// (RUNTIME_SPEC.md §4.2): {id, patch{...}} → last-write-wins through
// EntityWrite (R10), record.updated event in the same tx. The RECORD_UPDATE
// delta (Path "records") is caller-side.
//
// Config: {"entity":"lead","field_allowlist":[...]}
type UpdateRecordExecutor struct {
	writer EntityWriter
	tmpl   domain.OperationTemplate
}

var _ ports.Executor = (*UpdateRecordExecutor)(nil)

// NewUpdateRecordExecutor wires the executor over the entity write path.
func NewUpdateRecordExecutor(writer EntityWriter) *UpdateRecordExecutor {
	return &UpdateRecordExecutor{writer: writer, tmpl: seedRow("update_record")}
}

func (e *UpdateRecordExecutor) Template() domain.OperationSpec        { return e.tmpl.Spec() }
func (e *UpdateRecordExecutor) TemplateRow() domain.OperationTemplate { return e.tmpl }

// SpecForTenant types the patch sub-object from the entity definition —
// EntityRecordSchema properties (∩ field_allowlist) with nothing required:
// a patch touches any subset.
func (e *UpdateRecordExecutor) SpecForTenant(ctx context.Context, t domain.Tenant, cfg map[string]any) (domain.OperationSpec, error) {
	spec := e.tmpl.Spec()
	entity := cfgString(cfg, "entity")
	if entity == "" {
		return spec, nil
	}
	def, sets, err := e.writer.LoadDefinition(ctx, t.ID, entity)
	if err != nil {
		return spec, nil
	}
	recordSchema := restrictToAllowlist(tools.EntityRecordSchema(def, sets), cfgStringSlice(cfg, "field_allowlist"))
	if recordSchema == nil {
		return spec, nil
	}
	spec.InputSchema = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{
				"type":               "string",
				"description":        fmt.Sprintf("Id of the %s record to update.", entityDisplayName(def, false)),
				domain.SchemaKeyUnit: string(domain.UnitIDRef),
			},
			"patch": map[string]any{
				"type":        "object",
				"description": "Fields to change.",
				"properties":  recordSchema["properties"],
			},
		},
		"required": []string{"id", "patch"},
	}
	spec.Description = fmt.Sprintf("Update a %s record. %s", entityDisplayName(def, false), spec.Description)
	return spec, nil
}

// Execute applies the patch through EntityWrite. Input arrives
// validated+coerced by the registry (patch values unit-coerced at one
// nesting level); EntityWrite re-validates the patch against the
// definition as defense in depth.
func (e *UpdateRecordExecutor) Execute(ctx context.Context, octx domain.OperationContext, input map[string]any) (*domain.OperationResult, error) {
	id, _ := input["id"].(string)
	patch, _ := input["patch"].(map[string]any)
	if id == "" || len(patch) == 0 {
		return failure(e.tmpl.Name, e.tmpl.Kind, domain.OutcomeInvalid, "invalid: id and a non-empty patch are required"), nil
	}

	rec, err := e.writer.UpdateRecord(ctx, octx.TenantID, octx.TenantSlug, id, patch)
	if err != nil {
		return entityFailure(e.tmpl.Name, e.tmpl.Kind, err)
	}
	return &domain.OperationResult{
		Kind:       e.tmpl.Kind,
		Outcome:    domain.OutcomeOK,
		Count:      1,
		EntityKind: rec.EntitySlug,
		RecordID:   rec.ID,
		Summary:    fmt.Sprintf("ok: updated %s %s", rec.EntitySlug, rec.ID),
		Output:     map[string]any{"recordId": rec.ID},
		Metadata:   map[string]any{"entity": rec.EntitySlug, "status": rec.Status},
	}, nil
}
