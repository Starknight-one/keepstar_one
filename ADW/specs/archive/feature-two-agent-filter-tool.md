# Feature: Two-Agent Pipeline - Filter Tool + Deltas (Phase 5)

## Feature Description

Implement the `filter_products` tool for the Two-Agent Pipeline. This enables:
- Filter current products by price, brand, rating without new search
- Incremental state updates via deltas
- Chain of queries: search → filter → filter → ...
- Delta history preserved for potential rollback

This is Phase 5 from SPEC_TWO_AGENT_PIPELINE.md - the filter/refinement step.

## Objective

Enable the flow: "покажи ноутбуки" → products → "только до 100000" → filtered products

**Verification**: After search + filter, state contains filtered products and both deltas are recorded.

## Expertise Context

Expertise used:
- **backend**: Hexagonal architecture, tools layer, StatePort, Delta structure, Agent1SystemPrompt

Key insights from expertise:
- Tools follow ToolExecutor interface: Definition() + Execute()
- Registry.Register() adds tool, definitions go to LLM
- Tools write to state, return "ok"/"empty" (not data)
- Delta has: step, trigger, action, result, template, createdAt
- Agent1 stops after first tool call
- StatePort.AddDelta() records history
- Filter tool works on state.Current.Data.Products (in-memory filter)

## Relevant Files

### Existing Files (to modify)
- `project/backend/internal/tools/tool_registry.go` - Register filter tool
- `project/backend/internal/prompts/prompt_analyze_query.go` - Add filter to Agent1 examples

### Existing Files (reference)
- `project/backend/internal/tools/tool_search_products.go` - Tool pattern
- `project/backend/internal/domain/state_entity.go` - Delta, ActionType
- `project/backend/internal/ports/state_port.go` - StatePort interface
- `project/backend/internal/domain/product_entity.go` - Product fields

### New Files
- `project/backend/internal/tools/tool_filter_products.go` - Filter tool implementation

## Step by Step Tasks

IMPORTANT: Execute strictly in order.

### 1. Create Filter Products Tool

File: `project/backend/internal/tools/tool_filter_products.go`

```go
package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"keepstar/internal/domain"
	"keepstar/internal/ports"
)

// FilterProductsTool filters current products in state
type FilterProductsTool struct {
	statePort ports.StatePort
}

// NewFilterProductsTool creates the tool
func NewFilterProductsTool(statePort ports.StatePort) *FilterProductsTool {
	return &FilterProductsTool{
		statePort: statePort,
	}
}

// Definition returns the tool definition for LLM
func (t *FilterProductsTool) Definition() domain.ToolDefinition {
	return domain.ToolDefinition{
		Name:        "filter_products",
		Description: "Filter current products by criteria. Use when user wants to narrow down existing results (e.g., 'только дешевле 50000', 'only Nike'). Does NOT do new search.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"min_price": map[string]interface{}{
					"type":        "number",
					"description": "Minimum price filter",
				},
				"max_price": map[string]interface{}{
					"type":        "number",
					"description": "Maximum price filter",
				},
				"brand": map[string]interface{}{
					"type":        "string",
					"description": "Brand filter (case-insensitive)",
				},
				"min_rating": map[string]interface{}{
					"type":        "number",
					"description": "Minimum rating (0-5)",
				},
				"in_stock": map[string]interface{}{
					"type":        "boolean",
					"description": "Only show in-stock items",
				},
			},
		},
	}
}

// Execute filters products in state, returns "ok" or "empty"
func (t *FilterProductsTool) Execute(ctx context.Context, sessionID string, input map[string]interface{}) (*domain.ToolResult, error) {
	// Get current state
	state, err := t.statePort.GetState(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}

	// Check if we have products to filter
	if len(state.Current.Data.Products) == 0 {
		return &domain.ToolResult{
			Content: "empty: no products to filter",
		}, nil
	}

	// Parse filter params
	minPrice := 0
	maxPrice := 0
	brand := ""
	minRating := 0.0
	inStock := false

	if mp, ok := input["min_price"].(float64); ok {
		minPrice = int(mp)
	}
	if mp, ok := input["max_price"].(float64); ok {
		maxPrice = int(mp)
	}
	if b, ok := input["brand"].(string); ok {
		brand = strings.ToLower(b)
	}
	if mr, ok := input["min_rating"].(float64); ok {
		minRating = mr
	}
	if is, ok := input["in_stock"].(bool); ok {
		inStock = is
	}

	// Filter products
	filtered := make([]domain.Product, 0)
	for _, p := range state.Current.Data.Products {
		if !matchesFilter(p, minPrice, maxPrice, brand, minRating, inStock) {
			continue
		}
		filtered = append(filtered, p)
	}

	if len(filtered) == 0 {
		return &domain.ToolResult{
			Content: "empty: no products match filter",
		}, nil
	}

	// Update state with filtered products
	state.Current.Data.Products = filtered
	state.Current.Meta.Count = len(filtered)
	state.Step++

	if err := t.statePort.UpdateState(ctx, state); err != nil {
		return nil, fmt.Errorf("update state: %w", err)
	}

	// Create and save delta
	delta := &domain.Delta{
		Step:    state.Step,
		Trigger: domain.TriggerUserQuery,
		Action: domain.Action{
			Type:   domain.ActionFilter,
			Tool:   "filter_products",
			Params: input,
		},
		Result: domain.ResultMeta{
			Count:  len(filtered),
			Fields: state.Current.Meta.Fields,
		},
		CreatedAt: time.Now(),
	}

	if err := t.statePort.AddDelta(ctx, sessionID, delta); err != nil {
		return nil, fmt.Errorf("add delta: %w", err)
	}

	return &domain.ToolResult{
		Content: fmt.Sprintf("ok: %d products match filter", len(filtered)),
	}, nil
}

// matchesFilter checks if product matches all filter criteria
func matchesFilter(p domain.Product, minPrice, maxPrice int, brand string, minRating float64, inStock bool) bool {
	// Price filter (prices in kopecks, convert to rubles for comparison)
	priceRubles := p.Price / 100
	if minPrice > 0 && priceRubles < minPrice {
		return false
	}
	if maxPrice > 0 && priceRubles > maxPrice {
		return false
	}

	// Brand filter (case-insensitive)
	if brand != "" && !strings.EqualFold(p.Brand, brand) {
		return false
	}

	// Rating filter
	if minRating > 0 && p.Rating < minRating {
		return false
	}

	// Stock filter
	if inStock && p.StockQuantity <= 0 {
		return false
	}

	return true
}
```

