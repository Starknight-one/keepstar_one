# Feature: Two-Agent Pipeline - Agent 2 + Template (Phase 3)

## Feature Description

Implement Agent 2 (Template Builder) for the Two-Agent Pipeline. This creates the visualization layer where:
- Agent 2 is triggered after Agent 1 completes
- Agent 2 receives ONLY meta (count, fields) - NOT raw data
- Agent 2 returns a widget template (JSON structure)
- Backend applies template to actual data
- Result is Formation JSON ready for frontend

This is Phase 3 from SPEC_TWO_AGENT_PIPELINE.md - the visualization step.

## Objective

Enable the flow: Agent 1 completes → trigger → Agent 2 (meta only) → template → backend applies → Formation JSON

**Verification**: After "покажи ноутбуки", receive Formation JSON with widgets populated with product data.

## Expertise Context

Expertise used:
- **backend**: Hexagonal architecture, existing domain entities (Atom, Widget, Formation), LLMPort, prompts patterns

Key insights from expertise:
- Domain already has AtomType, Atom, WidgetType, Widget, FormationType, Formation
- AtomType uses lowercase values: "text", "image", "number", "rating", etc.
- FormationType has: "grid", "list", "carousel", "single" (no "comparison")
- Need to extend Formation to match SPEC (grid rows/cols, widgetTemplate)
- Agent 2 does NOT use tools - just returns JSON template
- LLMPort.Chat() takes single message string (concatenate system + user prompt)
- Template uses field references (e.g., `"field": "price"`) not actual values
- Agent1ExecuteUseCase requires logger as 4th parameter

## Relevant Files

### Existing Files (to modify)
- `project/backend/internal/domain/formation_entity.go` - Extend with grid config and widgetTemplate
- `project/backend/internal/domain/widget_entity.go` - Add Size enum
- `project/backend/internal/prompts/prompt_compose_widgets.go` - Agent 2 system prompt

### Existing Files (reference)
- `project/backend/internal/domain/atom_entity.go` - Atom types already defined
- `project/backend/internal/domain/state_entity.go` - StateMeta for Agent 2 input
- `project/backend/internal/ports/llm_port.go` - LLMPort interface
- `project/backend/internal/usecases/agent1_execute.go` - Agent 1 pattern (from Phase 2)

### New Files
- `project/backend/internal/domain/template_entity.go` - WidgetTemplate, FormationTemplate types
- `project/backend/internal/usecases/agent2_execute.go` - Agent 2 use case
- `project/backend/internal/usecases/template_apply.go` - Apply template to data
- `project/backend/internal/usecases/pipeline_execute.go` - Orchestrate Agent 1 → Agent 2

## Step by Step Tasks

IMPORTANT: Execute strictly in order. Phase 1 and Phase 2 must be complete first.

### 1. Create Template Domain Types

File: `project/backend/internal/domain/template_entity.go`

**NOTE**: WidgetSize is defined in widget_entity.go (Task 2), not here - to avoid duplicate type.

```go
package domain

// AtomTemplate defines an atom with field reference (not actual value)
type AtomTemplate struct {
    Type   AtomType `json:"type"`
    Field  string   `json:"field"`            // Field name from product (e.g., "price", "name")
    Style  string   `json:"style,omitempty"`  // For text: heading/body/caption
    Format string   `json:"format,omitempty"` // For number: currency/percent/compact
    Size   string   `json:"size,omitempty"`   // For image: small/medium/large
}

// WidgetTemplate defines a widget structure without data
// Uses WidgetSize from widget_entity.go
type WidgetTemplate struct {
    Size     WidgetSize     `json:"size"`
    Priority int            `json:"priority,omitempty"`
    Atoms    []AtomTemplate `json:"atoms"`
}

// GridConfig defines grid layout
type GridConfig struct {
    Rows int `json:"rows"`
    Cols int `json:"cols"`
}

// FormationTemplate is what Agent 2 produces
type FormationTemplate struct {
    Mode           FormationType  `json:"mode"`
    Grid           *GridConfig    `json:"grid,omitempty"`
    WidgetTemplate WidgetTemplate `json:"widgetTemplate"`
}

// FormationWithData is the final result after applying template
type FormationWithData struct {
    Mode    FormationType `json:"mode"`
    Grid    *GridConfig   `json:"grid,omitempty"`
    Widgets []Widget      `json:"widgets"`
}
```

