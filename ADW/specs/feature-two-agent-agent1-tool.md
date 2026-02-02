# Feature: Two-Agent Pipeline - Agent 1 + Tool (Phase 2)

## Feature Description

Implement Agent 1 (Tool Caller) with the `search_products` tool for the Two-Agent Pipeline. This creates a minimal working pipeline where:
- User query goes to Agent 1 (Claude with tools)
- Agent 1 calls `search_products` tool
- Tool executes, writes data to State, returns "ok"/"empty"
- Delta is recorded

This is Phase 2 from SPEC_TWO_AGENT_PIPELINE.md - the first functional pipeline step.

## Objective

Enable the flow: user query → Agent 1 → tool call → data in state

**Verification**: "покажи ноутбуки" → state.data.products is populated

## Expertise Context

Expertise used:
- **backend**: Hexagonal architecture, Anthropic adapter, LLMPort interface, usecase patterns

Key insights from expertise:
- Current `LLMPort.Chat()` is single-message, returns string only
- Anthropic client uses raw HTTP (no SDK), API version `2023-06-01`
- Need to extend for tool calling (API version `2024-06-01`+)
- Existing `CatalogPort.ListProducts()` can be used for actual search
- State storage from Phase 1 ready to receive deltas

## Relevant Files

### Existing Files (to modify)
- `project/backend/internal/ports/llm_port.go` - Extend with ChatWithTools
- `project/backend/internal/adapters/anthropic/anthropic_client.go` - Add tool support
- `project/backend/internal/prompts/prompt_analyze_query.go` - Agent 1 system prompt
- `project/backend/cmd/server/main.go` - Wire new components

### Existing Files (reference)
- `project/backend/internal/ports/state_port.go` - StatePort from Phase 1
- `project/backend/internal/adapters/postgres/postgres_state.go` - State adapter
- `project/backend/internal/ports/catalog_port.go` - CatalogPort for search
- `project/backend/internal/domain/state_entity.go` - State, Delta types

### New Files
- `project/backend/internal/domain/tool_entity.go` - Tool definitions domain types
- `project/backend/internal/tools/tool_registry.go` - Tool registry and executor
- `project/backend/internal/tools/tool_search_products.go` - search_products implementation
- `project/backend/internal/usecases/agent1_execute.go` - Agent 1 use case

## Step by Step Tasks

IMPORTANT: Execute strictly in order. Phase 1 (State + Storage) must be complete first.

### 1. Create Tool Domain Types

File: `project/backend/internal/domain/tool_entity.go`

```go
package domain

// ToolDefinition describes a tool for the LLM
type ToolDefinition struct {
    Name        string                 `json:"name"`
    Description string                 `json:"description"`
    InputSchema map[string]interface{} `json:"input_schema"`
}

// ToolCall represents a tool invocation from the LLM
type ToolCall struct {
    ID    string                 `json:"id"`
    Name  string                 `json:"name"`
    Input map[string]interface{} `json:"input"`
}

// ToolResult represents the result of a tool execution
type ToolResult struct {
    ToolUseID string `json:"tool_use_id"`
    Content   string `json:"content"` // "ok", "empty", or error message
    IsError   bool   `json:"is_error,omitempty"`
}

// LLMMessage represents a message in conversation (extended for tools)
type LLMMessage struct {
    Role       string       `json:"role"`    // "user", "assistant"
    Content    string       `json:"content,omitempty"`
    ToolCalls  []ToolCall   `json:"tool_calls,omitempty"`  // For assistant
    ToolResult *ToolResult  `json:"tool_result,omitempty"` // For user (tool_result)
}

// LLMResponse represents response from LLM with potential tool calls
type LLMResponse struct {
    Text       string     `json:"text,omitempty"`
    ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
    StopReason string     `json:"stop_reason"` // "end_turn", "tool_use"
}
```

### 2. Extend LLMPort Interface

File: `project/backend/internal/ports/llm_port.go`

Add new method to existing interface:

```go
package ports

import (
    "context"
    "keepstar/internal/domain"
)

// LLMPort defines operations for LLM interaction
type LLMPort interface {
    // Chat sends a simple message and gets text response (existing)
    Chat(ctx context.Context, message string) (string, error)

    // ChatWithTools sends messages with tool definitions, returns potential tool calls
    ChatWithTools(
        ctx context.Context,
        systemPrompt string,
        messages []domain.LLMMessage,
        tools []domain.ToolDefinition,
    ) (*domain.LLMResponse, error)
}
```

### 3. Update Anthropic Client for Tool Calling

