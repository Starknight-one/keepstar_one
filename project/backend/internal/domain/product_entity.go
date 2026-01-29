package domain

// Product represents a product/service in the catalog
type Product struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Price       float64                `json:"price,omitempty"`
	Currency    string                 `json:"currency,omitempty"`
	Images      []string               `json:"images,omitempty"`
	Rating      float64                `json:"rating,omitempty"`
	Category    string                 `json:"category,omitempty"`
	Tags        []string               `json:"tags,omitempty"`
	Attributes  map[string]interface{} `json:"attributes,omitempty"`
}
