package domain

import "time"

type Product struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenantId"`
	MasterProductID string    `json:"masterProductId,omitempty"`
	Name            string    `json:"name"`
	Description     string    `json:"description,omitempty"`
	Price           int       `json:"price"`
	PriceFormatted  string    `json:"priceFormatted,omitempty"`
	Currency        string    `json:"currency,omitempty"`
	Images          []string  `json:"images,omitempty"`
	Rating          float64   `json:"rating,omitempty"`
	StockQuantity   int       `json:"stockQuantity"`
	Brand           string    `json:"brand,omitempty"`
	Category        string    `json:"category,omitempty"`
	Tags            []string  `json:"tags,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`

	// PIM structured fields (from master_products)
	ProductForm    string   `json:"productForm,omitempty"`
	Texture        string   `json:"texture,omitempty"`
	RoutineStep    string   `json:"routineStep,omitempty"`
	SkinType       []string `json:"skinType,omitempty"`
	Concern        []string `json:"concern,omitempty"`
	KeyIngredients []string `json:"keyIngredients,omitempty"`
	TargetArea     []string `json:"targetArea,omitempty"`
	MarketingClaim string   `json:"marketingClaim,omitempty"`
	Benefits       []string `json:"benefits,omitempty"`
}

type MasterProduct struct {
	ID            string    `json:"id"`
	SKU           string    `json:"sku"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Brand         string    `json:"brand"`
	CategoryID    string    `json:"categoryId"`
	CategoryName  string    `json:"categoryName,omitempty"`
	Images        []string  `json:"images"`
	OwnerTenantID string    `json:"ownerTenantId"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`

	// PIM structured fields
	OriginalName      string   `json:"originalName,omitempty"`
	ProductLine       string   `json:"productLine,omitempty"`
	ProductForm       string   `json:"productForm,omitempty"`
	Texture           string   `json:"texture,omitempty"`
	RoutineStep       string   `json:"routineStep,omitempty"`
	RoutineTime       string   `json:"routineTime,omitempty"`
	ApplicationMethod string   `json:"applicationMethod,omitempty"`
	SkinType          []string `json:"skinType,omitempty"`
	Concern           []string `json:"concern,omitempty"`
	KeyIngredients    []string `json:"keyIngredients,omitempty"`
	TargetArea        []string `json:"targetArea,omitempty"`
	FreeFrom          []string `json:"freeFrom,omitempty"`
	MarketingClaim    string   `json:"marketingClaim,omitempty"`
	Benefits          []string `json:"benefits,omitempty"`
	HowToUse          string   `json:"howToUse,omitempty"`
	EnrichmentVersion int      `json:"enrichmentVersion,omitempty"`
}

type AdminProductFilter struct {
	Search     string `json:"search,omitempty"`
	CategoryID string `json:"categoryId,omitempty"`
	Limit      int    `json:"limit"`
	Offset     int    `json:"offset"`
}

type ProductUpdate struct {
	Name        *string  `json:"name,omitempty"`
	Description *string  `json:"description,omitempty"`
	Price       *int     `json:"price,omitempty"`
	Stock       *int     `json:"stock,omitempty"`
	Rating      *float64 `json:"rating,omitempty"`
}
