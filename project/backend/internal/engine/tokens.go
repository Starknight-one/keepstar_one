package engine

// DesignTokensV2 maps semantic token names to resolved numeric values.
// The LLM uses tokens ("lg", "md"); the engine resolves them to pixels/numbers.
type DesignTokensV2 struct {
	FontSize  map[string]int `json:"fontSize"`  // "xs"→10, "sm"→12, "md"→14, "lg"→18, "xl"→24, "2xl"→30, "3xl"→36
	FontWeight map[string]int `json:"fontWeight"` // "light"→300, "normal"→400, "medium"→500, "semibold"→600, "bold"→700
	Spacing   map[string]int `json:"spacing"`   // "none"→0, "xs"→2, "sm"→4, "md"→8, "lg"→12, "xl"→16, "2xl"→24
	Radius    map[string]int `json:"radius"`    // "none"→0, "sm"→4, "md"→8, "lg"→12, "full"→9999
	IconSize  map[string]int `json:"iconSize"`  // "sm"→16, "md"→20, "lg"→24
}

// DefaultDesignTokensV2 returns the default token resolution map.
func DefaultDesignTokensV2() DesignTokensV2 {
	return DesignTokensV2{
		FontSize: map[string]int{
			"xs":  10,
			"sm":  12,
			"md":  14,
			"lg":  18,
			"xl":  24,
			"2xl": 30,
			"3xl": 36,
		},
		FontWeight: map[string]int{
			"light":    300,
			"normal":   400,
			"medium":   500,
			"semibold": 600,
			"bold":     700,
		},
		Spacing: map[string]int{
			"none": 0,
			"xs":   2,
			"sm":   4,
			"md":   8,
			"lg":   12,
			"xl":   16,
			"2xl":  24,
		},
		Radius: map[string]int{
			"none": 0,
			"sm":   4,
			"md":   8,
			"lg":   12,
			"full": 9999,
		},
		IconSize: map[string]int{
			"sm": 16,
			"md": 20,
			"lg": 24,
		},
	}
}

// ResolveFontSize resolves a semantic token or returns a fallback.
func (t DesignTokensV2) ResolveFontSize(token string) int {
	if v, ok := t.FontSize[token]; ok {
		return v
	}
	return t.FontSize["md"]
}

// ResolveSpacing resolves a spacing token or returns a fallback.
func (t DesignTokensV2) ResolveSpacing(token string) int {
	if v, ok := t.Spacing[token]; ok {
		return v
	}
	return t.Spacing["md"]
}

// ResolveFontWeight resolves a font weight token.
func (t DesignTokensV2) ResolveFontWeight(token string) int {
	if v, ok := t.FontWeight[token]; ok {
		return v
	}
	return t.FontWeight["normal"]
}
