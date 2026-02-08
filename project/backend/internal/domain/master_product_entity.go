package domain

import "time"

type MasterProduct struct {
	ID            string         `json:"id"`
	SKU           string         `json:"sku"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	Brand         string         `json:"brand"`
	CategoryID    string         `json:"categoryId"`
	CategoryName  string         `json:"categoryName,omitempty"` // populated by JOIN in some queries
	Images        []string       `json:"images"`
	Attributes    map[string]any `json:"attributes"`
	OwnerTenantID string         `json:"ownerTenantId"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
}