### 2. Update Widget Entity

File: `project/backend/internal/domain/widget_entity.go`

Add Size field to existing Widget struct:

```go
package domain

// WidgetType defines the type of composed widget
type WidgetType string

const (
    WidgetTypeProductCard     WidgetType = "product_card"
    WidgetTypeProductList     WidgetType = "product_list"
    WidgetTypeComparisonTable WidgetType = "comparison_table"
    WidgetTypeImageCarousel   WidgetType = "image_carousel"
    WidgetTypeTextBlock       WidgetType = "text_block"
    WidgetTypeQuickReplies    WidgetType = "quick_replies"
)

// WidgetSize defines widget size constraints
type WidgetSize string

const (
    WidgetSizeTiny   WidgetSize = "tiny"   // 80-110px, max 2 atoms
    WidgetSizeSmall  WidgetSize = "small"  // 160-220px, max 3 atoms
    WidgetSizeMedium WidgetSize = "medium" // 280-350px, max 5 atoms
    WidgetSizeLarge  WidgetSize = "large"  // 384-460px, max 10 atoms
)

// Widget is a composed UI element made of atoms
type Widget struct {
    ID       string                 `json:"id"`
    Type     WidgetType             `json:"type"`
    Size     WidgetSize             `json:"size,omitempty"`
    Priority int                    `json:"priority,omitempty"`
    Atoms    []Atom                 `json:"atoms"`
    Children []Widget               `json:"children,omitempty"`
    Meta     map[string]interface{} `json:"meta,omitempty"`
}
```

### 3. Create Agent 2 Prompt

File: `project/backend/internal/prompts/prompt_compose_widgets.go`

```go
package prompts

import (
    "encoding/json"
    "fmt"

    "keepstar/internal/domain"
)

// Agent2SystemPrompt is the system prompt for Agent 2 (Template Builder)
// NOTE: Atom types use lowercase to match domain.AtomType values
const Agent2SystemPrompt = `You are Agent 2 - a template builder for an e-commerce chat widget.

Your job: create a widget template based on metadata. You do NOT see actual data.

Input you receive:
- count: number of items
- fields: available field names (e.g., ["name", "price", "rating", "images"])
- layout_hint: suggested layout (optional)

Output: JSON template with this structure:
{
  "mode": "grid" | "carousel" | "single" | "list",
  "grid": {"rows": N, "cols": M},  // only for grid mode
  "widgetTemplate": {
    "size": "tiny" | "small" | "medium" | "large",
    "atoms": [
      {"type": "image", "field": "images", "size": "medium"},
      {"type": "text", "field": "name", "style": "heading"},
      {"type": "number", "field": "price", "format": "currency"},
      {"type": "rating", "field": "rating"}
    ]
  }
}

Rules:
1. ONLY output valid JSON. No explanations.
2. Use fields that exist in the input.
3. Choose appropriate widget size based on atom count.
4. Size constraints:
   - tiny: max 2 atoms
   - small: max 3 atoms
   - medium: max 5 atoms
   - large: max 10 atoms
5. Choose mode based on count:
   - 1 item → "single"
   - 2-6 items → "grid" (2 cols)
   - 7+ items → "carousel" or "grid" (3 cols)

Atom types (lowercase):
- text: for strings (style: heading/body/caption)
- number: for numbers (format: currency/percent/compact)
- price: for prices with currency
- image: for image URLs (size: small/medium/large)
- rating: for 0-5 ratings
- badge: for status labels (variant: success/warning/danger)
- button: for actions (label, action)
`

// BuildAgent2Prompt builds the user message for Agent 2
func BuildAgent2Prompt(meta domain.StateMeta, layoutHint string) string {
    input := map[string]interface{}{
        "count":  meta.Count,
        "fields": meta.Fields,
    }
    if layoutHint != "" {
        input["layout_hint"] = layoutHint
    }

    jsonBytes, _ := json.Marshal(input)
    return fmt.Sprintf("Create a widget template for this data:\n%s", string(jsonBytes))
}
```

### 4. Create Agent 2 Use Case

File: `project/backend/internal/usecases/agent2_execute.go`

