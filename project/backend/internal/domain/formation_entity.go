package domain

// FormationType defines the layout type for widgets
type FormationType string

const (
	FormationTypeGrid     FormationType = "grid"
	FormationTypeList     FormationType = "list"
	FormationTypeCarousel FormationType = "carousel"
	FormationTypeSingle     FormationType = "single"
	FormationTypeComparison FormationType = "comparison"
)

// Formation defines how widgets are laid out
type Formation struct {
	Type    FormationType `json:"type"`
	Columns int           `json:"columns,omitempty"`
	Gap     int           `json:"gap,omitempty"`
}
