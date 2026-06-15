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

// SettingVisible reads a hide-only visibility flag from tenant settings.
// Default is visible: a nil map, a missing key, or any non-bool value all
// return true. Only an explicit boolean false hides the field. This is the
// single source of truth for price/stock visibility across the catalog tool
// (typed + JSONB suppression) and the Agent2 <fields> prompt block.
func SettingVisible(settings map[string]any, key string) bool {
	if settings == nil {
		return true
	}
	if v, ok := settings[key].(bool); ok {
		return v
	}
	return true
}
