# Feature: Entity Types & Presets

## Feature Description
Реализация системы типов сущностей (product/service) и пресетов отображения. LLM (Agent2) выбирает какие пресеты применить через tool calls вместо генерации raw JSON. Это упрощает промпт и делает систему более предсказуемой.

## Objective
- Ввести EntityType enum для разделения product/service
- Создать Preset структуру с конфигурацией slots и fields
- Реализовать готовые пресеты: product_grid, product_card, product_compact, service_card, service_list
- Создать render_*_preset tools для Agent2
- Обновить Agent2 промпт для работы с tools вместо JSON generation

## Expertise Context
Expertise used:
- **backend-domain**: Существующие entity (Product, Widget, Atom, Formation), slots (hero, badge, title, primary, price, secondary), AtomType/AtomSlot enums
- **backend-pipeline**: ToolExecutor interface, Registry pattern, Agent1/Agent2 flow
- **frontend-entities**: ProductCardTemplate с slot-based rendering, WidgetTemplate enum

## Relevant Files

### Existing Files
- `project/backend/internal/domain/atom_entity.go` - AtomType, AtomSlot enums (для использования в Preset)
- `project/backend/internal/domain/widget_entity.go` - WidgetSize enum, Widget struct
- `project/backend/internal/domain/formation_entity.go` - FormationType enum
- `project/backend/internal/domain/template_entity.go` - AtomTemplate, WidgetTemplate, FormationTemplate
- `project/backend/internal/domain/state_entity.go` - StateData, StateMeta (нужно расширить для services)
- `project/backend/internal/tools/tool_registry.go` - Registry для регистрации tools
- `project/backend/internal/tools/tool_search_products.go` - пример ToolExecutor
- `project/backend/internal/prompts/prompt_compose_widgets.go` - Agent2 промпт (нужно обновить)
- `project/backend/internal/usecases/agent2_execute.go` - Agent2 use case (нужно переписать на tool calling)
- `project/backend/internal/usecases/pipeline_execute.go` - Pipeline orchestrator (убрать ApplyTemplate)
- `project/frontend/src/entities/widget/widgetModel.js` - WidgetTemplate enum (нужно расширить)
- `project/frontend/src/entities/widget/templates/ProductCardTemplate.jsx` - существующий template

### New Files
- `project/backend/internal/domain/entity_type.go` - EntityType enum
- `project/backend/internal/domain/preset_entity.go` - Preset, FieldConfig, SlotConfig structs
- `project/backend/internal/domain/service_entity.go` - Service entity (аналог Product)
- `project/backend/internal/presets/product_presets.go` - ProductGridPreset, ProductCardPreset, ProductCompactPreset
- `project/backend/internal/presets/service_presets.go` - ServiceCardPreset, ServiceListPreset
- `project/backend/internal/presets/preset_registry.go` - PresetRegistry для lookup
- `project/backend/internal/tools/tool_render_preset.go` - render_product_preset, render_service_preset tools
- `project/frontend/src/entities/widget/templates/ServiceCardTemplate.jsx` - Service template
- `project/frontend/src/entities/widget/templates/ServiceCardTemplate.css` - Service styles

## Step by Step Tasks

### 1. Create EntityType enum
**File**: `project/backend/internal/domain/entity_type.go`

```go
package domain

type EntityType string

const (
    EntityTypeProduct EntityType = "product"
    EntityTypeService EntityType = "service"
)
```

### 2. Create Service entity
**File**: `project/backend/internal/domain/service_entity.go`

```go
package domain

type Service struct {
    ID           string                 `json:"id"`
    TenantID     string                 `json:"tenantId"`
    Name         string                 `json:"name"`
    Description  string                 `json:"description,omitempty"`
    Price        int                    `json:"price,omitempty"`        // in kopecks
    PriceFormatted string               `json:"priceFormatted,omitempty"`
    Currency     string                 `json:"currency,omitempty"`
    Duration     string                 `json:"duration,omitempty"`     // "30 min", "1 hour"
    Images       []string               `json:"images,omitempty"`
    Rating       float64                `json:"rating,omitempty"`
    Category     string                 `json:"category,omitempty"`
    Provider     string                 `json:"provider,omitempty"`     // service provider name
    Availability string                 `json:"availability,omitempty"` // "available", "busy"
    Attributes   map[string]interface{} `json:"attributes,omitempty"`
}
```

### 3. Create Preset domain entities
**File**: `project/backend/internal/domain/preset_entity.go`

```go
package domain

type FieldConfig struct {
    Name     string   `json:"name"`     // field name: "price", "rating", "duration"
    Slot     AtomSlot `json:"slot"`     // target slot: hero, title, primary, etc.
    AtomType AtomType `json:"atomType"` // how to render: text, price, rating, image
    Priority int      `json:"priority"` // higher = show first
    Required bool     `json:"required"` // must include
}

type SlotConfig struct {
    MaxAtoms     int        `json:"maxAtoms"`
    AllowedTypes []AtomType `json:"allowedTypes"`
}

type Preset struct {
    Name        string               `json:"name"`        // "product_grid", "service_card"
    EntityType  EntityType           `json:"entityType"`
    Template    string               `json:"template"`    // widget template name
    Slots       map[AtomSlot]SlotConfig `json:"slots"`
    Fields      []FieldConfig        `json:"fields"`
    DefaultMode FormationType        `json:"defaultMode"` // grid, list, carousel
    DefaultSize WidgetSize           `json:"defaultSize"` // small, medium, large
}

type PresetName string

const (
    PresetProductGrid    PresetName = "product_grid"
    PresetProductCard    PresetName = "product_card"
    PresetProductCompact PresetName = "product_compact"
    PresetServiceCard    PresetName = "service_card"
    PresetServiceList    PresetName = "service_list"
)
```

