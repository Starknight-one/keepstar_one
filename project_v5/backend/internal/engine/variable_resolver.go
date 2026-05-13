package engine

import (
	"fmt"
	"strconv"
	"strings"
)

// ResolveContext supplies the document and active theme selections used
// during variable resolution.
type ResolveContext struct {
	Document     *Document
	ActiveThemes map[string]string
}

// VariableResolver resolves $variable references against a Document.
//
// Faithful to v9: chained variables are resolved one level deep (no infinite
// loops), themed variables match by all keys present in the entry's theme
// record, fallback values are returned when a variable can't be resolved
// (magenta for color, 0 for number, true for bool, "" for string).
type VariableResolver struct{}

// NewVariableResolver returns the singleton resolver. Stateless.
func NewVariableResolver() *VariableResolver { return &VariableResolver{} }

// ResolveColor returns a concrete color hex. Unresolved variables collapse
// to "#FF00FFFF" (opaque magenta) — matches v9 semantics.
func (r *VariableResolver) ResolveColor(value any, ctx *ResolveContext) string {
	if !IsVariable(value) {
		s, _ := value.(string)
		return s
	}
	resolved := r.resolveVariable(value.(string), ctx)
	if s, ok := resolved.(string); ok && !IsVariable(s) {
		return s
	}
	return "#FF00FFFF"
}

// ResolveNumber returns a concrete float64.
func (r *VariableResolver) ResolveNumber(value any, ctx *ResolveContext) float64 {
	if n, ok := AsNumber(value); ok {
		return n
	}
	if !IsVariable(value) {
		// String that's not a $variable — try parse as number
		if s, ok := value.(string); ok {
			n, err := strconv.ParseFloat(s, 64)
			if err == nil {
				return n
			}
		}
		return 0
	}
	resolved := r.resolveVariable(value.(string), ctx)
	if n, ok := AsNumber(resolved); ok {
		return n
	}
	if s, ok := resolved.(string); ok {
		n, err := strconv.ParseFloat(s, 64)
		if err == nil {
			return n
		}
	}
	return 0
}

// ResolveBoolean returns a concrete bool.
func (r *VariableResolver) ResolveBoolean(value any, ctx *ResolveContext) bool {
	if b, ok := value.(bool); ok {
		return b
	}
	if !IsVariable(value) {
		if s, ok := value.(string); ok {
			return s == "true"
		}
		return false
	}
	resolved := r.resolveVariable(value.(string), ctx)
	if b, ok := resolved.(bool); ok {
		return b
	}
	if s, ok := resolved.(string); ok {
		return s == "true"
	}
	return false
}

// ResolveString returns a concrete string.
func (r *VariableResolver) ResolveString(value any, ctx *ResolveContext) string {
	if !IsVariable(value) {
		s, _ := value.(string)
		return s
	}
	resolved := r.resolveVariable(value.(string), ctx)
	if resolved == nil {
		s, _ := value.(string)
		return s
	}
	switch v := resolved.(type) {
	case string:
		return v
	case bool:
		if v {
			return "true"
		}
		return "false"
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64)
	}
	return fmt.Sprintf("%v", resolved)
}

// Resolve passes plain values through unchanged; variable references are
// resolved against ctx and returned (or returned unchanged if unresolvable).
func (r *VariableResolver) Resolve(value any, ctx *ResolveContext) any {
	if IsVariable(value) {
		if resolved := r.resolveVariable(value.(string), ctx); resolved != nil {
			return resolved
		}
	}
	return value
}

// ListVariables returns all variable names defined in the document.
func (r *VariableResolver) ListVariables(doc *Document) []string {
	if doc == nil || doc.Variables == nil {
		return nil
	}
	out := make([]string, 0, len(doc.Variables))
	for name := range doc.Variables {
		out = append(out, name)
	}
	return out
}

