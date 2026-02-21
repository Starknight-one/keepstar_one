package domain

// WidgetType defines the type of composed widget (legacy)
type WidgetType string

const (
	WidgetTypeProductCard     WidgetType = "product_card"
	WidgetTypeProductList     WidgetType = "product_list"
	WidgetTypeComparisonTable WidgetType = "comparison_table"
	WidgetTypeImageCarousel   WidgetType = "image_carousel"
	WidgetTypeTextBlock       WidgetType = "text_block"
	WidgetTypeQuickReplies    WidgetType = "quick_replies"
)

// WidgetTemplateName defines template names (new system)
const (
	WidgetTemplateProductCard       = "ProductCard"
	WidgetTemplateProductComparison = "ProductComparison"
)

// WidgetSize defines widget size constraints
type WidgetSize string

const (
	WidgetSizeTiny   WidgetSize = "tiny"   // 80-110px, max 2 atoms
	WidgetSizeSmall  WidgetSize = "small"  // 160-220px, max 3 atoms
	WidgetSizeMedium WidgetSize = "medium" // 280-350px, max 5 atoms
	WidgetSizeLarge  WidgetSize = "large"  // 384-460px, max 10 atoms
)

// Widget is a composed UI element made of atoms
// Template field defines the layout, atoms fill the slots
type Widget struct {
	ID        string                 `json:"id"`
	Type      WidgetType             `json:"type,omitempty"`      // Legacy
	Template  string                 `json:"template,omitempty"`  // New: "ProductCard"
	Size      WidgetSize             `json:"size,omitempty"`
	Priority  int                    `json:"priority,omitempty"`
	Atoms     []Atom                 `json:"atoms"`
	Children  []Widget               `json:"children,omitempty"`
	Meta      map[string]interface{} `json:"meta,omitempty"`
	EntityRef *EntityRef             `json:"entityRef,omitempty"` // For click handling
}
