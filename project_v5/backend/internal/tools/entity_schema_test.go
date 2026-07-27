package tools

import (
	"errors"
	"strings"
	"testing"

	"keepstar_v5/internal/domain"
)

// leadDefinition is a demo-shaped fixture: the lead entity of the realtor
// scenario (R11) — enum status over a value set, money, datetime, phone,
// ref into the catalog plane.
func leadDefinition() (*domain.EntityDefinition, map[string]domain.ValueSet) {
	def := &domain.EntityDefinition{
		Slug: "lead", Name: "Lead", StatusField: "status",
		Fields: []domain.FieldDef{
			{Key: "name", Label: "Name", Type: domain.FieldText, Required: true},
			{Key: "phone", Label: "Phone", Type: domain.FieldPhone, Required: true},
			{Key: "email", Label: "Email", Type: domain.FieldEmail},
			{Key: "status", Label: "Status", Type: domain.FieldEnum, ValueSetRef: "lead_pipeline", Default: "new"},
			{Key: "preferredTime", Label: "Preferred time", Type: domain.FieldDatetime},
			{Key: "budget", Label: "Budget", Type: domain.FieldMoney},
			{Key: "area", Label: "Area", Type: domain.FieldNumber, Unit: domain.UnitM2},
			{Key: "listingId", Label: "Listing", Type: domain.FieldRef, RefTarget: "product"},
			{Key: "moveIn", Label: "Move-in date", Type: domain.FieldDate},
			{Key: "verified", Label: "Verified", Type: domain.FieldBool},
		},
	}
	sets := map[string]domain.ValueSet{
		"lead_pipeline": {Slug: "lead_pipeline", Name: "Lead pipeline", Values: []domain.ValueSetEntry{
			{Value: "new", Label: "New"},
			{Value: "contacted", Label: "Contacted"},
			{Value: "showing_booked", Label: "Showing booked"},
			{Value: "closed", Label: "Closed"},
		}},
	}
	return def, sets
}

func TestEntityRecordSchemaDialect(t *testing.T) {
	def, sets := leadDefinition()
	schema := EntityRecordSchema(def, sets)
	if schema == nil {
		t.Fatal("nil schema for a populated definition")
	}
	props, _ := schema["properties"].(map[string]interface{})
	if len(props) != len(def.Fields) {
		t.Fatalf("want %d properties, got %d", len(def.Fields), len(props))
	}

	// x-unit annotations (units-aware, R18) via the shared dialect key.
	wantUnits := map[string]domain.UnitName{
		"phone": domain.UnitPhoneE164, "email": domain.UnitEmail,
		"status": domain.UnitEnumValueSet, "preferredTime": domain.UnitDatetimeISO8601,
		"budget": domain.UnitUSD, "area": domain.UnitM2,
		"listingId": domain.UnitIDRef, "moveIn": domain.UnitDateISO8601,
	}
	for key, unit := range wantUnits {
		p, _ := props[key].(map[string]interface{})
		if p == nil {
			t.Fatalf("missing property %q", key)
		}
		if got, _ := p[domain.SchemaKeyUnit].(string); got != string(unit) {
			t.Errorf("%s: x-unit = %q, want %q", key, got, unit)
		}
	}
	for _, key := range []string{"name", "verified"} {
		p, _ := props[key].(map[string]interface{})
		if _, has := p[domain.SchemaKeyUnit]; has {
			t.Errorf("%s: unexpected x-unit", key)
		}
	}

	// Money speaks dollars to the LLM (registry coerces to cents).
	budget, _ := props["budget"].(map[string]interface{})
	if desc, _ := budget["description"].(string); !strings.Contains(desc, "dollars") {
		t.Errorf("budget description must declare dollars, got %q", desc)
	}

	// Enum values come from the value set, in array order (R27).
	status, _ := props["status"].(map[string]interface{})
	enum, _ := status["enum"].([]string)
	if len(enum) != 4 || enum[0] != "new" || enum[2] != "showing_booked" {
		t.Errorf("status enum wrong: %v", enum)
	}

	// No FieldDef carries a sensitive flag in v1 → nothing may be marked
	// x-sensitive (credentials never travel through entity records, R6).
	if flagged := domain.SensitiveKeys(schema); len(flagged) != 0 {
		t.Errorf("unexpected x-sensitive keys: %v", flagged)
	}

	required, _ := schema["required"].([]string)
	if len(required) != 2 || required[0] != "name" || required[1] != "phone" {
		t.Errorf("required wrong: %v", required)
	}

	if EntityRecordSchema(nil, nil) != nil {
		t.Error("nil definition must yield nil schema")
	}
}

