package engine

// Effect type discriminators.
const (
	EffectBlur           = "blur"
	EffectBackgroundBlur = "background_blur"
	EffectShadow         = "shadow"
)

// Shadow subtype values.
const (
	ShadowInner = "inner"
	ShadowOuter = "outer"
)

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
