package domain

// AtomType defines the 6 base types of atomic data
type AtomType string

const (
	AtomTypeText   AtomType = "text"
	AtomTypeNumber AtomType = "number"
	AtomTypeImage  AtomType = "image"
	AtomTypeIcon   AtomType = "icon"
	AtomTypeVideo  AtomType = "video"
	AtomTypeAudio  AtomType = "audio"
)

// AtomSubtype defines the data format within a type
type AtomSubtype string

const (
	// text subtypes
	SubtypeString   AtomSubtype = "string"
	SubtypeDate     AtomSubtype = "date"
	SubtypeDatetime AtomSubtype = "datetime"
	SubtypeURL      AtomSubtype = "url"
	SubtypeEmail    AtomSubtype = "email"
	SubtypePhone    AtomSubtype = "phone"

	// number subtypes
	SubtypeInt      AtomSubtype = "int"
	SubtypeFloat    AtomSubtype = "float"
	SubtypeCurrency AtomSubtype = "currency"
	SubtypePercent  AtomSubtype = "percent"
	SubtypeRating   AtomSubtype = "rating"

	// image subtypes
	SubtypeImageURL    AtomSubtype = "url"
	SubtypeImageBase64 AtomSubtype = "base64"

	// icon subtypes
	SubtypeIconName  AtomSubtype = "name"
	SubtypeIconEmoji AtomSubtype = "emoji"
	SubtypeIconSVG   AtomSubtype = "svg"
)

// AtomSlot defines where atom should be placed in template
type AtomSlot string

const (
	AtomSlotHero        AtomSlot = "hero"        // Main image/carousel
	AtomSlotBadge       AtomSlot = "badge"       // Badge overlay
	AtomSlotTitle       AtomSlot = "title"       // Product title
	AtomSlotPrimary     AtomSlot = "primary"     // Primary attributes (shown immediately)
	AtomSlotPrice       AtomSlot = "price"       // Price block
	AtomSlotSecondary   AtomSlot = "secondary"   // Secondary attributes (expandable)
	AtomSlotGallery     AtomSlot = "gallery"     // Full gallery (not just hero)
	AtomSlotStock       AtomSlot = "stock"       // Availability indicator
	AtomSlotDescription AtomSlot = "description" // Full description block
	AtomSlotTags        AtomSlot = "tags"        // Tags chips
	AtomSlotSpecs       AtomSlot = "specs"       // Specifications table
)

// AtomFormat defines how the raw value is transformed into display text
type AtomFormat string

const (
	FormatCurrency     AtomFormat = "currency"      // "$329.00"
	FormatStars        AtomFormat = "stars"          // "★★★★☆"
	FormatStarsText    AtomFormat = "stars-text"     // "4.2/5"
	FormatStarsCompact AtomFormat = "stars-compact"  // "★ 4.2"
	FormatPercent      AtomFormat = "percent"        // "85%"
	FormatNumber       AtomFormat = "number"         // "329"
	FormatDate         AtomFormat = "date"           // "Feb 25, 2026"
	FormatText         AtomFormat = "text"           // as-is
)

// Rigidity controls how the engine may adjust an atom.
type Rigidity string

const (
	RigidityLocked    Rigidity = "locked"    // Agent explicitly set — engine never touches
	RigidityPreferred Rigidity = "preferred" // Preset set — engine can adjust if needed
	RigidityFlexible  Rigidity = "flexible"  // Default — engine freely adjusts
)

// TextStyle defines typography separated from visual container.
type TextStyle struct {
	FontSize       string `json:"fontSize,omitempty"`
	FontWeight     string `json:"fontWeight,omitempty"`
	Color          string `json:"color,omitempty"`
	TextDecoration string `json:"textDecoration,omitempty"`
	TextTransform  string `json:"textTransform,omitempty"`
	LineClamp      int    `json:"lineClamp,omitempty"`
	Truncate       int    `json:"truncate,omitempty"`
	LineHeight     string `json:"lineHeight,omitempty"`
	LetterSpacing  string `json:"letterSpacing,omitempty"`
}

// MediaStyle defines styling for image/video/audio atoms.
type MediaStyle struct {
	AspectRatio string `json:"aspectRatio,omitempty"`
	ObjectFit   string `json:"objectFit,omitempty"`
	Controls    bool   `json:"controls,omitempty"`
	Autoplay    bool   `json:"autoplay,omitempty"`
	Muted       bool   `json:"muted,omitempty"`
	Poster      string `json:"poster,omitempty"`
}

// IconStyle defines styling for icon atoms.
type IconStyle struct {
	Size  string `json:"size,omitempty"`
	Color string `json:"color,omitempty"`
	Style string `json:"style,omitempty"`
}

// WrapperConfig defines the visual container around an atom value.
type WrapperConfig struct {
	Type         string   `json:"type"`
	Variant      string   `json:"variant,omitempty"`
	Rigidity     Rigidity `json:"rigidity,omitempty"`
	Background   string   `json:"background,omitempty"`
	BorderRadius string   `json:"borderRadius,omitempty"`
	Padding      string   `json:"padding,omitempty"`
	ContentFit   string   `json:"contentFit,omitempty"`
	Margin       string   `json:"margin,omitempty"`
}

// Atom is the atomic UI building block with separated textStyle and wrapper.
type Atom struct {
	ID         string                 `json:"id,omitempty"`
	Type       AtomType               `json:"type"`
	Subtype    AtomSubtype            `json:"subtype,omitempty"`
	Value      interface{}            `json:"value"`
	Label      string                 `json:"label,omitempty"`
	Unit       string                 `json:"unit,omitempty"`
	Format     AtomFormat             `json:"format,omitempty"`
	TextStyle  *TextStyle             `json:"textStyle,omitempty"`
	Wrapper    *WrapperConfig         `json:"wrapper,omitempty"`
	MediaStyle *MediaStyle            `json:"mediaStyle,omitempty"`
	IconStyle  *IconStyle             `json:"iconStyle,omitempty"`
	Slot       AtomSlot               `json:"slot,omitempty"`
	Rigidity   Rigidity               `json:"rigidity,omitempty"`
	FieldName  string                 `json:"fieldName,omitempty"`
	Priority   int                    `json:"priority,omitempty"`
	Meta       map[string]interface{} `json:"meta,omitempty"`
}
