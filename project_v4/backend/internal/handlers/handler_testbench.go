package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"keepstar_v4/internal/domain"
	engine_v4 "keepstar_v4/internal/engine_v4"
	"keepstar_v4/internal/ports"
	"keepstar_v4/internal/tools"
)

// TestbenchHandler handles testbench API requests
type TestbenchHandler struct {
	catalogPort ports.CatalogPort
	engine      *engine_v4.Engine
}

// NewTestbenchHandler creates a testbench handler
func NewTestbenchHandler(catalogPort ports.CatalogPort, eng *engine_v4.Engine) *TestbenchHandler {
	return &TestbenchHandler{
		catalogPort: catalogPort,
		engine:      eng,
	}
}

// TestbenchRequest is the request body for testbench
type TestbenchRequest struct {
	TenantSlug string                 `json:"tenantSlug"`
	Count      int                    `json:"count"`
	Params     map[string]interface{} `json:"params"`
}

// TestbenchResponse is the response body for testbench
// TestbenchEntityData is a compact view of a product's raw data for debugging
type TestbenchEntityData struct {
	ID             string      `json:"id"`
	Name           string      `json:"name"`
	Brand          string      `json:"brand,omitempty"`
	Category       string      `json:"category,omitempty"`
	Price          int         `json:"price"`
	Currency       string      `json:"currency,omitempty"`
	Rating         float64     `json:"rating,omitempty"`
	Images         int         `json:"images"`
	StockQuantity  int         `json:"stockQuantity,omitempty"`
	HasDescription bool        `json:"hasDescription"`
	HasTags        bool        `json:"hasTags"`
	ProductForm    string      `json:"productForm,omitempty"`
	SkinType       string      `json:"skinType,omitempty"`
	Concern        string      `json:"concern,omitempty"`
	KeyIngredients string      `json:"keyIngredients,omitempty"`
}

type TestbenchResponse struct {
	Formation *FormationResponse    `json:"formation,omitempty"`
	Entities  []TestbenchEntityData `json:"entities"`
	Warnings  []string              `json:"warnings,omitempty"`
	Config    interface{}           `json:"config,omitempty"`
}

// HandleTestbench handles POST /api/v1/testbench
func (h *TestbenchHandler) HandleTestbench(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req TestbenchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	warnings := make([]string, 0)

	// Resolve tenant
	tenantSlug := req.TenantSlug
	if tenantSlug == "" {
		tenantSlug = "hey-babes-cosmetics"
	}
	tenant, err := h.catalogPort.GetTenantBySlug(ctx, tenantSlug)
	if err != nil {
		http.Error(w, fmt.Sprintf("tenant not found: %s", tenantSlug), http.StatusBadRequest)
		return
	}

	// Load products
	count := req.Count
	if count <= 0 {
		count = 6
	}
	if count > 50 {
		count = 50
	}

	products, _, err := h.catalogPort.ListProducts(ctx, tenant.ID, ports.ProductFilter{Limit: count})
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to load products: %v", err), http.StatusInternalServerError)
		return
	}

	if len(products) == 0 {
		writeJSON(w, http.StatusOK, TestbenchResponse{
			Warnings: []string{"no products found for tenant"},
		})
		return
	}

	// Build formation using V4 engine
	params := req.Params
	if params == nil {
		params = make(map[string]interface{})
	}

	// Convert products to data maps
	data := make([]map[string]interface{}, len(products))
	for i, p := range products {
		data[i] = tools.ProductToMap(p)
	}

	// Build engine input
	input := engine_v4.ExecuteInput{
		Preset:     "product_card_grid",
		EntityType: string(domain.EntityTypeProduct),
		Data:       data,
	}

	// Apply overrides from params
	if presetName, ok := params["preset"].(string); ok && presetName != "" {
		input.Preset = presetName
	}
	if layout, ok := params["layout"].(string); ok && layout != "" {
		input.Layout = layout
	}
	if size, ok := params["size"].(string); ok && size != "" {
		input.Size = size
	}

	output := h.engine.Execute(input)
	formation := output.Formation
	warnings = append(warnings, output.Warnings...)

	// Build entity data for debugging
	entityData := make([]TestbenchEntityData, 0, len(products))
	for _, p := range products {
		ed := TestbenchEntityData{
			ID:             p.ID,
			Name:           p.Name,
			Brand:          p.Brand,
			Category:       p.Category,
			Price:          p.Price,
			Currency:       p.Currency,
			Rating:         p.Rating,
			Images:         len(p.Images),
			StockQuantity:  p.StockQuantity,
			HasDescription: p.Description != "",
			HasTags:        len(p.Tags) > 0,
			ProductForm:    p.ProductForm,
		}
		if len(p.SkinType) > 0 {
			ed.SkinType = strings.Join(p.SkinType, ", ")
		}
		if len(p.Concern) > 0 {
			ed.Concern = strings.Join(p.Concern, ", ")
		}
		if len(p.KeyIngredients) > 0 {
			ed.KeyIngredients = strings.Join(p.KeyIngredients, ", ")
		}
		entityData = append(entityData, ed)
	}

	resp := TestbenchResponse{
		Formation: &FormationResponse{
			Mode:    string(formation.Mode),
			Grid:    formation.Grid,
			Widgets: formation.Widgets,
		},
		Entities: entityData,
		Warnings: warnings,
		Config: map[string]interface{}{
			"entityCount": len(products),
		},
	}

	writeJSON(w, http.StatusOK, resp)
}

// buildTestbenchOps converts testbench params into engine_v4 Ops.
// TODO: port advanced testbench features (show/hide/color/display/format overrides) to V4 ops
func buildTestbenchOps(params map[string]interface{}) []engine_v4.Op {
	_ = params // reserved for future ops-based overrides
	return nil
}
