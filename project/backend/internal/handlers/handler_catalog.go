package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"keepstar/internal/domain"
	"keepstar/internal/logger"
	"keepstar/internal/ports"
	"keepstar/internal/usecases"
)

// CatalogHandler handles catalog HTTP endpoints
type CatalogHandler struct {
	listProducts *usecases.ListProductsUseCase
	getProduct   *usecases.GetProductUseCase
	log          *logger.Logger
}

// NewCatalogHandler creates a new CatalogHandler
func NewCatalogHandler(listProducts *usecases.ListProductsUseCase, getProduct *usecases.GetProductUseCase, log *logger.Logger) *CatalogHandler {
	return &CatalogHandler{
		listProducts: listProducts,
		getProduct:   getProduct,
		log:          log,
	}
}

// ProductResponse is the API response for a product
type ProductResponse struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	Price          int            `json:"price"`
	PriceFormatted string         `json:"priceFormatted"`
	Currency       string         `json:"currency"`
	Images         []string       `json:"images"`
	Rating         float64        `json:"rating"`
	StockQuantity  int            `json:"stockQuantity"`
	Brand          string         `json:"brand"`
	Category       string         `json:"category"`
	Tags           []string       `json:"tags,omitempty"`
	Attributes     map[string]any `json:"attributes,omitempty"`
}

// ListProductsResponse is the API response for product list
type ListProductsResponse struct {
	Products []ProductResponse `json:"products"`
	Total    int               `json:"total"`
}

// HandleListProducts handles GET /api/v1/tenants/{slug}/products
func (h *CatalogHandler) HandleListProducts(w http.ResponseWriter, r *http.Request) {
	if sc := domain.SpanFromContext(r.Context()); sc != nil {
		endSpan := sc.Start("handler.catalog_list")
		defer endSpan()
	}

	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	// Get tenant slug from URL path
	slug := extractTenantSlug(r.URL.Path)
	if slug == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tenant slug required"})
		return
	}

	// Parse query parameters
	query := r.URL.Query()
	filter := ports.ProductFilter{
		CategoryID: query.Get("category"),
		Brand:      query.Get("brand"),
		Search:     query.Get("search"),
	}

	if minPrice := query.Get("minPrice"); minPrice != "" {
		if val, err := strconv.Atoi(minPrice); err == nil {
			filter.MinPrice = val
		}
	}

	if maxPrice := query.Get("maxPrice"); maxPrice != "" {
		if val, err := strconv.Atoi(maxPrice); err == nil {
			filter.MaxPrice = val
		}
	}

	if limit := query.Get("limit"); limit != "" {
		if val, err := strconv.Atoi(limit); err == nil {
			filter.Limit = val
		}
	}

	if offset := query.Get("offset"); offset != "" {
		if val, err := strconv.Atoi(offset); err == nil {
			filter.Offset = val
		}
	}

	// Execute use case
	result, err := h.listProducts.Execute(r.Context(), usecases.ListProductsRequest{
		TenantSlug: slug,
		Filter:     filter,
	})

	if err != nil {
		if err == domain.ErrTenantNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "tenant not found"})
			return
		}
		h.log.Error("list_products_error", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	// Map to response
	products := make([]ProductResponse, len(result.Products))
	for i, p := range result.Products {
		products[i] = mapProductToResponse(p)
	}

	writeJSON(w, http.StatusOK, ListProductsResponse{
		Products: products,
		Total:    result.Total,
	})
}

// HandleGetProduct handles GET /api/v1/tenants/{slug}/products/{id}
func (h *CatalogHandler) HandleGetProduct(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	// Get tenant slug and product ID from URL path
	slug, productID := extractTenantAndProductID(r.URL.Path)
	if slug == "" || productID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tenant slug and product id required"})
		return
	}

	// Execute use case
	product, err := h.getProduct.Execute(r.Context(), usecases.GetProductRequest{
		TenantSlug: slug,
		ProductID:  productID,
	})

	if err != nil {
		if err == domain.ErrTenantNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "tenant not found"})
			return
		}
		if err == domain.ErrProductNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "product not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	writeJSON(w, http.StatusOK, mapProductToResponse(*product))
}

// extractTenantSlug extracts tenant slug from path /api/v1/tenants/{slug}/...
func extractTenantSlug(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if part == "tenants" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// extractTenantAndProductID extracts tenant slug and product ID from path
func extractTenantAndProductID(path string) (string, string) {
	parts := strings.Split(path, "/")
	var slug, productID string
	for i, part := range parts {
		if part == "tenants" && i+1 < len(parts) {
			slug = parts[i+1]
		}
		if part == "products" && i+1 < len(parts) {
			productID = parts[i+1]
		}
	}
	return slug, productID
}

// mapProductToResponse maps domain.Product to ProductResponse
func mapProductToResponse(p domain.Product) ProductResponse {
	images := p.Images
	if images == nil {
		images = []string{}
	}

	attrs := p.Attributes
	if attrs == nil {
		attrs = map[string]any{}
	}

	return ProductResponse{
		ID:             p.ID,
		Name:           p.Name,
		Description:    p.Description,
		Price:          p.Price,
		PriceFormatted: p.PriceFormatted,
		Currency:       p.Currency,
		Images:         images,
		Rating:         p.Rating,
		StockQuantity:  p.StockQuantity,
		Brand:          p.Brand,
		Category:       p.Category,
		Tags:           p.Tags,
		Attributes:     attrs,
	}
}
