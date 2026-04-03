package domain

// AtomTemplate defines an atom with field reference (not actual value)
type AtomTemplate struct {
	Type   AtomType `json:"type"`
	Field  string   `json:"field"`            // Field name from product (e.g., "price", "name")
	Style  string   `json:"style,omitempty"`  // For text: heading/body/caption
	Format string   `json:"format,omitempty"` // For number: currency/percent/compact
	Size   string   `json:"size,omitempty"`   // For image: small/medium/large
}

// WidgetTemplate defines a widget structure without data
// Uses WidgetSize from widget_entity.go
type WidgetTemplate struct {
	Size     WidgetSize     `json:"size"`
	Priority int            `json:"priority,omitempty"`
	Atoms    []AtomTemplate `json:"atoms"`
}

// GridConfig defines grid layout
type GridConfig struct {
	Rows int `json:"rows"`
	Cols int `json:"cols"`
}

// FormationTemplate is what Agent 2 produces
type FormationTemplate struct {
	Mode           FormationType  `json:"mode"`
	Grid           *GridConfig    `json:"grid,omitempty"`
	WidgetTemplate WidgetTemplate `json:"widgetTemplate"`
}

// FieldSpec describes a single field in RenderConfig (what Agent 2 decided to show)
type FieldSpec struct {
	Name    string `json:"name"`              // "images", "name", "price"
	Slot    string `json:"slot"`              // "hero", "title", "price"
	Format  string `json:"format,omitempty"`  // value transform: "currency", "stars-compact"
	Display string `json:"display"`           // visual wrapper: "badge", "h2", "tag"
}

// RenderConfig captures how Agent 2 rendered this formation (for next-turn context)
type RenderConfig struct {
	EntityType string        `json:"entity_type"`
	Preset     string        `json:"preset,omitempty"`
	Mode       FormationType `json:"mode"`
	Size       WidgetSize    `json:"size"`
	Fields     []FieldSpec   `json:"fields,omitempty"`
}

// FormationSection represents a section within a composed formation
type FormationSection struct {
	Mode    FormationType `json:"mode"`
	Grid    *GridConfig   `json:"grid,omitempty"`
	Widgets []Widget      `json:"widgets"`
	Label   string        `json:"label,omitempty"`
}

// PaginationMeta contains pagination info for large result sets
type PaginationMeta struct {
	Total   int  `json:"total"`
	Offset  int  `json:"offset"`
	Limit   int  `json:"limit"`
	HasMore bool `json:"hasMore"`
}

// FormationWithData is the final result after applying template
type FormationWithData struct {
	Mode       FormationType     `json:"mode"`
	Grid       *GridConfig       `json:"grid,omitempty"`
	Widgets    []Widget          `json:"widgets"`
	Config     *RenderConfig     `json:"config,omitempty"`
	Sections   []FormationSection `json:"sections,omitempty"`
	Pagination *PaginationMeta   `json:"pagination,omitempty"`
}
