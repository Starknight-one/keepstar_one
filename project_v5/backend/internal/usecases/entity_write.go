package usecases

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
	"keepstar_v5/internal/tools"
)

// EntityWrite is the single write path of the entity plane (RUNTIME_SPEC.md
// §4.4, R10): operation executors call it, never ports.EntityPort directly.
// Every mutation runs validate → tx(record + v5_events outbox, inside the
// port) → post-commit inline automation dispatch (flat event→operation v1,
// R12: no sweeper) → notification via the notify operation.
//
// Tenant identity travels as (tenantID, tenantSlug) pairs: the entity plane
// stores UUIDs, but the operation registry's instance resolution and spec
// cache key on the slug — both are already known at every call site
// (OperationContext), so threading the pair beats a reverse id→slug lookup
// that no port offers.
type EntityWrite struct {
	entities ports.EntityPort
	catalog  ports.CatalogPort // refTitle/refImage denormalization; nil skips
	runner   automationRunner  // set post-construction (SetAutomationRunner)
	log      *slog.Logger
}

// automationRunner is the slice of OperationRunner DispatchEvent needs.
// Package-internal: breaks the construction cycle EntityWrite → runner →
// registry → executors → EntityWrite (the runner is wired after the
// registry exists; a nil runner disables dispatch with a warning).
type automationRunner interface {
	Run(ctx context.Context, tenantID, tenantSlug, operationSlug string, params, eventPayload map[string]any) error
}

// NewEntityWrite wires the use case. catalog may be nil (no catalog-link
// denormalization). The automation runner arrives later via
// SetAutomationRunner — executors register against the registry first.
func NewEntityWrite(entities ports.EntityPort, catalog ports.CatalogPort, log *slog.Logger) *EntityWrite {
	if log == nil {
		log = slog.Default()
	}
	return &EntityWrite{entities: entities, catalog: catalog, log: log}
}

// SetAutomationRunner plugs the operation runner in once the registry is
// built (boot wiring). Nil-safe: without a runner, events commit and stay
// unprocessed (curator-visible), nothing crashes.
func (uc *EntityWrite) SetAutomationRunner(r *OperationRunner) { uc.runner = r }

// LoadDefinition returns the entity definition and the tenant's value sets
// keyed by slug — the shared read the executors' SpecForTenant derivations
// and this use case's own validation both run on.
func (uc *EntityWrite) LoadDefinition(ctx context.Context, tenantID, entitySlug string) (*domain.EntityDefinition, map[string]domain.ValueSet, error) {
	def, err := uc.entities.GetEntityDefinition(ctx, tenantID, entitySlug)
	if err != nil {
		return nil, nil, err
	}
	sets, err := uc.entities.ListValueSets(ctx, tenantID)
	if err != nil {
		return nil, nil, err
	}
	byKey := make(map[string]domain.ValueSet, len(sets))
	for _, vs := range sets {
		byKey[vs.Slug] = vs
	}
	return def, byKey, nil
}

// CreateRecord validates data against the definition (defaults applied,
// units already coerced by the registry validator), denormalizes the
// catalog link, writes record + event in one tx and dispatches automations
// inline post-commit. ref may be nil — it is then derived from the first
// ref-typed field with a value (the definition knows its own links).
func (uc *EntityWrite) CreateRecord(ctx context.Context, tenantID, tenantSlug, entitySlug string, data map[string]any, ref *domain.EntityRef, createdBy string) (*domain.EntityRecord, error) {
	def, sets, err := uc.LoadDefinition(ctx, tenantID, entitySlug)
	if err != nil {
		return nil, err
	}
	cleaned, derivedStatus, err := tools.ValidateRecordData(def, sets, data)
	if err != nil {
		return nil, err
	}
	if ref == nil {
		ref = catalogRefFromData(def, cleaned)
	}

	rec := &domain.EntityRecord{
		TenantID:   tenantID,
		EntitySlug: entitySlug,
		Data:       cleaned,
		Status:     derivedStatus,
		CreatedBy:  createdBy,
	}
	if ref != nil && ref.ID != "" {
		rec.RefEntityType = string(ref.Type)
		rec.RefEntityID = ref.ID
		if err := uc.denormalizeRef(ctx, tenantID, ref, cleaned); err != nil {
			return nil, err
		}
	}

	rec, ev, err := uc.entities.CreateRecord(ctx, rec)
	if err != nil {
		return nil, err
	}
	uc.DispatchEvent(ctx, tenantSlug, ev)
	return rec, nil
}

