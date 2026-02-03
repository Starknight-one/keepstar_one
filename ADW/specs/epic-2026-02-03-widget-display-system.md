# Epic: Widget Display System

## Meta: Why We're Doing This

### Context
- **Date**: 2026-02-03
- **Scale Target**: 50 бизнесов × 20-50k users/month = до 2.5M запросов/месяц
- **Data Scale**: Фантастически много товаров и услуг

### Core Problem
- 80+ атрибутов на товар/услугу → нельзя показать всё сразу
- Разные сущности (товар vs услуга) = разные пресеты отображения
- Нужны "раскрытия" — drill-down в детали с возможностью вернуться
- Каждый запрос как новый = дорого по токенам и времени

### Key Decisions Made

1. **Три базовые сущности**:
   - **Product** (товар) — в БД, свои пресеты
   - **Service** (услуга) — в БД, свои пресеты
   - **Application** (приложение) — будущее, отдельный флоу

2. **Два режима (пока фокус на первом)**:
   - **Preset mode** — LLM выбирает какие пресеты применить через tools (быстрый, простой промпт)
   - **Freestyle mode** — (out of scope пока) пользователь описывает layout

3. **Drill-down/Expand механика**:
   - Клик на товар → больше информации
   - State change → можно вернуться назад
   - History для navigation

4. **Кэширование и дельты**:
   - State как посредник со ссылками (не сырые данные!)
   - Delta-based изменения
   - Validation через hooks

---

## Core Architecture: Agents & State

### Mental Model

```
┌──────────────────────────────────────────────────────────────┐
│                                                              │
│   User: "покажи кроссовки и доставку"                       │
│                    ↓                                         │
│   ┌────────────────────────────────────┐                    │
│   │            AGENTS                   │                    │
│   │  (stateless функции)               │                    │
│   │                                     │                    │
│   │  • Понимают intent из natural lang │                    │
│   │  • Вызывают tools                   │                    │
│   │  • Манипулируют state              │                    │
│   │  • Распределены (Agent1, Agent2+)  │                    │
│   └─────────────┬──────────────────────┘                    │
│                 │                                            │
│                 ↓ tools                                      │
│   ┌────────────────────────────────────┐                    │
│   │            STATE                    │                    │
│   │  (посредник со ссылками)           │                    │
│   │                                     │                    │
│   │  НЕ хранит сырые данные!           │                    │
│   │  Хранит:                           │                    │
│   │  • refs на products/services в БД  │                    │
│   │  • view stack (что показано)       │                    │
│   │  • interactions history            │                    │
│   │  • attribute aliases               │                    │
│   │  • ready-to-call queries           │                    │
│   └─────────────┬──────────────────────┘                    │
│                 │                                            │
│                 ↓ render(state)                              │
│   ┌────────────────────────────────────┐                    │
│   │           RENDERER                  │                    │
│   │  (pure function)                   │                    │
│   │                                     │                    │
│   │  state + DB fetch → UI             │                    │
│   └────────────────────────────────────┘                    │
│                                                              │
└──────────────────────────────────────────────────────────────┘
```

### Agents = Stateless Functions

```go
// Agent — stateless, только понимает и вызывает tools
type Agent interface {
    // Input: natural language + current state refs
    // Output: tool calls that modify state
    Process(ctx context.Context, query string, stateRef *StateRef) ([]ToolCall, error)
}

// Tools распределены между агентами
// Agent1: data tools (search, filter, sort)
// Agent2: display tools (render_product_preset, render_service_preset)
// AgentN: custom tools for specific domains
```

### State = Index + Refs (NOT Data Storage)

```go
type SessionState struct {
    SessionID string

    // References, not raw data
    Refs struct {
        Products   []ProductRef   // IDs + ready queries for details
        Services   []ServiceRef   // IDs + ready queries for details
    }

    // What's currently displayed
    View struct {
        Mode       ViewMode       // "grid" | "detail" | "comparison"
        Formation  FormationRef   // How items are laid out
        Focused    *EntityRef     // Currently expanded item (if any)
    }

    // Navigation history for back/forward
    ViewStack []ViewSnapshot

    // Interactions
    Interactions []Interaction   // clicks, expands, filters applied

    // Attribute mappings (aliases without actual data)
    AttrMappings map[string]AttrMapping

    // Metadata
    Meta StateMeta
}

type ProductRef struct {
    ID          string
    TenantID    string
    // Ready-to-call for drill-down
    DetailQuery string // "SELECT ... WHERE id = $1"
    // Cached minimal info for grid display
    Preview     ProductPreview // name, price, thumbnail only
}

type ViewSnapshot struct {
    Mode      ViewMode
    Formation FormationRef
    Refs      []EntityRef
    Timestamp time.Time
}
```