### 4. Update StateData for services
**File**: `project/backend/internal/domain/state_entity.go`

Add Services field to StateData:
```go
type StateData struct {
    Products []Product `json:"products,omitempty"`
    Services []Service `json:"services,omitempty"`  // NEW
}
```

Update StateMeta to include entity type info:
```go
type StateMeta struct {
    Count       int               `json:"count"`
    ProductCount int              `json:"productCount,omitempty"` // NEW
    ServiceCount int              `json:"serviceCount,omitempty"` // NEW
    Fields      []string          `json:"fields"`
    Aliases     map[string]string `json:"aliases,omitempty"`
}
```

### 5. Create preset definitions
**File**: `project/backend/internal/presets/product_presets.go`

```go
package presets

import "keepstar/internal/domain"

var ProductGridPreset = domain.Preset{
    Name:        string(domain.PresetProductGrid),
    EntityType:  domain.EntityTypeProduct,
    Template:    domain.WidgetTemplateProductCard,
    DefaultMode: domain.FormationTypeGrid,
    DefaultSize: domain.WidgetSizeMedium,
    Fields: []domain.FieldConfig{
        {Name: "images", Slot: domain.AtomSlotHero, AtomType: domain.AtomTypeImage, Priority: 1, Required: true},
        {Name: "name", Slot: domain.AtomSlotTitle, AtomType: domain.AtomTypeText, Priority: 2, Required: true},
        {Name: "brand", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Priority: 3, Required: false},
        {Name: "price", Slot: domain.AtomSlotPrice, AtomType: domain.AtomTypePrice, Priority: 4, Required: true},
        {Name: "rating", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeRating, Priority: 5, Required: false},
    },
}

var ProductCardPreset = domain.Preset{
    Name:        string(domain.PresetProductCard),
    EntityType:  domain.EntityTypeProduct,
    Template:    domain.WidgetTemplateProductCard,
    DefaultMode: domain.FormationTypeSingle,
    DefaultSize: domain.WidgetSizeLarge,
    Fields: []domain.FieldConfig{
        {Name: "images", Slot: domain.AtomSlotHero, AtomType: domain.AtomTypeImage, Priority: 1, Required: true},
        {Name: "name", Slot: domain.AtomSlotTitle, AtomType: domain.AtomTypeText, Priority: 2, Required: true},
        {Name: "brand", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Priority: 3, Required: false},
        {Name: "category", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Priority: 4, Required: false},
        {Name: "price", Slot: domain.AtomSlotPrice, AtomType: domain.AtomTypePrice, Priority: 5, Required: true},
        {Name: "rating", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeRating, Priority: 6, Required: false},
        {Name: "description", Slot: domain.AtomSlotSecondary, AtomType: domain.AtomTypeText, Priority: 7, Required: false},
    },
}

var ProductCompactPreset = domain.Preset{
    Name:        string(domain.PresetProductCompact),
    EntityType:  domain.EntityTypeProduct,
    Template:    domain.WidgetTemplateProductCard,
    DefaultMode: domain.FormationTypeList,
    DefaultSize: domain.WidgetSizeSmall,
    Fields: []domain.FieldConfig{
        {Name: "name", Slot: domain.AtomSlotTitle, AtomType: domain.AtomTypeText, Priority: 1, Required: true},
        {Name: "price", Slot: domain.AtomSlotPrice, AtomType: domain.AtomTypePrice, Priority: 2, Required: true},
    },
}
```

**File**: `project/backend/internal/presets/service_presets.go`

```go
package presets

import "keepstar/internal/domain"

var ServiceCardPreset = domain.Preset{
    Name:        string(domain.PresetServiceCard),
    EntityType:  domain.EntityTypeService,
    Template:    "ServiceCard",  // new template
    DefaultMode: domain.FormationTypeGrid,
    DefaultSize: domain.WidgetSizeMedium,
    Fields: []domain.FieldConfig{
        {Name: "images", Slot: domain.AtomSlotHero, AtomType: domain.AtomTypeImage, Priority: 1, Required: false},
        {Name: "name", Slot: domain.AtomSlotTitle, AtomType: domain.AtomTypeText, Priority: 2, Required: true},
        {Name: "provider", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Priority: 3, Required: false},
        {Name: "duration", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Priority: 4, Required: false},
        {Name: "price", Slot: domain.AtomSlotPrice, AtomType: domain.AtomTypePrice, Priority: 5, Required: true},
        {Name: "rating", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeRating, Priority: 6, Required: false},
    },
}

var ServiceListPreset = domain.Preset{
    Name:        string(domain.PresetServiceList),
    EntityType:  domain.EntityTypeService,
    Template:    "ServiceCard",
    DefaultMode: domain.FormationTypeList,
    DefaultSize: domain.WidgetSizeSmall,
    Fields: []domain.FieldConfig{
        {Name: "name", Slot: domain.AtomSlotTitle, AtomType: domain.AtomTypeText, Priority: 1, Required: true},
        {Name: "duration", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Priority: 2, Required: false},
        {Name: "price", Slot: domain.AtomSlotPrice, AtomType: domain.AtomTypePrice, Priority: 3, Required: true},
    },
}
```

