package domain

import "time"

// VariantKind classifies whether a variant is a real product, an addon
// (gift wrap, engraving, warranty), or a bundle. addon/bundle are excluded
// from search/embedding/chat-rendering.
type VariantKind string

const (
	VariantKindReal   VariantKind = "real"
	VariantKindAddon  VariantKind = "addon"
	VariantKindBundle VariantKind = "bundle"
)

// MasterVariant is one SKU of a master product. Spec §3.1 (Option C parent-child).
// One master can have many variants (e.g. MacBook 14" → M3/512GB/Silver,
// M3/1TB/Silver, M3/512GB/Black). All identification facts (GTIN, SKU,
// dimensions) live here, not on master_products.
type MasterVariant struct {
	ID              string            `json:"id"`
	MasterProductID string            `json:"masterProductId"`
	SKU             string            `json:"sku,omitempty"`
	GTINs           []string          `json:"gtins"` // primary match key, multiple per variant possible
	ImageURL        string            `json:"imageUrl,omitempty"`
	WeightG         *int              `json:"weightG,omitempty"`
	VolumeML        *int              `json:"volumeML,omitempty"`
	LengthMM        *int              `json:"lengthMm,omitempty"`
	WidthMM         *int              `json:"widthMm,omitempty"`
	HeightMM        *int              `json:"heightMm,omitempty"`
	Color           string            `json:"color,omitempty"`
	Size            string            `json:"size,omitempty"` // textual: "236ml" / "M" / "14 inch"
	Material        string            `json:"material,omitempty"`
	Axes            map[string]string `json:"axes,omitempty"` // raw axis values from source: {"size":"236ml"}
	VariantKind     VariantKind       `json:"variantKind"`

	// Dual-store fields: typed parsed value + raw source string + parser status.
	// Spec §1.12 / §5: raw kept for re-parse when registry improves.
	WeightRaw   string            `json:"weightRaw,omitempty"`
	VolumeRaw   string            `json:"volumeRaw,omitempty"`
	ParseStatus map[string]string `json:"parseStatus,omitempty"` // {"volume_ml":"ok","weight_g":"failed"}

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
