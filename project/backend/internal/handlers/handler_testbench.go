package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"keepstar/internal/domain"
	"keepstar/internal/engine"
	"keepstar/internal/ports"
	"keepstar/internal/presets"
)

// TestbenchHandler handles testbench API requests
type TestbenchHandler struct {
	catalogPort      ports.CatalogPort
	fieldDefPort     ports.FieldDefinitionPort
	presetV2Registry *presets.PresetV2Registry
}

// NewTestbenchHandler creates a testbench handler
func NewTestbenchHandler(catalogPort ports.CatalogPort, fieldDefPort ports.FieldDefinitionPort, presetV2Registry *presets.PresetV2Registry) *TestbenchHandler {
	return &TestbenchHandler{
		catalogPort:      catalogPort,
		fieldDefPort:     fieldDefPort,
		presetV2Registry: presetV2Registry,
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

	// Build formation using V2 engine
	params := req.Params
	if params == nil {
		params = make(map[string]interface{})
	}

	// Build AgentInstructions from testbench params
	instructions := buildTestbenchInstructions(params)

	// Look up V2 preset if specified
	var preset *domain.PresetV2
	if presetName, ok := params["preset"].(string); ok && presetName != "" {
		if h.presetV2Registry != nil {
			if p, ok := h.presetV2Registry.Get(presetName); ok {
				preset = &p
			} else {
				warnings = append(warnings, fmt.Sprintf("unknown preset: %s", presetName))
			}
		} else {
			warnings = append(warnings, "preset registry not available (engine v2 not enabled)")
		}
	}

	// Load field definitions if available
	var fieldDefs []engine.FieldDefinitionEntry
	if h.fieldDefPort != nil {
		if defs, err := h.fieldDefPort.ListFieldDefinitions(ctx, tenantSlug, "product"); err == nil {
			fieldDefs = make([]engine.FieldDefinitionEntry, len(defs))
			for i, d := range defs {
				fieldDefs[i] = engine.FieldDefinitionEntry{
					FieldName:      d.FieldName,
					AtomType:       d.AtomType,
					AtomSubtype:    d.AtomSubtype,
					DefaultDisplay: d.DefaultDisplay,
					DefaultSlot:    d.DefaultSlot,
					Label:          d.Label,
					Priority:       d.Priority,
				}
			}
		}
	}

	// Execute V2 engine
	eng := engine.NewEngineV2()
	output := eng.Execute(engine.EngineV2Input{
		EntityType:   domain.EntityTypeProduct,
		Products:     products,
		FieldDefs:    fieldDefs,
		Instructions: instructions,
		Preset:       preset,
	})
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

// buildTestbenchInstructions converts testbench params into AgentInstructions for EngineV2.
func buildTestbenchInstructions(params map[string]interface{}) *engine.AgentInstructions {
	inst := &engine.AgentInstructions{}
	hasAny := false

	if showRaw, ok := params["show"].([]interface{}); ok && len(showRaw) > 0 {
		for _, s := range showRaw {
			if name, ok := s.(string); ok {
				inst.Show = append(inst.Show, name)
			}
		}
		hasAny = true
	}
	if hideRaw, ok := params["hide"].([]interface{}); ok && len(hideRaw) > 0 {
		for _, s := range hideRaw {
			if name, ok := s.(string); ok {
				inst.Hide = append(inst.Hide, name)
			}
		}
		hasAny = true
	}
	if orderRaw, ok := params["order"].([]interface{}); ok && len(orderRaw) > 0 {
		for _, s := range orderRaw {
			if name, ok := s.(string); ok {
				inst.Order = append(inst.Order, name)
			}
		}
		hasAny = true
	}
	if l, ok := params["layout"].(string); ok && l != "" {
		inst.Layout = l
		hasAny = true
	}
	if s, ok := params["size"].(string); ok && s != "" {
		inst.Size = s
		hasAny = true
	}
	if d, ok := params["direction"].(string); ok && d != "" {
		inst.Direction = d
		hasAny = true
	}
	// Color overrides → atom overrides
	if colorRaw, ok := params["color"].(map[string]interface{}); ok && len(colorRaw) > 0 {
		if inst.Atoms == nil {
			inst.Atoms = make(map[string]engine.AtomOverride)
		}
		for field, c := range colorRaw {
			if cs, ok := c.(string); ok {
				override := inst.Atoms[field]
				override.Color = cs
				inst.Atoms[field] = override
			}
		}
		hasAny = true
	}
	// Display overrides → atom textStyle/wrapper overrides (legacy display string → v2 TextStyle+Wrapper)
	if displayRaw, ok := params["display"].(map[string]interface{}); ok && len(displayRaw) > 0 {
		if inst.Atoms == nil {
			inst.Atoms = make(map[string]engine.AtomOverride)
		}
		for field, d := range displayRaw {
			if ds, ok := d.(string); ok {
				override := inst.Atoms[field]
				ts, wr := engine.DisplayToTextStyleWrapper(ds)
				override.TextStyle = ts
				override.Wrapper = wr
				inst.Atoms[field] = override
			}
		}
		hasAny = true
	}
	// Format overrides → atom format overrides
	if formatRaw, ok := params["format"].(map[string]interface{}); ok && len(formatRaw) > 0 {
		if inst.Atoms == nil {
			inst.Atoms = make(map[string]engine.AtomOverride)
		}
		for field, f := range formatRaw {
			if fs, ok := f.(string); ok {
				override := inst.Atoms[field]
				override.Format = fs
				inst.Atoms[field] = override
			}
		}
		hasAny = true
	}

	if !hasAny {
		return nil
	}
	return inst
}
