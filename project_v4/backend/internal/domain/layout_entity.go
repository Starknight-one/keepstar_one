package domain

// LayoutNodeType defines the layout grouping type in the v2 layout tree.
type LayoutNodeType string

const (
	LayoutNodeRow    LayoutNodeType = "row"    // Horizontal flex
	LayoutNodeColumn LayoutNodeType = "column" // Vertical flex (stack)
	LayoutNodeFlow   LayoutNodeType = "flow"   // Flex-wrap (tags, badges)
	LayoutNodeSpan   LayoutNodeType = "span"   // Full-width spanning element
)

// LayoutNode represents a node in the v2 layout tree.
// Replaces the flat Zone[] array with a recursive structure.
type LayoutNode struct {
	ID           string         `json:"id,omitempty"`   // Stable addressable ID for tree operations
	Type         LayoutNodeType `json:"type"`
	Children     []LayoutChild  `json:"children"`
	Gap          string         `json:"gap,omitempty"`          // Semantic token: "none","xs","sm","md","lg","xl"
	Align        string         `json:"align,omitempty"`        // "start","center","end","stretch","baseline"
	Wrap         bool           `json:"wrap,omitempty"`         // Allow wrapping (for flow)
	GroupWrapper string         `json:"groupWrapper,omitempty"` // "collapse","carousel" (v1 scope)
	Rigidity     Rigidity       `json:"rigidity,omitempty"`
	Name         string         `json:"name,omitempty"`         // Optional semantic name for debugging

	// Visual container properties
	Background   string `json:"background,omitempty"`   // CSS background value
	BorderRadius string `json:"borderRadius,omitempty"` // Token: "none","sm","md","lg","xl","full"
	Shadow       string `json:"shadow,omitempty"`       // Token: "none","sm","md","lg"
	Padding      string `json:"padding,omitempty"`      // Token: "none","xs","sm","md","lg","xl"
	Border       string `json:"border,omitempty"`       // "1px solid #E5E7EB"
	BorderColor  string `json:"borderColor,omitempty"`  // Token or hex
	BorderWidth  string `json:"borderWidth,omitempty"`  // "1px", "2px"
	Opacity      string `json:"opacity,omitempty"`      // "0"-"100" (percentage)
	Overflow     string `json:"overflow,omitempty"`     // "truncate"/"wrap"/"scroll"/"hide"/"visible"

	// Layout distribution
	Distribution string `json:"distribution,omitempty"` // "start","center","end","between","around","evenly"
}

// LayoutChild is either an atom reference or a nested layout node.
// Exactly one of AtomIndex or Node must be set.
type LayoutChild struct {
	AtomIndex *int        `json:"atomIndex,omitempty"` // Index into widget's Atoms slice
	Node      *LayoutNode `json:"node,omitempty"`      // Nested layout group
	Grow      int         `json:"grow,omitempty"`      // flex-grow
	SelfAlign string      `json:"selfAlign,omitempty"` // align-self override
	Sizing    string      `json:"sizing,omitempty"`    // "hug" / "fill" / "fixed"
	MinWidth  string      `json:"minWidth,omitempty"`
	MaxWidth  string      `json:"maxWidth,omitempty"`
	MinHeight string      `json:"minHeight,omitempty"`
	MaxHeight string      `json:"maxHeight,omitempty"`
}

// NewAtomChild creates a LayoutChild referencing an atom by index.
func NewAtomChild(index int) LayoutChild {
	return LayoutChild{AtomIndex: &index}
}

// NewNodeChild creates a LayoutChild containing a nested layout node.
func NewNodeChild(node *LayoutNode) LayoutChild {
	return LayoutChild{Node: node}
}

// WidgetActionType defines a user interaction on a widget (v2 engine).
// Named differently from state_entity.ActionType which represents pipeline actions.
type WidgetActionType string

const (
	WidgetActionOpenDetail WidgetActionType = "open_detail"
	WidgetActionNavigate   WidgetActionType = "navigate"
	WidgetActionShowMore   WidgetActionType = "show_more"
	WidgetActionAddToCart  WidgetActionType = "add_to_cart"
	WidgetActionQuantity   WidgetActionType = "quantity"
	WidgetActionQuickBuy   WidgetActionType = "quick_buy"
	WidgetActionFavorite   WidgetActionType = "favorite"
	WidgetActionCompare    WidgetActionType = "compare"
	WidgetActionLike       WidgetActionType = "like"
	WidgetActionShare      WidgetActionType = "share"
	WidgetActionCopyLink   WidgetActionType = "copy_link"
	WidgetActionHelpful    WidgetActionType = "helpful"
	WidgetActionReport     WidgetActionType = "report"
)

// ActionDef defines a user action attached to a widget (v2 engine).
type ActionDef struct {
	Type    WidgetActionType       `json:"type"`
	Label   string                 `json:"label,omitempty"`
	Icon    string                 `json:"icon,omitempty"`
	Params  map[string]interface{} `json:"params,omitempty"`
}

// WidgetStates defines hover/active visual state overrides.
type WidgetStates struct {
	Hover  map[string]string `json:"hover,omitempty"`  // CSS variable overrides on hover
	Active map[string]string `json:"active,omitempty"` // CSS variable overrides on active/click
}