### Validation via Hooks

```go
type StateHook interface {
    // Called after tool execution, before state write
    OnToolComplete(ctx context.Context, tool ToolCall, result ToolResult) error

    // Validates what's being written to state
    ValidateStateChange(ctx context.Context, before, after *SessionState) error
}

// Example: validate that render_product_preset only writes product refs
type PresetValidationHook struct{}

func (h *PresetValidationHook) ValidateStateChange(ctx, before, after *SessionState) error {
    // Check that new refs match expected entity type
    // Check that view mode is valid for this preset
    // etc.
}
```

---

## Feature 1: Entity Types & Presets

### Objective
Разные пресеты для разных типов сущностей. LLM выбирает какие пресеты нужны.

### Entity Types

```go
type EntityType string
const (
    EntityTypeProduct EntityType = "product"
    EntityTypeService EntityType = "service"
    // Future: EntityTypeApplication
)
```

### Preset System

```go
type Preset struct {
    Name       string     // "product_grid", "service_card"
    EntityType EntityType

    // Slot configuration
    Slots      map[AtomSlot]SlotConfig

    // Which fields to include (priority order)
    Fields     []FieldConfig

    // Display hints
    DefaultMode    FormationType // grid, list, carousel
    DefaultSize    WidgetSize    // small, medium, large
}

type FieldConfig struct {
    Name     string    // "price", "rating", "duration"
    Slot     AtomSlot  // where to place
    Priority int       // higher = show first
    Required bool      // must include
}

type SlotConfig struct {
    MaxAtoms     int
    AllowedTypes []AtomType
}
```

### Preset Tools (Agent2)

```go
// Agent2 calls these tools based on what's in state
var PresetTools = []ToolDefinition{
    {
        Name: "render_product_preset",
        Description: "Render products using product preset template",
        Parameters: map[string]interface{}{
            "preset": "product_grid | product_card | product_compact",
            "refs":   "product refs from state",
        },
    },
    {
        Name: "render_service_preset",
        Description: "Render services using service preset template",
        Parameters: map[string]interface{}{
            "preset": "service_card | service_list",
            "refs":   "service refs from state",
        },
    },
}
```

### Example Flow

```
User: "покажи кроссовки и экспресс-доставку"

Agent1:
  → tool: search_products(query="кроссовки")
  → tool: search_services(query="экспресс-доставка")
  → state.Refs.Products = [refs...]
  → state.Refs.Services = [refs...]

Agent2 (simple LLM, sees state.Meta):
  → "есть 5 products и 2 services"
  → tool: render_product_preset(preset="product_grid", refs=state.Refs.Products)
  → tool: render_service_preset(preset="service_card", refs=state.Refs.Services)
  → state.View.Formation = combined formation
```

### Files

| File | Action | Purpose |
|------|--------|---------|
| `domain/entity_type.go` | Create | EntityType enum |
| `domain/preset_entity.go` | Create | Preset, FieldConfig, SlotConfig |
| `presets/product_presets.go` | Create | ProductGridPreset, ProductCardPreset |
| `presets/service_presets.go` | Create | ServiceCardPreset, ServiceListPreset |
| `tools/tool_render_preset.go` | Create | render_product_preset, render_service_preset |

### Acceptance Criteria
- [ ] EntityType enum (product/service)
- [ ] Preset структура с slots и fields
- [ ] Product presets (grid, card, compact)
- [ ] Service presets (card, list)
- [ ] render_*_preset tools для Agent2
- [ ] Agent2 выбирает правильные пресеты по state.Meta

---

## Feature 2: Drill-down / Expand

### Objective
Пользователь кликает на товар → видит больше информации → может вернуться.

### State Changes

```go
// Expand action
type ExpandAction struct {
    EntityType EntityType
    EntityID   string
    From       ViewMode // where we came from (grid, list)
}

// When user clicks on product in grid:
func (uc *ExpandUseCase) Execute(ctx context.Context, req ExpandRequest) error {
    state := getState(req.SessionID)

    // 1. Save current view to stack
    state.ViewStack = append(state.ViewStack, ViewSnapshot{
        Mode:      state.View.Mode,
        Formation: state.View.Formation,
        Refs:      state.Refs.Products, // or services
        Timestamp: time.Now(),
    })

    // 2. Update view to detail mode
    state.View.Mode = ViewModeDetail
    state.View.Focused = &EntityRef{
        Type: req.EntityType,
        ID:   req.EntityID,
    }

    // 3. Record interaction
    state.Interactions = append(state.Interactions, Interaction{
        Type:      InteractionExpand,
        EntityRef: state.View.Focused,
        Timestamp: time.Now(),
    })

    return saveState(state)
}
```