func TestValidateRecordDataHappyPath(t *testing.T) {
	def, sets := leadDefinition()
	cleaned, status, err := ValidateRecordData(def, sets, map[string]any{
		"name":          "Ana Souza",
		"phone":         "+5511998765432",
		"email":         "ana@example.com",
		"preferredTime": "2026-07-28T15:00:00Z",
		"budget":        125000, // integer cents ($1,250.00) — post-registry form
		"area":          float64(72),
		"listingId":     "3b241101-e2bb-4255-8caf-4136c566a962",
		"moveIn":        "2026-09-01",
		"verified":      true,
		"undeclared":    "dropped silently",
	})
	if err != nil {
		t.Fatalf("valid record rejected: %v", err)
	}
	if status != "new" {
		t.Errorf("derived status = %q, want %q (default applied)", status, "new")
	}
	if cleaned["status"] != "new" {
		t.Errorf("default not applied: %v", cleaned["status"])
	}
	if _, leaked := cleaned["undeclared"]; leaked {
		t.Error("undeclared key survived (dialect is closed)")
	}
	if got, ok := cleaned["budget"].(int); !ok || got != 125000 {
		t.Errorf("budget = %v (%T), want int 125000", cleaned["budget"], cleaned["budget"])
	}
}

func TestValidateRecordDataViolations(t *testing.T) {
	def, sets := leadDefinition()
	cases := []struct {
		name    string
		data    map[string]any
		wantErr string
	}{
		{"missing required", map[string]any{"phone": "+5511998765432"}, "name: missing required field"},
		{"bad phone shape", map[string]any{"name": "A", "phone": "11 99876-5432"}, "E.164"},
		{"bad email", map[string]any{"name": "A", "phone": "+5511998765432", "email": "not-an-email"}, "email"},
		{"enum not in set", map[string]any{"name": "A", "phone": "+5511998765432", "status": "hot"}, "not one of"},
		{"bad datetime", map[string]any{"name": "A", "phone": "+5511998765432", "preferredTime": "tomorrow 3pm"}, "ISO-8601"},
		{"bad date", map[string]any{"name": "A", "phone": "+5511998765432", "moveIn": "01/09/2026"}, "ISO-8601"},
		{"fractional money", map[string]any{"name": "A", "phone": "+5511998765432", "budget": 1250.5}, "integer cents"},
		{"wrong type", map[string]any{"name": 42, "phone": "+5511998765432"}, "name: expected string"},
		{"empty ref", map[string]any{"name": "A", "phone": "+5511998765432", "listingId": ""}, "listingId"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := ValidateRecordData(def, sets, c.data)
			if err == nil {
				t.Fatal("invalid record accepted")
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("error %q does not mention %q", err.Error(), c.wantErr)
			}
			var de *domain.Error
			if !errors.As(err, &de) || de.Code != "RECORD_INVALID" {
				t.Errorf("want typed RECORD_INVALID error, got %T %v", err, err)
			}
		})
	}
}

// Missing value set is a definition/content bug, not an LLM input bug — it
// must still surface as a violation, never panic.
func TestValidateRecordDataMissingValueSet(t *testing.T) {
	def, _ := leadDefinition()
	_, _, err := ValidateRecordData(def, map[string]domain.ValueSet{}, map[string]any{
		"name": "A", "phone": "+5511998765432", "status": "new",
	})
	if err == nil || !strings.Contains(err.Error(), `value set "lead_pipeline" not found`) {
		t.Fatalf("want value-set-not-found violation, got %v", err)
	}
}