### 6. Create PresetRegistry
**File**: `project/backend/internal/presets/preset_registry.go`

```go
package presets

import "keepstar/internal/domain"

type PresetRegistry struct {
    presets map[domain.PresetName]domain.Preset
}

func NewPresetRegistry() *PresetRegistry {
    r := &PresetRegistry{
        presets: make(map[domain.PresetName]domain.Preset),
    }

    // Register product presets
    r.Register(ProductGridPreset)
    r.Register(ProductCardPreset)
    r.Register(ProductCompactPreset)

    // Register service presets
    r.Register(ServiceCardPreset)
    r.Register(ServiceListPreset)

    return r
}

func (r *PresetRegistry) Register(preset domain.Preset) {
    r.presets[domain.PresetName(preset.Name)] = preset
}

func (r *PresetRegistry) Get(name domain.PresetName) (domain.Preset, bool) {
    p, ok := r.presets[name]
    return p, ok
}

func (r *PresetRegistry) GetByEntityType(entityType domain.EntityType) []domain.Preset {
    var result []domain.Preset
    for _, p := range r.presets {
        if p.EntityType == entityType {
            result = append(result, p)
        }
    }
    return result
}
```

### 7. Create render_preset tools
**File**: `project/backend/internal/tools/tool_render_preset.go`

