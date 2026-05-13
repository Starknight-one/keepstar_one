package domain

import "time"

type TenantType string

const (
	TenantTypeBrand    TenantType = "brand"
	TenantTypeRetailer TenantType = "retailer"
	TenantTypeReseller TenantType = "reseller"
)

type Tenant struct {
	ID        string         `json:"id"`
	Slug      string         `json:"slug"`
	Name      string         `json:"name"`
	Type      TenantType     `json:"type"`
	Settings  map[string]any `json:"settings"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
}
