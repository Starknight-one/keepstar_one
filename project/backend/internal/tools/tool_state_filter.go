package tools

import (
	"context"
	"fmt"
	"strings"

	"keepstar/internal/domain"
	"keepstar/internal/ports"
)

// StateFilterTool filters already-loaded data in state (in-memory)
type StateFilterTool struct {
	statePort ports.StatePort
}

// NewStateFilterTool creates the state filter tool
func NewStateFilterTool(statePort ports.StatePort) *StateFilterTool {
	return &StateFilterTool{statePort: statePort}
}

// Definition returns the tool definition for LLM
func (t *StateFilterTool) Definition() domain.ToolDefinition {
	return domain.ToolDefinition{
		Name:        "_internal_state_filter",
		Description: "Filter already loaded products in state. Use when user wants a SUBSET of existing data (e.g. 'только COSRX', 'дешевле 5000'). Does NOT search catalog — only filters in-memory.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"entity_type": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"product", "service"},
					"description": "Entity type to filter. Default: product.",
				},
				"brand": map[string]interface{}{
					"type":        "string",
					"description": "Filter by brand name (case-insensitive contains).",
				},
				"category": map[string]interface{}{
					"type":        "string",
					"description": "Filter by category name (case-insensitive contains).",
				},
				"min_price": map[string]interface{}{
					"type":        "number",
					"description": "Minimum price in RUBLES.",
				},
				"max_price": map[string]interface{}{
					"type":        "number",
					"description": "Maximum price in RUBLES.",
				},
				"min_rating": map[string]interface{}{
					"type":        "number",
					"description": "Minimum rating (0-5).",
				},
				"text_match": map[string]interface{}{
					"type":        "string",
					"description": "Free text match against name and description (case-insensitive).",
				},
			},
		},
	}
}

// Execute filters in-memory products/services and writes filtered data to state
func (t *StateFilterTool) Execute(ctx context.Context, toolCtx ToolContext, input map[string]interface{}) (*domain.ToolResult, error) {
	state, err := t.statePort.GetState(ctx, toolCtx.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}

	// Parse filters
	brand, _ := input["brand"].(string)
	category, _ := input["category"].(string)
	textMatch, _ := input["text_match"].(string)
	var minPrice, maxPrice int
	if v, ok := input["min_price"].(float64); ok {
		minPrice = int(v) * 100 // rubles → kopecks
	}
	if v, ok := input["max_price"].(float64); ok {
		maxPrice = int(v) * 100 // rubles → kopecks
	}
	var minRating float64
	if v, ok := input["min_rating"].(float64); ok {
		minRating = v
	}

	originalProducts := state.Current.Data.Products
	originalServices := state.Current.Data.Services

	// Filter products
	var filteredProducts []domain.Product
	for _, p := range originalProducts {
		if !matchProduct(p, brand, category, textMatch, minPrice, maxPrice, minRating) {
			continue
		}
		filteredProducts = append(filteredProducts, p)
	}

	// Filter services
	var filteredServices []domain.Service
	for _, s := range originalServices {
		if !matchService(s, brand, category, textMatch, minPrice, maxPrice, minRating) {
			continue
		}
		filteredServices = append(filteredServices, s)
	}

	total := len(filteredProducts) + len(filteredServices)

	// If 0 results — preserve existing data
	if total == 0 {
		info := domain.DeltaInfo{
			TurnID:    toolCtx.TurnID,
			Trigger:   domain.TriggerUserQuery,
			Source:    domain.SourceLLM,
			ActorID:   toolCtx.ActorID,
			DeltaType: domain.DeltaTypeUpdate,
			Path:      "data.products",
			Action:    domain.Action{Type: domain.ActionFilter, Tool: "_internal_state_filter", Params: input},
			Result:    domain.ResultMeta{Count: 0},
		}
		if _, err := t.statePort.AddDelta(ctx, toolCtx.SessionID, info.ToDelta()); err != nil {
			return nil, fmt.Errorf("add empty delta: %w", err)
		}
		return &domain.ToolResult{
			Content: fmt.Sprintf("empty: 0 results from %d products, data preserved", len(originalProducts)),
		}, nil
	}

	// Extract fields from first result
	var fields []string
	if len(filteredProducts) > 0 {
		fields = catalogExtractProductFields(filteredProducts[0])
	} else if len(filteredServices) > 0 {
		fields = catalogExtractServiceFields(filteredServices[0])
	}

	// Write filtered data to state
	data := domain.StateData{Products: filteredProducts, Services: filteredServices}
	stateMeta := domain.StateMeta{
		Count:        total,
		ProductCount: len(filteredProducts),
		ServiceCount: len(filteredServices),
		Fields:       fields,
		Aliases:      state.Current.Meta.Aliases,
	}

	info := domain.DeltaInfo{
		TurnID:    toolCtx.TurnID,
		Trigger:   domain.TriggerUserQuery,
		Source:    domain.SourceLLM,
		ActorID:   toolCtx.ActorID,
		DeltaType: domain.DeltaTypeUpdate,
		Path:      "data.products",
		Action:    domain.Action{Type: domain.ActionFilter, Tool: "_internal_state_filter", Params: input},
		Result:    domain.ResultMeta{Count: total, Fields: fields},
	}
	if _, err := t.statePort.UpdateData(ctx, toolCtx.SessionID, data, stateMeta, info); err != nil {
		return nil, fmt.Errorf("update data: %w", err)
	}

	return &domain.ToolResult{
		Content: fmt.Sprintf("ok: filtered %d items from %d products", total, len(originalProducts)+len(originalServices)),
	}, nil
}

// matchProduct checks if a product matches all specified filters
func matchProduct(p domain.Product, brand, category, textMatch string, minPrice, maxPrice int, minRating float64) bool {
	if brand != "" && !containsCI(p.Brand, brand) {
		return false
	}
	if category != "" && !containsCI(p.Category, category) {
		return false
	}
	if minPrice > 0 && p.Price < minPrice {
		return false
	}
	if maxPrice > 0 && p.Price > maxPrice {
		return false
	}
	if minRating > 0 && p.Rating < minRating {
		return false
	}
	if textMatch != "" && !containsCI(p.Name, textMatch) && !containsCI(p.Description, textMatch) {
		return false
	}
	return true
}

// matchService checks if a service matches all specified filters
func matchService(s domain.Service, brand, category, textMatch string, minPrice, maxPrice int, minRating float64) bool {
	if brand != "" && !containsCI(s.Provider, brand) {
		return false
	}
	if category != "" && !containsCI(s.Category, category) {
		return false
	}
	if minPrice > 0 && s.Price < minPrice {
		return false
	}
	if maxPrice > 0 && s.Price > maxPrice {
		return false
	}
	if minRating > 0 && s.Rating < minRating {
		return false
	}
	if textMatch != "" && !containsCI(s.Name, textMatch) && !containsCI(s.Description, textMatch) {
		return false
	}
	return true
}

// containsCI is case-insensitive string contains
func containsCI(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
