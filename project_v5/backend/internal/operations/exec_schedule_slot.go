package operations

import (
	"context"
	"fmt"
	"time"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
	"keepstar_v5/internal/tools"
)

// scheduleDisplayLayout renders the confirmed datetime in summaries —
// same human-readable form as the binding layer's `<key>Formatted`.
const scheduleDisplayLayout = "Jan 2, 2006 3:04 PM"

// ScheduleSlotExecutor books a datetime against a catalog item (R11):
// create_record semantics plus deterministic datetime guards — reject the
// past, enforce the configured business-hours window — with human-readable
// reasons so the LLM (or the booking form) self-corrects. NO overlap scan
// in v1 (no slot inventory exists). Emits record.created → the seeded
// automation notifies staff. Demo instance: book_showing over entity lead.
//
// Config: {"entity":"lead", "datetime_field":"preferredTime", "link_field":
// "listingId", "reject_past":true, "hours":{"from":9,"to":18},
// "defaults":{"status":"new"}}
type ScheduleSlotExecutor struct {
	writer EntityWriter
	tmpl   domain.OperationTemplate
	now    func() time.Time // injectable clock for the guard tests
}

var _ ports.Executor = (*ScheduleSlotExecutor)(nil)

// NewScheduleSlotExecutor wires the executor over the entity write path.
func NewScheduleSlotExecutor(writer EntityWriter) *ScheduleSlotExecutor {
	return &ScheduleSlotExecutor{writer: writer, tmpl: seedRow("schedule_slot"), now: time.Now}
}

func (e *ScheduleSlotExecutor) Template() domain.OperationSpec        { return e.tmpl.Spec() }
func (e *ScheduleSlotExecutor) TemplateRow() domain.OperationTemplate { return e.tmpl }

// SpecForTenant derives the booking schema from the entity definition:
// the full record schema with the datetime field (and the catalog link
// field, when configured) forced required — a booking without a time or a
// target is meaningless.
func (e *ScheduleSlotExecutor) SpecForTenant(ctx context.Context, t domain.Tenant, cfg map[string]any) (domain.OperationSpec, error) {
	spec := e.tmpl.Spec()
	entity := cfgString(cfg, "entity")
	dtField := cfgString(cfg, "datetime_field")
	if entity == "" || dtField == "" {
		return spec, nil
	}
	def, sets, err := e.writer.LoadDefinition(ctx, t.ID, entity)
	if err != nil {
		return spec, nil
	}
	schema := tools.EntityRecordSchema(def, sets)
	if schema == nil {
		return spec, nil
	}
	required := map[string]bool{dtField: true}
	if link := cfgString(cfg, "link_field"); link != "" {
		required[link] = true
	}
	switch req := schema["required"].(type) {
	case []string:
		for _, k := range req {
			required[k] = true
		}
	case []any:
		for _, raw := range req {
			if k, ok := raw.(string); ok {
				required[k] = true
			}
		}
	}
	reqList := make([]string, 0, len(required))
	if props, ok := schema["properties"].(map[string]any); ok {
		for _, f := range def.Fields { // definition order — deterministic
			if required[f.Key] {
				if _, declared := props[f.Key]; declared {
					reqList = append(reqList, f.Key)
				}
			}
		}
	}
	schema["required"] = reqList
	spec.InputSchema = schema
	spec.Description = fmt.Sprintf("Book a time slot as a %s record. %s", entityDisplayName(def, false), spec.Description)
	return spec, nil
}

// Execute runs the R11 guards, then create_record semantics through
// EntityWrite (defaults, catalog-link denormalization, record + event in
// one tx, inline automation dispatch).
func (e *ScheduleSlotExecutor) Execute(ctx context.Context, octx domain.OperationContext, input map[string]any) (*domain.OperationResult, error) {
	cfg := octx.Config
	entity := cfgString(cfg, "entity")
	dtField := cfgString(cfg, "datetime_field")
	if entity == "" || dtField == "" {
		return failure(e.tmpl.Name, e.tmpl.Kind, domain.OutcomeError, "error: schedule_slot instance config needs entity and datetime_field"), nil
	}

	raw, _ := input[dtField].(string)
	if raw == "" {
		return failure(e.tmpl.Name, e.tmpl.Kind, domain.OutcomeInvalid,
			fmt.Sprintf("invalid: %s is required (ISO-8601 datetime)", dtField)), nil
	}
	slot, ok := parseScheduleDatetime(raw)
	if !ok {
		return failure(e.tmpl.Name, e.tmpl.Kind, domain.OutcomeInvalid,
			fmt.Sprintf("invalid: %s %q is not an ISO-8601 datetime", dtField, raw)), nil
	}

	// ── Deterministic guards (R11) ──
	if cfgBool(cfg, "reject_past", true) && slot.Before(e.now()) {
		return failure(e.tmpl.Name, e.tmpl.Kind, domain.OutcomeInvalid,
			fmt.Sprintf("invalid: %s is in the past — choose a future time", slot.Format(scheduleDisplayLayout))), nil
	}
	if hours := cfgMap(cfg, "hours"); hours != nil {
		from := cfgInt(hours, "from", 0)
		to := cfgInt(hours, "to", 24)
		if h := slot.Hour(); h < from || h >= to {
			return failure(e.tmpl.Name, e.tmpl.Kind, domain.OutcomeInvalid,
				fmt.Sprintf("invalid: requested time %s is outside business hours (%02d:00-%02d:00)",
					slot.Format("15:04"), from, to)), nil
		}
	}

	// ── create_record semantics ──
	data := applyDefaults(input, cfgMap(cfg, "defaults"))
	var ref *domain.EntityRef
	if link := cfgString(cfg, "link_field"); link != "" {
		if id, ok := data[link].(string); ok && id != "" {
			ref = &domain.EntityRef{Type: domain.EntityTypeProduct, ID: id}
		}
	}
	rec, err := e.writer.CreateRecord(ctx, octx.TenantID, octx.TenantSlug, entity, data, ref, actorOrSession(octx))
	if err != nil {
		return entityFailure(e.tmpl.Name, e.tmpl.Kind, err)
	}
	return &domain.OperationResult{
		Kind:       e.tmpl.Kind,
		Outcome:    domain.OutcomeOK,
		Count:      1,
		EntityKind: rec.EntitySlug,
		RecordID:   rec.ID,
		Summary:    fmt.Sprintf("ok: booked %s %s for %s", rec.EntitySlug, rec.ID, slot.Format(scheduleDisplayLayout)),
		Output:     map[string]any{"recordId": rec.ID},
		Metadata:   map[string]any{"entity": rec.EntitySlug, "status": rec.Status, "slot": slot.Format(time.RFC3339)},
	}, nil
}

// parseScheduleDatetime accepts full RFC 3339 (what the derived schema's
// x-unit validation lets through) plus the zone-less spellings, mirroring
// the binding layer's parseEntityDatetime. Guards evaluate in the value's
// own location — v1 business hours are the hours the visitor asked for.
func parseScheduleDatetime(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
