package engine

// TextGrowth controls how a Text node sizes itself.
type TextGrowth string

const (
	TextGrowthAuto             TextGrowth = "auto"
	TextGrowthFixedWidth       TextGrowth = "fixed-width"
	TextGrowthFixedWidthHeight TextGrowth = "fixed-width-height"
)

// Text-related fields live as plain entries in the Node map; this file
// reserves the constants used across the engine.
