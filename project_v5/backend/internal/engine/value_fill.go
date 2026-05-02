package engine

// Fills (single fill or array) appear on Frame/Rectangle/Ellipse/etc.
// In the engine layer we keep them as untyped any — v9's TS treats them as
// loose union types. NormalizeFills coerces shape for downstream renderers.

// NormalizeFills returns the input as a []any, coercing both single-fill and
// array shapes. Returns an empty slice for nil.
func NormalizeFills(fills any) []any {
	if fills == nil {
		return []any{}
	}
	switch v := fills.(type) {
	case []any:
		return v
	}
	return []any{fills}
}