```go
package usecases

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "keepstar/internal/domain"
    "keepstar/internal/ports"
    "keepstar/internal/prompts"
)

// Agent2ExecuteRequest is the input for Agent 2
type Agent2ExecuteRequest struct {
    SessionID  string
    LayoutHint string // Optional hint for layout
}

// Agent2ExecuteResponse is the output from Agent 2
type Agent2ExecuteResponse struct {
    Template  *domain.FormationTemplate
    LatencyMs int
}

// Agent2ExecuteUseCase executes Agent 2 (Template Builder)
type Agent2ExecuteUseCase struct {
    llm       ports.LLMPort
    statePort ports.StatePort
}

// NewAgent2ExecuteUseCase creates Agent 2 use case
func NewAgent2ExecuteUseCase(
    llm ports.LLMPort,
    statePort ports.StatePort,
) *Agent2ExecuteUseCase {
    return &Agent2ExecuteUseCase{
        llm:       llm,
        statePort: statePort,
    }
}

// Execute runs Agent 2: meta → LLM → template → save to state
func (uc *Agent2ExecuteUseCase) Execute(ctx context.Context, req Agent2ExecuteRequest) (*Agent2ExecuteResponse, error) {
    start := time.Now()

    // Get current state (must exist after Agent 1)
    state, err := uc.statePort.GetState(ctx, req.SessionID)
    if err != nil {
        return nil, fmt.Errorf("get state: %w", err)
    }

    // Check if we have data
    if state.Current.Meta.Count == 0 {
        // No data, return empty template
        return &Agent2ExecuteResponse{
            Template:  nil,
            LatencyMs: int(time.Since(start).Milliseconds()),
        }, nil
    }

    // Build prompt with meta only (NOT raw data)
    userPrompt := prompts.BuildAgent2Prompt(state.Current.Meta, req.LayoutHint)

    // Call LLM (simple Chat, no tools)
    response, err := uc.llm.Chat(ctx, prompts.Agent2SystemPrompt+"\n\n"+userPrompt)
    if err != nil {
        return nil, fmt.Errorf("llm call: %w", err)
    }

    // Parse template from response
    var template domain.FormationTemplate
    if err := json.Unmarshal([]byte(response), &template); err != nil {
        return nil, fmt.Errorf("parse template: %w (response: %s)", err, response)
    }

    // Save template to state
    state.Current.Template = map[string]interface{}{
        "mode":           template.Mode,
        "grid":           template.Grid,
        "widgetTemplate": template.WidgetTemplate,
    }
    if err := uc.statePort.UpdateState(ctx, state); err != nil {
        return nil, fmt.Errorf("update state: %w", err)
    }

    return &Agent2ExecuteResponse{
        Template:  &template,
        LatencyMs: int(time.Since(start).Milliseconds()),
    }, nil
}
```

### 5. Create Template Apply Use Case

File: `project/backend/internal/usecases/template_apply.go`

```go
package usecases

import (
    "fmt"
    "reflect"

    "github.com/google/uuid"
    "keepstar/internal/domain"
)

// ApplyTemplate applies a FormationTemplate to products, producing FormationWithData
func ApplyTemplate(template *domain.FormationTemplate, products []domain.Product) (*domain.FormationWithData, error) {
    if template == nil {
        return nil, fmt.Errorf("template is nil")
    }

    formation := &domain.FormationWithData{
        Mode: template.Mode,
        Grid: template.Grid,
    }

    // Apply widget template to each product
    for i, product := range products {
        widget, err := applyWidgetTemplate(template.WidgetTemplate, product, i)
        if err != nil {
            return nil, fmt.Errorf("apply widget template for product %d: %w", i, err)
        }
        formation.Widgets = append(formation.Widgets, *widget)
    }

    return formation, nil
}

// applyWidgetTemplate creates a Widget from template and product data
func applyWidgetTemplate(wt domain.WidgetTemplate, product domain.Product, index int) (*domain.Widget, error) {
    widget := &domain.Widget{
        ID:       uuid.New().String(),
        Type:     domain.WidgetTypeProductCard,
        Size:     wt.Size,
        Priority: index,
        Atoms:    make([]domain.Atom, 0, len(wt.Atoms)),
    }

    // Apply each atom template
    for _, atomTpl := range wt.Atoms {
        value := getFieldValue(product, atomTpl.Field)
        if value == nil {
            continue // Skip if field not found
        }

        atom := domain.Atom{
            Type:  atomTpl.Type,
            Value: value,
            Meta:  make(map[string]interface{}),
        }

        // Add style/format metadata
        if atomTpl.Style != "" {
            atom.Meta["style"] = atomTpl.Style
        }
        if atomTpl.Format != "" {
            atom.Meta["format"] = atomTpl.Format
        }
        if atomTpl.Size != "" {
            atom.Meta["size"] = atomTpl.Size
        }

        widget.Atoms = append(widget.Atoms, atom)
    }

    return widget, nil
}

// getFieldValue extracts a field value from product using reflection
func getFieldValue(product domain.Product, fieldName string) interface{} {
    // Map template field names to Product struct fields
    fieldMap := map[string]string{
        "id":          "ID",
        "name":        "Name",
        "description": "Description",
        "price":       "Price",
        "currency":    "Currency",
        "images":      "Images",
        "image_url":   "Images", // First image
        "rating":      "Rating",
        "brand":       "Brand",
        "category":    "Category",
        "stock":       "StockQuantity",
    }

    structField, ok := fieldMap[fieldName]
    if !ok {
        structField = fieldName // Try direct match
    }

    v := reflect.ValueOf(product)
    field := v.FieldByName(structField)
    if !field.IsValid() {
        return nil
    }

    value := field.Interface()

    // Special handling for images - return first image URL
    if fieldName == "image_url" || fieldName == "images" {
        if images, ok := value.([]string); ok && len(images) > 0 {
            return images[0]
        }
    }

    return value
}
```