// GetVariableValue returns the resolved value of a variable by name, or nil
// if the variable doesn't exist.
func (r *VariableResolver) GetVariableValue(name string, ctx *ResolveContext) any {
	if ctx == nil || ctx.Document == nil || ctx.Document.Variables == nil {
		return nil
	}
	def, ok := ctx.Document.Variables[name]
	if !ok {
		return nil
	}
	return r.resolveVariableDef(def, ctx)
}

// resolveVariable looks up "$name" in ctx.Document.Variables and resolves
// the def. Returns nil if unknown.
func (r *VariableResolver) resolveVariable(ref string, ctx *ResolveContext) any {
	if ctx == nil || ctx.Document == nil || ctx.Document.Variables == nil {
		return nil
	}
	name := strings.TrimPrefix(ref, "$")
	def, ok := ctx.Document.Variables[name]
	if !ok {
		return nil
	}
	return r.resolveVariableDef(def, ctx)
}

// resolveVariableDef returns the concrete value for a VariableDef, taking
// active theme selections into account. Single-level chained variable
// resolution.
func (r *VariableResolver) resolveVariableDef(def VariableDef, ctx *ResolveContext) any {
	value := def.Value

	// Direct (non-themed) value: scalar.
	if !isThemedValueArray(value) {
		// Chained variable? Resolve once.
		if s, ok := value.(string); ok && IsVariable(s) {
			inner := r.resolveVariable(s, ctx)
			if inner != nil {
				return inner
			}
			return s
		}
		return value
	}

	// Themed value array: best match.
	themed := toThemedValues(value)
	if len(themed) == 0 {
		return fallbackForType(def.Type)
	}
	matched := findThemedMatch(themed, ctx.ActiveThemes)
	raw := matched.Value
	if s, ok := raw.(string); ok && IsVariable(s) {
		inner := r.resolveVariable(s, ctx)
		if inner != nil {
			return inner
		}
		return s
	}
	return raw
}

// isThemedValueArray reports whether v is a []ThemedValue or []any of
// theme-shaped objects.
func isThemedValueArray(v any) bool {
	switch t := v.(type) {
	case []ThemedValue:
		return true
	case []any:
		// Check if first element looks like ThemedValue.
		if len(t) == 0 {
			return false
		}
		_, isMap := t[0].(map[string]any)
		_, isTV := t[0].(ThemedValue)
		return isMap || isTV
	}
	return false
}

// toThemedValues coerces a value into []ThemedValue. Accepts native
// []ThemedValue or []any of map[string]any (from JSON unmarshal).
func toThemedValues(v any) []ThemedValue {
	if tv, ok := v.([]ThemedValue); ok {
		return tv
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]ThemedValue, 0, len(arr))
	for _, item := range arr {
		if tv, ok := item.(ThemedValue); ok {
			out = append(out, tv)
			continue
		}
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		var entry ThemedValue
		entry.Value = m["value"]
		if t, ok := m["theme"].(map[string]string); ok {
			entry.Theme = t
		} else if t, ok := m["theme"].(map[string]any); ok {
			conv := make(map[string]string, len(t))
			for k, x := range t {
				if s, ok := x.(string); ok {
					conv[k] = s
				}
			}
			entry.Theme = conv
		}
		out = append(out, entry)
	}
	return out
}

// findThemedMatch returns the entry whose theme matches all active
// selections. Falls back to first untyped, then to first entry overall.
func findThemedMatch(values []ThemedValue, active map[string]string) ThemedValue {
	for _, entry := range values {
		if entry.Theme == nil {
			continue
		}
		matches := true
		for k, v := range entry.Theme {
			if active[k] != v {
				matches = false
				break
			}
		}
		if matches {
			return entry
		}
	}
	for _, entry := range values {
		if len(entry.Theme) == 0 {
			return entry
		}
	}
	return values[0]
}

func fallbackForType(t VariableType) any {
	switch t {
	case VariableTypeColor:
		return "#FF00FFFF"
	case VariableTypeNumber:
		return float64(0)
	case VariableTypeBoolean:
		return true
	case VariableTypeString:
		return ""
	}
	return nil
}
