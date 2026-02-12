package domain

// StockUpdate represents a single item in a bulk stock update request.
type StockUpdate struct {
	SKU      string `json:"sku"`
	Quantity int    `json:"quantity"`
	Price    *int   `json:"price,omitempty"` // optional price update
}