File: `project/backend/internal/adapters/anthropic/anthropic_client.go`

Add tool support structures and ChatWithTools method:

```go
// Add new request/response types for tools

type anthropicToolRequest struct {
    Model     string               `json:"model"`
    MaxTokens int                  `json:"max_tokens"`
    System    string               `json:"system,omitempty"`
    Messages  []anthropicToolMsg   `json:"messages"`
    Tools     []anthropicTool      `json:"tools,omitempty"`
}

type anthropicTool struct {
    Name        string                 `json:"name"`
    Description string                 `json:"description"`
    InputSchema map[string]interface{} `json:"input_schema"`
}

type anthropicToolMsg struct {
    Role    string      `json:"role"`
    Content interface{} `json:"content"` // string or []contentBlock
}

type contentBlock struct {
    Type      string                 `json:"type"` // "text", "tool_use", "tool_result"
    Text      string                 `json:"text,omitempty"`
    ID        string                 `json:"id,omitempty"`
    Name      string                 `json:"name,omitempty"`
    Input     map[string]interface{} `json:"input,omitempty"`
    ToolUseID string                 `json:"tool_use_id,omitempty"`
    Content   string                 `json:"content,omitempty"`
    IsError   bool                   `json:"is_error,omitempty"`
}

type anthropicToolResponse struct {
    Content    []contentBlock `json:"content"`
    StopReason string         `json:"stop_reason"`
    Error      *struct {
        Message string `json:"message"`
    } `json:"error,omitempty"`
}

// ChatWithTools implements LLMPort.ChatWithTools
func (c *Client) ChatWithTools(
    ctx context.Context,
    systemPrompt string,
    messages []domain.LLMMessage,
    tools []domain.ToolDefinition,
) (*domain.LLMResponse, error) {
    // Convert domain messages to Anthropic format
    anthroMsgs := make([]anthropicToolMsg, 0, len(messages))
    for _, msg := range messages {
        anthroMsgs = append(anthroMsgs, convertToAnthropicMessage(msg))
    }

    // Convert tools
    anthroTools := make([]anthropicTool, 0, len(tools))
    for _, t := range tools {
        anthroTools = append(anthroTools, anthropicTool{
            Name:        t.Name,
            Description: t.Description,
            InputSchema: t.InputSchema,
        })
    }

    reqBody := anthropicToolRequest{
        Model:     c.model,
        MaxTokens: 4096,
        System:    systemPrompt,
        Messages:  anthroMsgs,
        Tools:     anthroTools,
    }

    jsonBody, err := json.Marshal(reqBody)
    if err != nil {
        return nil, fmt.Errorf("marshal request: %w", err)
    }

    req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonBody))
    if err != nil {
        return nil, fmt.Errorf("create request: %w", err)
    }

    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("x-api-key", c.apiKey)
    req.Header.Set("anthropic-version", "2024-06-01") // Updated for tools

    resp, err := c.httpClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("send request: %w", err)
    }
    defer resp.Body.Close()

    var anthroResp anthropicToolResponse
    if err := json.NewDecoder(resp.Body).Decode(&anthroResp); err != nil {
        return nil, fmt.Errorf("decode response: %w", err)
    }

    if anthroResp.Error != nil {
        return nil, fmt.Errorf("anthropic error: %s", anthroResp.Error.Message)
    }

    // Convert response
    result := &domain.LLMResponse{
        StopReason: anthroResp.StopReason,
    }

    for _, block := range anthroResp.Content {
        switch block.Type {
        case "text":
            result.Text = block.Text
        case "tool_use":
            result.ToolCalls = append(result.ToolCalls, domain.ToolCall{
                ID:    block.ID,
                Name:  block.Name,
                Input: block.Input,
            })
        }
    }

    return result, nil
}

// Helper to convert domain message to Anthropic format
func convertToAnthropicMessage(msg domain.LLMMessage) anthropicToolMsg {
    if msg.ToolResult != nil {
        // Tool result message
        return anthropicToolMsg{
            Role: "user",
            Content: []contentBlock{{
                Type:      "tool_result",
                ToolUseID: msg.ToolResult.ToolUseID,
                Content:   msg.ToolResult.Content,
                IsError:   msg.ToolResult.IsError,
            }},
        }
    }

    if len(msg.ToolCalls) > 0 {
        // Assistant message with tool calls
        blocks := make([]contentBlock, 0, len(msg.ToolCalls))
        for _, tc := range msg.ToolCalls {
            blocks = append(blocks, contentBlock{
                Type:  "tool_use",
                ID:    tc.ID,
                Name:  tc.Name,
                Input: tc.Input,
            })
        }
        return anthropicToolMsg{Role: "assistant", Content: blocks}
    }

    // Simple text message
    return anthropicToolMsg{Role: msg.Role, Content: msg.Content}
}
```