### Back Navigation

```go
func (uc *BackUseCase) Execute(ctx context.Context, req BackRequest) error {
    state := getState(req.SessionID)

    if len(state.ViewStack) == 0 {
        return ErrNothingToGoBackTo
    }

    // Pop from stack
    prev := state.ViewStack[len(state.ViewStack)-1]
    state.ViewStack = state.ViewStack[:len(state.ViewStack)-1]

    // Restore view
    state.View.Mode = prev.Mode
    state.View.Formation = prev.Formation
    state.View.Focused = nil

    return saveState(state)
}
```

### Frontend

```jsx
function FormationRenderer({ formation, state }) {
    const handleItemClick = (entityType, entityId) => {
        // Call expand API
        api.expand(state.sessionId, entityType, entityId);
    };

    const handleBack = () => {
        api.back(state.sessionId);
    };

    // If in detail mode, show detail view with back button
    if (state.view.mode === 'detail') {
        return (
            <DetailView
                entityRef={state.view.focused}
                onBack={handleBack}
            />
        );
    }

    // Otherwise render formation (grid, carousel, etc.)
    return (
        <div className={getLayoutClass(formation.mode)}>
            {formation.widgets.map(widget => (
                <WidgetRenderer
                    key={widget.id}
                    widget={widget}
                    onClick={() => handleItemClick(widget.entityType, widget.entityId)}
                />
            ))}
        </div>
    );
}
```

### Files

| File | Action | Purpose |
|------|--------|---------|
| `domain/view_entity.go` | Create | ViewMode, ViewSnapshot, ViewStack |
| `domain/interaction_entity.go` | Create | Interaction, InteractionType |
| `usecases/expand_execute.go` | Create | ExpandUseCase |
| `usecases/back_execute.go` | Create | BackUseCase |
| `handlers/handler_navigation.go` | Create | POST /expand, POST /back |
| `frontend/.../DetailView.jsx` | Create | Detail view component |

### Acceptance Criteria
- [ ] ViewStack для navigation history
- [ ] ExpandUseCase сохраняет текущий view и переключает на detail
- [ ] BackUseCase восстанавливает предыдущий view
- [ ] Frontend показывает detail view с back button
- [ ] Interactions логируются

---

## Feature 3: Caching Layer

### Objective
Оптимизировать расходы и latency через кэширование.

### Cache Strategy

State уже содержит refs, не данные. Кэширование нужно для:
1. **LLM responses** — одинаковые запросы = одинаковые ответы
2. **Session context** — не терять контекст между запросами
3. **Formation cache** — готовые formations для быстрого back

### 3.1 LLM Cache

```go
type LLMCacheKey struct {
    PromptHash string // SHA256(system + user prompt)
    Model      string
}

type LLMCacheEntry struct {
    Response  string
    Usage     LLMUsage
    TTL       time.Duration // 1 hour
    CreatedAt time.Time
}

// Wrapped LLM port with caching
type CachedLLMAdapter struct {
    inner LLMPort
    cache LLMCachePort
}

func (c *CachedLLMAdapter) Chat(ctx, system, user string) (string, error) {
    key := LLMCacheKey{PromptHash: sha256(system + user)}

    if entry, _ := c.cache.Get(ctx, key); entry != nil {
        return entry.Response, nil // Cache hit
    }

    resp, err := c.inner.Chat(ctx, system, user)
    if err != nil {
        return "", err
    }

    c.cache.Set(ctx, key, &LLMCacheEntry{
        Response: resp,
        TTL:      1 * time.Hour,
    })

    return resp, nil
}
```

### 3.2 Session Context Cache

```go
type SessionContext struct {
    SessionID    string
    TenantID     string

    // Conversation context for LLM
    LastQuery    string
    LastIntent   string
    ShownRefs    []EntityRef // What user has seen

    // For continuity
    ConversationSummary string // Compressed history for context

    TTL          time.Duration // 30 min sliding
}
```

### 3.3 Formation Cache (for quick back)

