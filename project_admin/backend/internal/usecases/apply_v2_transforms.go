// Package usecases — apply_v2_transforms holds the deterministic side of
// apply_v2: path lookup on raw JSONB, transform engine, and per-target
// value coercion. No DB access here.
package usecases

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// getPath fetches a value out of a parsed JSONB map by a "a.b.c" dotted
// path. Returns nil if any segment is missing or not a map at intermediate
// levels. Supports top-level only when path has no dots.
//
// We intentionally keep this simple — no array indexing, no jsonpath. If
// the agent needs nested structure access it should map the parent and let
// apply route to tier3.
func getPath(raw map[string]any, path string) any {
	if path == "" || raw == nil {
		return nil
	}
	if !strings.Contains(path, ".") {
		return raw[path]
	}
	parts := strings.Split(path, ".")
	cur := any(raw)
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = m[p]
		if cur == nil {
			return nil
		}
	}
	return cur
}

// applyTransform runs a named transform on an input value. transform may be
// empty (no-op). Returns the transformed value or an error if the input is
// incompatible with the transform.
//
// Supported names: lowercase, trim, split:<delim>, ml_from_string,
// g_from_string, bool_from_yesno, int, numeric.
func applyV2Transform(in any, transform string) (any, error) {
	if transform == "" {
		return in, nil
	}
	switch {
	case transform == "lowercase":
		s, ok := asString(in)
		if !ok {
			return nil, fmt.Errorf("lowercase: not a string (%T)", in)
		}
		return strings.ToLower(s), nil

	case transform == "trim":
		s, ok := asString(in)
		if !ok {
			return nil, fmt.Errorf("trim: not a string (%T)", in)
		}
		return strings.TrimSpace(s), nil

	case strings.HasPrefix(transform, "split:"):
		s, ok := asString(in)
		if !ok {
			return nil, fmt.Errorf("split: not a string (%T)", in)
		}
		delim := strings.TrimPrefix(transform, "split:")
		if delim == "" {
			delim = ","
		}
		parts := strings.Split(s, delim)
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			t := strings.TrimSpace(p)
			if t != "" {
				out = append(out, t)
			}
		}
		return out, nil

	case transform == "ml_from_string":
		return parseUnitFromString(in, []string{"ml", "milliliter", "millilitres"})

	case transform == "g_from_string":
		return parseUnitFromString(in, []string{"g", "gram", "gr"})

	case transform == "bool_from_yesno":
		s, ok := asString(in)
		if !ok {
			return nil, fmt.Errorf("bool_from_yesno: not a string (%T)", in)
		}
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "yes", "true", "1", "y", "t":
			return true, nil
		case "no", "false", "0", "n", "f", "":
			return false, nil
		default:
			return nil, fmt.Errorf("bool_from_yesno: cannot parse %q", s)
		}

	case transform == "int":
		s, ok := asString(in)
		if !ok {
			// already a number?
			if f, ok := in.(float64); ok {
				return int(f), nil
			}
			return nil, fmt.Errorf("int: not a string or number (%T)", in)
		}
		return strconv.Atoi(strings.TrimSpace(s))

	case transform == "numeric":
		s, ok := asString(in)
		if !ok {
			if f, ok := in.(float64); ok {
				return f, nil
			}
			return nil, fmt.Errorf("numeric: not a string or number (%T)", in)
		}
		return strconv.ParseFloat(strings.TrimSpace(s), 64)

	default:
		// Unknown transform — pass through. apply_v2 logs a warning.
		return in, nil
	}
}

// unitRegex captures the first numeric run from strings like "30 ml" or "200g".
var unitRegex = regexp.MustCompile(`(-?\d+(?:\.\d+)?)\s*([A-Za-z]+)?`)

// parseUnitFromString extracts the integer value from strings like "30 ml".
// hintUnits is currently unused beyond documenting intent; the regex grabs
// the first number it sees regardless of unit. Result is int (rounded down
// for fractional). Returns nil for non-string or no-numeric inputs.
func parseUnitFromString(in any, _ []string) (any, error) {
	s, ok := asString(in)
	if !ok {
		// already a number?
		if f, ok := in.(float64); ok {
			return int(f), nil
		}
		return nil, fmt.Errorf("unit_from_string: not a string or number (%T)", in)
	}
	m := unitRegex.FindStringSubmatch(s)
	if m == nil {
		return nil, fmt.Errorf("unit_from_string: no number in %q", s)
	}
	f, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return nil, fmt.Errorf("unit_from_string: parse %q: %w", m[1], err)
	}
	return int(f), nil
}

// asString coerces common JSON types to a string representation.
func asString(in any) (string, bool) {
	switch v := in.(type) {
	case nil:
		return "", false
	case string:
		return v, true
	case float64:
		// JSON numbers come as float64 from encoding/json. Format without
		// the trailing decimal for ints.
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10), true
		}
		return strconv.FormatFloat(v, 'f', -1, 64), true
	case bool:
		return strconv.FormatBool(v), true
	default:
		return fmt.Sprintf("%v", in), true
	}
}

// asInt coerces an arbitrary value into an int, returning ok=false on
// failure. Used to extract listing-side numerics (price, stock).
func asInt(in any) (int, bool) {
	switch v := in.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(v))
		if err == nil {
			return i, true
		}
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err == nil {
			return int(f), true
		}
	}
	return 0, false
}

// asStringSlice tries to coerce a value into a []string. Accepts []string
// directly (output of split:), []any of strings (raw JSON arrays), or one
// string (wraps as single-element slice).
func asStringSlice(in any) ([]string, bool) {
	switch v := in.(type) {
	case nil:
		return nil, false
	case []string:
		return v, true
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			s, ok := asString(x)
			if !ok {
				continue
			}
			out = append(out, s)
		}
		return out, true
	case string:
		return []string{v}, true
	}
	return nil, false
}