### 6. Create Pipeline Orchestrator

File: `project/backend/internal/usecases/pipeline_execute.go`

```go
package usecases

import (
    "context"
    "fmt"
    "time"

    "keepstar/internal/domain"
    "keepstar/internal/logger"
    "keepstar/internal/ports"
    "keepstar/internal/tools"
)

// PipelineExecuteRequest is the input for the full pipeline
type PipelineExecuteRequest struct {
    SessionID string
    Query     string
}

// PipelineExecuteResponse is the output from the full pipeline
type PipelineExecuteResponse struct {
    Formation   *domain.FormationWithData
    Delta       *domain.Delta
    Agent1Ms    int
    Agent2Ms    int
    TotalMs     int
}

// PipelineExecuteUseCase orchestrates Agent 1 → Agent 2 → Formation
type PipelineExecuteUseCase struct {
    agent1UC  *Agent1ExecuteUseCase
    agent2UC  *Agent2ExecuteUseCase
    statePort ports.StatePort
    log       *logger.Logger
}

// NewPipelineExecuteUseCase creates the pipeline orchestrator
// NOTE: logger is required because Agent1ExecuteUseCase needs it
func NewPipelineExecuteUseCase(
    llm ports.LLMPort,
    statePort ports.StatePort,
    toolRegistry *tools.Registry,
    log *logger.Logger,
) *PipelineExecuteUseCase {
    return &PipelineExecuteUseCase{
        agent1UC:  NewAgent1ExecuteUseCase(llm, statePort, toolRegistry, log),
        agent2UC:  NewAgent2ExecuteUseCase(llm, statePort),
        statePort: statePort,
        log:       log,
    }
}

// Execute runs the full pipeline: query → Agent 1 → Agent 2 → Formation
func (uc *PipelineExecuteUseCase) Execute(ctx context.Context, req PipelineExecuteRequest) (*PipelineExecuteResponse, error) {
    start := time.Now()

    // Step 1: Agent 1 (Tool Caller)
    agent1Resp, err := uc.agent1UC.Execute(ctx, Agent1ExecuteRequest{
        SessionID: req.SessionID,
        Query:     req.Query,
    })
    if err != nil {
        return nil, fmt.Errorf("agent 1: %w", err)
    }

    // Step 2: Agent 2 (Template Builder) - triggered after Agent 1
    agent2Resp, err := uc.agent2UC.Execute(ctx, Agent2ExecuteRequest{
        SessionID: req.SessionID,
    })
    if err != nil {
        return nil, fmt.Errorf("agent 2: %w", err)
    }

    // Step 3: Apply template to data
    state, err := uc.statePort.GetState(ctx, req.SessionID)
    if err != nil {
        return nil, fmt.Errorf("get state: %w", err)
    }

    var formation *domain.FormationWithData
    if agent2Resp.Template != nil && len(state.Current.Data.Products) > 0 {
        formation, err = ApplyTemplate(agent2Resp.Template, state.Current.Data.Products)
        if err != nil {
            return nil, fmt.Errorf("apply template: %w", err)
        }
    }

    // Update delta with template (if Agent 2 produced one)
    if agent1Resp.Delta != nil && agent2Resp.Template != nil {
        agent1Resp.Delta.Template = state.Current.Template
        // Note: delta already saved in Agent 1, template added to state in Agent 2
    }

    return &PipelineExecuteResponse{
        Formation: formation,
        Delta:     agent1Resp.Delta,
        Agent1Ms:  agent1Resp.LatencyMs,
        Agent2Ms:  agent2Resp.LatencyMs,
        TotalMs:   int(time.Since(start).Milliseconds()),
    }, nil
}
```

