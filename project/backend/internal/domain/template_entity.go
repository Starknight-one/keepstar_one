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

// FormationWithData is the final result after applying template
type FormationWithData struct {
	Mode    FormationType `json:"mode"`
	Grid    *GridConfig   `json:"grid,omitempty"`
	Widgets []Widget      `json:"widgets"`
}
