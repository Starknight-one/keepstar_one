package engine

import (
	"regexp"
	"strconv"
	"strings"
)

// SizingMode describes how a Size value should be interpreted by the layout
// engine.
type SizingMode string

const (
	SizingFixed SizingMode = "fixed"
	SizingFill  SizingMode = "fill"
	SizingFit   SizingMode = "fit"
)

// ParsedSizing is the result of decoding a Size value.
type ParsedSizing struct {
	Mode     SizingMode
	Fallback float64
}

var sizingFallbackRE = regexp.MustCompile(`\((\d+)\)`)

// ParseSizing decodes a Size value. Accepts:
//   - nil → fixed/0
//   - number (any numeric) → fixed/value
//   - "fill_container" / "fill_container(N)" → fill/N (or 0)
//   - "fit_content"     / "fit_content(N)"   → fit/N  (or 0)
//   - other string → fixed/0
func ParseSizing(v any) ParsedSizing {
	if v == nil {
		return ParsedSizing{Mode: SizingFixed, Fallback: 0}
	}
	if n, ok := AsNumber(v); ok {
		return ParsedSizing{Mode: SizingFixed, Fallback: n}
	}
	s, ok := v.(string)
	if !ok {
		return ParsedSizing{Mode: SizingFixed, Fallback: 0}
	}
	if strings.HasPrefix(s, "fill_container") {
		return ParsedSizing{Mode: SizingFill, Fallback: extractFallback(s)}
	}
	if strings.HasPrefix(s, "fit_content") {
		return ParsedSizing{Mode: SizingFit, Fallback: extractFallback(s)}
	}
	return ParsedSizing{Mode: SizingFixed, Fallback: 0}
}

func extractFallback(s string) float64 {
	m := sizingFallbackRE.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	n, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0
	}
	return n
}
