# Engine V2 — Implementation Specification

> **Date**: 2026-03-17
> **Status**: All 6 phases implemented. Build clean. Tests pass.
> **Activation**: `ENGINE_VERSION=v2` + `AGENT2_PROMPT_VERSION=v2` in `.env`
> **Default**: v1 (zero breaking changes without env vars)

---

## Table of Contents

1. [Overview](#1-overview)
2. [Architecture](#2-architecture)
3. [Phase 0: Field Definitions + Generic Field Access](#3-phase-0)
4. [Phase 1: New Domain Entities](#4-phase-1)
5. [Phase 2: Engine Core — Two-Pass Layout](#5-phase-2)
6. [Phase 3: Tool API v2 + Presets](#6-phase-3)
7. [Phase 4: Agent 2 Prompt V2](#7-phase-4)
8. [Phase 5: Frontend V2 Rendering](#8-phase-5)
9. [Phase 6: Migration & Wiring](#9-phase-6)
10. [Activation Guide](#10-activation)
11. [File Index](#11-file-index)
12. [Design Tokens Reference](#12-tokens)
13. [Rules Reference](#13-rules)
14. [Test Coverage](#14-tests)

---

## 1. Overview

Engine V2 is a complete replacement of the visual assembly engine. It addresses the fundamental limitations of v1:

| Problem (v1) | Solution (v2) |
|---|---|
| Hardcoded 14 field names (`FieldTypeMap`, `ProductFieldGetter`) | DB-driven `field_definitions` table, generic `ProductToMap`/`ServiceToMap` |
| Single `display` string mixes typography and visual container | Separate `TextStyle` (font) + `WrapperConfig` (container) |
| Flat zone-based layout (`Zone[]`) | Recursive `LayoutNode` tree (row/column/flow/span) |
| No concept of field importance/flexibility | `Rigidity` system: locked / preferred / flexible |
| Raw pixel values in prompts | Semantic tokens: `fontSize: "lg"`, `fontWeight: "bold"` |
| Field names in LLM prompts | Human-readable labels from DB: `"Цена"` not `"price"` |
| Switch-case field access | Generic `map[string]interface{}` with `Extra` for custom fields |

**Key invariant**: v2 is fully backward-compatible. The engine produces v1-compatible `Atoms[]` + `Zones[]` through compat converters alongside v2 `AtomsV2[]` + `Layout`. Frontend detects v2 data and uses v2 renderer; otherwise falls back to v1.

---

## 2. Architecture

### 2.1 Data Flow

```
Agent 2 tool call → visual_assembly tool
                         ↓
                  ┌──────────────────┐
                  │ v1 path (default) │  ← existing code, unchanged
                  │ v2 path           │  ← ENGINE_VERSION=v2
                  └──────────────────┘
                         ↓ (v2)
              FieldDefinitionPort.ListFieldDefinitions()
                         ↓
              EngineV2.Execute(input)
                         ↓
              10-step pipeline (see §5)
                         ↓
              FormationWithData + Warnings
                         ↓
              WidgetV2ToLegacy() — populates v1 Atoms/Zones
                         ↓
              State → Frontend
                         ↓
              WidgetRenderer detects widget.layout?
                   ├── YES → GenericCardV2Template → LayoutTreeRenderer → AtomV2Renderer
                   └── NO  → GenericCardTemplate (v1)
```

### 2.2 Package Dependencies

```
domain/           ← AtomV2, LayoutNode, PresetV2, Rigidity, TextStyle, WrapperConfig
    ↑
engine/           ← EngineV2, AutoLayout, BudgetDown/NeedsUp, Rules, Tokens, Compat
    ↑
presets/          ← PresetV2Registry (uses domain.PresetV2)
    ↑
tools/            ← VisualAssemblyTool.executeV2(), convertV1ParamsToV2
    ↑
usecases/         ← Agent2ExecuteUseCase (v2 prompt, field labels)
    ↑
cmd/server/       ← wiring: FieldDefinitionAdapter, NewRegistryV2, NewAgent2ExecuteUseCaseV2
```

---

## 3. Phase 0: Field Definitions + Generic Field Access

**Goal**: Replace hardcoded `FieldTypeMap`, `ProductFieldGetter`, `ServiceFieldGetter` with DB-driven metadata.

### 3.1 Database Table

```sql
CREATE TABLE catalog.field_definitions (
    tenant_id    TEXT NOT NULL,
    field_name   TEXT NOT NULL,
    entity_type  TEXT NOT NULL,   -- 'product' or 'service'
    atom_type    TEXT NOT NULL,   -- 'text', 'number', 'image'
    atom_subtype TEXT NOT NULL,   -- 'currency', 'rating', 'string'
    unit         TEXT DEFAULT '',
    label        TEXT NOT NULL,   -- Human-readable: 'Цена', 'Бренд'
    default_display TEXT NOT NULL,
    default_slot TEXT NOT NULL,
    priority     INT NOT NULL,   -- Lower = more important
    created_at   TIMESTAMPTZ DEFAULT NOW(),
    updated_at   TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (tenant_id, entity_type, field_name)
);
```

Seeded with 22 fields (13 product + 9 service) for all existing tenants.

### 3.2 Port & Adapter

**`ports/field_definition_port.go`**:
```go
type FieldDefinition struct {
    TenantID, FieldName string
    EntityType domain.EntityType
    AtomType domain.AtomType
    AtomSubtype domain.AtomSubtype
    Unit, Label, DefaultDisplay string
    DefaultSlot domain.AtomSlot
    Priority int
}

type FieldDefinitionPort interface {
    ListFieldDefinitions(ctx, tenantID, entityType) ([]FieldDefinition, error)
    GetFieldDefinition(ctx, tenantID, entityType, fieldName) (*FieldDefinition, error)
}
```

**`adapters/postgres/field_definition_adapter.go`** — PostgreSQL implementation.

### 3.3 Generic Field Access

**`engine/formation.go`** — new functions:
- `ProductToMap(p Product) map[string]interface{}` — converts all product fields to a generic map, including `p.Extra` custom fields. Skips empty strings, zero prices, empty slices.
- `ServiceToMap(s Service) map[string]interface{}` — same for services.
- `GenericFieldGetter(data map[string]interface{}) FieldGetter` — replaces type-specific getters.

**`engine/defaults.go`** — new functions:
- `FieldDefinitionEntry` — lightweight struct (no `ports` import) for passing DB data to engine.
- `BuildFieldConfigsFromDefinitions(defs, fields, displayOverrides, formatOverrides)` — builds `FieldConfig[]` from DB definitions.
- `FieldRankingFromDefinitions(defs) []string` — extracts field name order from definitions.

### 3.4 Domain Changes

- `domain/product_entity.go` — added `Extra map[string]interface{}` for tenant-defined custom fields.
- `domain/service_entity.go` — same.

---

## 4. Phase 1: New Domain Entities

**Goal**: New data structures that separate typography from visual container, support recursive layouts, and introduce rigidity.

### 4.1 Rigidity System

```go
type Rigidity string
const (
    RigidityLocked    Rigidity = "locked"    // User explicit — NEVER touch
    RigidityPreferred Rigidity = "preferred" // Preset default — adjust if needed
    RigidityFlexible  Rigidity = "flexible"  // Engine default — freely adjustable
)
```

All constraint rules check rigidity before modifying an atom. Locked atoms are never touched by any rule.

### 4.2 TextStyle + WrapperConfig (replaces `display`)

```go
// TextStyle — HOW text looks (typography)
type TextStyle struct {
    FontSize       string  // Semantic tokens: xs, sm, md, lg, xl, 2xl, 3xl
    FontWeight     string  // light, normal, medium, semibold, bold
    Color          string  // Named: green, red, muted, error, success... or hex
    TextDecoration string  // line-through, underline
    TextTransform  string  // capitalize, uppercase, lowercase
    LineClamp      int     // CSS -webkit-line-clamp
    Truncate       int     // Max characters
}

// WrapperConfig — WHAT visual container wraps the value
type WrapperConfig struct {
    Type    string   // none, badge, tag, pill, avatar, tooltip, alert, link, progress, button
    Variant string   // badge: success/error/warning; button: primary/secondary/outline; tag: active
    Rigidity Rigidity
}
```

### 4.3 AtomV2

```go
type AtomV2 struct {
    Type      AtomType        // text, number, image, icon, video, audio
    Subtype   AtomSubtype     // currency, rating, string, url, date...
    Value     interface{}
    Label     string          // Human-readable: "Цена", "Бренд"
    Unit      string          // "RUB", "min"
    Format    AtomFormat      // currency, stars, stars-compact, percent, date, text
    TextStyle *TextStyle
    Wrapper   *WrapperConfig
    Slot      AtomSlot        // hero, title, price, primary, secondary, badge
    Rigidity  Rigidity
    FieldName string          // "price", "brand" — internal key
    Priority  int             // Lower = more important
    Meta      map[string]interface{}
}
```

### 4.4 LayoutNode (replaces Zone[])

```go
type LayoutNodeType string
const (
    LayoutNodeRow    = "row"    // Horizontal flex
    LayoutNodeColumn = "column" // Vertical stack
    LayoutNodeFlow   = "flow"   // Flex-wrap (tags, badges)
    LayoutNodeSpan   = "span"   // Full-width block
)

type LayoutNode struct {
    Type         LayoutNodeType
    Children     []LayoutChild
    Gap          string          // Semantic: none, xs, sm, md, lg, xl, 2xl
    Align        string          // flex align-items
    Wrap         bool
    GroupWrapper string          // "collapse" (expandable) or "carousel" (scroll)
    Rigidity     Rigidity
    Name         string          // Debug label: "hero", "price-rating", "tags"
}

type LayoutChild struct {
    AtomIndex *int        // Leaf: index into Widget.AtomsV2
    Node      *LayoutNode // Nested group
}
```

### 4.5 Widget V2 Fields

Added to existing `domain.Widget` struct (coexists with v1 fields):

```go
Layout  *LayoutNode   `json:"layout,omitempty"`
AtomsV2 []AtomV2      `json:"atomsV2,omitempty"`
Actions []ActionDef   `json:"actions,omitempty"`
States  *WidgetStates `json:"states,omitempty"`
```

### 4.6 Actions & States

```go
type WidgetActionType string // 13 types: open_detail, navigate, add_to_cart, favorite, compare...

type ActionDef struct {
    Type   WidgetActionType
    Label  string
    Icon   string
    Params map[string]interface{}
}

type WidgetStates struct {
    Hover  map[string]string // CSS custom properties for :hover
    Active map[string]string // CSS custom properties for :active
}
```

### 4.7 Design Tokens

```go
type DesignTokensV2 struct {
    FontSize   map[string]int  // xs→10, sm→12, md→14, lg→18, xl→24, 2xl→30, 3xl→36
    FontWeight map[string]int  // light→300, normal→400, medium→500, semibold→600, bold→700
    Spacing    map[string]int  // none→0, xs→2, sm→4, md→8, lg→12, xl→16, 2xl→24
    Radius     map[string]int  // none→0, sm→4, md→8, lg→12, full→9999
    IconSize   map[string]int  // sm→16, md→20, lg→24
}
```

---

## 5. Phase 2: Engine Core — Two-Pass Layout

**Goal**: 10-step pipeline with two-pass layout algorithm, rigidity-aware constraint rules on 5 junctions.

### 5.1 Pipeline

```
EngineV2.Execute(input EngineV2Input) → EngineV2Output

Step 1:  buildTypedAtoms    — create AtomV2[] from data using field definitions
Step 2:  applyValues        — apply agent instructions (show/hide/order/textStyle/wrapper)
Step 3:  applyAtomConstraints — per-atom rules (badge overflow, text truncation)
Step 4:  buildLayout        — AutoLayout → LayoutNode tree
Step 5:  budgetDown         — distribute available space top-down
Step 6:  needsUp            — calculate actual needs bottom-up
Step 7:  (if needs > budget: junction rules, max 2 iterations)
Step 8:  applyWidgetConstraints — per-widget rules
Step 9:  buildFormation     — assemble FormationWithData
Step 10: applyCrossWidgetConstraints — cross-widget consistency
Post:    WidgetV2ToLegacy   — populate v1 Atoms/Zones for backward compat
```

### 5.2 AutoLayout (replaces CalculateZones)

Groups atoms into a LayoutNode tree by type/subtype/wrapper:

| Bucket | Condition | Node Type | Name |
|---|---|---|---|
| 1 | `type == image` | span | "hero" |
| 2 | `fontSize in {3xl, 2xl, xl}` | column | "headings" |
| 3 | `slot == price` or `subtype == currency` | row | "price-rating" |
| 4 | `subtype == rating` | (merged into price-rating) | — |
| 5 | `wrapper.type in {tag, badge, pill}` | flow | "tags" |
| 6 | `wrapper.type == button` | row | "actions" |
| 7 | `type in {text, number}` (remaining) | column | "body" |
| 8 | Everything else | column | "other" |

Root is always `column` with gap `"sm"`.

Flow nodes with >9 items get `GroupWrapper: "collapse"` (expandable section).

### 5.3 BudgetDown / NeedsUp

**BudgetDown** — distributes screen space top-down:
- FormationWidth = ViewportWidth (default 400px for chat)
- Columns from `ResolvedDefaults`
- WidgetWidth = FormationWidth / Columns - gaps
- WidgetHeight by size: tiny=100, small=200, medium=340, large=460
- FormationHeight = rows × WidgetHeight + gaps

**NeedsUp** — calculates actual pixel needs bottom-up:
- Image atom = 200px
- Text atom = fontSize × 1.5 + wrapper padding (badge/tag: +8, button: +16)
- Widget needs = sum of atom heights + inter-atom gap (4px)

### 5.4 Constraint Rules

**15 rules on 5 junctions** — all respect rigidity (locked atoms never modified):

#### Per-Atom Rules (6)
| Rule | Description |
|---|---|
| A1 | Badge text > 20 chars → downgrade to tag |
| A2 | Tag text > 40 chars → unwrap to plain text |
| A4 | Badge text → capitalize first letter |
| A5 | Rating < 3.0 → force `stars-compact` format |
| D5 | Truncate text by slot (title: 80, primary: 60, secondary: 120, badge: 20, price: 15) |
| D6 | Large heading → downgrade by text length (3xl+60chars→2xl, 2xl+80chars→xl) |

#### Per-Widget Rules (4)
| Rule | Description |
|---|---|
| W1 | Max 2 badges per widget; 3rd+ → tag |
| W2 | Max 5 tags per widget; excess → removed |
| W4 | Max 1 large heading (3xl/2xl) per widget; extras → xl |
| W8 | Tiny size → remove all image atoms |

#### Cross-Widget Rules (3)
| Rule | Description |
|---|---|
| C1 | Field present in <70% of widgets → remove from all (consistency) |
| C2 | Missing common fields → placeholder "—" (or 0 for numbers) |
| C3 | Same field → same format across all widgets |

#### Junction Rules (4)
| Rule | Description |
|---|---|
| J-overflow-switch | Rows with >3 atom children → convert to column |
| J-downgrade | TotalHeight > Budget → reduce fontSize on flexible atoms |
| J-priority-hide | TotalHeight > Budget×2 → hide lowest-priority flexible atoms |
| J-viewport-fit | WidgetWidth < 150px → reduce column count |

### 5.5 Compat Converters

- `AtomV2ToLegacy(AtomV2) → Atom` — maps TextStyle+Wrapper back to single `display` string
- `LayoutToZones(*LayoutNode) → []Zone` — flattens tree to flat zone list
- `WidgetV2ToLegacy(*Widget)` — populates `Atoms` and `Zones` from `AtomsV2` and `Layout`

---

## 6. Phase 3: Tool API v2 + Presets

**Goal**: v2 code path in visual_assembly tool, v1-to-v2 parameter conversion, v2 presets.

### 6.1 AgentInstructions

```go
type AgentInstructions struct {
    Preset string            // Preset name
    Show   []string          // Fields to add
    Hide   []string          // Fields to remove
    Order  []string          // Field display order
    Atoms  map[string]AtomOverride // Per-atom overrides (keyed by field name)
    Layout string            // grid, list, single, carousel, comparison
    Size   string            // tiny, small, medium, large
    Limit  int
    Offset int
}

type AtomOverride struct {
    TextStyle *TextStyle
    Wrapper   *WrapperConfig
    Format    string
    Color     string
    Rigidity  Rigidity
}
```

### 6.2 V1-to-V2 Parameter Conversion

`convertV1ParamsToV2(input) → *AgentInstructions` maps:
- `preset`, `show`, `hide`, `order`, `layout`, `size`, `limit`, `offset` → direct copy
- `display: {"brand": "badge"}` → `Atoms["brand"].TextStyle + Wrapper` via `DisplayToTextStyleWrapper`
- `color: {"brand": "red"}` → `Atoms["brand"].Color = "red"`

### 6.3 V2 Presets

5 default presets in `presets/preset_v2.go`:

| Preset | Entity | Layout | Size | Fields |
|---|---|---|---|---|
| `product_card_grid` | product | grid | medium | images, name, brand(tag), price, rating |
| `product_card_detail` | product | single | large | +category, description, tags, stockQuantity |
| `product_row` | product | list | small | name, price |
| `service_card` | service | grid | medium | images, name, provider, duration, price, rating |
| `service_detail` | service | single | large | +availability, description, attributes |

Each field has `TextStyle`, `Wrapper`, `Format`, `Slot`, `Priority`, `Rigidity`.

### 6.4 Execution Routing

```go
// tools/tool_visual_assembly.go
func (t *VisualAssemblyTool) Execute(ctx, toolCtx, input) {
    if t.engineVersion == "v2" {
        return t.executeV2(ctx, toolCtx, input)
    }
    // v1 path unchanged
}
```

---

## 7. Phase 4: Agent 2 Prompt V2

**Goal**: LLM uses semantic tokens and labels instead of raw values and field names.

### 7.1 System Prompt Changes

**v1 prompt** (`Agent2ToolSystemPrompt`): uses `display: {"brand": "badge"}`, raw pixel values, field names.

**v2 prompt** (`Agent2ToolSystemPromptV2`): uses:
- `atoms: {"brand": {"wrapper": {"type": "badge"}}}` — separate textStyle/wrapper
- Semantic tokens: `fontSize: "lg"` not `24`
- `rigidity: "locked"` for explicit user requests
- Wrapper types listed: none, badge, tag, pill, avatar, tooltip, alert, link, progress, button
- Format values listed: currency, stars, stars-text, stars-compact, percent, number, date, text

### 7.2 Field Labels in Context

`BuildAgent2ToolPromptV2(... fieldLabels map[string]string)` adds a `field_labels` object to the prompt JSON:

```json
{
  "productCount": 5,
  "fields": ["images", "name", "price", "brand"],
  "field_labels": {
    "name": "Название",
    "price": "Цена",
    "brand": "Бренд",
    "rating": "Рейтинг"
  }
}
```

Labels are loaded from `field_definitions` table via `FieldDefinitionPort`.

### 7.3 Feature Flag

`AGENT2_PROMPT_VERSION` env var:
- `v1` (default) — existing prompt, `BuildAgent2ToolPrompt()`
- `v2` — new prompt, `BuildAgent2ToolPromptV2()` with field labels

### 7.4 Wiring

```go
// usecases/agent2_execute.go
NewAgent2ExecuteUseCaseV2(llm, statePort, toolRegistry, log, fieldDefPort)
// Reads AGENT2_PROMPT_VERSION env var
// If "v2": uses Agent2ToolSystemPromptV2 + BuildAgent2ToolPromptV2
// loadFieldLabels() queries FieldDefinitionPort for tenant's field labels
```

`TenantSlug` added to `Agent2ExecuteRequest`, piped from `PipelineExecuteUseCase`.

---

## 8. Phase 5: Frontend V2 Rendering

**Goal**: Render LayoutNode tree, wrapper components, hover/active states.

### 8.1 Component Hierarchy

```
WidgetRenderer
  ├── (widget.layout || widget.atomsV2?) → GenericCardV2Template
  │     ├── ImageCarousel (hero images)
  │     ├── LayoutTreeRenderer (recursive)
  │     │     ├── layout-row/column/flow/span (CSS flex)
  │     │     ├── CollapseGroup (expandable)
  │     │     ├── CarouselGroup (horizontal scroll)
  │     │     └── AtomV2Renderer (leaf)
  │     │           ├── Text content (inline textStyle CSS)
  │     │           └── Wrapper component (badge/tag/pill/avatar/tooltip/alert/link/progress/button)
  │     ├── Cart button
  │     └── Hover/active CSS variables from widget.states
  │
  └── (else) → GenericCardTemplate (v1, unchanged)
```

### 8.2 AtomV2Renderer

`entities/atom/AtomV2Renderer.jsx` — renders a v2 atom with separate textStyle + wrapper.

**TextStyle → inline CSS**:
```js
const FONT_SIZE_TOKENS = { xs: 10, sm: 12, md: 14, lg: 18, xl: 24, '2xl': 30, '3xl': 36 };
const FONT_WEIGHT_TOKENS = { light: 300, normal: 400, medium: 500, semibold: 600, bold: 700 };

// atom.textStyle → { fontSize: '18px', fontWeight: 600, color: '#22C55E' }
```

**Value formatting**: same logic as v1 AtomRenderer — currency, stars, stars-text, stars-compact, percent, number, date, text.

**Wrapper components**:

| Wrapper | Rendered as | Styling |
|---|---|---|
| badge | `<span class="atom-v2-badge">` | Colored pill, max-width 160px |
| tag | `<span class="atom-v2-tag">` | Outlined chip, border |
| pill | `<span class="atom-v2-pill">` | Rounded pill (larger than badge) |
| avatar | `<span class="atom-v2-avatar">` | 40px circle, overflow hidden |
| tooltip | `<span class="atom-v2-tooltip">` | Dotted underline, title attr |
| alert | `<div class="atom-v2-alert">` | Left-border colored box |
| link | `<a class="atom-v2-link">` | Purple, underline on hover |
| progress | `<div class="atom-v2-progress">` | 6px bar with fill width |
| button | `<button class="atom-v2-button">` | Rounded 8px, primary/secondary/outline |

**Variant colors**: success=#22C55E, error=#EF4444, warning=#F59E0B, info=#3B82F6, primary=#8B5CF6, secondary=#6B7280.

### 8.3 LayoutTreeRenderer

`entities/widget/templates/LayoutTreeRenderer.jsx` — recursive component.

**Node type → CSS**:
| Node Type | CSS |
|---|---|
| row | `display: flex; flex-direction: row; align-items: center` |
| column | `display: flex; flex-direction: column` |
| flow | `display: flex; flex-direction: row; flex-wrap: wrap` |
| span | `display: block; width: 100%` |

**Group wrappers**:
- `collapse` → `CollapseGroup`: hidden content + "Показать ещё" toggle button
- `carousel` → `CarouselGroup`: horizontal scroll, hidden scrollbar

**Spacing tokens** (same as backend):
```js
const SPACING_TOKENS = { none: 0, xs: 2, sm: 4, md: 8, lg: 12, xl: 16, '2xl': 24 };
```

### 8.4 GenericCardV2Template

`entities/widget/templates/GenericCardV2Template.jsx`:

1. Extracts hero images from `atomsV2` (slot=hero, type=image)
2. Renders `ImageCarousel` for hero images (same as v1)
3. Renders `LayoutTreeRenderer` for content layout
4. Renders cart/favorite buttons (same as v1)
5. Applies `widget.states` as CSS custom properties for hover/active

### 8.5 V2 Routing in WidgetRenderer

```jsx
// WidgetRenderer.jsx — renderTemplate()
if (widget.layout || widget.atomsV2) {
    return <GenericCardV2Template atomsV2={widget.atomsV2} layout={widget.layout} ... />;
}
// else: existing v1 switch/case
```

### 8.6 CSS

`entities/atom/AtomV2.css` — styles for:
- All wrapper components (badge, tag, pill, avatar, tooltip, alert, link, progress, button)
- Variant colors (success, error, warning)
- Layout tree classes (layout-row, layout-column, layout-flow, layout-span)
- Collapse/carousel group wrappers
- V2 card hover/active states via CSS custom properties

---

## 9. Phase 6: Migration & Wiring

**Goal**: Wire v2 in main.go, env-based activation.

### 9.1 Tool Registry V2

`tools/tool_registry.go` — added `NewRegistryV2()`:

```go
func NewRegistryV2(statePort, catalogPort, presetRegistry, embeddingPort,
                   fieldDefPort FieldDefinitionPort, engineVersion string) *Registry {
    // Same data tools as v1 (catalog_search, state_filter, history_lookup)
    // Render tool: v2 if engineVersion=="v2" && fieldDefPort!=nil, else v1
}
```

### 9.2 main.go Wiring

```go
// Read ENGINE_VERSION env var
engineVersion := os.Getenv("ENGINE_VERSION") // "v1" or "v2"

// Create FieldDefinitionAdapter if v2
if dbClient != nil && engineVersion == "v2" {
    fieldDefAdapter = postgres.NewFieldDefinitionAdapter(dbClient)
}

// Tool registry: v1 or v2
if engineVersion == "v2" && fieldDefAdapter != nil {
    toolRegistry = tools.NewRegistryV2(stateAdapter, catalogAdapter, presetRegistry,
                                       embeddingClient, fieldDefAdapter, engineVersion)
} else {
    toolRegistry = tools.NewRegistry(stateAdapter, catalogAdapter, presetRegistry, embeddingClient)
}

// Agent2: v1 or v2
if engineVersion == "v2" && fieldDefAdapter != nil {
    agent2UC = usecases.NewAgent2ExecuteUseCaseV2(llmClient, stateAdapter, toolRegistry,
                                                   appLog, fieldDefAdapter)
} else {
    agent2UC = usecases.NewAgent2ExecuteUseCase(llmClient, stateAdapter, toolRegistry, appLog)
}
```

### 9.3 Environment Variables

| Variable | Values | Default | Effect |
|---|---|---|---|
| `ENGINE_VERSION` | `v1`, `v2` | v1 | Selects engine pipeline + tool registry |
| `AGENT2_PROMPT_VERSION` | `v1`, `v2` | v1 | Selects Agent 2 system prompt |

Both must be `v2` for full v2 operation.

---

## 10. Activation Guide

### Safe mode (default — no changes needed)

Everything works as v1. No env vars needed.

### Enable V2

Add to `project/.env`:
```
ENGINE_VERSION=v2
AGENT2_PROMPT_VERSION=v2
```

Restart backend. The following happens:
1. `FieldDefinitionAdapter` is created
2. `field_definitions` migration runs (already ran during catalog migrations)
3. Tool registry uses `NewVisualAssemblyToolV2` with v2 engine
4. Agent 2 uses v2 prompt with semantic tokens + field labels
5. Frontend auto-detects `widget.layout` or `widget.atomsV2` and renders v2

### Rollback

Remove env vars or set to `v1`. Restart. Everything reverts to v1.

---

## 11. File Index

### New Files (Backend)

| File | Lines | Description |
|---|---|---|
| `ports/field_definition_port.go` | ~32 | FieldDefinition struct + FieldDefinitionPort interface |
| `adapters/postgres/field_definition_adapter.go` | ~80 | PostgreSQL implementation |
| `domain/layout_entity.go` | ~75 | LayoutNode, LayoutChild, WidgetActionType, ActionDef, WidgetStates |
| `domain/preset_v2_entity.go` | ~23 | PresetV2, PresetV2Field |
| `engine/engine_v2.go` | ~400 | EngineV2 10-step pipeline, DisplayToTextStyleWrapper |
| `engine/auto_layout.go` | ~180 | AutoLayout (replaces CalculateZones) |
| `engine/layout_pass.go` | ~180 | BudgetDown, NeedsUp |
| `engine/rules.go` | ~376 | 15 constraint rules on 5 junctions |
| `engine/tokens.go` | ~78 | DesignTokensV2, token resolution |
| `engine/compat.go` | ~172 | AtomV2ToLegacy, LayoutToZones, WidgetV2ToLegacy |
| `engine/instructions.go` | ~37 | AgentInstructions, AtomOverride |
| `engine/engine_v2_test.go` | ~310 | 12 v2 tests |
| `presets/preset_v2.go` | ~119 | PresetV2Registry with 5 presets |

### Modified Files (Backend)

| File | Changes |
|---|---|
| `domain/atom_entity.go` | +AtomV2, Rigidity, TextStyle, WrapperConfig (~65 lines) |
| `domain/product_entity.go` | +Extra map[string]interface{} |
| `domain/service_entity.go` | +Extra map[string]interface{} |
| `domain/widget_entity.go` | +Layout, AtomsV2, Actions, States fields |
| `engine/defaults.go` | +FieldDefinitionEntry, BuildFieldConfigsFromDefinitions, FieldRankingFromDefinitions (~100 lines) |
| `engine/formation.go` | +ProductToMap, ServiceToMap, GenericFieldGetter (~100 lines) |
| `tools/tool_visual_assembly.go` | +executeV2, convertV1ParamsToV2, NewVisualAssemblyToolV2 (~240 lines) |
| `tools/tool_registry.go` | +NewRegistryV2 (~25 lines) |
| `adapters/postgres/catalog_migrations.go` | +migrationCatalogFieldDefinitions + seed (~55 lines) |
| `prompts/prompt_compose_widgets.go` | +Agent2ToolSystemPromptV2, BuildAgent2ToolPromptV2 (~130 lines) |
| `usecases/agent2_execute.go` | +NewAgent2ExecuteUseCaseV2, loadFieldLabels, TenantSlug field (~60 lines) |
| `usecases/pipeline_execute.go` | +TenantSlug passthrough to Agent2 (1 line) |
| `cmd/server/main.go` | +v2 wiring (~20 lines) |

### New Files (Frontend)

| File | Description |
|---|---|
| `entities/atom/AtomV2Renderer.jsx` | V2 atom renderer: textStyle→CSS, wrapper→component |
| `entities/atom/AtomV2.css` | Wrapper + layout tree CSS |
| `entities/widget/templates/LayoutTreeRenderer.jsx` | Recursive LayoutNode tree renderer |
| `entities/widget/templates/GenericCardV2Template.jsx` | V2 card template with layout tree |

### Modified Files (Frontend)

| File | Changes |
|---|---|
| `entities/widget/WidgetRenderer.jsx` | +v2 routing (widget.layout → GenericCardV2Template) |
| `entities/widget/templates/index.js` | +GenericCardV2Template export |

---

## 12. Design Tokens Reference

### Font Size
| Token | px |
|---|---|
| xs | 10 |
| sm | 12 |
| md | 14 |
| lg | 18 |
| xl | 24 |
| 2xl | 30 |
| 3xl | 36 |

### Font Weight
| Token | Value |
|---|---|
| light | 300 |
| normal | 400 |
| medium | 500 |
| semibold | 600 |
| bold | 700 |

### Spacing
| Token | px |
|---|---|
| none | 0 |
| xs | 2 |
| sm | 4 |
| md | 8 |
| lg | 12 |
| xl | 16 |
| 2xl | 24 |

### Border Radius
| Token | px |
|---|---|
| none | 0 |
| sm | 4 |
| md | 8 |
| lg | 12 |
| full | 9999 |

---

## 13. Rules Reference

### DisplayToTextStyleWrapper Mapping (28 entries)

| v1 Display | → TextStyle | → Wrapper |
|---|---|---|
| h1 | 3xl, bold | — |
| h2 | 2xl, semibold | — |
| h3 | xl, semibold | — |
| h4 | lg, medium | — |
| body-lg | lg | — |
| body | md | — |
| body-sm | sm | — |
| caption | xs | — |
| badge | xs, medium | badge |
| badge-success | xs, medium | badge(success) |
| badge-error | xs, medium | badge(error) |
| badge-warning | xs, medium | badge(warning) |
| tag | xs | tag |
| tag-active | xs | tag(active) |
| price | lg, bold | — |
| price-lg | xl, bold | — |
| price-old | md, line-through, color:muted | — |
| price-discount | lg, bold, color:error | — |
| avatar | — | avatar |
| button-primary | sm, medium | button(primary) |
| button-secondary | sm, medium | button(secondary) |
| button-outline | sm, medium | button(outline) |
| progress | — | progress |

---

## 14. Test Coverage

### engine/engine_v2_test.go (12 tests)

| Test | Covers |
|---|---|
| `TestEngineV2_EmptyInput` | Empty input → 0 widgets, non-nil formation |
| `TestEngineV2_SingleProduct` | 1 product → AtomsV2 + Layout + v1 compat |
| `TestEngineV2_MultipleProducts_GridLayout` | 4 products → grid + 4 widgets |
| `TestEngineV2_WithFieldDefinitions` | Custom field defs → correct atoms + labels |
| `TestEngineV2_WithInstructions_ShowHide` | Show/hide filtering |
| `TestEngineV2_V1Compat_AtomsAndZones` | V1 Atoms+Zones populated from V2 data |
| `TestAutoLayout_GroupsByType` | 6 atom types → correct group structure |
| `TestAtomV2Constraints_BadgeOverflow` | A1 rule: badge > 20 chars → tag |
| `TestAtomV2Constraints_LockedNotTouched` | Locked atoms bypass all rules |
| `TestProductToMap_BasicFields` | ProductToMap basic field extraction |
| `TestProductToMap_EmptyFieldsOmitted` | Empty/zero fields not included |
| `TestGenericFieldGetter` | Generic getter returns correct values |

### Pre-existing tests (19 in engine/)

All continue to pass — v1 code is untouched.

### Known pre-existing failures (NOT related to V2)

- `tools/` tests: `mockStatePort` missing `UpdateActions` method
- `usecases/navigation_test.go`: same mock issue
- `adapters/postgres/` relevance tests: require live DB with specific data
- `handlers/` middleware test: tenant resolution edge case