### 4. Create Tool Registry

File: `project/backend/internal/tools/tool_registry.go`

```go
package tools

import (
    "context"
    "fmt"

    "keepstar/internal/domain"
    "keepstar/internal/ports"
)

// ToolExecutor executes a tool and writes results to state
type ToolExecutor interface {
    // Definition returns the tool definition for LLM
    Definition() domain.ToolDefinition

    // Execute runs the tool with given input, writes to state, returns "ok"/"empty"
    Execute(ctx context.Context, sessionID string, input map[string]interface{}) (*domain.ToolResult, error)
}

// Registry holds all available tools
type Registry struct {
    tools      map[string]ToolExecutor
    statePort  ports.StatePort
    catalogPort ports.CatalogPort
}

// NewRegistry creates a tool registry with dependencies
func NewRegistry(statePort ports.StatePort, catalogPort ports.CatalogPort) *Registry {
    r := &Registry{
        tools:       make(map[string]ToolExecutor),
        statePort:   statePort,
        catalogPort: catalogPort,
    }

    // Register available tools
    r.Register(NewSearchProductsTool(statePort, catalogPort))

    return r
}

// Register adds a tool to the registry
func (r *Registry) Register(tool ToolExecutor) {
    def := tool.Definition()
    r.tools[def.Name] = tool
}

// GetDefinitions returns all tool definitions for LLM
func (r *Registry) GetDefinitions() []domain.ToolDefinition {
    defs := make([]domain.ToolDefinition, 0, len(r.tools))
    for _, tool := range r.tools {
        defs = append(defs, tool.Definition())
    }
    return defs
}

// Execute runs a tool by name
func (r *Registry) Execute(ctx context.Context, sessionID string, toolCall domain.ToolCall) (*domain.ToolResult, error) {
    tool, ok := r.tools[toolCall.Name]
    if !ok {
        return &domain.ToolResult{
            ToolUseID: toolCall.ID,
            Content:   fmt.Sprintf("unknown tool: %s", toolCall.Name),
            IsError:   true,
        }, nil
    }

    result, err := tool.Execute(ctx, sessionID, toolCall.Input)
    if err != nil {
        return &domain.ToolResult{
            ToolUseID: toolCall.ID,
            Content:   fmt.Sprintf("tool error: %v", err),
            IsError:   true,
        }, nil
    }

    result.ToolUseID = toolCall.ID
    return result, nil
}
```

### 5. Create search_products Tool

File: `project/backend/internal/tools/tool_search_products.go`