### 7. Update Anthropic Client for Agent 2

The existing `Chat()` method in `project/backend/internal/adapters/anthropic/anthropic_client.go` should work for Agent 2 since it doesn't need tools.

However, we need to ensure the system prompt can be passed. Update the Chat method signature if needed, or have Agent 2 concatenate system prompt with user message (as shown in Step 4).

### 8. Wire Components in main.go

Update `project/backend/cmd/server/main.go`:

```go
// After creating Agent 1 use case (from Phase 2):

// Create Agent 2 use case
agent2UC := usecases.NewAgent2ExecuteUseCase(
    anthropicClient,
    stateAdapter,
)

// Create Pipeline orchestrator
// NOTE: logger (appLog) is required because Agent1ExecuteUseCase needs it
pipelineUC := usecases.NewPipelineExecuteUseCase(
    anthropicClient,
    stateAdapter,
    toolRegistry,
    appLog,  // logger is required
)

// Pipeline is now ready to be called from handlers
// Example: POST /api/v1/chat/pipeline
```

### 9. Validation

Run validation commands:

```bash
cd project/backend && go build ./...
cd project/backend && go test ./...
```

Manual verification:
1. Create a test session
2. Call PipelineExecuteUseCase with query "покажи ноутбуки"
3. Verify Agent 1 populates state.data.products
4. Verify Agent 2 produces template in state.current.template
5. Verify Formation JSON has widgets with actual product data

## Validation Commands

From ADW/adw.yaml:
- `cd project/backend && go build ./...` (required)
- `cd project/backend && go test ./...` (optional)

## Acceptance Criteria

- [ ] Template domain types defined (AtomTemplate, WidgetTemplate, FormationTemplate)
- [ ] WidgetSize enum added to Widget
- [ ] Agent 2 system prompt focuses on template generation
- [ ] Agent 2 receives ONLY meta (count, fields) - NOT raw data
- [ ] Agent 2 produces valid FormationTemplate JSON
- [ ] Template saved to state.current.template
- [ ] ApplyTemplate correctly maps field references to actual values
- [ ] ApplyTemplate handles missing fields gracefully
- [ ] FormationWithData contains populated Widgets
- [ ] PipelineExecuteUseCase orchestrates Agent 1 → Agent 2 → Apply
- [ ] Backend builds without errors
- [ ] "покажи ноутбуки" → Formation JSON with product widgets

## Notes

- Agent 2 does NOT see raw data - key principle from SPEC
- Agent 2 uses simple Chat() not ChatWithTools() - no tools needed
- **LLMPort.Chat() limitation**: Current Chat() doesn't support separate system prompt. Agent 2 concatenates system prompt with user message. This works but isn't ideal. Future improvement: use ChatWithTools() with empty tools array (supports system param).
- Template uses field references (e.g., `"field": "price"`) not actual values
- Backend applies template to data, not LLM
- This follows SPEC: "Agent 2 получает meta: count, fields, aliases. Экономия токенов."
- Similarity routing (same template for similar products) is automatic - single template applied N times
- **AtomType values**: Use lowercase ("text", "image", "number") to match domain.AtomType constants
- **FormationType values**: Use existing types ("grid", "carousel", "single", "list") - no "comparison"

## Dependencies

- **Phase 1 (State + Storage)** must be complete
- **Phase 2 (Agent 1 + Tool)** must be complete
- StatePort and StateAdapter must be working
- Agent 1 must populate state.current.data and state.current.meta
