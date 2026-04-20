package domain

import "time"

// Stock represents stock levels for a product, stored separately from products
// for future high write-load isolation.
type Stock struct {
	TenantID  string    `json:"tenantId"`
	ProductID string    `json:"productId"`
	Quantity  int       `json:"quantity"`
	Reserved  int       `json:"reserved"`
	UpdatedAt time.Time `json:"updatedAt"`
}
