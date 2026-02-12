package domain

import "time"

// MasterService is the shared catalog entity for services (analogous to MasterProduct).
type MasterService struct {
	ID            string         `json:"id"`
	SKU           string         `json:"sku"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	Brand         string         `json:"brand"`
	CategoryID    string         `json:"categoryId"`
	CategoryName  string         `json:"categoryName,omitempty"`
	Images        []string       `json:"images"`
	Attributes    map[string]any `json:"attributes"`
	Duration      string         `json:"duration,omitempty"`
	Provider      string         `json:"provider,omitempty"`
	OwnerTenantID string         `json:"ownerTenantId"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
}
