package usecases

import (
	"testing"

	"keepstar_v5/internal/domain"
)

// leadFields is the definition the model produced on the live run: a
// status enum bound to a value set, a ref into the catalog, and NO
// datetime field — while book_viewing's config claimed one.
func leadFields() []domain.FieldDef {
	return []domain.FieldDef{
		{Key: "name", Label: "Name", Type: domain.FieldText, Required: true},
		{Key: "phone", Label: "Phone", Type: domain.FieldPhone},
		{Key: "status", Label: "Status", Type: domain.FieldEnum, ValueSetRef: "lead_pipeline"},
		{Key: "propertyInterest", Label: "Property Interest", Type: domain.FieldRef, RefTarget: "product"},
	}
}

// define_entity cannot carry statusField (it is not in the tool schema),
// so every model-built tenant landed with status_field NULL and
// transition_status died with NO_STATUS_FIELD — "mark it contacted" was
// impossible. The engine infers it.
func TestInferStatusField(t *testing.T) {
	cases := []struct {
		name   string
		fields []domain.FieldDef
		want   string
	}{
		{"exact status enum", leadFields(), "status"},
		{"single value-set enum under another name", []domain.FieldDef{
			{Key: "name", Type: domain.FieldText},
			{Key: "stage", Type: domain.FieldEnum, ValueSetRef: "deal_stages"},
		}, "stage"},
		{"two pipelines — no guess", []domain.FieldDef{
			{Key: "stage", Type: domain.FieldEnum, ValueSetRef: "deal_stages"},
			{Key: "priority", Type: domain.FieldEnum, ValueSetRef: "priorities"},
		}, ""},
		{"enum without a value set is not a pipeline", []domain.FieldDef{
			{Key: "kind", Type: domain.FieldEnum},
		}, ""},
		{"no enums", []domain.FieldDef{{Key: "name", Type: domain.FieldText}}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := inferStatusField(tc.fields); got != tc.want {
				t.Errorf("inferStatusField = %q, want %q", got, tc.want)
			}
		})
	}
}

// The live booking failure: datetime_field named a field that existed
// nowhere, so the derived input schema dropped it and every booking was
// "invalid: viewingTime is required". The applier adds the field.
func TestReconcileAddsTheMissingDatetimeField(t *testing.T) {
	def := &domain.EntityDefinition{Slug: "lead", Fields: leadFields()}
	cfg := map[string]any{"entity": "lead", "datetime_field": "viewingTime", "link_field": "propertyInterest"}

	out, changed, healed := reconcileInstanceConfig(cfg, def)

	if changed == nil {
		t.Fatal("definition not marked dirty — the added field would never persist")
	}
	if out["datetime_field"] != "viewingTime" {
		t.Errorf("datetime_field = %v, want the model's own name kept", out["datetime_field"])
	}
	var added *domain.FieldDef
	for i := range changed.Fields {
		if changed.Fields[i].Key == "viewingTime" {
			added = &changed.Fields[i]
		}
	}
	if added == nil {
		t.Fatalf("viewingTime not added: %+v", changed.Fields)
	}
	if added.Type != domain.FieldDatetime {
		t.Errorf("added field type = %q, want datetime", added.Type)
	}
	if added.Label != "Viewing Time" {
		t.Errorf("added field label = %q, want a humanized label", added.Label)
	}
	if len(healed) == 0 {
		t.Error("reconciliation must report what it healed")
	}
	// Reconciliation is additive: nothing the model declared is lost.
	if len(changed.Fields) != len(leadFields())+1 {
		t.Errorf("field count = %d, want %d — reconciliation must be additive only",
			len(changed.Fields), len(leadFields())+1)
	}
}

// When the entity already has exactly one field of the required type, a
// misnamed config points at it instead of growing a duplicate column.
func TestReconcileRetargetsToTheOnlyFieldOfThatType(t *testing.T) {
	fields := append(leadFields(), domain.FieldDef{Key: "visitAt", Label: "Visit At", Type: domain.FieldDatetime})
	def := &domain.EntityDefinition{Slug: "lead", Fields: fields}
	cfg := map[string]any{"entity": "lead", "datetime_field": "viewingTime"}

	out, changed, _ := reconcileInstanceConfig(cfg, def)

	if out["datetime_field"] != "visitAt" {
		t.Errorf("datetime_field = %v, want visitAt (the entity's only datetime field)", out["datetime_field"])
	}
	if changed != nil {
		t.Errorf("definition rewritten when a retarget sufficed: %+v", changed.Fields)
	}
}

// A consistent manifest must pass through untouched.
func TestReconcileLeavesAConsistentConfigAlone(t *testing.T) {
	fields := append(leadFields(), domain.FieldDef{Key: "viewingTime", Type: domain.FieldDatetime})
	def := &domain.EntityDefinition{Slug: "lead", Fields: fields}
	cfg := map[string]any{"entity": "lead", "datetime_field": "viewingTime", "link_field": "propertyInterest", "field": "status"}

	out, changed, healed := reconcileInstanceConfig(cfg, def)

	if changed != nil || len(healed) != 0 {
		t.Errorf("consistent config was rewritten: healed=%v", healed)
	}
	for k, v := range cfg {
		if out[k] != v {
			t.Errorf("config[%q] = %v, want %v", k, out[k], v)
		}
	}
}

// The status role never invents a field: a pipeline needs a value set,
// which the engine cannot fabricate — leave it and let execution report.
func TestReconcileNeverInventsAStatusField(t *testing.T) {
	def := &domain.EntityDefinition{Slug: "lead", Fields: []domain.FieldDef{
		{Key: "name", Type: domain.FieldText},
	}}
	cfg := map[string]any{"entity": "lead", "field": "pipelineStage"}

	out, changed, _ := reconcileInstanceConfig(cfg, def)

	if changed != nil {
		t.Errorf("status role added a field: %+v", changed.Fields)
	}
	if out["field"] != "pipelineStage" {
		t.Errorf("field = %v, want the staged name kept", out["field"])
	}
}

// A name that exists with the wrong type is a real conflict — retyping is
// not additive, so the config is kept and the mismatch surfaces at
// execution rather than being silently "fixed".
func TestReconcileKeepsAConflictingFieldName(t *testing.T) {
	def := &domain.EntityDefinition{Slug: "lead", Fields: []domain.FieldDef{
		{Key: "viewingTime", Label: "Viewing Time", Type: domain.FieldText},
	}}
	cfg := map[string]any{"entity": "lead", "datetime_field": "viewingTime"}

	out, changed, healed := reconcileInstanceConfig(cfg, def)

	if changed != nil {
		t.Errorf("conflicting field retyped: %+v", changed.Fields)
	}
	if out["datetime_field"] != "viewingTime" {
		t.Errorf("datetime_field = %v, want unchanged", out["datetime_field"])
	}
	if len(healed) != 1 {
		t.Errorf("conflict must be reported: %v", healed)
	}
}
