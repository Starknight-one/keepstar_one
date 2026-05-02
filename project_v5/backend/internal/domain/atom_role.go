package domain

// Atom-* enums describe how a catalog field should render: not the v9
// scene-graph node type (frame/text/rectangle/...), but the semantic role —
// "this is currency", "this is a rating with stars", "this is a hero image".
// FieldDefinition exposes these so Agent2 / preset-builder knows what kind
// of node to construct for a given field. Mapping enum → v9 node type
// happens at preset-build time, not at binding time.

type AtomType string

const (
	AtomTypeText   AtomType = "text"
	AtomTypeNumber AtomType = "number"
	AtomTypeImage  AtomType = "image"
	AtomTypeIcon   AtomType = "icon"
	AtomTypeVideo  AtomType = "video"
	AtomTypeAudio  AtomType = "audio"
)

type AtomSubtype string

const (
	SubtypeString   AtomSubtype = "string"
	SubtypeDate     AtomSubtype = "date"
	SubtypeDatetime AtomSubtype = "datetime"
	SubtypeURL      AtomSubtype = "url"
	SubtypeEmail    AtomSubtype = "email"
	SubtypePhone    AtomSubtype = "phone"

	SubtypeInt      AtomSubtype = "int"
	SubtypeFloat    AtomSubtype = "float"
	SubtypeCurrency AtomSubtype = "currency"
	SubtypePercent  AtomSubtype = "percent"
	SubtypeRating   AtomSubtype = "rating"

	SubtypeImageURL    AtomSubtype = "url"
	SubtypeImageBase64 AtomSubtype = "base64"

	SubtypeIconName  AtomSubtype = "name"
	SubtypeIconEmoji AtomSubtype = "emoji"
	SubtypeIconSVG   AtomSubtype = "svg"
)

type AtomSlot string

const (
	AtomSlotHero        AtomSlot = "hero"
	AtomSlotBadge       AtomSlot = "badge"
	AtomSlotTitle       AtomSlot = "title"
	AtomSlotPrimary     AtomSlot = "primary"
	AtomSlotPrice       AtomSlot = "price"
	AtomSlotSecondary   AtomSlot = "secondary"
	AtomSlotGallery     AtomSlot = "gallery"
	AtomSlotStock       AtomSlot = "stock"
	AtomSlotDescription AtomSlot = "description"
	AtomSlotTags        AtomSlot = "tags"
	AtomSlotSpecs       AtomSlot = "specs"
)

type AtomFormat string

const (
	FormatCurrency     AtomFormat = "currency"
	FormatStars        AtomFormat = "stars"
	FormatStarsText    AtomFormat = "stars-text"
	FormatStarsCompact AtomFormat = "stars-compact"
	FormatPercent      AtomFormat = "percent"
	FormatNumber       AtomFormat = "number"
	FormatDate         AtomFormat = "date"
	FormatText         AtomFormat = "text"
)

// AtomWrapper is a stylistic envelope for a leaf node: badge / tag / button
// etc. Backend stores the value on the node as a pass-through property; the
// frontend renderer reads it and picks the matching CSS class / element.
// Identical role to V4's WrapperConfig.Type; values mirror V4's enum so the
// existing renderer knowledge transfers when the frontend port lands.
type AtomWrapper string

const (
	WrapperNone     AtomWrapper = "none"
	WrapperBadge    AtomWrapper = "badge"
	WrapperTag      AtomWrapper = "tag"
	WrapperPill     AtomWrapper = "pill"
	WrapperAvatar   AtomWrapper = "avatar"
	WrapperTooltip  AtomWrapper = "tooltip"
	WrapperAlert    AtomWrapper = "alert"
	WrapperLink     AtomWrapper = "link"
	WrapperProgress AtomWrapper = "progress"
	WrapperButton   AtomWrapper = "button"
)
