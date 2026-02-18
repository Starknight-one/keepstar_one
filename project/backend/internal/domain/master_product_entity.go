package domain

import "time"

type MasterProduct struct {
	ID            string    `json:"id"`
	SKU           string    `json:"sku"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Brand         string    `json:"brand"`
	CategoryID    string    `json:"categoryId"`
	CategoryName  string    `json:"categoryName,omitempty"` // populated by JOIN in some queries
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
