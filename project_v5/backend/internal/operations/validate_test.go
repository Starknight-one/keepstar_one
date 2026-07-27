package operations

import (
	"reflect"
	"strings"
	"testing"

	"keepstar_v5/internal/domain"
)

func schemaWith(props map[string]any, required ...string) map[string]any {
	s := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func TestValidateInputRequiredMissing(t *testing.T) {
	schema := schemaWith(map[string]any{
		"query": map[string]any{"type": "string"},
	}, "query")
	_, violations := ValidateInput(schema, map[string]any{})
	if len(violations) != 1 || !strings.Contains(violations[0], `"query"`) {
		t.Fatalf("want one missing-required violation for query, got %v", violations)
	}
}

func TestValidateInputTypeAndEnum(t *testing.T) {
	schema := schemaWith(map[string]any{
		"status": map[string]any{"type": "string", "enum": []string{"new", "contacted"}},
		"limit":  map[string]any{"type": "integer"},
		"flag":   map[string]any{"type": "boolean"},
	})

	if _, v := ValidateInput(schema, map[string]any{"status": "closed"}); len(v) != 1 {
		t.Errorf("enum violation expected, got %v", v)
	}
	if _, v := ValidateInput(schema, map[string]any{"limit": 3.5}); len(v) != 1 {
		t.Errorf("fractional integer must violate, got %v", v)
	}
	if _, v := ValidateInput(schema, map[string]any{"flag": "yes"}); len(v) != 1 {
		t.Errorf("string-for-boolean must violate, got %v", v)
	}

	cleaned, v := ValidateInput(schema, map[string]any{"status": "new", "limit": float64(3), "flag": true})
	if len(v) != 0 {
		t.Fatalf("valid input violated: %v", v)
	}
	if cleaned["limit"] != 3 {
		t.Errorf("integer not coerced to int: %#v", cleaned["limit"])
	}
}

func TestValidateInputUSDCoercesToCents(t *testing.T) {
	schema := schemaWith(map[string]any{
		"price": map[string]any{"type": "number", domain.SchemaKeyUnit: string(domain.UnitUSD)},
	})
	cleaned, v := ValidateInput(schema, map[string]any{"price": 12.5})
	if len(v) != 0 {
		t.Fatalf("unexpected violations: %v", v)
	}
	if cleaned["price"] != 1250 {
		t.Errorf("12.5 USD → %v, want 1250 cents", cleaned["price"])
	}
}

func TestValidateInputDatetimeAndDate(t *testing.T) {
	schema := schemaWith(map[string]any{
		"at": map[string]any{"type": "string", domain.SchemaKeyUnit: string(domain.UnitDatetimeISO8601)},
		"on": map[string]any{"type": "string", domain.SchemaKeyUnit: string(domain.UnitDateISO8601)},
	})
	if _, v := ValidateInput(schema, map[string]any{"at": "tomorrow at noon"}); len(v) != 1 {
		t.Errorf("non-ISO datetime must violate, got %v", v)
	}
	if _, v := ValidateInput(schema, map[string]any{"at": "2026-07-27T15:00:00Z", "on": "2026-07-27"}); len(v) != 0 {
		t.Errorf("valid ISO-8601 violated: %v", v)
	}
	if _, v := ValidateInput(schema, map[string]any{"on": "27/07/2026"}); len(v) != 1 {
		t.Errorf("non-ISO date must violate, got %v", v)
	}
}

func TestValidateInputPhoneE164(t *testing.T) {
	schema := schemaWith(map[string]any{
		"phone": map[string]any{"type": "string", domain.SchemaKeyUnit: string(domain.UnitPhoneE164)},
	})
	if _, v := ValidateInput(schema, map[string]any{"phone": "415-555-0101"}); len(v) != 1 {
		t.Errorf("non-E.164 must violate, got %v", v)
	}
	if _, v := ValidateInput(schema, map[string]any{"phone": "+14155550101"}); len(v) != 0 {
		t.Errorf("valid E.164 violated: %v", v)
	}
}

func TestValidateInputNestedObjectOneLevel(t *testing.T) {
	schema := schemaWith(map[string]any{
		"filters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"brand": map[string]any{"type": "string"},
			},
			"required": []string{"brand"},
		},
	})
	cleaned, v := ValidateInput(schema, map[string]any{
		"filters": map[string]any{"brand": "COSRX", "undeclared": "dropped"},
	})
	if len(v) != 0 {
		t.Fatalf("unexpected violations: %v", v)
	}
	filters := cleaned["filters"].(map[string]any)
	if filters["brand"] != "COSRX" {
		t.Errorf("declared sub-key lost: %#v", filters)
	}
	if _, ok := filters["undeclared"]; ok {
		t.Errorf("undeclared sub-key must be dropped")
	}
	if _, v := ValidateInput(schema, map[string]any{"filters": map[string]any{}}); len(v) != 1 {
		t.Errorf("missing nested required must violate, got %v", v)
	}
}

