package engine

import (
	"fmt"
	"strconv"
	"strings"
)

// Color is a hex string: "#RGB", "#RRGGBB", or "#RRGGBBAA".
type Color = string

// IsVariable reports whether v is a variable reference like "$name".
func IsVariable(v any) bool {
	s, ok := v.(string)
	return ok && strings.HasPrefix(s, "$")
}

// ParsedColor holds normalized RGBA channels in [0,1].
type ParsedColor struct {
	R, G, B, A float64
}

// ParseColor parses a 3/6/8-digit hex color string into normalized RGBA.
// Faithful port of v9's parseColor. Returns ParsedColor + nil on success;
// returns zero-value + error if length/parse fails.
func ParseColor(hex string) (ParsedColor, error) {
	h := strings.TrimPrefix(hex, "#")
	if len(h) == 3 {
		h = string(h[0]) + string(h[0]) + string(h[1]) + string(h[1]) + string(h[2]) + string(h[2])
	}
	if len(h) != 6 && len(h) != 8 {
		return ParsedColor{}, fmt.Errorf("color: invalid length %d for %q (expected 3, 6, or 8 hex digits)", len(h), hex)
	}
	r, err := parseHexByte(h[0:2])
	if err != nil {
		return ParsedColor{}, fmt.Errorf("color: %w", err)
	}
	g, err := parseHexByte(h[2:4])
	if err != nil {
		return ParsedColor{}, fmt.Errorf("color: %w", err)
	}
	b, err := parseHexByte(h[4:6])
	if err != nil {
		return ParsedColor{}, fmt.Errorf("color: %w", err)
	}
	a := 1.0
	if len(h) == 8 {
		av, err := parseHexByte(h[6:8])
		if err != nil {
			return ParsedColor{}, fmt.Errorf("color: %w", err)
		}
		a = av
	}
	return ParsedColor{R: r, G: g, B: b, A: a}, nil
}

func parseHexByte(s string) (float64, error) {
	v, err := strconv.ParseUint(s, 16, 8)
	if err != nil {
		return 0, err
	}
	return float64(v) / 255.0, nil
}

// AsBool unwraps a BooleanOrVariable. Returns (resolved, true) for a literal
// bool; (false, false) for a variable reference (caller must resolve via
// VariableResolver) or any other shape.
func AsBool(v any) (bool, bool) {
	if b, ok := v.(bool); ok {
		return b, true
	}
	return false, false
}

// AsNumber unwraps a NumberOrVariable. Accepts float64/int/int64 literals.
// Variable references and other shapes return (0, false).
func AsNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	}
	return 0, false
}

// AsString unwraps a StringOrVariable as a literal string. A variable
// reference (starting with "$") returns (s, false) — caller must resolve.
// Returns ("", false) for non-strings.
func AsString(v any) (string, bool) {
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	if strings.HasPrefix(s, "$") {
		return s, false
	}
	return s, true
}