```go
package tools

import (
    "context"
    "fmt"
    "sort"

    "keepstar/internal/domain"
    "keepstar/internal/ports"
    "keepstar/internal/presets"
)

// RenderProductPresetTool renders products using a preset
type RenderProductPresetTool struct {
    statePort      ports.StatePort
    presetRegistry *presets.PresetRegistry
}

func NewRenderProductPresetTool(statePort ports.StatePort, presetRegistry *presets.PresetRegistry) *RenderProductPresetTool {
    return &RenderProductPresetTool{
        statePort:      statePort,
        presetRegistry: presetRegistry,
    }
}

func (t *RenderProductPresetTool) Definition() domain.ToolDefinition {
    return domain.ToolDefinition{
        Name:        "render_product_preset",
        Description: "Render products from state using a preset template. Call this after search_products.",
        InputSchema: map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "preset": map[string]interface{}{
                    "type":        "string",
                    "enum":        []string{"product_grid", "product_card", "product_compact"},
                    "description": "Preset to use: product_grid (for multiple items), product_card (for single detail), product_compact (for list)",
                },
            },
            "required": []string{"preset"},
        },
    }
}

func (t *RenderProductPresetTool) Execute(ctx context.Context, sessionID string, input map[string]interface{}) (*domain.ToolResult, error) {
    presetName, _ := input["preset"].(string)

    preset, ok := t.presetRegistry.Get(domain.PresetName(presetName))
    if !ok {
        return &domain.ToolResult{Content: "error: unknown preset", IsError: true}, nil
    }

    state, err := t.statePort.GetState(ctx, sessionID)
    if err != nil {
        return nil, fmt.Errorf("get state: %w", err)
    }

    products := state.Current.Data.Products
    if len(products) == 0 {
        return &domain.ToolResult{Content: "error: no products in state"}, nil
    }

    // Build formation from preset
    formation := buildFormationFromPreset(preset, products)

    // Store formation in state template
    state.Current.Template = map[string]interface{}{
        "formation": formation,
    }

    if err := t.statePort.UpdateState(ctx, state); err != nil {
        return nil, fmt.Errorf("update state: %w", err)
    }

    return &domain.ToolResult{
        Content: fmt.Sprintf("ok: rendered %d products with %s", len(products), presetName),
    }, nil
}

// RenderServicePresetTool renders services using a preset
type RenderServicePresetTool struct {
    statePort      ports.StatePort
    presetRegistry *presets.PresetRegistry
}

func NewRenderServicePresetTool(statePort ports.StatePort, presetRegistry *presets.PresetRegistry) *RenderServicePresetTool {
    return &RenderServicePresetTool{
        statePort:      statePort,
        presetRegistry: presetRegistry,
    }
}

func (t *RenderServicePresetTool) Definition() domain.ToolDefinition {
    return domain.ToolDefinition{
        Name:        "render_service_preset",
        Description: "Render services from state using a preset template. Call this after search_services.",
        InputSchema: map[string]interface{}{
            "type": "object",
            "properties": map[string]interface{}{
                "preset": map[string]interface{}{
                    "type":        "string",
                    "enum":        []string{"service_card", "service_list"},
                    "description": "Preset to use: service_card (for grid), service_list (for compact list)",
                },
            },
            "required": []string{"preset"},
        },
    }
}

func (t *RenderServicePresetTool) Execute(ctx context.Context, sessionID string, input map[string]interface{}) (*domain.ToolResult, error) {
    presetName, _ := input["preset"].(string)

    preset, ok := t.presetRegistry.Get(domain.PresetName(presetName))
    if !ok {
        return &domain.ToolResult{Content: "error: unknown preset", IsError: true}, nil
    }

    state, err := t.statePort.GetState(ctx, sessionID)
    if err != nil {
        return nil, fmt.Errorf("get state: %w", err)
    }

    services := state.Current.Data.Services
    if len(services) == 0 {
        return &domain.ToolResult{Content: "error: no services in state"}, nil
    }

    // Build formation from preset
    formation := buildServiceFormationFromPreset(preset, services)

    // Merge with existing formation if products already rendered
    if existing, ok := state.Current.Template["formation"].(*domain.FormationWithData); ok {
        formation.Widgets = append(existing.Widgets, formation.Widgets...)
    }

    state.Current.Template = map[string]interface{}{
        "formation": formation,
    }

    if err := t.statePort.UpdateState(ctx, state); err != nil {
        return nil, fmt.Errorf("update state: %w", err)
    }

    return &domain.ToolResult{
        Content: fmt.Sprintf("ok: rendered %d services with %s", len(services), presetName),
    }, nil
}

// buildFormationFromPreset creates formation from preset and products
func buildFormationFromPreset(preset domain.Preset, products []domain.Product) *domain.FormationWithData {
    widgets := make([]domain.Widget, 0, len(products))

    // Sort fields by priority
    fields := make([]domain.FieldConfig, len(preset.Fields))
    copy(fields, preset.Fields)
    sort.Slice(fields, func(i, j int) bool {
        return fields[i].Priority < fields[j].Priority
    })

    for i, product := range products {
        atoms := buildAtomsFromProduct(fields, product)
        widget := domain.Widget{
            ID:       fmt.Sprintf("widget-%d", i),
            Template: preset.Template,
            Size:     preset.DefaultSize,
            Atoms:    atoms,
        }
        widgets = append(widgets, widget)
    }

    return &domain.FormationWithData{
        Mode:    preset.DefaultMode,
        Widgets: widgets,
    }
}

func buildAtomsFromProduct(fields []domain.FieldConfig, product domain.Product) []domain.Atom {
    atoms := make([]domain.Atom, 0)

    for _, field := range fields {
        value := getProductFieldValue(product, field.Name)
        if value == nil && !field.Required {
            continue
        }

        atom := domain.Atom{
            Type:  field.AtomType,
            Value: value,
            Slot:  field.Slot,
        }
        atoms = append(atoms, atom)
    }

    return atoms
}

func getProductFieldValue(p domain.Product, fieldName string) interface{} {
    switch fieldName {
    case "id":
        return p.ID
    case "name":
        return p.Name
    case "description":
        if p.Description == "" { return nil }
        return p.Description
    case "price":
        return p.Price
    case "images":
        if len(p.Images) == 0 { return nil }
        return p.Images
    case "rating":
        if p.Rating == 0 { return nil }
        return p.Rating
    case "brand":
        if p.Brand == "" { return nil }
        return p.Brand
    case "category":
        if p.Category == "" { return nil }
        return p.Category
    default:
        return nil
    }
}

// buildServiceFormationFromPreset creates formation from preset and services
func buildServiceFormationFromPreset(preset domain.Preset, services []domain.Service) *domain.FormationWithData {
    widgets := make([]domain.Widget, 0, len(services))

    fields := make([]domain.FieldConfig, len(preset.Fields))
    copy(fields, preset.Fields)
    sort.Slice(fields, func(i, j int) bool {
        return fields[i].Priority < fields[j].Priority
    })

    for i, service := range services {
        atoms := buildAtomsFromService(fields, service)
        widget := domain.Widget{
            ID:       fmt.Sprintf("service-widget-%d", i),
            Template: preset.Template,
            Size:     preset.DefaultSize,
            Atoms:    atoms,
        }
        widgets = append(widgets, widget)
    }

    return &domain.FormationWithData{
        Mode:    preset.DefaultMode,
        Widgets: widgets,
    }
}

func buildAtomsFromService(fields []domain.FieldConfig, service domain.Service) []domain.Atom {
    atoms := make([]domain.Atom, 0)

    for _, field := range fields {
        value := getServiceFieldValue(service, field.Name)
        if value == nil && !field.Required {
            continue
        }

        atom := domain.Atom{
            Type:  field.AtomType,
            Value: value,
            Slot:  field.Slot,
        }
        atoms = append(atoms, atom)
    }

    return atoms
}

func getServiceFieldValue(s domain.Service, fieldName string) interface{} {
    switch fieldName {
    case "id":
        return s.ID
    case "name":
        return s.Name
    case "description":
        if s.Description == "" { return nil }
        return s.Description
    case "price":
        return s.Price
    case "images":
        if len(s.Images) == 0 { return nil }
        return s.Images
    case "rating":
        if s.Rating == 0 { return nil }
        return s.Rating
    case "duration":
        if s.Duration == "" { return nil }
        return s.Duration
    case "provider":
        if s.Provider == "" { return nil }
        return s.Provider
    default:
        return nil
    }
}
```

