package operations

import (
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"keepstar_v5/internal/domain"
)

// Input validation + coercion for the restricted operation schema dialect
// (RUNTIME_SPEC.md §4.1): flat property maps {type, enum, description,
// "x-unit", "x-sensitive"} + required[], one nesting level for a
// filters-style sub-object, array-of-string. Violations surface as
// OutcomeInvalid (IsError tool_result) so the LLM self-corrects.
//
// Unit coercion (R18): usd → integer cents; datetime_iso8601 /
// date_iso8601 → parse check; phone_e164 → shape check. Everything else is
// type-checked only — money storage is cents, the LLM speaks dollars.

// phoneE164 is the E.164 shape gate: leading +, 7-15 digits total.
var phoneE164 = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)

// ValidateInput validates and coerces input against a restricted-dialect
// schema. Returns the cleaned input (declared keys only, units coerced) and
// the list of human-readable violations (empty = valid). A nil/empty schema
// validates everything (cleaned = input verbatim).
func ValidateInput(schema, input map[string]any) (map[string]any, []string) {
	if input == nil {
		input = map[string]any{}
	}
	props, _ := schema["properties"].(map[string]any)
	if props == nil {
		return input, nil
	}
	var violations []string

	for _, req := range requiredKeys(schema) {
		if _, ok := input[req]; !ok {
			violations = append(violations, fmt.Sprintf("missing required property %q", req))
		}
	}

	cleaned := make(map[string]any, len(input))
	for key, raw := range input {
		propRaw, declared := props[key]
		if !declared {
			continue // undeclared keys are dropped silently (dialect is closed)
		}
		prop, _ := propRaw.(map[string]any)
		val, errs := validateValue(key, prop, raw, true)
		if len(errs) > 0 {
			violations = append(violations, errs...)
			continue
		}
		cleaned[key] = val
	}
	return cleaned, violations
}

// validateValue checks one value against its property schema and returns
// the (possibly coerced) value. allowNesting permits exactly one level of
// sub-object recursion (the filters-style sub-object).
func validateValue(path string, prop map[string]any, raw any, allowNesting bool) (any, []string) {
	if prop == nil {
		return raw, nil
	}
	typ, _ := prop["type"].(string)
	switch typ {
	case "string":
		s, ok := raw.(string)
		if !ok {
			return nil, []string{fmt.Sprintf("%s: expected string", path)}
		}
		if enum := enumValues(prop); enum != nil && !containsString(enum, s) {
			return nil, []string{fmt.Sprintf("%s: %q is not one of [%s]", path, s, strings.Join(enum, ", "))}
		}
		return coerceStringUnit(path, prop, s)
	case "integer":
		f, ok := raw.(float64)
		if !ok {
			return nil, []string{fmt.Sprintf("%s: expected integer", path)}
		}
		if f != math.Trunc(f) {
			return nil, []string{fmt.Sprintf("%s: expected integer, got %g", path, f)}
		}
		return int(f), nil
	case "number":
		f, ok := raw.(float64)
		if !ok {
			return nil, []string{fmt.Sprintf("%s: expected number", path)}
		}
		if unitOf(prop) == domain.UnitUSD {
			// LLM speaks dollars; storage and executors speak integer cents.
			return int(math.Round(f * 100)), nil
		}
		return f, nil
	case "boolean":
		b, ok := raw.(bool)
		if !ok {
			return nil, []string{fmt.Sprintf("%s: expected boolean", path)}
		}
		return b, nil
	case "array":
		items, ok := raw.([]any)
		if !ok {
			return nil, []string{fmt.Sprintf("%s: expected array", path)}
		}
		itemProp, _ := prop["items"].(map[string]any)
		out := make([]any, 0, len(items))
		var errs []string
		for i, item := range items {
			v, e := validateValue(fmt.Sprintf("%s[%d]", path, i), itemProp, item, false)
			if len(e) > 0 {
				errs = append(errs, e...)
				continue
			}
			out = append(out, v)
		}
		if len(errs) > 0 {
			return nil, errs
		}
		return out, nil
	case "object":
		obj, ok := raw.(map[string]any)
		if !ok {
			return nil, []string{fmt.Sprintf("%s: expected object", path)}
		}
		subProps, _ := prop["properties"].(map[string]any)
		if subProps == nil || !allowNesting {
			return obj, nil // open sub-object (schema is advisory below this point)
		}
		var errs []string
		cleaned := make(map[string]any, len(obj))
		for k, v := range obj {
			spRaw, declared := subProps[k]
			if !declared {
				continue
			}
			sp, _ := spRaw.(map[string]any)
			val, e := validateValue(path+"."+k, sp, v, false)
			if len(e) > 0 {
				errs = append(errs, e...)
				continue
			}
			cleaned[k] = val
		}
		for _, req := range requiredKeys(prop) {
			if _, ok := obj[req]; !ok {
				errs = append(errs, fmt.Sprintf("%s: missing required property %q", path, req))
			}
		}
		if len(errs) > 0 {
			return nil, errs
		}
		return cleaned, nil
	default:
		return raw, nil // untyped property — pass through
	}
}