// UpdateRecord validates the patch against the definition (declared keys
// only — undeclared silently dropped, closed dialect) and applies it
// last-write-wins through the port, which re-mirrors the status column and
// writes the record.updated event in the same tx.
func (uc *EntityWrite) UpdateRecord(ctx context.Context, tenantID, tenantSlug, recordID string, patch map[string]any) (*domain.EntityRecord, error) {
	current, err := uc.entities.GetRecord(ctx, tenantID, recordID)
	if err != nil {
		return nil, err
	}
	def, sets, err := uc.LoadDefinition(ctx, tenantID, current.EntitySlug)
	if err != nil {
		return nil, err
	}
	cleaned, err := validatePatch(def, sets, patch)
	if err != nil {
		return nil, err
	}

	rec, ev, err := uc.entities.UpdateRecord(ctx, tenantID, recordID, cleaned)
	if err != nil {
		return nil, err
	}
	uc.DispatchEvent(ctx, tenantSlug, ev)
	return rec, nil
}

// TransitionStatus moves a record through its pipeline. Value-set
// membership is enforced by the port (typed INVALID_STATUS); the optional
// allowed map ({from: [to, ...]} — transition_status instance config)
// narrows it further here.
func (uc *EntityWrite) TransitionStatus(ctx context.Context, tenantID, tenantSlug, recordID, toStatus string, allowed map[string][]string) (*domain.EntityRecord, error) {
	if len(allowed) > 0 {
		current, err := uc.entities.GetRecord(ctx, tenantID, recordID)
		if err != nil {
			return nil, err
		}
		targets := allowed[current.Status]
		ok := false
		for _, t := range targets {
			if t == toStatus {
				ok = true
				break
			}
		}
		if !ok {
			return nil, &domain.Error{Code: "INVALID_STATUS", Message: fmt.Sprintf(
				"transition %q → %q is not allowed (from %q: %s)",
				current.Status, toStatus, current.Status, strings.Join(targets, ", "))}
		}
	}

	rec, ev, err := uc.entities.TransitionStatus(ctx, tenantID, recordID, toStatus)
	if err != nil {
		return nil, err
	}
	uc.DispatchEvent(ctx, tenantSlug, ev)
	return rec, nil
}

// DispatchEvent runs the inline post-commit automation pass (R12): list the
// tenant's enabled automations for the event type, match entity, evaluate
// the flat predicate deterministically, run the operation (notify-only
// allowlist enforced by the runner) and stamp processed_at. Best-effort
// end to end — the record already committed; failures log, never propagate.
func (uc *EntityWrite) DispatchEvent(ctx context.Context, tenantSlug string, ev *domain.RuntimeEvent) {
	if ev == nil {
		return
	}
	ctx, span := withSpan(ctx, "automation.fire."+ev.EventType)
	defer span.End()
	span.SetAttrs(map[string]any{
		"tenant": tenantSlug,
		"entity": ev.EntitySlug,
		"event":  ev.EventType,
	})

	automations, err := uc.entities.ListAutomations(ctx, ev.TenantID, ev.EventType)
	if err != nil {
		span.SetError(err)
		uc.log.Warn("entity_write: automation listing failed — event stays unprocessed",
			"tenant", tenantSlug, "event", ev.EventType, "err", err)
		return
	}

	fields := eventFields(ev)
	payload := runnerPayload(ev)
	fired := 0
	for _, a := range automations {
		if a.EntitySlug != "" && a.EntitySlug != ev.EntitySlug {
			continue
		}
		if !predicateMatches(a.Predicate, fields) {
			continue
		}
		if uc.runner == nil {
			uc.log.Warn("entity_write: automation matched but no runner wired",
				"automation", a.Name, "tenant", tenantSlug)
			continue
		}
		if err := uc.runner.Run(ctx, ev.TenantID, tenantSlug, a.OperationSlug, a.OperationParams, payload); err != nil {
			uc.log.Warn("entity_write: automation run failed",
				"automation", a.Name, "operation", a.OperationSlug, "tenant", tenantSlug, "err", err)
			continue
		}
		fired++
	}
	span.SetAttrs(map[string]any{"matched": fired, "listed": len(automations)})

	if err := uc.entities.MarkEventProcessed(ctx, ev.ID); err != nil {
		uc.log.Warn("entity_write: mark event processed failed", "event_id", ev.ID, "err", err)
	}
}

