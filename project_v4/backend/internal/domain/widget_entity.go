package domain

// WidgetSize defines widget size constraints
type WidgetSize string

const (
	WidgetSizeTiny   WidgetSize = "tiny"
	WidgetSizeSmall  WidgetSize = "small"
	WidgetSizeMedium WidgetSize = "medium"
	WidgetSizeLarge  WidgetSize = "large"
)

// Widget is a composed UI element — a tree of layout nodes containing atoms.
type Widget struct {
	ID        string                 `json:"id"`
	Size      WidgetSize             `json:"size,omitempty"`
	Priority  int                    `json:"priority,omitempty"`
	Meta      map[string]interface{} `json:"meta,omitempty"`
	EntityRef *EntityRef             `json:"entityRef,omitempty"`

	Layout  *LayoutNode   `json:"layout,omitempty"`
	Atoms   []Atom        `json:"atomsV2,omitempty"` // JSON name kept for frontend compatibility
	Actions []ActionDef   `json:"actions,omitempty"`
	States  *WidgetStates `json:"states,omitempty"`
}