func TestValidateInputUnknownKeysDropped(t *testing.T) {
	schema := schemaWith(map[string]any{"query": map[string]any{"type": "string"}})
	cleaned, v := ValidateInput(schema, map[string]any{"query": "x", "hallucinated": 1})
	if len(v) != 0 {
		t.Fatalf("unexpected violations: %v", v)
	}
	if _, ok := cleaned["hallucinated"]; ok {
		t.Error("undeclared top-level key must be dropped")
	}
}

func TestValidateInputArrayOfString(t *testing.T) {
	schema := schemaWith(map[string]any{
		"tags": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
	})
	if _, v := ValidateInput(schema, map[string]any{"tags": []any{"a", "b"}}); len(v) != 0 {
		t.Errorf("valid string array violated: %v", v)
	}
	if _, v := ValidateInput(schema, map[string]any{"tags": []any{"a", float64(2)}}); len(v) != 1 {
		t.Errorf("non-string item must violate, got %v", v)
	}
}

func TestValidateInputEmptySchemaPassesThrough(t *testing.T) {
	in := map[string]any{"anything": "goes"}
	cleaned, v := ValidateInput(nil, in)
	if len(v) != 0 || !reflect.DeepEqual(cleaned, in) {
		t.Errorf("nil schema must pass input through, got %v / %v", cleaned, v)
	}
}

func TestRedactSensitive(t *testing.T) {
	schema := schemaWith(map[string]any{
		"email":    map[string]any{"type": "string"},
		"password": map[string]any{"type": "string", domain.SchemaKeySensitive: true},
		"account": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"token": map[string]any{"type": "string", domain.SchemaKeySensitive: true},
				"name":  map[string]any{"type": "string"},
			},
		},
	})
	in := map[string]any{
		"email":    "a@b.c",
		"password": "hunter2",
		"account":  map[string]any{"token": "secret", "name": "acme"},
	}
	out := RedactSensitive(schema, in)

	if out["password"] != redactedPlaceholder {
		t.Errorf("password not redacted: %#v", out["password"])
	}
	sub := out["account"].(map[string]any)
	if sub["token"] != redactedPlaceholder || sub["name"] != "acme" {
		t.Errorf("nested redaction wrong: %#v", sub)
	}
	if out["email"] != "a@b.c" {
		t.Errorf("unflagged key mutated: %#v", out["email"])
	}
	// The input map must not be mutated (audit copies, callers keep originals).
	if in["password"] != "hunter2" || in["account"].(map[string]any)["token"] != "secret" {
		t.Error("RedactSensitive mutated the input map")
	}
}

func TestRedactSensitiveNoFlagsReturnsSameMap(t *testing.T) {
	schema := schemaWith(map[string]any{"q": map[string]any{"type": "string"}})
	in := map[string]any{"q": "x"}
	if out := RedactSensitive(schema, in); !reflect.DeepEqual(out, in) {
		t.Errorf("no-flag redaction changed the map: %#v", out)
	}
}
