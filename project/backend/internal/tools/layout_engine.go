package tools

import (
	"fmt"
	"strings"

	"keepstar/internal/domain"
)

// DesignTokens controls layout engine thresholds
type DesignTokens struct {
	FoldMaxVisible int // max atoms in flow zone before fold (default 9)
}

// DefaultDesignTokens returns default design tokens
func DefaultDesignTokens() DesignTokens {
	return DesignTokens{FoldMaxVisible: 9}
}

// CalculateZones classifies atoms into layout zones based on display/type/slot.
// Each atom is placed in exactly one bucket, then buckets are assembled into zones
// in a fixed visual order: hero → headings → price+rating → body → flow → buttons → other.
func CalculateZones(atoms []domain.Atom, tokens DesignTokens) []domain.Zone {
	if len(atoms) == 0 {
		return nil
	}

	// Classification buckets — each holds atom indices
	var (
		heroIndices    []int
		headingIndices []int
		priceIndices   []int
		ratingIndices  []int
		flowIndices    []int
		bodyIndices    []int
		buttonIndices  []int
		otherIndices   []int
	)

	for i, a := range atoms {
		display := a.Display

		switch {
		// 1. Image atoms → hero
		case a.Type == domain.AtomTypeImage:
			heroIndices = append(heroIndices, i)

		// 2. Headings → stack
		case display == "h1" || display == "h2" || display == "h3" || display == "h4":
			headingIndices = append(headingIndices, i)

		// 3. Price slot or price display → price group
		case a.Slot == domain.AtomSlotPrice || strings.HasPrefix(display, "price"):
			priceIndices = append(priceIndices, i)

		// 4. Rating display → rating group (merged with price into row)
		case strings.HasPrefix(display, "rating"):
			ratingIndices = append(ratingIndices, i)

		// 5. Tags and badges → flow
		case strings.HasPrefix(display, "tag") || strings.HasPrefix(display, "badge"):
			flowIndices = append(flowIndices, i)

		// 6. Body text and utility displays → body stack
		case display == "body-lg" || display == "body" || display == "body-sm" || display == "caption" ||
			display == "divider" || display == "spacer" || display == "percent" || display == "progress":
			bodyIndices = append(bodyIndices, i)

		// 7. Buttons → button row
		case strings.HasPrefix(display, "button"):
			buttonIndices = append(buttonIndices, i)

		// 8. Everything else → other stack
		default:
			otherIndices = append(otherIndices, i)
		}
	}

	// Assemble zones in fixed visual order, skipping empty groups
	var zones []domain.Zone

	// Hero zone
	if len(heroIndices) > 0 {
		zones = append(zones, domain.Zone{
			Type:        domain.ZoneHero,
			AtomIndices: heroIndices,
		})
	}

	// Headings zone (stack)
	if len(headingIndices) > 0 {
		zones = append(zones, domain.Zone{
			Type:        domain.ZoneStack,
			AtomIndices: headingIndices,
		})
	}

	// Price + rating row
	priceRatingIndices := append(priceIndices, ratingIndices...)
	if len(priceRatingIndices) > 0 {
		zones = append(zones, domain.Zone{
			Type:        domain.ZoneRow,
			AtomIndices: priceRatingIndices,
		})
	}

	// Body text (stack)
	if len(bodyIndices) > 0 {
		zones = append(zones, domain.Zone{
			Type:        domain.ZoneStack,
			AtomIndices: bodyIndices,
		})
	}

	// Flow zone (tags/badges) — with fold if exceeds threshold
	if len(flowIndices) > 0 {
		if len(flowIndices) <= tokens.FoldMaxVisible {
			zones = append(zones, domain.Zone{
				Type:        domain.ZoneFlow,
				AtomIndices: flowIndices,
			})
		} else {
			// Visible portion
			zones = append(zones, domain.Zone{
				Type:        domain.ZoneFlow,
				AtomIndices: flowIndices[:tokens.FoldMaxVisible],
				MaxVisible:  tokens.FoldMaxVisible,
			})
			// Collapsed portion
			remaining := len(flowIndices) - tokens.FoldMaxVisible
			zones = append(zones, domain.Zone{
				Type:        domain.ZoneCollapsed,
				AtomIndices: flowIndices[tokens.FoldMaxVisible:],
				FoldLabel:   fmt.Sprintf("+%d ещё", remaining),
			})
		}
	}

	// Buttons row
	if len(buttonIndices) > 0 {
		zones = append(zones, domain.Zone{
			Type:        domain.ZoneRow,
			AtomIndices: buttonIndices,
		})
	}

	// Other (stack)
	if len(otherIndices) > 0 {
		zones = append(zones, domain.Zone{
			Type:        domain.ZoneStack,
			AtomIndices: otherIndices,
		})
	}

	return zones
}
