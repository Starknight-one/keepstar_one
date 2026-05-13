package domain

// EntityType discriminates a Product from a Service in EntityRef / CartItem.
type EntityType string

const (
	EntityTypeProduct EntityType = "product"
	EntityTypeService EntityType = "service"
)

// Product mirrors V4's Product entity verbatim. V5 binding will read from
// these typed fields plus Extra/Tier2 maps.
type Product struct {
	ID              string   `json:"id"`
	TenantID        string   `json:"tenantId"`
	MasterProductID string   `json:"masterProductId,omitempty"`
	MasterVariantID string   `json:"masterVariantId,omitempty"`
	Name            string   `json:"name"`
	DisplayName     string   `json:"displayName,omitempty"`
	OriginalName    string   `json:"originalName,omitempty"`
	Description     string   `json:"description,omitempty"`
	Price           int      `json:"price,omitempty"`
	PriceFormatted  string   `json:"priceFormatted,omitempty"`
	Currency        string   `json:"currency,omitempty"`
	Images          []string `json:"images,omitempty"`
	Rating          float64  `json:"rating,omitempty"`
	StockQuantity   int      `json:"stockQuantity"`
	Brand           string   `json:"brand,omitempty"`
	Category        string   `json:"category,omitempty"`
	Tags            []string `json:"tags,omitempty"`

	SKU      string   `json:"sku,omitempty"`
	GTINs    []string `json:"gtins,omitempty"`
	Size     string   `json:"size,omitempty"`
	Color    string   `json:"color,omitempty"`
	WeightG  *int     `json:"weightG,omitempty"`
	VolumeML *int     `json:"volumeMl,omitempty"`

	ProductForm    string   `json:"productForm,omitempty"`
	Texture        string   `json:"texture,omitempty"`
	RoutineStep    string   `json:"routineStep,omitempty"`
	SkinType       []string `json:"skinType,omitempty"`
	Concern        []string `json:"concern,omitempty"`
	KeyIngredients []string `json:"keyIngredients,omitempty"`
	TargetArea     []string `json:"targetArea,omitempty"`
	MarketingClaim string   `json:"marketingClaim,omitempty"`
	Benefits       []string `json:"benefits,omitempty"`

	Extra map[string]interface{} `json:"extra,omitempty"`
	Tier2 map[string]interface{} `json:"tier2,omitempty"`
}