// coerceStringUnit applies the string-typed unit checks (R18): ISO-8601
// parse for datetime/date, E.164 shape for phone. Values pass through
// unmodified on success — executors re-parse as needed.
func coerceStringUnit(path string, prop map[string]any, s string) (any, []string) {
	switch unitOf(prop) {
	case domain.UnitDatetimeISO8601:
		if _, err := time.Parse(time.RFC3339, s); err != nil {
			return nil, []string{fmt.Sprintf("%s: %q is not an ISO-8601 datetime (RFC 3339, e.g. 2026-07-27T15:00:00Z)", path, s)}
		}
	case domain.UnitDateISO8601:
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return nil, []string{fmt.Sprintf("%s: %q is not an ISO-8601 date (YYYY-MM-DD)", path, s)}
		}
	case domain.UnitPhoneE164:
		if !phoneE164.MatchString(s) {
			return nil, []string{fmt.Sprintf("%s: %q is not an E.164 phone number (e.g. +14155550101)", path, s)}
		}
	}
	return s, nil
}

// ValidateOutput type-checks an executor's output against its OutputSchema.
// Log-only fail-loud at the registry boundary: returns violations, never
// blocks the result.
func ValidateOutput(schema, output map[string]any) []string {
	props, _ := schema["properties"].(map[string]any)
	if props == nil || output == nil {
		return nil
	}
	var violations []string
	for key, propRaw := range props {
		raw, ok := output[key]
		if !ok || raw == nil {
			continue
		}
		prop, _ := propRaw.(map[string]any)
		typ, _ := prop["type"].(string)
		okType := true
		switch typ {
		case "string":
			_, okType = raw.(string)
		case "integer", "number":
			switch raw.(type) {
			case float64, int, int64:
			default:
				okType = false
			}
		case "boolean":
			_, okType = raw.(bool)
		case "array":
			_, okType = raw.([]any)
		case "object":
			_, okType = raw.(map[string]any)
		}
		if !okType {
			violations = append(violations, fmt.Sprintf("%s: expected %s, got %T", key, typ, raw))
		}
	}
	return violations
}

// redactedPlaceholder replaces every x-sensitive value before persistence
// (R6). The key survives so audit rows show THAT a value was supplied,
// never WHAT it was.
const redactedPlaceholder = "[REDACTED]"

// RedactSensitive returns a copy of m with every "x-sensitive"-flagged key
// of schema replaced by the redaction placeholder. Paths are "key" or
// "parent.child" (one nesting level, per domain.SensitiveKeys). Returns m
// unchanged (no copy) when nothing is flagged or m is nil.
func RedactSensitive(schema, m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	paths := domain.SensitiveKeys(schema)
	if len(paths) == 0 {
		return m
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	for _, p := range paths {
		parent, child, nested := strings.Cut(p, ".")
		if !nested {
			if _, ok := out[parent]; ok {
				out[parent] = redactedPlaceholder
			}
			continue
		}
		sub, ok := out[parent].(map[string]any)
		if !ok {
			continue
		}
		if _, ok := sub[child]; !ok {
			continue
		}
		subCopy := make(map[string]any, len(sub))
		for k, v := range sub {
			subCopy[k] = v
		}
		subCopy[child] = redactedPlaceholder
		out[parent] = subCopy
	}
	return out
}

// requiredKeys reads schema["required"] tolerating both []string (Go-built
// schemas) and []any (JSON-decoded schemas).
func requiredKeys(schema map[string]any) []string {
	switch req := schema["required"].(type) {
	case []string:
		return req
	case []any:
		out := make([]string, 0, len(req))
		for _, r := range req {
			if s, ok := r.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// enumValues reads prop["enum"] tolerating []string and []any. nil = no enum.
func enumValues(prop map[string]any) []string {
	switch e := prop["enum"].(type) {
	case []string:
		return e
	case []any:
		out := make([]string, 0, len(e))
		for _, v := range e {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func unitOf(prop map[string]any) domain.UnitName {
	u, _ := prop[domain.SchemaKeyUnit].(string)
	return domain.UnitName(u)
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
