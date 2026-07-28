package operations

import (
	"context"
	"encoding/json"
	"log/slog"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// PresetOperationBinder is the late-binding seam between the preset
// library and a tenant's operation surface (RUNTIME_SPEC.md §4.8).
//
// Presets are library data authored against the demo vocabulary: the
// seeded booking_form submits `book_showing` and posts `preferredTime` /
// `listingId`. Instance names and config field names are the MODEL's —
// the live 2026-07-28 tenant called the instance `book_viewing` over
// fields `viewingTime` / `propertyInterest`, so the submit denied with
// `unknown operation "book_showing"` and would have been `invalid` right
// after. Rewriting the seeds is no fix: the next tenant invents different
// names.
//
// So a preset declares INTENT and the runtime binds IDENTITY, per tenant,
// at read time (same law as the upload-arm pass: a user action must never
// depend on the model having wired things up):
//
//	{"kind":"operation_invoke","operation":"book_showing",
//	 "operationKind":"schedule_slot"}          → the tenant's schedule_slot instance
//	{"type":"datetime","name":"preferredTime",
//	 "operationField":"datetime_field"}        → that instance's configured field
//
// Late (read-time) rather than at adopt_presets, so renaming an operation
// never leaves an adopted preset lying. Annotations are consumed here and
// never reach the wire. Unresolvable intent keeps the authored name — the
// denial stays honest instead of invoking something the author never
// named.
type PresetOperationBinder struct {
	inner    ports.PresetPort
	resolver OperationKindResolver
	log      *slog.Logger
}

// OperationKindResolver is the slice of the registry the binder needs.
// Satisfied by *Registry; nil (unset) degrades the binder to a pure
// pass-through that still strips the annotations.
type OperationKindResolver interface {
	ResolveForKind(ctx context.Context, tenantSlug, name string, kind domain.OperationKind) (ResolvedOperation, bool)
}

var _ ports.PresetPort = (*PresetOperationBinder)(nil)

// Annotation keys — authored in preset JSON, consumed by this binder.
const (
	// AnnotationOperationKind declares which operation KIND a submit needs
	// (schedule_slot, create_record, transition_status, …).
	AnnotationOperationKind = "operationKind"
	// AnnotationOperationField declares which config key of that operation
	// names the entity field this input carries (datetime_field, …).
	AnnotationOperationField = "operationField"
)

// NewPresetOperationBinder wraps a preset port. The resolver arrives via
// SetResolver after the registry is built (the registry itself consumes a
// preset port — same deferred-wiring pattern as SetAutomationRunner).
func NewPresetOperationBinder(inner ports.PresetPort, log *slog.Logger) *PresetOperationBinder {
	if log == nil {
		log = slog.Default()
	}
	return &PresetOperationBinder{inner: inner, log: log}
}

// SetResolver closes the wiring cycle. Until it is called the binder only
// strips annotations — never leave this out of boot wiring.
func (b *PresetOperationBinder) SetResolver(r OperationKindResolver) { b.resolver = r }

// GetPublishedPreset returns the preset with its operation intent bound to
// the tenant. A malformed document passes through untouched — rendering
// downstream owns that error.
func (b *PresetOperationBinder) GetPublishedPreset(ctx context.Context, tenantSlugOrID, name string) (*domain.Preset, error) {
	p, err := b.inner.GetPublishedPreset(ctx, tenantSlugOrID, name)
	if err != nil || p == nil {
		return p, err
	}
	bound := *p
	bound.DocumentJSON = b.bindDocument(ctx, tenantSlugOrID, name, p.DocumentJSON)
	return &bound, nil
}

// ListPublishedPresets binds every document in the list.
func (b *PresetOperationBinder) ListPublishedPresets(ctx context.Context, tenantSlugOrID string) ([]domain.Preset, error) {
	list, err := b.inner.ListPublishedPresets(ctx, tenantSlugOrID)
	if err != nil {
		return list, err
	}
	for i := range list {
		list[i].DocumentJSON = b.bindDocument(ctx, tenantSlugOrID, list[i].Name, list[i].DocumentJSON)
	}
	return list, nil
}

// bindDocument rewrites one preset document. Each top-level child is bound
// independently — a document with two forms binds each against its own
// operation.
func (b *PresetOperationBinder) bindDocument(ctx context.Context, tenantSlugOrID, presetName string, doc json.RawMessage) json.RawMessage {
	if len(doc) == 0 {
		return doc
	}
	var root map[string]any
	if err := json.Unmarshal(doc, &root); err != nil {
		return doc
	}
	children, _ := root["children"].([]any)
	if len(children) == 0 {
		// Annotation-free documents are the common case; only pay the
		// re-marshal when a subtree actually carried intent.
		if !b.bindSubtree(ctx, tenantSlugOrID, presetName, root) {
			return doc
		}
	} else {
		touched := false
		for _, child := range children {
			if b.bindSubtree(ctx, tenantSlugOrID, presetName, child) {
				touched = true
			}
		}
		if !touched {
			return doc
		}
	}
	out, err := json.Marshal(root)
	if err != nil {
		return doc
	}
	return out
}

// bindSubtree resolves the operation intent inside one subtree and
// rewrites the submit action + the annotated input names. Reports whether
// anything changed.
func (b *PresetOperationBinder) bindSubtree(ctx context.Context, tenantSlugOrID, presetName string, node any) bool {
	actions := collectAnnotated(node, "action")
	fields := collectAnnotated(node, AnnotationOperationField)
	if len(actions) == 0 && len(fields) == 0 {
		return false
	}

	var cfg map[string]any
	changed := false
	for _, act := range actions {
		kindHint, _ := act[AnnotationOperationKind].(string)
		if _, ok := act[AnnotationOperationKind]; ok {
			delete(act, AnnotationOperationKind)
			changed = true
		}
		authored, _ := act["operation"].(string)
		if authored == "" || b.resolver == nil {
			continue
		}
		resolved, ok := b.resolver.ResolveForKind(ctx, tenantSlugOrID, authored, domain.OperationKind(kindHint))
		if !ok {
			b.log.Warn("preset binding: operation intent unresolved — authored name kept",
				"tenant", tenantSlugOrID, "preset", presetName, "operation", authored, "kind", kindHint)
			continue
		}
		if cfg == nil {
			cfg = resolved.Config
		}
		if resolved.Name != authored {
			act["operation"] = resolved.Name
			changed = true
			b.log.Info("preset binding: operation bound to tenant instance",
				"tenant", tenantSlugOrID, "preset", presetName, "authored", authored, "bound", resolved.Name)
		}
	}

	for _, f := range fields {
		role, _ := f[AnnotationOperationField].(string)
		delete(f, AnnotationOperationField)
		changed = true
		if role == "" || cfg == nil {
			continue
		}
		target, _ := cfg[role].(string)
		if target == "" {
			continue
		}
		if current, _ := f["name"].(string); current != target {
			f["name"] = target
			b.log.Info("preset binding: input name bound to instance config",
				"tenant", tenantSlugOrID, "preset", presetName, "role", role, "name", target)
		}
	}
	return changed
}

// collectAnnotated walks a scene-graph subtree and returns every map that
// carries the given key: for "action" the action objects themselves, for
// an annotation key the annotated node. Order is document order, so the
// first resolved action supplies the config for the subtree's fields.
func collectAnnotated(node any, key string) []map[string]any {
	var out []map[string]any
	var walk func(any)
	walk = func(n any) {
		switch v := n.(type) {
		case map[string]any:
			if key == "action" {
				if act, ok := v["action"].(map[string]any); ok {
					if kind, _ := act["kind"].(string); kind == string(domain.UserActionOperationInvoke) {
						out = append(out, act)
					}
				}
			} else if _, ok := v[key]; ok {
				out = append(out, v)
			}
			for _, child := range v {
				walk(child)
			}
		case []any:
			for _, item := range v {
				walk(item)
			}
		}
	}
	walk(node)
	return out
}
