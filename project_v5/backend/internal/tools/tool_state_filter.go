package tools

import (
	"context"
	"fmt"
	"strings"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// StateFilterTool filters products already loaded in state.Current.Data —
// no DB hit. Used when the user asks for a SUBSET of the current screen
// ("только COSRX", "дешевле 5000"). On 0 results, original data is
// preserved (V4 pattern at tool_state_filter.go:111-128).
type StateFilterTool struct {
	state ports.StatePort
}

// NewStateFilterTool wires the tool. Only StatePort is needed — no catalog,
// no LLM.
func NewStateFilterTool(state ports.StatePort) *StateFilterTool {
	return &StateFilterTool{state: state}
}

var _ Tool = (*StateFilterTool)(nil)

// Definition mirrors V4's tool_state_filter.go:23-62 minus the
// `service` entity_type (V5 catalog has products only).
func (t *StateFilterTool) Definition() domain.ToolDefinition {
	return domain.ToolDefinition{
		Name:        "_internal_state_filter",
		Description: "Filter already loaded products in state. Use when user wants a SUBSET of existing data (e.g. 'только COSRX', 'дешевле 5000'). Does NOT search catalog — only filters in-memory.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
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

// Execute applies the filter in-memory and writes the filtered subset back
// via UpdateData. Empty result preserves the original screen.
func (t *StateFilterTool) Execute(ctx context.Context, toolCtx domain.ToolContext, input map[string]interface{}) (*domain.ToolResult, error) {
	state, err := t.state.GetState(ctx, toolCtx.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}

	brand, _ := input["brand"].(string)
	category, _ := input["category"].(string)
	textMatch, _ := input["text_match"].(string)
	var minPriceKopecks, maxPriceKopecks int
	if v, ok := input["min_price"].(float64); ok {
		minPriceKopecks = int(v) * 100
	}
	if v, ok := input["max_price"].(float64); ok {
		maxPriceKopecks = int(v) * 100
	}
	var minRating float64
	if v, ok := input["min_rating"].(float64); ok {
		minRating = v
	}

	original := state.Current.Data.Products

	var filtered []domain.Product
	for _, p := range original {
		if !matchProduct(p, brand, category, textMatch, minPriceKopecks, maxPriceKopecks, minRating) {
			continue
		}
		filtered = append(filtered, p)
	}

	if len(filtered) == 0 {
		info := domain.DeltaInfo{
			TurnID:    toolCtx.TurnID,
			Trigger:   domain.TriggerUserQuery,
			Source:    domain.SourceLLM,
			ActorID:   "agent1",
			DeltaType: domain.DeltaTypeUpdate,
			Path:      "data.products",
			Action:    domain.Action{Type: domain.ActionFilter, Tool: "_internal_state_filter", Params: input},
			Result:    domain.ResultMeta{Count: 0},
		}
		if _, err := t.state.AddDelta(ctx, toolCtx.SessionID, info.ToDelta()); err != nil {
			return nil, fmt.Errorf("add empty delta: %w", err)
		}
		return &domain.ToolResult{
			Content: fmt.Sprintf("empty: 0 results from %d products, data preserved", len(original)),
		}, nil
	}

	// Products here come from state, where catalog_search already suppressed
	// price for tenants that hide it (Price zeroed). Mirror that: advertise a
	// price field only when the data actually carries one.
	fields := extractProductFields(filtered[0], filtered[0].Price > 0)

	data := domain.StateData{Products: filtered}
	stateMeta := domain.StateMeta{
		Count:        len(filtered),
		ProductCount: len(filtered),
		Fields:       fields,
		Aliases:      state.Current.Meta.Aliases,
	}
	info := domain.DeltaInfo{
		TurnID:    toolCtx.TurnID,
		Trigger:   domain.TriggerUserQuery,
		Source:    domain.SourceLLM,
		ActorID:   "agent1",
		DeltaType: domain.DeltaTypeUpdate,
		Path:      "data.products",
		Action:    domain.Action{Type: domain.ActionFilter, Tool: "_internal_state_filter", Params: input},
		Result:    domain.ResultMeta{Count: len(filtered), Fields: fields},
	}
	if _, err := t.state.UpdateData(ctx, toolCtx.SessionID, data, stateMeta, info); err != nil {
		return nil, fmt.Errorf("update data: %w", err)
	}

	return &domain.ToolResult{
		Content: fmt.Sprintf("ok: filtered %d items from %d products", len(filtered), len(original)),
	}, nil
}

// matchProduct returns true when the product passes every populated filter.
// Empty filters are ignored. minPrice / maxPrice are kopecks (caller has
// converted from rubles).
func matchProduct(p domain.Product, brand, category, textMatch string, minPriceKopecks, maxPriceKopecks int, minRating float64) bool {
	if brand != "" && !containsCI(p.Brand, brand) {
		return false
	}
	if category != "" && !containsCI(p.Category, category) {
		return false
	}
	if minPriceKopecks > 0 && p.Price < minPriceKopecks {
		return false
	}
	if maxPriceKopecks > 0 && p.Price > maxPriceKopecks {
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

func containsCI(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
