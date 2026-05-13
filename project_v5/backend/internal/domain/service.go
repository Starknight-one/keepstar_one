package domain

// Service mirrors V4's Service entity verbatim.
type Service struct {
	ID              string                 `json:"id"`
	TenantID        string                 `json:"tenantId"`
	MasterServiceID string                 `json:"masterServiceId,omitempty"`
	Name            string                 `json:"name"`
	Description     string                 `json:"description,omitempty"`
	Price           int                    `json:"price,omitempty"`
	PriceFormatted  string                 `json:"priceFormatted,omitempty"`
	Currency        string                 `json:"currency,omitempty"`
	Duration        string                 `json:"duration,omitempty"`
	Images          []string               `json:"images,omitempty"`
	Rating          float64                `json:"rating,omitempty"`
	Category        string                 `json:"category,omitempty"`
	Provider        string                 `json:"provider,omitempty"`
	Availability    string                 `json:"availability,omitempty"`
	Tags            []string               `json:"tags,omitempty"`
	Attributes      map[string]interface{} `json:"attributes,omitempty"`

	Extra map[string]interface{} `json:"extra,omitempty"`
}