### 8. Update tool registry
**File**: `project/backend/internal/tools/tool_registry.go`

Add preset registry dependency and register new tools:
```go
func NewRegistry(statePort ports.StatePort, catalogPort ports.CatalogPort, presetRegistry *presets.PresetRegistry) *Registry {
    r := &Registry{
        tools:       make(map[string]ToolExecutor),
        statePort:   statePort,
        catalogPort: catalogPort,
    }

    // Data tools (Agent1)
    r.Register(NewSearchProductsTool(statePort, catalogPort))

    // Render tools (Agent2)
    r.Register(NewRenderProductPresetTool(statePort, presetRegistry))
    r.Register(NewRenderServicePresetTool(statePort, presetRegistry))

    return r
}
```

### 9. Update Agent2 to use tool calling (like Agent1)
**File**: `project/backend/internal/usecases/agent2_execute.go`

Переписать Agent2 чтобы он использовал `ChatWithTools` вместо JSON generation:

```go
// Agent2ExecuteUseCase executes Agent 2 (Preset Selector)
type Agent2ExecuteUseCase struct {
    llm          ports.LLMPort
    statePort    ports.StatePort
    toolRegistry *tools.Registry  // NEW: needs tool registry for render_* tools
}

func (uc *Agent2ExecuteUseCase) Execute(ctx context.Context, req Agent2ExecuteRequest) (*Agent2ExecuteResponse, error) {
    state, err := uc.statePort.GetState(ctx, req.SessionID)

    // Build user message with meta info
    userPrompt := fmt.Sprintf("State meta: { productCount: %d, serviceCount: %d, fields: %v }",
        state.Current.Meta.ProductCount,
        state.Current.Meta.ServiceCount,
        state.Current.Meta.Fields,
    )

    messages := []domain.LLMMessage{
        {Role: "user", Content: userPrompt},
    }

    // Get render tool definitions (filter only render_* tools)
    toolDefs := uc.getAgent2Tools()

    // Call LLM with tools (like Agent1)
    llmResp, err := uc.llm.ChatWithTools(ctx, prompts.Agent2SystemPrompt, messages, toolDefs)

    // Execute tool calls
    for _, toolCall := range llmResp.ToolCalls {
        result, err := uc.toolRegistry.Execute(ctx, req.SessionID, toolCall)
        // Tool writes formation to state
    }

    // Formation is now in state.Current.Template["formation"]
    return &Agent2ExecuteResponse{...}, nil
}

func (uc *Agent2ExecuteUseCase) getAgent2Tools() []domain.ToolDefinition {
    // Return only render_product_preset, render_service_preset
    allTools := uc.toolRegistry.GetDefinitions()
    var agent2Tools []domain.ToolDefinition
    for _, t := range allTools {
        if strings.HasPrefix(t.Name, "render_") {
            agent2Tools = append(agent2Tools, t)
        }
    }
    return agent2Tools
}
```

### 10. Update Agent2 system prompt
**File**: `project/backend/internal/prompts/prompt_compose_widgets.go`

Replace JSON generation prompt with tool-calling prompt:
```go
const Agent2SystemPrompt = `You are a UI composition agent. Your job is to render data using preset templates.

RULES:
1. ONLY call tools, never output text
2. Look at state.meta to see what data is available
3. Choose appropriate preset based on item count and context:
   - 1 item → use _card preset (detailed view)
   - 2-6 items → use _grid preset
   - 7+ items → use _grid or _compact preset
4. If both products and services exist, call both render tools

AVAILABLE PRESETS:
- Products: product_grid, product_card, product_compact
- Services: service_card, service_list

EXAMPLE:
State: { productCount: 5, serviceCount: 0 }
→ Call: render_product_preset(preset="product_grid")

State: { productCount: 1, serviceCount: 2 }
→ Call: render_product_preset(preset="product_card")
→ Call: render_service_preset(preset="service_card")`
```

### 11. Update PipelineExecuteUseCase
**File**: `project/backend/internal/usecases/pipeline_execute.go`

Убрать `ApplyTemplate` — formation уже в state после Agent2 tool call:

```go
func (uc *PipelineExecuteUseCase) Execute(...) {
    // Step 1: Agent 1 (Tool Caller) - searches, writes to state.data
    agent1Resp, err := uc.agent1UC.Execute(...)

    // Step 2: Agent 2 (Preset Selector) - calls render_* tool, writes to state.template.formation
    agent2Resp, err := uc.agent2UC.Execute(...)

    // Step 3: Get formation from state (already built by tool)
    state, _ := uc.statePort.GetState(ctx, req.SessionID)
    formation := state.Current.Template["formation"].(*domain.FormationWithData)

    // NO MORE ApplyTemplate - tool did it
    return &PipelineExecuteResponse{Formation: formation, ...}
}
```

### 12. Update frontend WidgetTemplate enum
**File**: `project/frontend/src/entities/widget/widgetModel.js`