```go
package tools

import (
    "context"
    "fmt"

    "keepstar/internal/domain"
    "keepstar/internal/ports"
)

// SearchProductsTool searches products and writes to state
type SearchProductsTool struct {
    statePort   ports.StatePort
    catalogPort ports.CatalogPort
}

// NewSearchProductsTool creates the tool
func NewSearchProductsTool(statePort ports.StatePort, catalogPort ports.CatalogPort) *SearchProductsTool {
    return &SearchProductsTool{
        statePort:   statePort,
        catalogPort: catalogPort,
    }
}

// Definition returns the tool definition for LLM
func (t *SearchProductsTool) Definition() domain.ToolDefinition {
    return domain.ToolDefinition{
        Name:        "search_products",
        Description: "Search for products by query. Results are written to state, not returned directly.",
        InputSchema: map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "query": map[string]interface{}{
                    "type":        "string",
                    "description": "Search query (e.g., 'ноутбуки', 'Nike shoes')",
                },
                "category": map[string]interface{}{
                    "type":        "string",
                    "description": "Category filter (optional)",
                },
                "brand": map[string]interface{}{
                    "type":        "string",
                    "description": "Brand filter (optional)",
                },
                "min_price": map[string]interface{}{
                    "type":        "number",
                    "description": "Minimum price (optional)",
                },
                "max_price": map[string]interface{}{
                    "type":        "number",
                    "description": "Maximum price (optional)",
                },
                "limit": map[string]interface{}{
                    "type":        "integer",
                    "description": "Max results (default 10)",
                },
            },
            "required": []string{"query"},
        },
    }
}

// Execute searches products, writes to state, returns "ok" or "empty"
func (t *SearchProductsTool) Execute(ctx context.Context, sessionID string, input map[string]interface{}) (*domain.ToolResult, error) {
    // Parse input
    query, _ := input["query"].(string)
    category, _ := input["category"].(string)
    brand, _ := input["brand"].(string)
    minPrice, _ := input["min_price"].(float64)
    maxPrice, _ := input["max_price"].(float64)
    limit := 10
    if l, ok := input["limit"].(float64); ok {
        limit = int(l)
    }

    // Build filter
    filter := domain.ProductFilter{
        Search:   query,
        Category: category,
        Brand:    brand,
        MinPrice: minPrice,
        MaxPrice: maxPrice,
        Limit:    limit,
    }

    // Get state (or create if not exists)
    state, err := t.statePort.GetState(ctx, sessionID)
    if err == domain.ErrSessionNotFound {
        state, err = t.statePort.CreateState(ctx, sessionID)
    }
    if err != nil {
        return nil, fmt.Errorf("get/create state: %w", err)
    }

    // Search products (using default tenant for now)
    // TODO: get tenant from session
    products, total, err := t.catalogPort.ListProducts(ctx, "", filter)
    if err != nil {
        return nil, fmt.Errorf("search products: %w", err)
    }

    if total == 0 {
        return &domain.ToolResult{Content: "empty"}, nil
    }

    // Extract field names from first product
    fields := extractProductFields(products[0])

    // Update state with products
    state.Current.Data.Products = products
    state.Current.Meta = domain.StateMeta{
        Count:   total,
        Fields:  fields,
        Aliases: make(map[string]string),
    }
    state.Step++

    if err := t.statePort.UpdateState(ctx, state); err != nil {
        return nil, fmt.Errorf("update state: %w", err)
    }

    return &domain.ToolResult{
        Content: fmt.Sprintf("ok: found %d products", total),
    }, nil
}

// extractProductFields gets field names from a product
func extractProductFields(p domain.Product) []string {
    fields := []string{"id", "name", "price"}
    if p.Description != "" {
        fields = append(fields, "description")
    }
    if p.Brand != "" {
        fields = append(fields, "brand")
    }
    if p.Category != "" {
        fields = append(fields, "category")
    }
    if p.Rating > 0 {
        fields = append(fields, "rating")
    }
    if len(p.Images) > 0 {
        fields = append(fields, "images")
    }
    return fields
}
```

### 6. Update Agent 1 Prompt

File: `project/backend/internal/prompts/prompt_analyze_query.go`

```go
package prompts

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
- search_products: Search for products by query, category, brand, price range

Examples:
- "покажи ноутбуки" → search_products(query="ноутбуки")
- "Nike shoes under $100" → search_products(query="Nike shoes", max_price=100)
- "дешевые телефоны Samsung" → search_products(query="телефоны", brand="Samsung", max_price=20000)
`
```

### 7. Create Agent 1 Use Case

File: `project/backend/internal/usecases/agent1_execute.go`

```go
package usecases

import (
    "context"
    "fmt"
    "time"

    "keepstar/internal/domain"
    "keepstar/internal/ports"
    "keepstar/internal/prompts"
    "keepstar/internal/tools"
)

// Agent1ExecuteRequest is the input for Agent 1
type Agent1ExecuteRequest struct {
    SessionID string
    Query     string
}

// Agent1ExecuteResponse is the output from Agent 1
type Agent1ExecuteResponse struct {
    Delta     *domain.Delta
    LatencyMs int
}

// Agent1ExecuteUseCase executes Agent 1 (Tool Caller)
type Agent1ExecuteUseCase struct {
    llm          ports.LLMPort
    statePort    ports.StatePort
    toolRegistry *tools.Registry
}

// NewAgent1ExecuteUseCase creates Agent 1 use case
func NewAgent1ExecuteUseCase(
    llm ports.LLMPort,
    statePort ports.StatePort,
    toolRegistry *tools.Registry,
) *Agent1ExecuteUseCase {
    return &Agent1ExecuteUseCase{
        llm:          llm,
        statePort:    statePort,
        toolRegistry: toolRegistry,
    }
}

