package engine

// Fill type discriminators for the "type" key on a fill object.
const (
	FillTypeColor        = "color"
	FillTypeGradient     = "gradient"
	FillTypeImage        = "image"
	FillTypeMeshGradient = "mesh_gradient"
)

// Gradient subtype values.
const (
	GradientLinear  = "linear"
	GradientRadial  = "radial"
	GradientAngular = "angular"
)

// Image fill modes.
const (
	ImageModeStretch = "stretch"
	ImageModeFill    = "fill"
	ImageModeFit     = "fit"
)

// BlendMode values for any element that supports compositing.
const (
	BlendNormal      = "normal"
	BlendDarken      = "darken"
	BlendMultiply    = "multiply"
	BlendLinearBurn  = "linearBurn"
	BlendColorBurn   = "colorBurn"
	BlendLight       = "light"
	BlendScreen      = "screen"
	BlendLinearDodge = "linearDodge"
	BlendColorDodge  = "colorDodge"
	BlendOverlay     = "overlay"
	BlendSoftLight   = "softLight"
	BlendHardLight   = "hardLight"
	BlendDifference  = "difference"
	BlendExclusion   = "exclusion"
	BlendHue         = "hue"
	BlendSaturation  = "saturation"
	BlendColor       = "color"
	BlendLuminosity  = "luminosity"
)

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