```go
// Cached formations for navigation history
type FormationCache struct {
    SessionID  string
    ViewIndex  int // Position in ViewStack
    Formation  *FormationWithData
    CreatedAt  time.Time
    TTL        time.Duration // 10 min
}

// When going back, check cache first
func (uc *BackUseCase) Execute(ctx, req) error {
    // Try formation cache
    cached := formationCache.Get(sessionID, prevViewIndex)
    if cached != nil {
        // Fast path: use cached formation
        return sendFormation(cached.Formation)
    }

    // Slow path: reconstruct from refs
    formation := renderFromRefs(prevView.Refs)
    return sendFormation(formation)
}
```

### Files

| File | Action | Purpose |
|------|--------|---------|
| `domain/cache_entity.go` | Create | LLMCacheKey, SessionContext, FormationCache |
| `ports/llm_cache_port.go` | Create | LLMCachePort interface |
| `adapters/cached_llm.go` | Create | CachedLLMAdapter |
| `adapters/postgres_llm_cache.go` | Create | PostgreSQL LLM cache |
| `adapters/formation_cache.go` | Create | In-memory formation cache |

### Acceptance Criteria
- [ ] LLM cache с hash-based lookup
- [ ] Session context с sliding TTL
- [ ] Formation cache для быстрого back
- [ ] Cache hit rate > 30% (измеримо)

---

## Feature 4: Delta State Management

### Objective
Инкрементальные изменения вместо полной перезаписи.

### Delta Types

```go
type DeltaType string
const (
    DeltaAdd     DeltaType = "add"     // Add refs
    DeltaRemove  DeltaType = "remove"  // Remove refs
    DeltaUpdate  DeltaType = "update"  // Update view/meta
    DeltaPush    DeltaType = "push"    // Push to ViewStack
    DeltaPop     DeltaType = "pop"     // Pop from ViewStack
)

type Delta struct {
    ID        string
    SessionID string
    Type      DeltaType
    Path      string      // "refs.products", "view.mode", "viewStack"
    Value     interface{} // New value or ref
    PrevValue interface{} // For rollback
    Timestamp time.Time
}
```

### State Operations

```go
type StatePort interface {
    // Get reconstructed state
    GetState(ctx, sessionID string) (*SessionState, error)

    // Append delta (fast, no full rewrite)
    AppendDelta(ctx, sessionID string, delta Delta) error

    // Get deltas for replay/debug
    GetDeltas(ctx, sessionID string, since time.Time) ([]Delta, error)

    // Compact old deltas into base state
    CompactDeltas(ctx, sessionID string) error
}

// State reconstruction
func ReconstructState(base *SessionState, deltas []Delta) *SessionState {
    state := base.Clone()
    for _, d := range deltas {
        switch d.Type {
        case DeltaAdd:
            state.ApplyAdd(d.Path, d.Value)
        case DeltaRemove:
            state.ApplyRemove(d.Path, d.Value)
        case DeltaUpdate:
            state.ApplyUpdate(d.Path, d.Value)
        case DeltaPush:
            state.ApplyPush(d.Path, d.Value)
        case DeltaPop:
            state.ApplyPop(d.Path)
        }
    }
    return state
}
```

### Compaction

```go
// Compact when:
// - Delta count > 100
// - Oldest delta > 30 min
// - On session close

func (p *PostgresStateAdapter) CompactDeltas(ctx, sessionID string) error {
    state, deltas := p.GetStateAndDeltas(ctx, sessionID)

    // Reconstruct current state
    current := ReconstructState(state.Base, deltas)

    // Replace base, clear deltas
    return p.tx(func(tx) error {
        tx.UpdateBase(sessionID, current)
        tx.ClearDeltas(sessionID)
        return nil
    })
}
```

### Files

| File | Action | Purpose |
|------|--------|---------|
| `domain/delta_entity.go` | Modify | Add DeltaType, Path, structured deltas |
| `domain/state_entity.go` | Modify | Add reconstruction methods |
| `ports/state_port.go` | Modify | Add AppendDelta, GetDeltas, CompactDeltas |
| `adapters/postgres_state.go` | Modify | Implement delta operations |
| `usecases/state_compact.go` | Create | Compaction logic |

### Acceptance Criteria
- [ ] DeltaType enum (add/remove/update/push/pop)
- [ ] AppendDelta без полной перезаписи
- [ ] ReconstructState из base + deltas
- [ ] CompactDeltas для оптимизации
- [ ] Delta operations < 10ms

---

## Feature 5: Formation Modes Enhancement

### Objective
Прокачать grid/carousel/single/list до production quality.

### 5.1 Grid

