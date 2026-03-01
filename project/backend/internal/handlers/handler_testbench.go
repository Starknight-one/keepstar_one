package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"keepstar/internal/domain"
	"keepstar/internal/engine"
	"keepstar/internal/ports"
	"keepstar/internal/presets"
)

// TestbenchHandler handles testbench API requests
type TestbenchHandler struct {
	catalogPort    ports.CatalogPort
	presetRegistry *presets.PresetRegistry
}

// NewTestbenchHandler creates a testbench handler
func NewTestbenchHandler(catalogPort ports.CatalogPort, presetRegistry *presets.PresetRegistry) *TestbenchHandler {
	return &TestbenchHandler{
		catalogPort:    catalogPort,
		presetRegistry: presetRegistry,
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

	// Build formation using visual assembly logic
	params := req.Params
	if params == nil {
		params = make(map[string]interface{})
	}

	entityType := "product"
	entityCount := len(products)
	resolved := engine.AutoResolve(entityType, entityCount)
	fields := resolved.Fields
	displayOverrides := make(map[string]string)
	layout := resolved.Layout
	size := resolved.Size

	// Apply preset if specified
	if presetName, ok := params["preset"].(string); ok && presetName != "" {
		if preset, ok := h.presetRegistry.Get(domain.PresetName(presetName)); ok {
			fields = make([]string, 0, len(preset.Fields))
			for _, f := range preset.Fields {
				fields = append(fields, f.Name)
				displayOverrides[f.Name] = string(f.Display)
			}
			layout = string(preset.DefaultMode)
			size = preset.DefaultSize
		} else {
			warnings = append(warnings, fmt.Sprintf("unknown preset: %s", presetName))
		}
	}

	// Apply show/hide
	if showRaw, ok := params["show"].([]interface{}); ok && len(showRaw) > 0 {
		for _, s := range showRaw {
			if name, ok := s.(string); ok {
				found := false
				for _, f := range fields {
					if f == name {
						found = true
						break
					}
				}
				if !found {
					fields = append(fields, name)
				}
			}
		}
	}
	if hideRaw, ok := params["hide"].([]interface{}); ok && len(hideRaw) > 0 {
		hideSet := make(map[string]bool)
		for _, h := range hideRaw {
			if name, ok := h.(string); ok {
				hideSet[name] = true
			}
		}
		filtered := make([]string, 0)
		for _, f := range fields {
			if !hideSet[f] {
				filtered = append(filtered, f)
			}
		}
		fields = filtered
	}

	// Apply order
	hasExplicitShow := false
	if showRaw, ok := params["show"].([]interface{}); ok && len(showRaw) > 0 {
		hasExplicitShow = true
	}
	if orderRaw, ok := params["order"].([]interface{}); ok && len(orderRaw) > 0 {
		ordered := make([]string, 0, len(fields))
		inOrder := make(map[string]bool)
		for _, o := range orderRaw {
			if name, ok := o.(string); ok {
				for _, f := range fields {
					if f == name && !inOrder[name] {
						ordered = append(ordered, name)
						inOrder[name] = true
						break
					}
				}
			}
		}
		for _, f := range fields {
			if !inOrder[f] {
				ordered = append(ordered, f)
			}
		}
		fields = ordered
	}

	// Apply layout/size overrides
	if l, ok := params["layout"].(string); ok && l != "" {
		layout = l
	}
	if s, ok := params["size"].(string); ok && s != "" {
		size = domain.WidgetSize(s)
	}

	// Apply display overrides
	if displayRaw, ok := params["display"].(map[string]interface{}); ok {
		for field, disp := range displayRaw {
			if d, ok := disp.(string); ok {
				displayOverrides[field] = d
			}
		}
	}

	// Apply format overrides
	formatOverrides := make(map[string]string)
	if formatRaw, ok := params["format"].(map[string]interface{}); ok {
		for field, f := range formatRaw {
			if fs, ok := f.(string); ok {
				formatOverrides[field] = fs
			}
		}
	}

	// Build field configs (with format inference)
	fieldConfigs := engine.BuildFieldConfigsWithFormat(fields, displayOverrides, formatOverrides)
	sort.Slice(fieldConfigs, func(i, j int) bool {
		return fieldConfigs[i].Priority < fieldConfigs[j].Priority
	})

	formationMode := parseTestbenchFormationType(layout)
	template := "GenericCard"

	// Build widgets
	widgets := make([]domain.Widget, 0, len(products))
	for i, p := range products {
		getter := engine.ProductFieldGetter(p)
		currency := p.Currency
		if currency == "" {
			currency = "$"
		}
		atoms := engine.BuildAtoms(fieldConfigs, getter, func() string { return currency })

		// Apply constraints
		atoms = engine.ApplyAtomConstraints(atoms)

		widget := domain.Widget{
			ID:       fmt.Sprintf("tb-%d", i),
			Template: template,
			Size:     size,
			Priority: i,
			Atoms:    atoms,
			EntityRef: &domain.EntityRef{
				Type: domain.EntityTypeProduct,
				ID:   p.ID,
			},
		}
		engine.ApplyWidgetConstraints(&widget)

		// Calculate layout zones
		widget.Zones = engine.CalculateZones(widget.Atoms, engine.DefaultDesignTokens())

		widgets = append(widgets, widget)
	}

	// Apply cross-widget constraints
	if !hasExplicitShow {
		engine.ApplyCrossWidgetConstraints(widgets, formationMode)
	}

	// Apply color, direction, shape
	colorMap := make(map[string]string)
	if colorRaw, ok := params["color"].(map[string]interface{}); ok {
		for field, c := range colorRaw {
			if cs, ok := c.(string); ok {
				colorMap[field] = cs
			}
		}
	}
	direction, _ := params["direction"].(string)

	for i := range widgets {
		if len(colorMap) > 0 {
			for ai := range widgets[i].Atoms {
				if color, ok := colorMap[widgets[i].Atoms[ai].FieldName]; ok {
					if widgets[i].Atoms[ai].Meta == nil {
						widgets[i].Atoms[ai].Meta = make(map[string]interface{})
					}
					widgets[i].Atoms[ai].Meta["color"] = color
				}
			}
		}
		if direction != "" {
			if widgets[i].Meta == nil {
				widgets[i].Meta = make(map[string]interface{})
			}
			widgets[i].Meta["direction"] = direction
		}
	}

	formation := &domain.FormationWithData{
		Mode:    formationMode,
		Widgets: widgets,
	}
	if formationMode == domain.FormationTypeGrid {
		formation.Grid = engine.CalcGridConfig(len(widgets), size)
	}

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
			"layout": layout,
			"size":   size,
			"fields": fields,
		},
	}

	writeJSON(w, http.StatusOK, resp)
}

func parseTestbenchFormationType(mode string) domain.FormationType {
	switch mode {
	case "grid":
		return domain.FormationTypeGrid
	case "list":
		return domain.FormationTypeList
	case "carousel":
		return domain.FormationTypeCarousel
	case "single":
		return domain.FormationTypeSingle
	case "comparison":
		return domain.FormationTypeComparison
	default:
		return domain.FormationTypeGrid
	}
}