// denormalizeRef copies display fields off the linked catalog product into
// the record data (refTitle, refImage — the EntityToMap layer-2 contract)
// so entity presets render the link without a join. Fail-closed on a
// missing product: a booking against a nonexistent listing is invalid, not
// silently unlinked.
func (uc *EntityWrite) denormalizeRef(ctx context.Context, tenantID string, ref *domain.EntityRef, data map[string]any) error {
	if uc.catalog == nil || ref.Type != domain.EntityTypeProduct {
		return nil
	}
	p, err := uc.catalog.GetProduct(ctx, tenantID, ref.ID)
	if err != nil {
		if errors.Is(err, domain.ErrProductNotFound) {
			return &domain.Error{Code: "RECORD_INVALID", Message: fmt.Sprintf(
				"linked product %q not found", ref.ID)}
		}
		return fmt.Errorf("denormalize ref %s: %w", ref.ID, err)
	}
	data["refTitle"] = p.Name
	if len(p.Images) > 0 {
		data["refImage"] = p.Images[0]
	}
	return nil
}

// catalogRefFromData derives the record's single catalog link (one per
// record, guardrail) from the definition: the first ref-typed field
// targeting 'product' that carries a value.
func catalogRefFromData(def *domain.EntityDefinition, data map[string]any) *domain.EntityRef {
	for _, f := range def.Fields {
		if f.Type != domain.FieldRef || f.RefTarget != string(domain.EntityTypeProduct) {
			continue
		}
		if id, ok := data[f.Key].(string); ok && id != "" {
			return &domain.EntityRef{Type: domain.EntityTypeProduct, ID: id}
		}
	}
	return nil
}

// validatePatch validates only the supplied keys by projecting the
// definition onto the patch: undeclared keys drop (closed dialect), each
// declared value passes its field's type/unit checks, absent fields are
// untouched (required-ness applies to the full record at create, not to a
// partial update — but patching a required field to nil is rejected).
func validatePatch(def *domain.EntityDefinition, sets map[string]domain.ValueSet, patch map[string]any) (map[string]any, error) {
	subset := *def
	subset.Fields = nil
	for _, f := range def.Fields {
		if _, present := patch[f.Key]; present {
			f.Default = nil // a patch never resurrects defaults
			subset.Fields = append(subset.Fields, f)
		}
	}
	if len(subset.Fields) == 0 {
		return nil, &domain.Error{Code: "RECORD_INVALID", Message: "patch contains no known fields"}
	}
	cleaned, _, err := tools.ValidateRecordData(&subset, sets, patch)
	if err != nil {
		return nil, err
	}
	return cleaned, nil
}

// eventFields flattens the event payload to the record's field map — the
// substitution and predicate surface: snapshot for record.created, the
// after-image for updates/transitions.
func eventFields(ev *domain.RuntimeEvent) map[string]any {
	if ev.Payload == nil {
		return map[string]any{}
	}
	if snap, ok := ev.Payload["snapshot"].(map[string]any); ok {
		return snap
	}
	if diff, ok := ev.Payload["diff"].(map[string]any); ok {
		if after, ok := diff["after"].(map[string]any); ok {
			return after
		}
	}
	return map[string]any{}
}

// runnerPayload augments the raw event payload with the event's own
// identity scalars so notify templates can reference {recordId} / {entity}
// alongside record fields.
func runnerPayload(ev *domain.RuntimeEvent) map[string]any {
	out := make(map[string]any, len(ev.Payload)+2)
	for k, v := range ev.Payload {
		out[k] = v
	}
	if ev.RecordID != "" {
		out["recordId"] = ev.RecordID
	}
	if ev.EntitySlug != "" {
		out["entity"] = ev.EntitySlug
	}
	return out
}

// predicateMatches evaluates the single flat automation condition
// deterministically (eq | neq | in — §4.4 guardrails). Values compare by
// normalized string so JSONB round-trips (float64 5) match Go writes
// (int 5). A nil predicate always matches; an unknown op never does.
func predicateMatches(p *domain.AutomationPredicate, fields map[string]any) bool {
	if p == nil {
		return true
	}
	raw, present := fields[p.Field]
	switch p.Op {
	case "eq":
		return present && normScalar(raw) == normScalar(p.Value)
	case "neq":
		return !present || normScalar(raw) != normScalar(p.Value)
	case "in":
		if !present {
			return false
		}
		items, ok := p.Value.([]any)
		if !ok {
			return false
		}
		for _, item := range items {
			if normScalar(raw) == normScalar(item) {
				return true
			}
		}
		return false
	}
	return false
}

// normScalar renders a scalar for comparison; %v collapses float64(5) and
// int(5) to "5".
func normScalar(v any) string { return fmt.Sprintf("%v", v) }
