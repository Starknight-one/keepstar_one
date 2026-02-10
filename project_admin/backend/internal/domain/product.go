package domain

import "time"

type Product struct {
	ID              string         `json:"id"`
	TenantID        string         `json:"tenantId"`
	MasterProductID string         `json:"masterProductId,omitempty"`
	Name            string         `json:"name"`
	Description     string         `json:"description,omitempty"`
	Price           int            `json:"price"`
	PriceFormatted  string         `json:"priceFormatted,omitempty"`
	Currency        string         `json:"currency,omitempty"`
	Images          []string       `json:"images,omitempty"`
	Rating          float64        `json:"rating,omitempty"`
	StockQuantity   int            `json:"stockQuantity"`
	Brand           string         `json:"brand,omitempty"`
	Category        string         `json:"category,omitempty"`
	Attributes      map[string]any `json:"attributes,omitempty"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
}

type MasterProduct struct {
	ID            string         `json:"id"`
	SKU           string         `json:"sku"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	Brand         string         `json:"brand"`
	CategoryID    string         `json:"categoryId"`
	CategoryName  string         `json:"categoryName,omitempty"`
	Images        []string       `json:"images"`
	Attributes    map[string]any `json:"attributes"`
	OwnerTenantID string         `json:"ownerTenantId"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
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
