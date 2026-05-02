package engine

// Padding constants help build padding values.
//
// v9 padding can be:
//   - a number / variable        → all four sides
//   - [v, h]                     → vertical, horizontal
//   - [top, right, bottom, left] → explicit per-side
//
// Variables (string-prefixed "$") are NOT resolved here — they must be
// resolved via VariableResolver before NormalizePadding is called, OR the
// caller must accept the 0 fallback that this function produces for
// unresolvable strings. v9 itself takes the 0 fallback.

// NormalizePadding decodes any padding shape into [top, right, bottom, left].
// Strings (variable refs) collapse to 0 in their slot — match v9 semantics.
func NormalizePadding(p any) [4]float64 {
	if p == nil {
		return [4]float64{0, 0, 0, 0}
	}
	if n, ok := AsNumber(p); ok {
		return [4]float64{n, n, n, n}
	}
	if _, ok := p.(string); ok {
		// Variable reference — caller resolves; 0 fallback.
		return [4]float64{0, 0, 0, 0}
	}
	// Slice forms.
	switch arr := p.(type) {
	case []float64:
		return paddingFromSlice(arr)
	case []any:
		nums := make([]float64, len(arr))
		for i, v := range arr {
			if n, ok := AsNumber(v); ok {
				nums[i] = n
			}
			// strings (variables) collapse to 0 — v9 behavior
		}
		return paddingFromSlice(nums)
	}
	return [4]float64{0, 0, 0, 0}
}

func paddingFromSlice(nums []float64) [4]float64 {
	switch len(nums) {
	case 2:
		v, h := nums[0], nums[1]
		return [4]float64{v, h, v, h}
	case 4:
		return [4]float64{nums[0], nums[1], nums[2], nums[3]}
	}
	return [4]float64{0, 0, 0, 0}
}