```js
export const WidgetTemplate = {
  PRODUCT_CARD: 'ProductCard',
  SERVICE_CARD: 'ServiceCard',  // NEW
};
```

### 13. Create ServiceCardTemplate component
**File**: `project/frontend/src/entities/widget/templates/ServiceCardTemplate.jsx`

Similar structure to ProductCardTemplate but with service-specific fields (duration, provider, availability).

### 14. Update templates index
**File**: `project/frontend/src/entities/widget/templates/index.js`

```js
export { ProductCardTemplate } from './ProductCardTemplate';
export { ServiceCardTemplate } from './ServiceCardTemplate';
```

### 15. Integration Tests
**File**: `project/backend/internal/usecases/preset_integration_test.go`

Создать полноценные интеграционные тесты (реальная БД + реальный LLM).

```go
package usecases_test

// TestPresetSelection_Integration - полный флоу с preset selection
// Проверяет что Agent2 вызывает правильный tool и formation строится корректно

func TestPresetSelection_Integration(t *testing.T) {
    // Setup: DB, LLM, adapters, presetRegistry, toolRegistry, pipeline
    // ... (как в существующих тестах)

    testCases := []struct {
        name           string
        query          string
        expectedPreset string
        expectedMode   domain.FormationType
        expectedTemplate string
        minWidgets     int
        requiredSlots  []domain.AtomSlot
    }{
        {
            name:           "Multiple products → grid preset",
            query:          "покажи кроссовки Nike",
            expectedPreset: "product_grid",
            expectedMode:   domain.FormationTypeGrid,
            expectedTemplate: "ProductCard",
            minWidgets:     2,
            requiredSlots:  []domain.AtomSlot{domain.AtomSlotHero, domain.AtomSlotTitle, domain.AtomSlotPrice},
        },
        {
            name:           "Single product → card preset",
            query:          "покажи Nike Air Max 90 детально",
            expectedPreset: "product_card",
            expectedMode:   domain.FormationTypeSingle,
            expectedTemplate: "ProductCard",
            minWidgets:     1,
            requiredSlots:  []domain.AtomSlot{domain.AtomSlotHero, domain.AtomSlotTitle, domain.AtomSlotPrice, domain.AtomSlotSecondary},
        },
        {
            name:           "Many products → compact preset",
            query:          "покажи все товары списком",
            expectedPreset: "product_compact",
            expectedMode:   domain.FormationTypeList,
            expectedTemplate: "ProductCard",
            minWidgets:     5,
            requiredSlots:  []domain.AtomSlot{domain.AtomSlotTitle, domain.AtomSlotPrice},
        },
    }

    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            sessionID := uuid.New().String()
            createSession(sessionID)

            // Execute pipeline
            resp, err := pipelineUC.Execute(ctx, usecases.PipelineExecuteRequest{
                SessionID: sessionID,
                Query:     tc.query,
            })
            require.NoError(t, err)
            require.NotNil(t, resp.Formation)

            // 1. Check formation mode
            assert.Equal(t, tc.expectedMode, resp.Formation.Mode,
                "Expected mode %s, got %s", tc.expectedMode, resp.Formation.Mode)

            // 2. Check widget count
            assert.GreaterOrEqual(t, len(resp.Formation.Widgets), tc.minWidgets,
                "Expected at least %d widgets, got %d", tc.minWidgets, len(resp.Formation.Widgets))

            // 3. Check widget template
            for _, w := range resp.Formation.Widgets {
                assert.Equal(t, tc.expectedTemplate, w.Template,
                    "Expected template %s, got %s", tc.expectedTemplate, w.Template)
            }

            // 4. Check required slots present
            firstWidget := resp.Formation.Widgets[0]
            presentSlots := make(map[domain.AtomSlot]bool)
            for _, atom := range firstWidget.Atoms {
                presentSlots[atom.Slot] = true
            }
            for _, requiredSlot := range tc.requiredSlots {
                assert.True(t, presentSlots[requiredSlot],
                    "Required slot %s not found in widget", requiredSlot)
            }

            // 5. Check state has formation
            state, _ := stateAdapter.GetState(ctx, sessionID)
            assert.NotNil(t, state.Current.Template["formation"])

            // Log for debugging
            t.Logf("Query: %s", tc.query)
            t.Logf("Mode: %s, Widgets: %d", resp.Formation.Mode, len(resp.Formation.Widgets))
            t.Logf("First widget atoms: %d", len(firstWidget.Atoms))
        })
    }
}
```

### 16. Service Integration Tests (when services added to DB)
**File**: `project/backend/internal/usecases/service_preset_test.go`

