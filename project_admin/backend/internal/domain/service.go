package domain

import "time"

// Service represents a tenant-scoped service listing.
type Service struct {
	ID              string         `json:"id"`
	TenantID        string         `json:"tenantId"`
	MasterServiceID string         `json:"masterServiceId,omitempty"`
	Name            string         `json:"name"`
	Description     string         `json:"description,omitempty"`
	Price           int            `json:"price"`
	PriceFormatted  string         `json:"priceFormatted,omitempty"`
	Currency        string         `json:"currency,omitempty"`
	Duration        string         `json:"duration,omitempty"`
	Images          []string       `json:"images,omitempty"`
	Rating          float64        `json:"rating,omitempty"`
	Category        string         `json:"category,omitempty"`
	Provider        string         `json:"provider,omitempty"`
	Availability    string         `json:"availability,omitempty"`
	Tags            []string       `json:"tags,omitempty"`
	Attributes      map[string]any `json:"attributes,omitempty"`
	CreatedAt       time.Time      `json:"createdAt"`
	UpdatedAt       time.Time      `json:"updatedAt"`
}

// MasterService is the shared catalog entity for services.
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