// Execute runs Agent 1: query → tool call → state update → delta
func (uc *Agent1ExecuteUseCase) Execute(ctx context.Context, req Agent1ExecuteRequest) (*Agent1ExecuteResponse, error) {
    start := time.Now()

    // Get or create state
    state, err := uc.statePort.GetState(ctx, req.SessionID)
    if err == domain.ErrSessionNotFound {
        state, err = uc.statePort.CreateState(ctx, req.SessionID)
    }
    if err != nil {
        return nil, fmt.Errorf("get state: %w", err)
    }

    // Build messages
    messages := []domain.LLMMessage{
        {Role: "user", Content: req.Query},
    }

    // Get tool definitions
    toolDefs := uc.toolRegistry.GetDefinitions()

    // Call LLM with tools
    llmResp, err := uc.llm.ChatWithTools(
        ctx,
        prompts.Agent1SystemPrompt,
        messages,
        toolDefs,
    )
    if err != nil {
        return nil, fmt.Errorf("llm call: %w", err)
    }

    // Process tool calls
    var delta *domain.Delta
    if len(llmResp.ToolCalls) > 0 {
        // Execute first tool call (Agent 1 should only call one)
        toolCall := llmResp.ToolCalls[0]

        result, err := uc.toolRegistry.Execute(ctx, req.SessionID, toolCall)
        if err != nil {
            return nil, fmt.Errorf("tool execute: %w", err)
        }

        // Get updated state for delta
        state, _ = uc.statePort.GetState(ctx, req.SessionID)

        // Create delta
        delta = &domain.Delta{
            Step:    state.Step,
            Trigger: domain.TriggerUserQuery,
            Action: domain.Action{
                Type:   domain.ActionSearch,
                Tool:   toolCall.Name,
                Params: toolCall.Input,
            },
            Result: domain.ResultMeta{
                Count:  state.Current.Meta.Count,
                Fields: state.Current.Meta.Fields,
            },
            CreatedAt: time.Now(),
        }

        // Save delta
        if err := uc.statePort.AddDelta(ctx, req.SessionID, delta); err != nil {
            return nil, fmt.Errorf("add delta: %w", err)
        }

        // Log tool result (Agent 1 doesn't see raw data, only "ok"/"empty")
        _ = result // "ok" or "empty" - Agent 1's feedback
    }

    return &Agent1ExecuteResponse{
        Delta:     delta,
        LatencyMs: int(time.Since(start).Milliseconds()),
    }, nil
}
```

### 8. Wire Components in main.go

Update `project/backend/cmd/server/main.go`:

```go
// After creating postgres adapters:
import "keepstar/internal/tools"

// Create state adapter (from Phase 1)
stateAdapter := postgres.NewStateAdapter(pgClient)

// Create tool registry
toolRegistry := tools.NewRegistry(stateAdapter, catalogAdapter)

// Create Agent 1 use case
agent1UC := usecases.NewAgent1ExecuteUseCase(
    anthropicClient,
    stateAdapter,
    toolRegistry,
)

// Agent 1 is now ready to be called from handlers
// (Handler integration can be added later or tested via use case directly)
```

### 9. Validation

Run validation commands:

```bash
cd project/backend && go build ./...
cd project/backend && go test ./...
```

Manual verification (with database running):
1. Create a test session
2. Call Agent1ExecuteUseCase with query "покажи ноутбуки"
3. Verify state.data.products is populated
4. Verify delta was recorded

## Validation Commands

From ADW/adw.yaml:
- `cd project/backend && go build ./...` (required)
- `cd project/backend && go test ./...` (optional)

## Acceptance Criteria

- [ ] Tool domain types defined (ToolDefinition, ToolCall, ToolResult, LLMMessage, LLMResponse)
- [ ] LLMPort extended with ChatWithTools method
- [ ] Anthropic client implements ChatWithTools with tool support
- [ ] Anthropic client uses API version 2024-06-01 for tools
- [ ] Tool registry created with dependency injection
- [ ] search_products tool implemented
- [ ] search_products writes products to state.current.data
- [ ] search_products updates state.current.meta (count, fields)
- [ ] search_products returns "ok" or "empty" (not raw data)
- [ ] Agent 1 system prompt defined (minimal, tool-focused)
- [ ] Agent1ExecuteUseCase orchestrates: query → LLM → tool → state → delta
- [ ] Delta recorded with action.tool and action.params
- [ ] Backend builds without errors
- [ ] "покажи ноутбуки" → state.data.products populated

## Notes

- Agent 1 does NOT see raw data - only "ok"/"empty" from tools
- This follows SPEC principle: "Агент НЕ видит данные из tools"
- Tool execution is synchronous for Phase 2 (can optimize later)
- Only one tool (search_products) in Phase 2, more in Phase 5
- Tenant handling simplified (uses default) - can enhance later
- This phase does NOT include Agent 2 or template generation

## Dependencies

- **Phase 1 (State + Storage)** must be complete before this phase
- StatePort and StateAdapter must be working
- CatalogPort must be available for product search