```go
func TestServicePreset_Integration(t *testing.T) {
    // Требует seed data для services в БД

    testCases := []struct {
        name           string
        query          string
        expectedMode   domain.FormationType
        expectedTemplate string
    }{
        {
            name:           "Services grid",
            query:          "покажи услуги доставки",
            expectedMode:   domain.FormationTypeGrid,
            expectedTemplate: "ServiceCard",
        },
        {
            name:           "Services list",
            query:          "покажи все услуги списком",
            expectedMode:   domain.FormationTypeList,
            expectedTemplate: "ServiceCard",
        },
    }

    // ... similar assertions
}

func TestMixedProductsAndServices_Integration(t *testing.T) {
    // Тест на смешанный запрос

    t.Run("Products and services together", func(t *testing.T) {
        sessionID := uuid.New().String()
        createSession(sessionID)

        resp, err := pipelineUC.Execute(ctx, usecases.PipelineExecuteRequest{
            SessionID: sessionID,
            Query:     "покажи кроссовки Nike и экспресс-доставку",
        })
        require.NoError(t, err)
        require.NotNil(t, resp.Formation)

        // Should have both ProductCard and ServiceCard widgets
        hasProductCard := false
        hasServiceCard := false
        for _, w := range resp.Formation.Widgets {
            if w.Template == "ProductCard" {
                hasProductCard = true
            }
            if w.Template == "ServiceCard" {
                hasServiceCard = true
            }
        }

        assert.True(t, hasProductCard, "Expected ProductCard widgets")
        assert.True(t, hasServiceCard, "Expected ServiceCard widgets")
    })
}
```

### 17. Preset Tool Unit Tests
**File**: `project/backend/internal/tools/tool_render_preset_test.go`

```go
func TestRenderProductPresetTool_Execute(t *testing.T) {
    // Unit test с mock state port

    testCases := []struct {
        name          string
        preset        string
        productsCount int
        expectedMode  domain.FormationType
        expectedSize  domain.WidgetSize
    }{
        {"grid preset", "product_grid", 5, domain.FormationTypeGrid, domain.WidgetSizeMedium},
        {"card preset", "product_card", 1, domain.FormationTypeSingle, domain.WidgetSizeLarge},
        {"compact preset", "product_compact", 10, domain.FormationTypeList, domain.WidgetSizeSmall},
    }

    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            // Setup mock state with tc.productsCount products
            mockState := createMockStateWithProducts(tc.productsCount)
            mockStatePort := &MockStatePort{state: mockState}

            tool := NewRenderProductPresetTool(mockStatePort, presets.NewPresetRegistry())

            result, err := tool.Execute(ctx, "session-1", map[string]interface{}{
                "preset": tc.preset,
            })

            require.NoError(t, err)
            assert.Contains(t, result.Content, "ok:")

            // Check formation in state
            formation := mockStatePort.state.Current.Template["formation"].(*domain.FormationWithData)
            assert.Equal(t, tc.expectedMode, formation.Mode)
            assert.Len(t, formation.Widgets, tc.productsCount)
            assert.Equal(t, tc.expectedSize, formation.Widgets[0].Size)
        })
    }
}

func TestBuildFormationFromPreset(t *testing.T) {
    // Проверяем rule-based логику маппинга полей в слоты

    preset := presets.ProductGridPreset
    products := []domain.Product{
        {
            ID:     "p1",
            Name:   "Test Product",
            Price:  9990, // kopecks
            Images: []string{"http://img.jpg"},
            Rating: 4.5,
            Brand:  "TestBrand",
        },
    }

    formation := buildFormationFromPreset(preset, products)

    // Check atoms mapping
    widget := formation.Widgets[0]
    atomsBySlot := make(map[domain.AtomSlot]domain.Atom)
    for _, atom := range widget.Atoms {
        atomsBySlot[atom.Slot] = atom
    }

    // Hero slot should have images
    heroAtom := atomsBySlot[domain.AtomSlotHero]
    assert.Equal(t, domain.AtomTypeImage, heroAtom.Type)
    assert.Equal(t, []string{"http://img.jpg"}, heroAtom.Value)

    // Title slot should have name
    titleAtom := atomsBySlot[domain.AtomSlotTitle]
    assert.Equal(t, domain.AtomTypeText, titleAtom.Type)
    assert.Equal(t, "Test Product", titleAtom.Value)

    // Price slot should have price
    priceAtom := atomsBySlot[domain.AtomSlotPrice]
    assert.Equal(t, domain.AtomTypePrice, priceAtom.Type)
    assert.Equal(t, 9990, priceAtom.Value)

    // Primary slot should have brand and rating
    primaryAtoms := filterAtomsBySlot(widget.Atoms, domain.AtomSlotPrimary)
    assert.Len(t, primaryAtoms, 2) // brand + rating
}
```

### 18. Agent2 Tool Calling Test
**File**: `project/backend/internal/usecases/agent2_tool_calling_test.go`

```go
func TestAgent2_CallsRenderPresetTool(t *testing.T) {
    // Проверяем что Agent2 вызывает tool, а не генерит JSON

    sessionID := uuid.New().String()
    createSession(sessionID)

    // First run Agent1 to populate state
    agent1UC.Execute(ctx, usecases.Agent1ExecuteRequest{
        SessionID: sessionID,
        Query:     "покажи кроссовки Nike",
    })

    // Run Agent2
    agent2Resp, err := agent2UC.Execute(ctx, usecases.Agent2ExecuteRequest{
        SessionID: sessionID,
    })
    require.NoError(t, err)

    // Agent2 should have called a tool (not generated JSON)
    assert.True(t, agent2Resp.ToolCalled, "Agent2 should call render_* tool")
    assert.Contains(t, agent2Resp.ToolName, "render_", "Tool should be render_* preset tool")

    // Formation should be in state (built by tool, not ApplyTemplate)
    state, _ := stateAdapter.GetState(ctx, sessionID)
    formation := state.Current.Template["formation"]
    assert.NotNil(t, formation, "Formation should be in state after tool execution")
}
```

