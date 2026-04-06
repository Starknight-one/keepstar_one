package domain

// WidgetSize defines widget size constraints
type WidgetSize string

const (
	WidgetSizeTiny   WidgetSize = "tiny"
	WidgetSizeSmall  WidgetSize = "small"
	WidgetSizeMedium WidgetSize = "medium"
	WidgetSizeLarge  WidgetSize = "large"
)

// ReplicateConfig describes how a widget template should be replicated by the engine.
// Engine-internal: never serialized to the frontend (`json:"-"` on the Widget field).
//
// - Enabled=true   → engine clones the widget once per data item (up to Limit).
// - DataIndex      → for non-replicated entity widgets, picks data[DataIndex] for EntityRef binding.
// - GroupID        → assigned by the engine after expansion; replicated clones from the same
//                    template share a GroupID so group-aware constraints can scope cross-widget
//                    rules to the gallery instead of mixing with literal widgets.
type ReplicateConfig struct {
	Enabled   bool
	Limit     int
	DataIndex int
	GroupID   string
}

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

	// ReplicateConfig is engine-internal — set by ops or the legacy top-level
	// Replicate flag, consumed by expandReplicatedWidgets, never reaches the frontend.
	ReplicateConfig *ReplicateConfig `json:"-"`
}
