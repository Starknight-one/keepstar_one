package domain

// AtomType defines the type of atomic UI element
type AtomType string

const (
	AtomTypeText     AtomType = "text"
	AtomTypeNumber   AtomType = "number"
	AtomTypePrice    AtomType = "price"
	AtomTypeImage    AtomType = "image"
	AtomTypeRating   AtomType = "rating"
	AtomTypeBadge    AtomType = "badge"
	AtomTypeButton   AtomType = "button"
	AtomTypeIcon     AtomType = "icon"
	AtomTypeDivider  AtomType = "divider"
	AtomTypeProgress AtomType = "progress"
)

// Atom is the smallest UI building block
type Atom struct {
	Type  AtomType               `json:"type"`
	Value interface{}            `json:"value"`
	Meta  map[string]interface{} `json:"meta,omitempty"`
}
