package operations

import (
	"context"
	"fmt"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// TransitionStatusExecutor moves a record through its status pipeline
// (RUNTIME_SPEC.md §4.2): to_status must belong to the configured value
// set (enforced in the entity plane) and to the optional transition map
// (enforced in EntityWrite) — anything else is OutcomeInvalid so the LLM
// self-corrects. Emits record.status_changed → automations. Demo instance:
// advance_lead. The RECORD_TRANSITION delta (Path "records") is
// caller-side.
//
// Config: {"entity":"lead", "field":"status", "value_set":"lead_pipeline",
// "transitions":{from:[to,...]}?}
type TransitionStatusExecutor struct {
	writer EntityWriter
	tmpl   domain.OperationTemplate
}

var _ ports.Executor = (*TransitionStatusExecutor)(nil)

// NewTransitionStatusExecutor wires the executor over the entity write path.
func NewTransitionStatusExecutor(writer EntityWriter) *TransitionStatusExecutor {
	return &TransitionStatusExecutor{writer: writer, tmpl: seedRow("transition_status")}
}

func (e *TransitionStatusExecutor) Template() domain.OperationSpec        { return e.tmpl.Spec() }
func (e *TransitionStatusExecutor) TemplateRow() domain.OperationTemplate { return e.tmpl }

// SpecForTenant enum-types to_status from the configured value set so the
// LLM sees the real pipeline vocabulary.
func (e *TransitionStatusExecutor) SpecForTenant(ctx context.Context, t domain.Tenant, cfg map[string]any) (domain.OperationSpec, error) {
	spec := e.tmpl.Spec()
	entity := cfgString(cfg, "entity")
	setSlug := cfgString(cfg, "value_set")
	if entity == "" || setSlug == "" {
		return spec, nil
	}
	def, sets, err := e.writer.LoadDefinition(ctx, t.ID, entity)
	if err != nil {
		return spec, nil
	}
	vs, ok := sets[setSlug]
	if !ok {
		return spec, nil
	}
	values := make([]string, len(vs.Values))
	for i, entry := range vs.Values {
		values[i] = entry.Value
	}
	spec.InputSchema = map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{
				"type":               "string",
				"description":        fmt.Sprintf("Id of the %s record to transition.", entityDisplayName(def, false)),
				domain.SchemaKeyUnit: string(domain.UnitIDRef),
			},
			"to_status": map[string]any{
				"type":               "string",
				"enum":               values,
				"description":        "Target status.",
				domain.SchemaKeyUnit: string(domain.UnitEnumValueSet),
			},
		},
		"required": []string{"id", "to_status"},
	}
	spec.Description = fmt.Sprintf("Advance a %s record's status. %s", entityDisplayName(def, false), spec.Description)
	return spec, nil
}

// Execute runs the transition through EntityWrite (value-set membership in
// the entity plane, optional transition map on top).
func (e *TransitionStatusExecutor) Execute(ctx context.Context, octx domain.OperationContext, input map[string]any) (*domain.OperationResult, error) {
	id, _ := input["id"].(string)
	toStatus, _ := input["to_status"].(string)
	if id == "" || toStatus == "" {
		return failure(e.tmpl.Name, e.tmpl.Kind, domain.OutcomeInvalid, "invalid: id and to_status are required"), nil
	}

	rec, err := e.writer.TransitionStatus(ctx, octx.TenantID, octx.TenantSlug, id, toStatus, transitionMap(octx.Config))
	if err != nil {
		return entityFailure(e.tmpl.Name, e.tmpl.Kind, err)
	}
	return &domain.OperationResult{
		Kind:       e.tmpl.Kind,
		Outcome:    domain.OutcomeOK,
		Count:      1,
		EntityKind: rec.EntitySlug,
		RecordID:   rec.ID,
		Summary:    fmt.Sprintf("ok: %s %s moved to %s", rec.EntitySlug, rec.ID, rec.Status),
		Output:     map[string]any{"recordId": rec.ID, "status": rec.Status},
		Metadata:   map[string]any{"entity": rec.EntitySlug, "status": rec.Status},
	}, nil
}

// transitionMap reads the optional config transition map, tolerating the
// JSONB-decoded {from: []any} shape.
func transitionMap(cfg map[string]any) map[string][]string {
	raw := cfgMap(cfg, "transitions")
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string][]string, len(raw))
	for from, targets := range raw {
		switch list := targets.(type) {
		case []string:
			out[from] = list
		case []any:
			var tos []string
			for _, t := range list {
				if s, ok := t.(string); ok {
					tos = append(tos, s)
				}
			}
			out[from] = tos
		}
	}
	return out
}
