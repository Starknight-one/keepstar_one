package engine

// NormalizeEffects mirrors NormalizeFills — coerces single effect or []any
// into a slice. nil → empty slice.
func NormalizeEffects(effects any) []any {
	if effects == nil {
		return []any{}
	}
	switch v := effects.(type) {
	case []any:
		return v
	}
	return []any{effects}
}