### 2. Register Filter Tool

File: `project/backend/internal/tools/tool_registry.go`

Update NewRegistry function to register filter tool:

```go
// NewRegistry creates a tool registry with dependencies
func NewRegistry(statePort ports.StatePort, catalogPort ports.CatalogPort) *Registry {
	r := &Registry{
		tools:       make(map[string]ToolExecutor),
		statePort:   statePort,
		catalogPort: catalogPort,
	}

	// Register available tools
	r.Register(NewSearchProductsTool(statePort, catalogPort))
	r.Register(NewFilterProductsTool(statePort))

	return r
}
```

### 3. Update Agent1 System Prompt

File: `project/backend/internal/prompts/prompt_analyze_query.go`

Update Agent1SystemPrompt to include filter tool:

```go
// Agent1SystemPrompt is the system prompt for Agent 1 (Tool Caller)
const Agent1SystemPrompt = `You are Agent 1 - a fast tool caller for an e-commerce chat.

Your ONLY job: understand user query and call the right tool. Nothing else.

Rules:
1. ALWAYS call a tool. Never respond with just text.
2. Do NOT explain what you're doing.
3. Do NOT ask clarifying questions - make best guess.
4. Tool results are written to state. You only get "ok" or "empty".
5. After getting "ok"/"empty", stop. Do not call more tools.

Available tools:
- search_products: Search for products by query, category, brand, price range. Use for NEW searches.
- filter_products: Filter CURRENT products by price, brand, rating. Use when user wants to narrow down existing results.

When to use which:
- "покажи ноутбуки" → search_products (new search)
- "найди кроссовки Nike" → search_products (new search)
- "только до 50000" → filter_products (narrowing current results)
- "покажи только Samsung" → filter_products (brand filter on current)
- "с рейтингом выше 4" → filter_products (rating filter)
- "дешевле 100000" → filter_products (price filter)

Examples:
- "покажи ноутбуки" → search_products(query="ноутбуки")
- "Nike shoes under $100" → search_products(query="Nike shoes", max_price=100)
- "дешевые телефоны Samsung" → search_products(query="телефоны", brand="Samsung", max_price=20000)
- "только до 50000" → filter_products(max_price=50000)
- "покажи только Asus" → filter_products(brand="Asus")
- "с рейтингом от 4.5" → filter_products(min_rating=4.5)
`
```

### 4. Add ActionFilter Constant (if not exists)

File: `project/backend/internal/domain/state_entity.go`

Verify ActionFilter exists in ActionType constants:

```go
const (
	ActionSearch   ActionType = "SEARCH"
	ActionFilter   ActionType = "FILTER"  // Should already exist
	ActionSort     ActionType = "SORT"
	ActionLayout   ActionType = "LAYOUT"
	ActionRollback ActionType = "ROLLBACK"
)
```

### 5. Validation

Run validation commands:

```bash
cd project/backend && go build ./...
cd project/backend && go test ./...
```

Manual verification:
1. Create a test session
2. Call Pipeline with query "покажи ноутбуки" → products in state
3. Call Pipeline with query "только до 100000" → filter applied
4. Verify state.Current.Data.Products is filtered
5. Verify GetDeltas returns both SEARCH and FILTER deltas
6. Verify delta.Action.Type is correct for each

## Validation Commands

From ADW/adw.yaml:
- `cd project/backend && go build ./...` (required)
- `cd project/backend && go test ./...` (optional)

## Acceptance Criteria

- [ ] FilterProductsTool implements ToolExecutor interface
- [ ] Tool filters by: min_price, max_price, brand, min_rating, in_stock
- [ ] Tool works on state.Current.Data.Products (in-memory)
- [ ] Tool updates state with filtered results
- [ ] Tool creates Delta with ActionFilter type
- [ ] Tool returns "ok: N products match filter" or "empty"
- [ ] Registry registers filter tool
- [ ] Agent1SystemPrompt includes filter_products with examples
- [ ] Agent1 correctly chooses between search and filter
- [ ] Backend builds without errors
- [ ] Chain "покажи ноутбуки" → "до 100000" produces 2 deltas

## Notes

- **Price in kopecks**: Product.Price is stored in kopecks (int). Filter converts user input (assumed rubles) to compare correctly.
- **In-memory filter**: filter_products works on current state products, not database query. This is intentional for speed and delta tracking.
- **No new LLM call for filter results**: After filter, Agent 2 is still triggered to rebuild template (count may change).
- **Delta history**: Each filter creates a delta. Future rollback can replay deltas to restore state.
- **Brand case-insensitive**: strings.EqualFold for brand comparison.
- Agent1 must learn to distinguish "new search" vs "filter current". Examples in prompt are critical.

## Dependencies

- **Phase 1 (State + Storage)** must be complete - StatePort working
- **Phase 2 (Agent 1 + search tool)** must be complete - Registry pattern
- **Phase 3 (Agent 2 + Template)** must be complete - Pipeline triggers Agent 2 after filter
