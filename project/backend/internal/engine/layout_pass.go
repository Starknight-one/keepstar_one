package engine

import (
	"keepstar/internal/domain"
)

// BudgetAllocation holds space budgets distributed top-down.
type BudgetAllocation struct {
	FormationWidth  int // Available width for the entire formation
	FormationHeight int // Available height (0 = unlimited/scrollable)
	WidgetWidth     int // Width per widget
	WidgetHeight    int // Max height per widget (0 = auto)
	Columns         int // Number of grid columns
}

// NeedsEstimate holds space requirements calculated bottom-up.
type NeedsEstimate struct {
	TotalHeight  int // Estimated total height of all widgets
	MaxWidgetH   int // Tallest widget height
	MinWidgetW   int // Narrowest widget minimum width
	AtomCounts   []int // Number of atoms per widget
}

// BudgetDown distributes available space from screen → formation → widget.
func BudgetDown(viewport ViewportConfig, resolved ResolvedDefaults, widgetCount int) BudgetAllocation {
	if widgetCount == 0 {
		return BudgetAllocation{}
	}

	width := viewport.Width
	if width == 0 {
		width = 400
	}

	// Calculate columns based on layout
	cols := 1
	switch resolved.Layout {
	case "grid":
		gc := CalcGridConfig(widgetCount, resolved.Size)
		cols = gc.Cols
	case "list":
		cols = 1
	case "single":
		cols = 1
	case "carousel":
		cols = widgetCount // All in one row conceptually
	}

	// Widget width = available width / columns (minus gap)
	gap := 12 // Default gap between widgets
	widgetWidth := width
	if cols > 1 {
		widgetWidth = (width - gap*(cols-1)) / cols
	}

	// Widget height based on size
	widgetHeight := 0 // 0 = auto
	switch resolved.Size {
	case domain.WidgetSizeTiny:
		widgetHeight = 100
	case domain.WidgetSizeSmall:
		widgetHeight = 200
	case domain.WidgetSizeMedium:
		widgetHeight = 340
	case domain.WidgetSizeLarge:
		widgetHeight = 460
	}

	// Formation height (from viewport, 0 = scrollable)
	formationHeight := viewport.Height

	return BudgetAllocation{
		FormationWidth:  width,
		FormationHeight: formationHeight,
		WidgetWidth:     widgetWidth,
		WidgetHeight:    widgetHeight,
		Columns:         cols,
	}
}

// NeedsUp calculates actual space requirements bottom-up from atoms.
func NeedsUp(widgets []domain.Widget, tokens DesignTokensV2) NeedsEstimate {
	if len(widgets) == 0 {
		return NeedsEstimate{}
	}

	estimate := NeedsEstimate{
		AtomCounts: make([]int, len(widgets)),
	}

	for i, w := range widgets {
		atomCount := len(w.AtomsV2)
		if atomCount == 0 {
			atomCount = len(w.Atoms)
		}
		estimate.AtomCounts[i] = atomCount

		// Estimate widget height from atom count and types
		h := estimateWidgetHeight(w, tokens)
		if h > estimate.MaxWidgetH {
			estimate.MaxWidgetH = h
		}
		estimate.TotalHeight += h
	}

	return estimate
}

// estimateWidgetHeight estimates the pixel height of a widget based on its atoms.
func estimateWidgetHeight(w domain.Widget, tokens DesignTokensV2) int {
	height := 0
	atoms := w.AtomsV2
	if len(atoms) == 0 {
		// Fallback to v1 atoms
		for _, a := range w.Atoms {
			height += estimateAtomHeight(a.Type, a.Display, tokens)
		}
		return height
	}

	for _, a := range atoms {
		height += estimateAtomV2Height(a, tokens)
	}
	return height
}

// estimateAtomV2Height estimates height of a single v2 atom.
func estimateAtomV2Height(a domain.AtomV2, tokens DesignTokensV2) int {
	if a.Type == domain.AtomTypeImage {
		return 200 // Default image height
	}

	// Text height based on fontSize token
	if a.TextStyle != nil && a.TextStyle.FontSize != "" {
		fontSize := tokens.ResolveFontSize(a.TextStyle.FontSize)
		lineHeight := int(float64(fontSize) * 1.5)

		// Add wrapper padding
		if a.Wrapper != nil {
			switch a.Wrapper.Type {
			case "badge", "tag", "pill":
				lineHeight += 8 // Vertical padding
			case "button":
				lineHeight += 16
			}
		}
		return lineHeight
	}

	return 24 // Default line height
}

// estimateAtomHeight estimates height of a v1 atom (fallback).
func estimateAtomHeight(atomType domain.AtomType, display string, tokens DesignTokensV2) int {
	if atomType == domain.AtomTypeImage {
		return 200
	}
	switch display {
	case "h1":
		return 54
	case "h2":
		return 45
	case "h3":
		return 36
	case "h4":
		return 27
	case "body-lg":
		return 27
	case "body":
		return 21
	case "body-sm", "caption":
		return 18
	case "badge", "tag":
		return 26
	case "button-primary", "button-secondary", "button-outline":
		return 40
	default:
		return 24
	}
}
