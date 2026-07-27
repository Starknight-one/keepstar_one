package domain

import (
	"reflect"
	"testing"
)

// The R14 ladder is a security gate: ordering must hold exactly, and any
// unknown role string must fail CLOSED — a corrupted session role can never
// satisfy a min_role check.
func TestRoleAtLeast(t *testing.T) {
	ladder := []Role{RoleVisitor, RoleStaff, RoleOwner, RoleSystem}
	for i, r := range ladder {
		for j, min := range ladder {
			if got, want := r.AtLeast(min), i >= j; got != want {
				t.Errorf("%s.AtLeast(%s) = %v, want %v", r, min, got, want)
			}
		}
	}
	if Role("member").AtLeast(RoleVisitor) {
		t.Error("unknown role must fail closed, even against visitor")
	}
	if RoleSystem.AtLeast(Role("admin")) {
		t.Error("unknown min role must fail closed")
	}
}

// SensitiveKeys feeds the R6 redaction choke point: it must find flags at
// the top level AND one nesting level down, deterministically ordered, and
// ignore non-boolean or absent flags.
func TestSensitiveKeys(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"password": map[string]any{"type": "string", SchemaKeySensitive: true},
			"email":    map[string]any{"type": "string"},
			"notFlag":  map[string]any{"type": "string", SchemaKeySensitive: "yes"},
			"account": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"secret": map[string]any{"type": "string", SchemaKeySensitive: true},
					"name":   map[string]any{"type": "string"},
				},
			},
		},
	}
	got := SensitiveKeys(schema)
	want := []string{"account.secret", "password"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SensitiveKeys = %v, want %v", got, want)
	}
	if keys := SensitiveKeys(map[string]any{}); keys != nil {
		t.Errorf("schema without properties: got %v, want nil", keys)
	}
}

// The R18 vocabulary is closed: exactly these 12 units validate.
func TestUnitNameValid(t *testing.T) {
	valid := []UnitName{
		UnitUSDCents, UnitUSD, UnitCount, UnitM2, UnitRooms,
		UnitDatetimeISO8601, UnitDateISO8601, UnitPhoneE164, UnitEmail,
		UnitURL, UnitIDRef, UnitEnumValueSet,
	}
	for _, u := range valid {
		if !u.Valid() {
			t.Errorf("unit %q must be valid", u)
		}
	}
	for _, u := range []UnitName{"percent", "duration_min", "eur", ""} {
		if u.Valid() {
			t.Errorf("unit %q must be invalid (closed vocabulary)", u)
		}
	}
}

// ToToolResult bridges structured outcomes to the LLM: invalid/denied/error
// are IsError (so the LLM self-corrects); ok and empty are not — empty
// mirrors today's legacy non-error "empty:" results byte-for-byte via
// Summary.
func TestOperationResultToToolResult(t *testing.T) {
	cases := map[OpOutcome]bool{
		OutcomeOK: false, OutcomeEmpty: false,
		OutcomeInvalid: true, OutcomeDenied: true, OutcomeError: true,
	}
	for outcome, wantErr := range cases {
		r := &OperationResult{Operation: "query", Outcome: outcome, Summary: "s"}
		tr := r.ToToolResult("tu_1")
		if tr.IsError != wantErr {
			t.Errorf("outcome %s: IsError = %v, want %v", outcome, tr.IsError, wantErr)
		}
		if tr.Content != "s" || tr.ToolUseID != "tu_1" {
			t.Errorf("outcome %s: content/toolUseID not preserved: %+v", outcome, tr)
		}
	}
}
