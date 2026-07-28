package usecases

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"keepstar_v5/internal/domain"
)

// Manifest reconciliation (owner law: a user action must never depend on
// the model having produced a consistent manifest).
//
// The onboarding agent stages an entity definition and a set of operation
// instances in separate steps, in the business's own vocabulary. Nothing
// used to check that the two agree, so the live 2026-07-28 run produced a
// workspace that could not book a showing:
//
//   - lead had a `status` enum field but statusField was NULL → every
//     transition_status call died with NO_STATUS_FIELD ("mark it
//     contacted" was structurally impossible on every tenant built so far);
//   - book_viewing's config named datetime_field "viewingTime" — a field
//     that existed nowhere in the entity → the derived input schema
//     dropped it and the booking was always "invalid".
//
// Both are engine duties, not model duties: the applier reconciles the
// staged config against the entity definition deterministically, adding
// the missing field or retargeting the config, and logs what it healed.
// Reconciliation is additive only — it never removes or retypes a field
// the model asked for.

// entityConfigRoles maps an operation-instance config key to the field
// type the executor requires of the field it names (§4.2/R11).
var entityConfigRoles = map[string]domain.FieldType{
	"datetime_field": domain.FieldDatetime,
	"link_field":     domain.FieldRef,
	"field":          domain.FieldEnum, // transition_status: the status field
}

// inferStatusField picks the entity's status field when the model left it
// unset: an exact `status` enum first, else the ONLY enum field bound to a
// value set. Ambiguity (two such enums, none named status) returns "" —
// the engine does not guess which pipeline is THE pipeline.
func inferStatusField(fields []domain.FieldDef) string {
	candidates := make([]string, 0, 2)
	for _, f := range fields {
		if f.Type != domain.FieldEnum || f.ValueSetRef == "" {
			continue
		}
		if f.Key == "status" {
			return f.Key
		}
		candidates = append(candidates, f.Key)
	}
	if len(candidates) == 1 {
		return candidates[0]
	}
	return ""
}

// reconcileInstanceConfig aligns one operation instance's config with the
// entity definition it targets. Returns the (possibly rewritten) config,
// the definition to persist when fields were added (nil = unchanged), and
// a human-readable list of what was healed (for the step log).
//
// Per role key the order is: keep a name that already exists → retarget to
// the entity's single field of the required type → add the named field.
// `field` (the status role) never adds a field: a status pipeline without
// a value set is meaningless, so an unresolvable status role is left as
// staged and surfaces at execution.
func reconcileInstanceConfig(cfg map[string]any, def *domain.EntityDefinition) (map[string]any, *domain.EntityDefinition, []string) {
	if cfg == nil || def == nil {
		return cfg, nil, nil
	}
	byKey := make(map[string]domain.FieldDef, len(def.Fields))
	for _, f := range def.Fields {
		byKey[f.Key] = f
	}

	var healed []string
	var dirty bool
	out := make(map[string]any, len(cfg))
	for k, v := range cfg {
		out[k] = v
	}

	for _, key := range []string{"datetime_field", "link_field", "field"} {
		want, roled := entityConfigRoles[key]
		if !roled {
			continue
		}
		named, _ := out[key].(string)
		if named == "" {
			continue
		}
		if f, ok := byKey[named]; ok && f.Type == want {
			continue // the model was consistent
		}
		if only, ok := onlyFieldOfType(def.Fields, want); ok {
			healed = append(healed, fmt.Sprintf("%s %q → %q (entity's only %s field)", key, named, only, want))
			out[key] = only
			continue
		}
		if _, taken := byKey[named]; taken {
			// The name exists but carries another type — retyping is not
			// additive, so leave it and let execution report the mismatch.
			healed = append(healed, fmt.Sprintf("%s %q kept: field exists with type %q, wanted %q",
				key, named, byKey[named].Type, want))
			continue
		}
		if key == "field" || len(def.Fields) >= domain.MaxFieldsPerEntity {
			continue
		}
		def.Fields = append(def.Fields, domain.FieldDef{
			Key:       named,
			Label:     humanizeFieldKey(named),
			Type:      want,
			RefTarget: refTargetFor(want),
		})
		byKey[named] = def.Fields[len(def.Fields)-1]
		dirty = true
		healed = append(healed, fmt.Sprintf("%s %q added to entity %q as %s", key, named, def.Slug, want))
	}

	if !dirty {
		return out, nil, healed
	}
	return out, def, healed
}

// onlyFieldOfType returns the single field of the given type, if exactly
// one exists (zero or many → no unambiguous retarget).
func onlyFieldOfType(fields []domain.FieldDef, t domain.FieldType) (string, bool) {
	found := ""
	for _, f := range fields {
		if f.Type != t {
			continue
		}
		if found != "" {
			return "", false
		}
		found = f.Key
	}
	return found, found != ""
}

// refTargetFor supplies the default link target for an added ref field —
// the catalog plane, which is what schedule_slot's link_field books
// against (R11). Non-ref types carry no target.
func refTargetFor(t domain.FieldType) string {
	if t == domain.FieldRef {
		return string(domain.EntityTypeProduct)
	}
	return ""
}

// humanizeFieldKey turns a camelCase key into a display label
// ("viewingTime" → "Viewing Time").
func humanizeFieldKey(key string) string {
	if key == "" {
		return key
	}
	var b strings.Builder
	for i, r := range key {
		if i > 0 && unicode.IsUpper(r) {
			b.WriteRune(' ')
		}
		if i == 0 {
			r = unicode.ToUpper(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}

// reconcileOperationEntity loads the entity an instance config targets and
// runs reconcileInstanceConfig, persisting the definition when fields were
// added. A missing entity (staged out of order, or a catalog-source
// operation with no entity at all) is not an error — the config passes
// through untouched.
func (ap *ManifestApplier) reconcileOperationEntity(ctx context.Context, tenantID, instance string, cfg map[string]any) map[string]any {
	entity, _ := cfg["entity"].(string)
	if entity == "" || ap.cfg.Entities == nil {
		return cfg
	}
	def, err := ap.cfg.Entities.GetEntityDefinition(ctx, tenantID, entity)
	if err != nil || def == nil {
		ap.log.Warn("enable_operations: entity not found for config reconcile — config kept as staged",
			"tenant", tenantID, "instance", instance, "entity", entity, "err", err)
		return cfg
	}
	out, changed, healed := reconcileInstanceConfig(cfg, def)
	if changed != nil {
		if err := ap.cfg.Entities.UpsertEntityDefinition(ctx, changed); err != nil {
			// Additive upsert failed — keep the staged config; execution will
			// report the real mismatch rather than this write error.
			ap.log.Warn("enable_operations: entity field add failed — config kept as staged",
				"tenant", tenantID, "instance", instance, "entity", entity, "err", err)
			return cfg
		}
	}
	if len(healed) > 0 {
		ap.log.Info("enable_operations: instance config reconciled with entity definition",
			"tenant", tenantID, "instance", instance, "entity", entity, "healed", strings.Join(healed, "; "))
	}
	return out
}
