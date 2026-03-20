package engine

import (
	"keepstar/internal/domain"
)

// AutoLayoutSequential preserves atom order as-is (used when agent specified explicit order).
// Images get a span node, everything else goes into a column in original order.
func AutoLayoutSequential(atoms []domain.AtomV2) *domain.LayoutNode {
	if len(atoms) == 0 {
		return nil
	}
	root := &domain.LayoutNode{
		Type:    domain.LayoutNodeColumn,
		Gap:     "sm",
		Name:    "root",
		Padding: "sm",
	}
	for i, a := range atoms {
		if a.Type == domain.AtomTypeImage {
			span := &domain.LayoutNode{
				Type:         domain.LayoutNodeSpan,
				Name:         "hero",
				BorderRadius: "md",
			}
			span.Children = append(span.Children, domain.NewAtomChild(i))
			root.Children = append(root.Children, domain.NewNodeChild(span))
		} else {
			root.Children = append(root.Children, domain.NewAtomChild(i))
		}
	}
	return root
}

// AutoLayout groups AtomV2 slice into a LayoutNode tree.
// Replaces CalculateZones for the v2 engine — groups by type/subtype/wrapper
// rather than hardcoded display-string matching.
func AutoLayout(atoms []domain.AtomV2) *domain.LayoutNode {
	if len(atoms) == 0 {
		return nil
	}

	// Classification buckets (by atom index)
	var (
		heroIndices    []int
		headingIndices []int
		priceIndices   []int
		ratingIndices  []int
		flowIndices    []int
		bodyIndices    []int
		buttonIndices  []int
		iconIndices    []int
		mediaIndices   []int // video/audio
		otherIndices   []int
	)

	for i, a := range atoms {
		switch {
		// 1. Image atoms → span (hero)
		case a.Type == domain.AtomTypeImage:
			heroIndices = append(heroIndices, i)

		// 1b. Video/Audio → media group
		case a.Type == domain.AtomTypeVideo || a.Type == domain.AtomTypeAudio:
			mediaIndices = append(mediaIndices, i)

		// 2. Large text (headings) → column
		case a.TextStyle != nil && isHeadingSize(a.TextStyle.FontSize):
			headingIndices = append(headingIndices, i)

		// 3. Price → row group
		case a.Slot == domain.AtomSlotPrice || a.Subtype == domain.SubtypeCurrency:
			priceIndices = append(priceIndices, i)

		// 4. Rating → row group (merged with price)
		case a.Subtype == domain.SubtypeRating:
			ratingIndices = append(ratingIndices, i)

		// 5. Tags/badges/pills → flow
		case a.Wrapper != nil && (a.Wrapper.Type == "tag" || a.Wrapper.Type == "badge" || a.Wrapper.Type == "pill"):
			flowIndices = append(flowIndices, i)

		// 6. Buttons → row
		case a.Wrapper != nil && a.Wrapper.Type == "button":
			buttonIndices = append(buttonIndices, i)

		// 6b. Icons → flow (inline with tags)
		case a.Type == domain.AtomTypeIcon:
			iconIndices = append(iconIndices, i)

		// 7. Body text → column
		case a.Type == domain.AtomTypeText || a.Type == domain.AtomTypeNumber:
			bodyIndices = append(bodyIndices, i)

		// 8. Everything else
		default:
			otherIndices = append(otherIndices, i)
		}
	}

	// Build root layout (vertical column of groups)
	root := &domain.LayoutNode{
		Type:    domain.LayoutNodeColumn,
		Gap:     "sm",
		Name:    "root",
		Padding: "sm",
	}

	// Hero (span)
	if len(heroIndices) > 0 {
		heroNode := &domain.LayoutNode{
			Type:         domain.LayoutNodeSpan,
			Name:         "hero",
			BorderRadius: "md",
		}
		for _, idx := range heroIndices {
			heroNode.Children = append(heroNode.Children, domain.NewAtomChild(idx))
		}
		root.Children = append(root.Children, domain.NewNodeChild(heroNode))
	}

	// Media (video/audio — span, like hero)
	if len(mediaIndices) > 0 {
		mediaNode := &domain.LayoutNode{
			Type:         domain.LayoutNodeSpan,
			Name:         "media",
			BorderRadius: "md",
		}
		for _, idx := range mediaIndices {
			mediaNode.Children = append(mediaNode.Children, domain.NewAtomChild(idx))
		}
		root.Children = append(root.Children, domain.NewNodeChild(mediaNode))
	}

	// Headings (column)
	if len(headingIndices) > 0 {
		headingNode := &domain.LayoutNode{
			Type: domain.LayoutNodeColumn,
			Gap:  "xs",
			Name: "headings",
		}
		for _, idx := range headingIndices {
			headingNode.Children = append(headingNode.Children, domain.NewAtomChild(idx))
		}
		root.Children = append(root.Children, domain.NewNodeChild(headingNode))
	}

	// Price + rating (row)
	priceRatingIndices := append(priceIndices, ratingIndices...)
	if len(priceRatingIndices) > 0 {
		priceNode := &domain.LayoutNode{
			Type:         domain.LayoutNodeRow,
			Gap:          "sm",
			Align:        "center",
			Name:         "price-rating",
			Distribution: "between",
		}
		for _, idx := range priceRatingIndices {
			priceNode.Children = append(priceNode.Children, domain.NewAtomChild(idx))
		}
		root.Children = append(root.Children, domain.NewNodeChild(priceNode))
	}

	// Body text (column)
	if len(bodyIndices) > 0 {
		bodyNode := &domain.LayoutNode{
			Type: domain.LayoutNodeColumn,
			Gap:  "xs",
			Name: "body",
		}
		for _, idx := range bodyIndices {
			bodyNode.Children = append(bodyNode.Children, domain.NewAtomChild(idx))
		}
		root.Children = append(root.Children, domain.NewNodeChild(bodyNode))
	}

	// Flow (tags/badges)
	if len(flowIndices) > 0 {
		flowNode := &domain.LayoutNode{
			Type: domain.LayoutNodeFlow,
			Gap:  "xs",
			Wrap: true,
			Name: "tags",
		}

		// V2 fold: if too many, add collapse group wrapper
		if len(flowIndices) > 9 {
			flowNode.GroupWrapper = "collapse"
		}

		for _, idx := range flowIndices {
			flowNode.Children = append(flowNode.Children, domain.NewAtomChild(idx))
		}
		root.Children = append(root.Children, domain.NewNodeChild(flowNode))
	}

	// Icons (flow, inline)
	if len(iconIndices) > 0 {
		iconNode := &domain.LayoutNode{
			Type: domain.LayoutNodeFlow,
			Gap:  "xs",
			Wrap: true,
			Name: "icons",
		}
		for _, idx := range iconIndices {
			iconNode.Children = append(iconNode.Children, domain.NewAtomChild(idx))
		}
		root.Children = append(root.Children, domain.NewNodeChild(iconNode))
	}

	// Buttons (row)
	if len(buttonIndices) > 0 {
		buttonNode := &domain.LayoutNode{
			Type:         domain.LayoutNodeRow,
			Gap:          "sm",
			Name:         "actions",
			Distribution: "start",
		}
		for _, idx := range buttonIndices {
			buttonNode.Children = append(buttonNode.Children, domain.NewAtomChild(idx))
		}
		root.Children = append(root.Children, domain.NewNodeChild(buttonNode))
	}

	// Other (column)
	if len(otherIndices) > 0 {
		otherNode := &domain.LayoutNode{
			Type: domain.LayoutNodeColumn,
			Gap:  "xs",
			Name: "other",
		}
		for _, idx := range otherIndices {
			otherNode.Children = append(otherNode.Children, domain.NewAtomChild(idx))
		}
		root.Children = append(root.Children, domain.NewNodeChild(otherNode))
	}

	return root
}

// isHeadingSize returns true if a font size token represents a heading.
func isHeadingSize(fontSize string) bool {
	switch fontSize {
	case "3xl", "2xl", "xl":
		return true
	default:
		return false
	}
}
