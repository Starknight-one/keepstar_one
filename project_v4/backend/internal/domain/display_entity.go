package domain

// AtomDisplay defines the visual format of an atom
type AtomDisplay string

const (
	// text displays
	DisplayH1           AtomDisplay = "h1"
	DisplayH2           AtomDisplay = "h2"
	DisplayH3           AtomDisplay = "h3"
	DisplayH4           AtomDisplay = "h4"
	DisplayBodyLg       AtomDisplay = "body-lg"
	DisplayBody         AtomDisplay = "body"
	DisplayBodySm       AtomDisplay = "body-sm"
	DisplayCaption      AtomDisplay = "caption"
	DisplayBadge        AtomDisplay = "badge"
	DisplayBadgeSuccess AtomDisplay = "badge-success"
	DisplayBadgeError   AtomDisplay = "badge-error"
	DisplayBadgeWarning AtomDisplay = "badge-warning"
	DisplayTag          AtomDisplay = "tag"
	DisplayTagActive    AtomDisplay = "tag-active"

	// number displays
	DisplayPrice         AtomDisplay = "price"
	DisplayPriceLg       AtomDisplay = "price-lg"
	DisplayPriceOld      AtomDisplay = "price-old"
	DisplayPriceDiscount AtomDisplay = "price-discount"
	DisplayRating        AtomDisplay = "rating"
	DisplayRatingText    AtomDisplay = "rating-text"
	DisplayRatingCompact AtomDisplay = "rating-compact"
	DisplayPercent       AtomDisplay = "percent"
	DisplayProgress      AtomDisplay = "progress"

	// image displays
	DisplayImage      AtomDisplay = "image"
	DisplayImageCover AtomDisplay = "image-cover"
	DisplayAvatar     AtomDisplay = "avatar"
	DisplayAvatarSm   AtomDisplay = "avatar-sm"
	DisplayAvatarLg   AtomDisplay = "avatar-lg"
	DisplayThumbnail  AtomDisplay = "thumbnail"
	DisplayGallery    AtomDisplay = "gallery"

	// icon displays
	DisplayIcon   AtomDisplay = "icon"
	DisplayIconSm AtomDisplay = "icon-sm"
	DisplayIconLg AtomDisplay = "icon-lg"

	// interactive displays
	DisplayButtonPrimary   AtomDisplay = "button-primary"
	DisplayButtonSecondary AtomDisplay = "button-secondary"
	DisplayButtonOutline   AtomDisplay = "button-outline"
	DisplayButtonGhost     AtomDisplay = "button-ghost"
	DisplayInput           AtomDisplay = "input"

	// layout displays
	DisplayDivider AtomDisplay = "divider"
	DisplaySpacer  AtomDisplay = "spacer"
)

// DisplayStyle is an alias for a set of displays
type DisplayStyle string

const (
	StyleProductHero    DisplayStyle = "product-hero"
	StyleProductCompact DisplayStyle = "product-compact"
	StyleProductDetail  DisplayStyle = "product-detail"
	StyleServiceCard    DisplayStyle = "service-card"
	StyleServiceDetail  DisplayStyle = "service-detail"
)

// DisplayStyles maps style aliases to slot→display mappings
var DisplayStyles = map[DisplayStyle]map[AtomSlot]AtomDisplay{
	StyleProductHero: {
		AtomSlotTitle:   DisplayH1,
		AtomSlotPrice:   DisplayPriceLg,
		AtomSlotBadge:   DisplayBadgeSuccess,
		AtomSlotPrimary: DisplayTag,
		AtomSlotHero:    DisplayImageCover,
	},
	StyleProductCompact: {
		AtomSlotTitle:   DisplayH3,
		AtomSlotPrice:   DisplayPrice,
		AtomSlotBadge:   DisplayTag,
		AtomSlotPrimary: DisplayCaption,
		AtomSlotHero:    DisplayThumbnail,
	},
	StyleProductDetail: {
		AtomSlotTitle:       DisplayH1,
		AtomSlotPrice:       DisplayPriceLg,
		AtomSlotBadge:       DisplayBadgeSuccess,
		AtomSlotPrimary:     DisplayTag,
		AtomSlotGallery:     DisplayGallery,
		AtomSlotDescription: DisplayBody,
		AtomSlotSpecs:       DisplayBodySm,
	},
	StyleServiceCard: {
		AtomSlotTitle:   DisplayH2,
		AtomSlotPrice:   DisplayPrice,
		AtomSlotPrimary: DisplayCaption,
		AtomSlotHero:    DisplayImageCover,
	},
	StyleServiceDetail: {
		AtomSlotTitle:       DisplayH1,
		AtomSlotPrice:       DisplayPriceLg,
		AtomSlotPrimary:     DisplayTag,
		AtomSlotGallery:     DisplayGallery,
		AtomSlotDescription: DisplayBody,
		AtomSlotSpecs:       DisplayBodySm,
	},
}

// GetDisplayForSlot returns the display for a slot given a style, with fallback
func GetDisplayForSlot(style DisplayStyle, slot AtomSlot, fallback AtomDisplay) AtomDisplay {
	if styles, ok := DisplayStyles[style]; ok {
		if display, ok := styles[slot]; ok {
			return display
		}
	}
	return fallback
}
