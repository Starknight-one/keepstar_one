package seed

import (
	"testing"

	"keepstar_v5/internal/domain"
)

// The seed set IS the §3.1 boot-seed table: 6 executor templates +
// compose_turn + 12 onboarding meta templates (the §4.3 eleven plus
// about_keepstar, owner ruling 2026-07-28), with the mode/agent/
// min_role/auto_enabled attributes the rulings pin (R8/R14/R16/R18).
func TestTemplatesMatchSpecTable(t *testing.T) {
	ts := Templates()
	if len(ts) != 19 {
		t.Fatalf("expected 19 seed templates (6 executors + compose_turn + 12 meta), got %d", len(ts))
	}

	byName := map[string]domain.OperationTemplate{}
	for _, tpl := range ts {
		if _, dup := byName[tpl.Name]; dup {
			t.Errorf("duplicate template name %q (v5_operations has UNIQUE(name))", tpl.Name)
		}
		byName[tpl.Name] = tpl
	}

	// The 6 executor templates: own kind, tenant modes only (R16: onboarding
	// never sees tenant data ops), agent data, enabled per tenant.
	executors := map[string]domain.OperationKind{
		"query":             domain.KindQuery,
		"create_record":     domain.KindCreateRecord,
		"update_record":     domain.KindUpdateRecord,
		"transition_status": domain.KindTransitionStatus,
		"schedule_slot":     domain.KindScheduleSlot,
		"notify":            domain.KindNotify,
	}
	for name, kind := range executors {
		tpl, ok := byName[name]
		if !ok {
			t.Errorf("missing executor template %q", name)
			continue
		}
		if tpl.Kind != kind {
			t.Errorf("%s: kind = %s, want %s", name, tpl.Kind, kind)
		}
		if tpl.AutoEnabled {
			t.Errorf("%s: executor templates are enabled per tenant, not auto_enabled", name)
		}
		assertModes(t, tpl, domain.ModeStorefront, domain.ModeCRM)
		if tpl.Agent != domain.AgentData {
			t.Errorf("%s: agent = %s, want data", name, tpl.Agent)
		}
	}

	// compose_turn: visual, onboarding+crm (NOT storefront — its
	// visual_assembly prompt cache stays untouched), auto_enabled.
	ct, ok := byName["compose_turn"]
	if !ok {
		t.Fatal("missing compose_turn template")
	}
	if ct.Kind != domain.KindVisual || ct.Agent != domain.AgentVisual || !ct.AutoEnabled {
		t.Errorf("compose_turn: want kind=visual agent=visual auto_enabled, got kind=%s agent=%s auto=%v", ct.Kind, ct.Agent, ct.AutoEnabled)
	}
	assertModes(t, ct, domain.ModeOnboarding, domain.ModeCRM)

	// The 12 meta templates: kind meta, onboarding-only, visitor (the
	// onboarding cookie + form gate does the work, R14), auto_enabled.
	meta := []string{
		"about_keepstar",
		"search_library", "create_tenant", "define_entity", "define_value_set",
		"define_automation", "enable_operations", "adopt_presets",
		"issue_ingest_door", "register_user", "issue_surface_urls", "apply_manifest",
	}
	for _, name := range meta {
		tpl, ok := byName[name]
		if !ok {
			t.Errorf("missing meta template %q", name)
			continue
		}
		if tpl.Kind != domain.KindMeta {
			t.Errorf("%s: kind = %s, want meta", name, tpl.Kind)
		}
		if tpl.MinRole != domain.RoleVisitor {
			t.Errorf("%s: min_role = %s, want visitor (R14)", name, tpl.MinRole)
		}
		if !tpl.AutoEnabled {
			t.Errorf("%s: meta templates are auto_enabled (onboarding form only)", name)
		}
		assertModes(t, tpl, domain.ModeOnboarding)
	}
}

// Every template must be embeddable and card-complete (beat-2 op-cards
// render input → does → output → why from these rows), and every field must
// stay inside the closed vocabularies the registry gates on.
func TestTemplatesWellFormed(t *testing.T) {
	validRoles := map[domain.Role]bool{domain.RoleVisitor: true, domain.RoleStaff: true, domain.RoleOwner: true, domain.RoleSystem: true}
	validEvents := map[string]bool{domain.EventRecordCreated: true, domain.EventRecordUpdated: true, domain.EventRecordStatusChanged: true}
	cardKeys := []string{"input_summary", "does", "output_summary", "why"}

	for _, tpl := range Templates() {
		if tpl.Title == "" || tpl.Description == "" {
			t.Errorf("%s: title and description are required (description is embedded)", tpl.Name)
		}
		if !validRoles[tpl.MinRole] {
			t.Errorf("%s: min_role %q outside the closed ladder", tpl.Name, tpl.MinRole)
		}
		for _, k := range cardKeys {
			if s, _ := tpl.Card[k].(string); s == "" {
				t.Errorf("%s: card.%s missing or empty", tpl.Name, k)
			}
		}
		for _, e := range tpl.Effects.Emits {
			if !validEvents[e.EventType] {
				t.Errorf("%s: emits unknown event_type %q", tpl.Name, e.EventType)
			}
		}
		assertUnitsValid(t, tpl.Name+".input_schema", tpl.InputSchema)
		assertUnitsValid(t, tpl.Name+".output_schema", tpl.OutputSchema)
		assertUnitsValid(t, tpl.Name+".config_schema", tpl.ConfigSchema)
	}
}

func assertModes(t *testing.T, tpl domain.OperationTemplate, want ...domain.PipelineMode) {
	t.Helper()
	if len(tpl.Modes) != len(want) {
		t.Errorf("%s: modes = %v, want %v", tpl.Name, tpl.Modes, want)
		return
	}
	have := map[domain.PipelineMode]bool{}
	for _, m := range tpl.Modes {
		have[m] = true
	}
	for _, m := range want {
		if !have[m] {
			t.Errorf("%s: modes = %v, want %v", tpl.Name, tpl.Modes, want)
			return
		}
	}
}

// assertUnitsValid walks a restricted-dialect schema (one nesting level +
// array items) and asserts every x-unit is in the closed R18 vocabulary.
func assertUnitsValid(t *testing.T, where string, schema map[string]any) {
	t.Helper()
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return
	}
	for name, raw := range props {
		prop, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if u, present := prop[domain.SchemaKeyUnit]; present {
			us, _ := u.(string)
			if !domain.UnitName(us).Valid() {
				t.Errorf("%s.%s: x-unit %v outside the closed R18 vocabulary", where, name, u)
			}
		}
		if sub, ok := prop["properties"].(map[string]any); ok {
			assertUnitsValid(t, where+"."+name, map[string]any{"properties": sub})
		}
		if items, ok := prop["items"].(map[string]any); ok {
			assertUnitsValid(t, where+"."+name+"[]", items)
		}
	}
}
