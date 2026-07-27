package tools

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"keepstar_v5/internal/domain"
)

// Entity schema generators + record validation (RUNTIME_SPEC.md §4.4) —
// the per-tenant typed-contract surface for the entity plane, same pattern
// as CatalogSearchSchemaForDigest. EntityRecordSchema feeds executor
// InputSchemas (create_record ∩ field_allowlist, §4.2); ValidateRecordData
// is what usecases.EntityWrite runs before every record write.
//
// Dialect: the restricted operation schema of §4.1 — flat props
// {type, enum, description, "x-unit", "x-sensitive"} + required[]. Schemas
// emitted here use domain.SchemaKeyUnit so the registry's unit coercion
// (usd→cents, ISO-8601, E.164) and x-sensitive redaction apply uniformly.
// FieldDef carries no sensitive flag in v1, so no entity property is
// flagged "x-sensitive" — credentials never travel through entity records
// (R6); the dialect key stays supported end to end.
//
// Money (R18, stated once): the LLM speaks DOLLARS ("x-unit": "usd"; the
// registry validator coerces to integer cents); storage and
// ValidateRecordData speak integer CENTS. A fractional money value reaching
// ValidateRecordData means un-coerced input — rejected, never rounded.

// phoneE164Shape mirrors operations/validate.go's phoneE164 — keep in sync
// (tools cannot import operations: operations wraps tools.Tool).
var phoneE164Shape = regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`)

// emailShape is a pragmatic local@domain.tld gate, not full RFC 5322.
var emailShape = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// EntityRecordSchema builds the restricted-dialect input schema for one
// entity's record data: one property per FieldDef (ordered by map
// semantics; callers needing byte-stable JSON sort keys), enum values
// resolved from the tenant's value sets, x-unit annotations per R18.
// Returns nil for a nil/fieldless definition.
func EntityRecordSchema(def *domain.EntityDefinition, sets map[string]domain.ValueSet) map[string]interface{} {
	if def == nil || len(def.Fields) == 0 {
		return nil
	}
	props := make(map[string]interface{}, len(def.Fields))
	var required []string
	for _, f := range def.Fields {
		props[f.Key] = fieldProperty(f, sets)
		if f.Required {
			required = append(required, f.Key)
		}
	}
	schema := map[string]interface{}{
		"type":       "object",
		"properties": props,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// fieldProperty maps one FieldDef onto a restricted-dialect property.
func fieldProperty(f domain.FieldDef, sets map[string]domain.ValueSet) map[string]interface{} {
	label := f.Label
	if label == "" {
		label = f.Key
	}
	switch f.Type {
	case domain.FieldNumber:
		p := map[string]interface{}{"type": "number", "description": label}
		if f.Unit != "" {
			p["description"] = fmt.Sprintf("%s (%s)", label, f.Unit)
			p[domain.SchemaKeyUnit] = string(f.Unit)
		}
		return p
	case domain.FieldMoney:
		return map[string]interface{}{
			"type":               "number",
			"description":        label + " in USD (dollars, major units)",
			domain.SchemaKeyUnit: string(domain.UnitUSD),
		}
	case domain.FieldBool:
		return map[string]interface{}{"type": "boolean", "description": label}
	case domain.FieldDate:
		return map[string]interface{}{
			"type":               "string",
			"description":        label + " (YYYY-MM-DD)",
			domain.SchemaKeyUnit: string(domain.UnitDateISO8601),
		}
	case domain.FieldDatetime:
		return map[string]interface{}{
			"type":               "string",
			"description":        label + " (ISO-8601 datetime, e.g. 2026-07-27T15:00:00Z)",
			domain.SchemaKeyUnit: string(domain.UnitDatetimeISO8601),
		}
	case domain.FieldEnum:
		p := map[string]interface{}{
			"type":               "string",
			"description":        label,
			domain.SchemaKeyUnit: string(domain.UnitEnumValueSet),
		}
		if vs, ok := sets[f.ValueSetRef]; ok {
			values := make([]string, len(vs.Values))
			for i, e := range vs.Values {
				values[i] = e.Value
			}
			p["enum"] = values
		}
		return p
	case domain.FieldPhone:
		return map[string]interface{}{
			"type":               "string",
			"description":        label + " (E.164, e.g. +14155550101)",
			domain.SchemaKeyUnit: string(domain.UnitPhoneE164),
		}
	case domain.FieldEmail:
		return map[string]interface{}{
			"type":               "string",
			"description":        label,
			domain.SchemaKeyUnit: string(domain.UnitEmail),
		}
	case domain.FieldRef:
		return map[string]interface{}{
			"type":               "string",
			"description":        fmt.Sprintf("%s — id of the linked %s", label, f.RefTarget),
			domain.SchemaKeyUnit: string(domain.UnitIDRef),
		}
	default: // FieldText
		return map[string]interface{}{"type": "string", "description": label}
	}
}

// ValidateRecordData validates data against a definition and its value
// sets. Returns (cleaned, derivedStatus, error): cleaned holds declared
// keys only (undeclared dropped — closed dialect, mirroring
// operations.ValidateInput), with defaults applied and per-type coercion
// done; derivedStatus is cleaned[def.StatusField] for the status mirror
// column. Violations aggregate into one typed RECORD_INVALID error whose
// message is human-readable — executors surface it as OutcomeInvalid so
// the LLM self-corrects.
func ValidateRecordData(def *domain.EntityDefinition, sets map[string]domain.ValueSet, data map[string]any) (map[string]any, string, error) {
	if def == nil {
		return nil, "", &domain.Error{Code: "RECORD_INVALID", Message: "nil entity definition"}
	}
	if data == nil {
		data = map[string]any{}
	}
	cleaned := make(map[string]any, len(def.Fields))
	var violations []string
	for _, f := range def.Fields {
		raw, present := data[f.Key]
		if !present || raw == nil {
			if f.Default != nil {
				raw, present = f.Default, true
			} else if f.Required {
				violations = append(violations, fmt.Sprintf("%s: missing required field", f.Key))
				continue
			} else {
				continue
			}
		}
		val, errs := validateFieldValue(f, sets, raw)
		if len(errs) > 0 {
			violations = append(violations, errs...)
			continue
		}
		cleaned[f.Key] = val
	}
	if len(violations) > 0 {
		return nil, "", &domain.Error{Code: "RECORD_INVALID", Message: strings.Join(violations, "; ")}
	}
	derivedStatus := ""
	if def.StatusField != "" {
		if s, ok := cleaned[def.StatusField].(string); ok {
			derivedStatus = s
		}
	}
	return cleaned, derivedStatus, nil
}

// validateFieldValue checks one value against its FieldDef and returns the
// coerced value. Violation messages never echo the offending value for
// free-text shapes beyond what the LLM already supplied in-context.
func validateFieldValue(f domain.FieldDef, sets map[string]domain.ValueSet, raw any) (any, []string) {
	switch f.Type {
	case domain.FieldText:
		s, ok := raw.(string)
		if !ok {
			return nil, []string{fmt.Sprintf("%s: expected string", f.Key)}
		}
		return s, nil
	case domain.FieldNumber:
		n, ok := asFloat(raw)
		if !ok {
			return nil, []string{fmt.Sprintf("%s: expected number", f.Key)}
		}
		return n, nil
	case domain.FieldMoney:
		n, ok := asFloat(raw)
		if !ok {
			return nil, []string{fmt.Sprintf("%s: expected number (integer cents)", f.Key)}
		}
		if n != float64(int64(n)) {
			// A fraction here means dollars slipped past the registry's
			// usd→cents coercion — reject, never round (R18).
			return nil, []string{fmt.Sprintf("%s: money must be integer cents, got %g", f.Key, n)}
		}
		return int(n), nil
	case domain.FieldBool:
		b, ok := raw.(bool)
		if !ok {
			return nil, []string{fmt.Sprintf("%s: expected boolean", f.Key)}
		}
		return b, nil
	case domain.FieldDate:
		s, ok := raw.(string)
		if !ok {
			return nil, []string{fmt.Sprintf("%s: expected string (YYYY-MM-DD)", f.Key)}
		}
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return nil, []string{fmt.Sprintf("%s: %q is not an ISO-8601 date (YYYY-MM-DD)", f.Key, s)}
		}
		return s, nil
	case domain.FieldDatetime:
		s, ok := raw.(string)
		if !ok {
			return nil, []string{fmt.Sprintf("%s: expected string (ISO-8601 datetime)", f.Key)}
		}
		if _, err := time.Parse(time.RFC3339, s); err != nil {
			return nil, []string{fmt.Sprintf("%s: %q is not an ISO-8601 datetime (RFC 3339, e.g. 2026-07-27T15:00:00Z)", f.Key, s)}
		}
		return s, nil
	case domain.FieldEnum:
		s, ok := raw.(string)
		if !ok {
			return nil, []string{fmt.Sprintf("%s: expected string", f.Key)}
		}
		vs, ok := sets[f.ValueSetRef]
		if !ok {
			return nil, []string{fmt.Sprintf("%s: value set %q not found", f.Key, f.ValueSetRef)}
		}
		allowed := make([]string, len(vs.Values))
		for i, e := range vs.Values {
			allowed[i] = e.Value
			if e.Value == s {
				return s, nil
			}
		}
		return nil, []string{fmt.Sprintf("%s: %q is not one of [%s]", f.Key, s, strings.Join(allowed, ", "))}
	case domain.FieldPhone:
		s, ok := raw.(string)
		if !ok {
			return nil, []string{fmt.Sprintf("%s: expected string (E.164 phone)", f.Key)}
		}
		if !phoneE164Shape.MatchString(s) {
			return nil, []string{fmt.Sprintf("%s: %q is not an E.164 phone number (e.g. +14155550101)", f.Key, s)}
		}
		return s, nil
	case domain.FieldEmail:
		s, ok := raw.(string)
		if !ok {
			return nil, []string{fmt.Sprintf("%s: expected string (email)", f.Key)}
		}
		if !emailShape.MatchString(s) {
			return nil, []string{fmt.Sprintf("%s: %q is not an email address", f.Key, s)}
		}
		return s, nil
	case domain.FieldRef:
		s, ok := raw.(string)
		if !ok || s == "" {
			return nil, []string{fmt.Sprintf("%s: expected a non-empty %s id", f.Key, f.RefTarget)}
		}
		return s, nil
	default:
		return nil, []string{fmt.Sprintf("%s: unknown field type %q", f.Key, f.Type)}
	}
}

// asFloat normalizes JSON-decoded and Go-native numerics to float64.
func asFloat(raw any) (float64, bool) {
	switch v := raw.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	}
	return 0, false
}
