package domain

// Service represents a service in the catalog (parallel to Product)
type Service struct {
	ID             string                 `json:"id"`
	TenantID       string                 `json:"tenantId"`
	Name           string                 `json:"name"`
	Description    string                 `json:"description,omitempty"`
	Price          int                    `json:"price,omitempty"`          // in kopecks
	PriceFormatted string                 `json:"priceFormatted,omitempty"`
	Currency       string                 `json:"currency,omitempty"`
	Duration       string                 `json:"duration,omitempty"`     // "30 min", "1 hour"
	Images         []string               `json:"images,omitempty"`
	Rating         float64                `json:"rating,omitempty"`
	Category       string                 `json:"category,omitempty"`
	Provider       string                 `json:"provider,omitempty"`     // service provider name
	Availability   string                 `json:"availability,omitempty"` // "available", "busy"
	Attributes     map[string]interface{} `json:"attributes,omitempty"`
}