```css
.formation-grid {
  display: grid;
  gap: var(--grid-gap, 16px);

  &.cols-2 {
    grid-template-columns: repeat(2, 1fr);
  }
  &.cols-3 {
    grid-template-columns: repeat(3, 1fr);
  }
  &.cols-auto {
    grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  }

  /* Responsive */
  @media (max-width: 768px) {
    &.cols-3 { grid-template-columns: repeat(2, 1fr); }
  }
  @media (max-width: 480px) {
    &.cols-2, &.cols-3 { grid-template-columns: 1fr; }
  }
}
```

### 5.2 Carousel (with controls)

```jsx
function CarouselFormation({ widgets }) {
    const [index, setIndex] = useState(0);
    const handlers = useSwipeable({
        onSwipedLeft: () => setIndex(i => Math.min(i + 1, widgets.length - 1)),
        onSwipedRight: () => setIndex(i => Math.max(i - 1, 0)),
    });

    return (
        <div className="formation-carousel" {...handlers}>
            <button className="carousel-prev" onClick={() => setIndex(i => i - 1)}>‹</button>
            <div className="carousel-track" style={{ transform: `translateX(-${index * 100}%)` }}>
                {widgets.map(w => <WidgetRenderer key={w.id} widget={w} />)}
            </div>
            <button className="carousel-next" onClick={() => setIndex(i => i + 1)}>›</button>
            <div className="carousel-dots">
                {widgets.map((_, i) => (
                    <button key={i} className={i === index ? 'active' : ''} onClick={() => setIndex(i)} />
                ))}
            </div>
        </div>
    );
}
```

### 5.3 Single (detail-ready)

```css
.formation-single {
  display: flex;
  justify-content: center;
  padding: 20px;

  .widget-renderer {
    max-width: 480px;
    width: 100%;
    animation: fade-scale-in 0.25s ease-out;
  }
}

@keyframes fade-scale-in {
  from { opacity: 0; transform: scale(0.96); }
  to { opacity: 1; transform: scale(1); }
}
```

### 5.4 List (compact/detailed variants)

```css
.formation-list {
  display: flex;
  flex-direction: column;
  gap: var(--list-gap, 12px);

  &.variant-compact {
    gap: 8px;
    .widget-renderer { padding: 8px 12px; }
  }

  &.variant-detailed {
    gap: 16px;
    .widget-renderer {
      padding: 16px;
      border: 1px solid var(--border-color, #e0e0e0);
      border-radius: 8px;
    }
  }
}
```

### Files

| File | Action | Purpose |
|------|--------|---------|
| `FormationRenderer.jsx` | Modify | Add carousel controls, variants |
| `Formation.css` | Rewrite | All improvements |
| `formationModel.js` | Modify | Add config (gap, variant) |

### Acceptance Criteria
- [ ] Grid responsive breakpoints
- [ ] Carousel: arrows, swipe, dots
- [ ] Single: animation on appear
- [ ] List: compact/detailed variants
- [ ] CSS custom properties

---

## Out of Scope (for now)

- **Freestyle mode** — пользователь описывает layout, LLM генерирует
- **Application entity** — страховки, кредиты (отдельный флоу)
- **Design systems** — сменные темы/стили
- **Tenant-specific presets** — кастомные пресеты на бизнес

---

## Validation Commands

```bash
cd project/backend && go build ./...
cd project/backend && go test ./...
cd project/frontend && npm run build
cd project/frontend && npm run lint
```

---

## Summary: What's Different Now

| Aspect | Before | After |
|--------|--------|-------|
| **State** | Data storage | Index + refs (посредник) |
| **Agents** | Monolithic | Stateless functions с tools |
| **Preset mode** | No LLM | Simple LLM выбирает пресеты |
| **Drill-down** | Not implemented | ViewStack + expand/back |
| **Validation** | None | Hooks after tool calls |
| **Formation** | Basic CSS | Responsive + controls + animations |

---

## Dependencies

```
Feature 1 (Presets) ←──┐
                       ├──→ Feature 2 (Drill-down)
Feature 3 (Cache) ←────┤
                       │
Feature 4 (Deltas) ←───┘

Feature 5 (Formations) ← независима
```

Рекомендуемый порядок:
1. Feature 1 (Presets) — фундамент
2. Feature 4 (Deltas) — нужен для drill-down
3. Feature 2 (Drill-down) — зависит от 1 и 4
4. Feature 3 (Cache) — оптимизация
5. Feature 5 (Formations) — параллельно с любой