### 19. E2E API Test
**File**: `project/backend/internal/handlers/pipeline_e2e_test.go`

```go
func TestPipelineAPI_E2E(t *testing.T) {
    // Тест через HTTP API

    server := setupTestServer()
    defer server.Close()

    testCases := []struct {
        name         string
        query        string
        checkMode    domain.FormationType
        checkWidgets int
    }{
        {"Grid response", "покажи кроссовки Nike", domain.FormationTypeGrid, 3},
        {"Single response", "покажи Nike Air Max 90", domain.FormationTypeSingle, 1},
    }

    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            // POST /api/v1/pipeline
            body := fmt.Sprintf(`{"query": "%s"}`, tc.query)
            resp, err := http.Post(server.URL+"/api/v1/pipeline", "application/json", strings.NewReader(body))
            require.NoError(t, err)
            defer resp.Body.Close()

            assert.Equal(t, http.StatusOK, resp.StatusCode)

            var result struct {
                SessionID string                  `json:"sessionId"`
                Formation domain.FormationWithData `json:"formation"`
            }
            json.NewDecoder(resp.Body).Decode(&result)

            // Check response
            assert.Equal(t, tc.checkMode, result.Formation.Mode)
            assert.GreaterOrEqual(t, len(result.Formation.Widgets), tc.checkWidgets)

            // Check widget structure
            w := result.Formation.Widgets[0]
            assert.NotEmpty(t, w.Template)
            assert.NotEmpty(t, w.Atoms)
        })
    }
}
```

### 20. Run Validation
```bash
cd project/backend && go build ./...
cd project/backend && go test -v -run TestPresetSelection ./internal/usecases/
cd project/backend && go test -v -run TestRenderProductPresetTool ./internal/tools/
cd project/backend && go test -v -run TestPipelineAPI_E2E ./internal/handlers/
cd project/frontend && npm run build
cd project/frontend && npm run lint
```

## Validation Commands
```bash
cd project/backend && go build ./...
cd project/backend && go test -v ./internal/usecases/ -run "Preset|Service|Mixed"
cd project/backend && go test -v ./internal/tools/ -run "RenderPreset"
cd project/backend && go test -v ./internal/handlers/ -run "E2E"
cd project/frontend && npm run build
cd project/frontend && npm run lint
```

## Acceptance Criteria

### Domain & Presets
- [ ] EntityType enum (product/service) created
- [ ] Service entity created with appropriate fields
- [ ] Preset struct с slots и fields created
- [ ] Product presets (grid, card, compact) defined
- [ ] Service presets (card, list) defined
- [ ] PresetRegistry для lookup presets

### Tools & Pipeline
- [ ] render_product_preset tool implemented
- [ ] render_service_preset tool implemented
- [ ] Tool registry updated with new tools
- [ ] Agent2 переписан на tool calling (не JSON generation)
- [ ] Pipeline убран ApplyTemplate (formation из state)

### Frontend
- [ ] WidgetTemplate enum updated с ServiceCard
- [ ] ServiceCardTemplate component created

### Tests Pass
- [ ] `TestPresetSelection_Integration` - multiple/single/compact products → правильный preset
- [ ] `TestRenderProductPresetTool_Execute` - tool строит formation корректно
- [ ] `TestBuildFormationFromPreset` - rule-based mapping полей в слоты
- [ ] `TestAgent2_CallsRenderPresetTool` - Agent2 вызывает tool, не генерит JSON
- [ ] `TestPipelineAPI_E2E` - POST /api/v1/pipeline возвращает formation

### Validation Commands Pass
- [ ] `go build ./...`
- [ ] `go test ./internal/usecases/ -run "Preset|Service"`
- [ ] `go test ./internal/tools/ -run "RenderPreset"`
- [ ] `npm run build`
- [ ] `npm run lint`

## Notes
- **Price stored in kopecks (int), not rubles (float)** - see backend-domain gotchas
- **sessionID must be valid UUID format**
- Service entity is parallel to Product entity, not a subtype
- Presets define the mapping from entity fields → atoms → slots
- Agent2 теперь вызывает tools вместо генерации JSON, что упрощает промпт и валидацию
- Formation строится из preset + data, не генерируется LLM напрямую

## Known Issues (TODO)
- ~~**Chat input blocked during history load**: В `ChatPanel.jsx` при открытии чата вызывается `setLoading(true)` во время загрузки истории сессии из БД (`getSession`). Это блокирует ввод пока история не загрузится. Решение: убрать `setLoading` при загрузке истории или использовать отдельный state `isLoadingHistory`.~~ - Не критично, загрузка быстрая.
- ~~**Session TTL not checked on read**: При GET /api/v1/session/{id} не проверялся TTL сессии, сессии казались "вечными".~~ - **FIXED**: Добавлена проверка `domain.SessionTTL` (5 минут) в `handler_session.go`. Сессия автоматически закрывается при чтении если неактивна более 5 минут.
