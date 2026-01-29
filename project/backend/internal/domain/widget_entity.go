package domain

// WidgetType defines the type of composed widget
type WidgetType string

const (
	WidgetTypeProductCard     WidgetType = "product_card"
	WidgetTypeProductList     WidgetType = "product_list"
	WidgetTypeComparisonTable WidgetType = "comparison_table"
	WidgetTypeImageCarousel   WidgetType = "image_carousel"
	WidgetTypeTextBlock       WidgetType = "text_block"
	WidgetTypeQuickReplies    WidgetType = "quick_replies"
)

// Widget is a composed UI element made of atoms
type Widget struct {
	ID       string                 `json:"id"`
	Type     WidgetType             `json:"type"`
	Atoms    []Atom                 `json:"atoms"`
	Children []Widget               `json:"children,omitempty"`
	Meta     map[string]interface{} `json:"meta,omitempty"`
}
