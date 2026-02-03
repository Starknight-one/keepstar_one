# Feature: Drill-down / Expand (k3m9x2p)

## Feature Description

Пользователь кликает на товар/услугу в grid/list view → переключается на detail view с полной информацией → может вернуться назад. Detail view использует preset-механизм: новые detail-пресеты определяют какие поля показывать и как, а новые detail-templates на фронте рендерят расширенный layout. Все действия записываются как дельты.

## Objective

- Клик на виджет → expand → detail view через preset system
- Новые пресеты: `product_detail`, `service_detail`
- Новые templates: `ProductDetailTemplate`, `ServiceDetailTemplate`
- Back button → возврат к предыдущему view
- Интеграция с delta state management (Feature 4)
- Navigation history через ViewStack

## Preset System Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│  PRESET defines:                                                        │
│  - Fields[] → какие поля сущности включать                             │
│  - Slot → куда класть (hero/title/primary/price/secondary/specs/...)   │
│  - AtomType → как рендерить (text/price/rating/image/badge/...)        │
│  - Template → какой компонент на фронте (ProductDetailTemplate)        │
│  - DefaultMode → layout (single для detail)                            │
│  - DefaultSize → размер (large для detail)                             │
├─────────────────────────────────────────────────────────────────────────┤
│  BuildFormation(preset, entities, idGetter):                            │
│    → Для каждой entity создаёт Widget с Atoms[] и EntityRef            │
│    → Atoms собираются из Fields через FieldGetter                      │
│    → EntityRef.ID берётся из idGetter для кликабельности               │
├─────────────────────────────────────────────────────────────────────────┤
│  Frontend Template:                                                     │
│    → groupAtomsBySlot(atoms) → рендер по слотам                        │
└─────────────────────────────────────────────────────────────────────────┘
```

## Relevant Files

### Existing Files (Backend)
- `project/backend/internal/ports/state_port.go` - StatePort interface (PushView, PopView, AddDelta ready)
- `project/backend/internal/adapters/postgres/postgres_state.go` - PostgreSQL implementation (PushView, PopView ready)
- `project/backend/internal/domain/state_entity.go` - ViewMode, ViewSnapshot, EntityRef, DeltaType ready
- `project/backend/internal/domain/preset_entity.go` - Preset, FieldConfig, PresetName (add new presets)
- `project/backend/internal/domain/widget_entity.go` - Widget struct (add EntityRef field)
- `project/backend/internal/presets/preset_registry.go` - PresetRegistry (register new presets)
- `project/backend/internal/presets/product_presets.go` - Add ProductDetailPreset
- `project/backend/internal/presets/service_presets.go` - Add ServiceDetailPreset
- `project/backend/internal/tools/tool_render_preset.go` - Export BuildFormation, extend FieldGetters, add EntityRef
- `project/backend/internal/handlers/routes.go` - Add new navigation routes

### Existing Files (Frontend)
- `project/frontend/src/shared/api/apiClient.js` - Add expand/back API calls
- `project/frontend/src/features/chat/ChatPanel.jsx` - Add navigation (sessionId уже здесь)
- `project/frontend/src/features/chat/useChatSubmit.js` - Extend for navigation callbacks
- `project/frontend/src/App.jsx` - Pass onExpand/onBack callbacks, show BackButton
- `project/frontend/src/entities/formation/FormationRenderer.jsx` - Pass click handler to widgets
- `project/frontend/src/entities/widget/WidgetRenderer.jsx` - Add onClick prop, register new templates
- `project/frontend/src/entities/widget/templates/index.js` - Export new templates

### New Files (Backend)
- `project/backend/internal/usecases/navigation_expand.go` - ExpandUseCase
- `project/backend/internal/usecases/navigation_back.go` - BackUseCase
- `project/backend/internal/handlers/handler_navigation.go` - NavigationHandler

### New Files (Frontend)
- `project/frontend/src/entities/widget/templates/ProductDetailTemplate.jsx` - Detail product view
- `project/frontend/src/entities/widget/templates/ProductDetailTemplate.css` - Styles
- `project/frontend/src/entities/widget/templates/ServiceDetailTemplate.jsx` - Detail service view
- `project/frontend/src/entities/widget/templates/ServiceDetailTemplate.css` - Styles
- `project/frontend/src/features/navigation/BackButton.jsx` - Back button component
- `project/frontend/src/features/navigation/BackButton.css` - Styles

## Step by Step Tasks

IMPORTANT: Execute strictly in order.

### 1. Backend: Add Detail Presets to Domain

Update `project/backend/internal/domain/preset_entity.go`:
- Add new PresetName constants:
```go
PresetProductDetail PresetName = "product_detail"
PresetServiceDetail PresetName = "service_detail"
```

- Add new AtomSlot for detail view:
```go
AtomSlotSpecs       AtomSlot = "specs"       // specifications table
AtomSlotGallery     AtomSlot = "gallery"     // full gallery (not just hero)
AtomSlotStock       AtomSlot = "stock"       // availability indicator
AtomSlotTags        AtomSlot = "tags"        // tags chips
AtomSlotDescription AtomSlot = "description" // full description block
```

### 2. Backend: Add EntityRef to Widget

Update `project/backend/internal/domain/widget_entity.go`:
```go
type Widget struct {
    ID        string                 `json:"id"`
    Type      WidgetType             `json:"type,omitempty"`
    Template  string                 `json:"template,omitempty"`
    Size      WidgetSize             `json:"size,omitempty"`
    Priority  int                    `json:"priority,omitempty"`
    Atoms     []Atom                 `json:"atoms"`
    Children  []Widget               `json:"children,omitempty"`
    Meta      map[string]interface{} `json:"meta,omitempty"`
    EntityRef *EntityRef             `json:"entityRef,omitempty"` // NEW: for click handling
}
```

### 3. Backend: Create Product Detail Preset

Update `project/backend/internal/presets/product_presets.go`:

```go
var ProductDetailPreset = domain.Preset{
    Name:        string(domain.PresetProductDetail),
    EntityType:  domain.EntityTypeProduct,
    Template:    "ProductDetail",
    DefaultMode: domain.FormationTypeSingle,
    DefaultSize: domain.WidgetSizeLarge,
    Fields: []domain.FieldConfig{
        {Name: "images", Slot: domain.AtomSlotGallery, AtomType: domain.AtomTypeImage, Priority: 1, Required: true},
        {Name: "name", Slot: domain.AtomSlotTitle, AtomType: domain.AtomTypeText, Priority: 2, Required: true},
        {Name: "brand", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Priority: 3, Required: false},
        {Name: "category", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Priority: 4, Required: false},
        {Name: "rating", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeRating, Priority: 5, Required: false},
        {Name: "price", Slot: domain.AtomSlotPrice, AtomType: domain.AtomTypePrice, Priority: 6, Required: true},
        {Name: "stockQuantity", Slot: domain.AtomSlotStock, AtomType: domain.AtomTypeNumber, Priority: 7, Required: false},
        {Name: "description", Slot: domain.AtomSlotDescription, AtomType: domain.AtomTypeText, Priority: 8, Required: false},
        {Name: "tags", Slot: domain.AtomSlotTags, AtomType: domain.AtomTypeText, Priority: 9, Required: false},
        {Name: "attributes", Slot: domain.AtomSlotSpecs, AtomType: domain.AtomTypeText, Priority: 10, Required: false},
    },
}
```

Register in `preset_registry.go`:
```go
r.Register(ProductDetailPreset)
```

### 4. Backend: Create Service Detail Preset

Update `project/backend/internal/presets/service_presets.go`:

```go
var ServiceDetailPreset = domain.Preset{
    Name:        string(domain.PresetServiceDetail),
    EntityType:  domain.EntityTypeService,
    Template:    "ServiceDetail",
    DefaultMode: domain.FormationTypeSingle,
    DefaultSize: domain.WidgetSizeLarge,
    Fields: []domain.FieldConfig{
        {Name: "images", Slot: domain.AtomSlotGallery, AtomType: domain.AtomTypeImage, Priority: 1, Required: false},
        {Name: "name", Slot: domain.AtomSlotTitle, AtomType: domain.AtomTypeText, Priority: 2, Required: true},
        {Name: "provider", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Priority: 3, Required: false},
        {Name: "duration", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeText, Priority: 4, Required: false},
        {Name: "rating", Slot: domain.AtomSlotPrimary, AtomType: domain.AtomTypeRating, Priority: 5, Required: false},
        {Name: "price", Slot: domain.AtomSlotPrice, AtomType: domain.AtomTypePrice, Priority: 6, Required: true},
        {Name: "availability", Slot: domain.AtomSlotStock, AtomType: domain.AtomTypeText, Priority: 7, Required: false},
        {Name: "description", Slot: domain.AtomSlotDescription, AtomType: domain.AtomTypeText, Priority: 8, Required: false},
        {Name: "attributes", Slot: domain.AtomSlotSpecs, AtomType: domain.AtomTypeText, Priority: 9, Required: false},
    },
}
```

Register in `preset_registry.go`:
```go
r.Register(ServiceDetailPreset)
```

### 5. Backend: Export BuildFormation and Add EntityRef Support

Update `project/backend/internal/tools/tool_render_preset.go`:

**5a. Add IDGetter type and update signature (export function):**
```go
// IDGetter extracts entity ID
type IDGetter func() string

// EntityGetterFunc now returns IDGetter too
type EntityGetterFunc func(i int) (FieldGetter, CurrencyGetter, IDGetter)

// BuildFormation creates formation from preset and entities (EXPORTED)
func BuildFormation(preset domain.Preset, count int, getEntity EntityGetterFunc) *domain.FormationWithData {
    widgets := make([]domain.Widget, 0, count)
    // ... sort fields ...

    for i := 0; i < count; i++ {
        fieldGetter, currencyGetter, idGetter := getEntity(i)
        atoms := buildAtoms(fields, fieldGetter, currencyGetter)
        widget := domain.Widget{
            ID:       uuid.New().String(),
            Template: preset.Template,
            Size:     preset.DefaultSize,
            Priority: i,
            Atoms:    atoms,
            EntityRef: &domain.EntityRef{
                Type: preset.EntityType,
                ID:   idGetter(),  // Get entity ID for click handling
            },
        }
        widgets = append(widgets, widget)
    }
    // ...
}
```

**5b. Update existing callers to provide IDGetter:**
```go
// In RenderProductPresetTool.Execute:
formation := BuildFormation(preset, len(products), func(i int) (FieldGetter, CurrencyGetter, IDGetter) {
    p := products[i]
    return productFieldGetter(p), func() string { return p.Currency }, func() string { return p.ID }
})

// In RenderServicePresetTool.Execute:
formation := BuildFormation(preset, len(services), func(i int) (FieldGetter, CurrencyGetter, IDGetter) {
    s := services[i]
    return serviceFieldGetter(s), func() string { return s.Currency }, func() string { return s.ID }
})
```

**5c. Extend FieldGetters:**
```go
// productFieldGetter - add:
case "stockQuantity":
    if p.StockQuantity == 0 {
        return nil
    }
    return p.StockQuantity
case "tags":
    if len(p.Tags) == 0 {
        return nil
    }
    return p.Tags
case "attributes":
    if len(p.Attributes) == 0 {
        return nil
    }
    return p.Attributes

// serviceFieldGetter - add:
case "availability":
    return nonEmpty(s.Availability)
case "attributes":
    if len(s.Attributes) == 0 {
        return nil
    }
    return s.Attributes
```

**5d. Update tool Definition enums:**
```go
// RenderProductPresetTool.Definition():
"enum": []string{"product_grid", "product_card", "product_compact", "product_detail"},

// RenderServicePresetTool.Definition():
"enum": []string{"service_card", "service_list", "service_detail"},
```

### 6. Backend: ExpandUseCase

Create `project/backend/internal/usecases/navigation_expand.go`:

```go
type ExpandRequest struct {
    SessionID  string
    EntityType domain.EntityType
    EntityID   string
}

type ExpandResponse struct {
    Success   bool
    Formation *domain.FormationWithData
    ViewMode  domain.ViewMode
    Focused   *domain.EntityRef
    StackSize int
}

type ExpandUseCase struct {
    statePort      ports.StatePort
    presetRegistry *presets.PresetRegistry
}
```

**Execute logic:**
```go
func (uc *ExpandUseCase) Execute(ctx context.Context, req ExpandRequest) (*ExpandResponse, error) {
    // 1. Get current state
    state, err := uc.statePort.GetState(ctx, req.SessionID)
    if err != nil {
        return nil, err
    }

    // 2. Find entity by ID
    var entity interface{}
    var preset domain.Preset
    if req.EntityType == domain.EntityTypeProduct {
        for _, p := range state.Current.Data.Products {
            if p.ID == req.EntityID {
                entity = p
                break
            }
        }
        preset, _ = uc.presetRegistry.Get(domain.PresetProductDetail)
    } else {
        for _, s := range state.Current.Data.Services {
            if s.ID == req.EntityID {
                entity = s
                break
            }
        }
        preset, _ = uc.presetRegistry.Get(domain.PresetServiceDetail)
    }
    if entity == nil {
        return nil, fmt.Errorf("entity not found: %s", req.EntityID)
    }

    // 3. Build refs from current data for snapshot
    refs := buildEntityRefs(state.Current.Data)

    // 4. Push current view to stack
    snapshot := &domain.ViewSnapshot{
        Mode:      state.View.Mode,
        Focused:   state.View.Focused,
        Refs:      refs,
        Step:      state.Step,
        CreatedAt: time.Now(),
    }
    uc.statePort.PushView(ctx, req.SessionID, snapshot)

    // 5. Create and save delta (IMPORTANT: increment step!)
    state.Step++
    delta := &domain.Delta{
        Step:      state.Step,
        Trigger:   domain.TriggerWidgetAction,
        Source:    domain.SourceUser,
        ActorID:   "user_expand",
        DeltaType: domain.DeltaTypePush,
        Path:      "viewStack",
        CreatedAt: time.Now(),
    }
    uc.statePort.AddDelta(ctx, req.SessionID, delta)

    // 6. Build detail formation
    formation := buildDetailFormation(preset, entity, req.EntityType)

    // 7. Update state
    state.Current.Template = map[string]interface{}{"formation": formation}
    state.View.Mode = domain.ViewModeDetail
    state.View.Focused = &domain.EntityRef{Type: req.EntityType, ID: req.EntityID}
    uc.statePort.UpdateState(ctx, state)

    // 8. Get stack size
    stack, _ := uc.statePort.GetViewStack(ctx, req.SessionID)

    return &ExpandResponse{
        Success:   true,
        Formation: formation,
        ViewMode:  state.View.Mode,
        Focused:   state.View.Focused,
        StackSize: len(stack),
    }, nil
}
```

### 7. Backend: BackUseCase

Create `project/backend/internal/usecases/navigation_back.go`:

```go
type BackRequest struct {
    SessionID string
}

type BackResponse struct {
    Success   bool
    Formation *domain.FormationWithData
    ViewMode  domain.ViewMode
    Focused   *domain.EntityRef
    StackSize int
    CanGoBack bool
}

func (uc *BackUseCase) Execute(ctx context.Context, req BackRequest) (*BackResponse, error) {
    // 1. Pop from stack
    snapshot, err := uc.statePort.PopView(ctx, req.SessionID)
    if err != nil {
        return nil, err
    }
    if snapshot == nil {
        return &BackResponse{Success: true, CanGoBack: false}, nil
    }

    // 2. Get current state
    state, _ := uc.statePort.GetState(ctx, req.SessionID)

    // 3. Create and save delta (IMPORTANT: increment step!)
    state.Step++
    delta := &domain.Delta{
        Step:      state.Step,
        Trigger:   domain.TriggerWidgetAction,
        Source:    domain.SourceUser,
        ActorID:   "user_back",
        DeltaType: domain.DeltaTypePop,
        Path:      "viewStack",
        CreatedAt: time.Now(),
    }
    uc.statePort.AddDelta(ctx, req.SessionID, delta)

    // 4. Rebuild formation from state data using grid preset
    formation := rebuildFormationFromState(state, uc.presetRegistry)

    // 5. Restore view
    state.Current.Template = map[string]interface{}{"formation": formation}
    state.View.Mode = snapshot.Mode
    state.View.Focused = snapshot.Focused
    uc.statePort.UpdateState(ctx, state)

    // 6. Get remaining stack size
    stack, _ := uc.statePort.GetViewStack(ctx, req.SessionID)

    return &BackResponse{
        Success:   true,
        Formation: formation,
        ViewMode:  state.View.Mode,
        Focused:   state.View.Focused,
        StackSize: len(stack),
        CanGoBack: len(stack) > 0,
    }, nil
}
```

### 8. Backend: NavigationHandler

Create `project/backend/internal/handlers/handler_navigation.go`:

```go
type NavigationHandler struct {
    expandUC *usecases.ExpandUseCase
    backUC   *usecases.BackUseCase
}

func NewNavigationHandler(expandUC *usecases.ExpandUseCase, backUC *usecases.BackUseCase) *NavigationHandler {
    return &NavigationHandler{expandUC: expandUC, backUC: backUC}
}

// POST /api/v1/navigation/expand
// Request: { sessionId, entityType, entityId }
func (h *NavigationHandler) HandleExpand(w http.ResponseWriter, r *http.Request) {
    // Parse request, call expandUC.Execute, return JSON
}

// POST /api/v1/navigation/back
// Request: { sessionId }
func (h *NavigationHandler) HandleBack(w http.ResponseWriter, r *http.Request) {
    // Parse request, call backUC.Execute, return JSON
}
```

### 9. Backend: Register Routes and Wire Up

Update `project/backend/internal/handlers/routes.go`:
```go
func SetupNavigationRoutes(mux *http.ServeMux, nav *NavigationHandler) {
    mux.HandleFunc("/api/v1/navigation/expand", nav.HandleExpand)
    mux.HandleFunc("/api/v1/navigation/back", nav.HandleBack)
}
```

Update `project/backend/cmd/server/main.go`:
```go
// After presetRegistry initialization:
var navigationHandler *handlers.NavigationHandler
if stateAdapter != nil && presetRegistry != nil {
    expandUC := usecases.NewExpandUseCase(stateAdapter, presetRegistry)
    backUC := usecases.NewBackUseCase(stateAdapter, presetRegistry)
    navigationHandler = handlers.NewNavigationHandler(expandUC, backUC)
    handlers.SetupNavigationRoutes(mux, navigationHandler)
    appLog.Info("navigation_routes_enabled", "status", "ok")
}
```

### 10. Frontend: ProductDetailTemplate

Create `project/frontend/src/entities/widget/templates/ProductDetailTemplate.jsx`:

```jsx
import { useState } from 'react';
import './ProductDetailTemplate.css';

const SLOTS = {
  GALLERY: 'gallery',
  TITLE: 'title',
  PRIMARY: 'primary',
  PRICE: 'price',
  STOCK: 'stock',
  DESCRIPTION: 'description',
  TAGS: 'tags',
  SPECS: 'specs',
};

export function ProductDetailTemplate({ atoms = [] }) {
  const slots = groupAtomsBySlot(atoms);

  return (
    <div className="product-detail-template">
      <div className="product-detail-layout">
        <div className="product-detail-gallery">
          <ImageGallery atoms={slots[SLOTS.GALLERY]} />
        </div>
        <div className="product-detail-info">
          <h1 className="product-detail-title">
            {slots[SLOTS.TITLE]?.[0]?.value}
          </h1>
          <div className="product-detail-primary">
            {slots[SLOTS.PRIMARY]?.map((atom, i) => (
              <span key={i} className="detail-chip">{atom.value}</span>
            ))}
          </div>
          <div className="product-detail-price">
            {slots[SLOTS.PRICE]?.[0]?.meta?.currency}{slots[SLOTS.PRICE]?.[0]?.value}
          </div>
          <StockIndicator atoms={slots[SLOTS.STOCK]} />
          <div className="product-detail-description">
            {slots[SLOTS.DESCRIPTION]?.[0]?.value}
          </div>
          <TagsList atoms={slots[SLOTS.TAGS]} />
          <SpecsTable atoms={slots[SLOTS.SPECS]} />
        </div>
      </div>
    </div>
  );
}

function groupAtomsBySlot(atoms) { /* same as ProductCardTemplate */ }
function ImageGallery({ atoms }) { /* gallery with thumbnails */ }
function StockIndicator({ atoms }) { /* "In stock: N" or "Out of stock" */ }
function TagsList({ atoms }) { /* tags as chips */ }
function SpecsTable({ atoms }) { /* key-value table for attributes */ }
```

Create `project/frontend/src/entities/widget/templates/ProductDetailTemplate.css`:
- Two-column layout (gallery 50%, info 50%)
- Responsive: stack on mobile (`@media max-width: 768px`)
- Gallery with main image + thumbnails

### 11. Frontend: ServiceDetailTemplate

Create similar to ProductDetailTemplate with service-specific styling.

### 12. Frontend: Register Templates

Update `project/frontend/src/entities/widget/templates/index.js`:
```js
export { ProductCardTemplate } from './ProductCardTemplate';
export { ServiceCardTemplate } from './ServiceCardTemplate';
export { ProductDetailTemplate } from './ProductDetailTemplate';
export { ServiceDetailTemplate } from './ServiceDetailTemplate';
```

Update `project/frontend/src/entities/widget/WidgetRenderer.jsx`:
```jsx
import { ProductDetailTemplate, ServiceDetailTemplate } from './templates';

function renderTemplate(widget) {
  switch (widget.template) {
    case 'ProductCard':
      return <ProductCardTemplate atoms={widget.atoms} size={widget.size} />;
    case 'ServiceCard':
      return <ServiceCardTemplate atoms={widget.atoms} size={widget.size} />;
    case 'ProductDetail':
      return <ProductDetailTemplate atoms={widget.atoms} />;
    case 'ServiceDetail':
      return <ServiceDetailTemplate atoms={widget.atoms} />;
    default:
      return <DefaultWidget widget={widget} />;
  }
}
```

### 13. Frontend: API Client

Update `project/frontend/src/shared/api/apiClient.js`:

```js
export async function expandView(sessionId, entityType, entityId) {
  const response = await fetch(`${API_BASE_URL}/navigation/expand`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ sessionId, entityType, entityId }),
  });
  if (!response.ok) throw new Error(`API error: ${response.status}`);
  return response.json();
}

export async function goBack(sessionId) {
  const response = await fetch(`${API_BASE_URL}/navigation/back`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ sessionId }),
  });
  if (!response.ok) throw new Error(`API error: ${response.status}`);
  return response.json();
}
```

### 14. Frontend: BackButton Component

Create `project/frontend/src/features/navigation/BackButton.jsx`:
```jsx
import './BackButton.css';

export function BackButton({ onClick, visible }) {
  if (!visible) return null;
  return (
    <button className="back-button" onClick={onClick}>
      ← Назад
    </button>
  );
}
```

Create `project/frontend/src/features/navigation/BackButton.css`

### 15. Frontend: Wire Up Clicks in WidgetRenderer

Update `project/frontend/src/entities/widget/WidgetRenderer.jsx`:
```jsx
export function WidgetRenderer({ widget, onClick }) {
  const handleClick = () => {
    if (onClick && widget.entityRef) {
      onClick(widget.entityRef.type, widget.entityRef.id);
    }
  };

  const content = widget.template ? renderTemplate(widget) : renderLegacy(widget);

  if (onClick && widget.entityRef) {
    return (
      <div className="widget-clickable" onClick={handleClick}>
        {content}
      </div>
    );
  }

  return content;
}
```

### 16. Frontend: Wire Up Clicks in FormationRenderer

Update `project/frontend/src/entities/formation/FormationRenderer.jsx`:
```jsx
export function FormationRenderer({ formation, onWidgetClick }) {
  if (!formation || !formation.widgets?.length) {
    return null;
  }

  const { mode, grid, widgets } = formation;
  const layoutClass = getLayoutClass(mode, grid?.cols || 2);

  return (
    <div className={layoutClass}>
      {widgets.map((widget) => (
        <WidgetRenderer
          key={widget.id}
          widget={widget}
          onClick={onWidgetClick}
        />
      ))}
    </div>
  );
}
```

### 17. Frontend: Integrate Navigation in ChatPanel

Update `project/frontend/src/features/chat/useChatSubmit.js`:
- Add `onExpand` and `onBack` to params
- Export navigation functions that use sessionId

Update `project/frontend/src/features/chat/ChatPanel.jsx`:
```jsx
export function ChatPanel({ onClose, onFormationReceived, onNavigationStateChange, hideFormation }) {
  const { sessionId, /* ... */ } = useChatMessages();

  const [canGoBack, setCanGoBack] = useState(false);

  const handleExpand = async (entityType, entityId) => {
    const result = await expandView(sessionId, entityType, entityId);
    onFormationReceived?.(result.formation);
    setCanGoBack(result.stackSize > 0);
    onNavigationStateChange?.({ canGoBack: result.stackSize > 0 });
  };

  const handleBack = async () => {
    const result = await goBack(sessionId);
    onFormationReceived?.(result.formation);
    setCanGoBack(result.canGoBack);
    onNavigationStateChange?.({ canGoBack: result.canGoBack });
  };

  // Pass to parent via callback
  useEffect(() => {
    onNavigationStateChange?.({
      canGoBack,
      onExpand: handleExpand,
      onBack: handleBack
    });
  }, [canGoBack, sessionId]);

  // ... rest
}
```

### 18. Frontend: Update App.jsx

Update `project/frontend/src/App.jsx`:
```jsx
import { BackButton } from './features/navigation/BackButton';

function App() {
  const [isChatOpen, setIsChatOpen] = useState(false);
  const [activeFormation, setActiveFormation] = useState(null);
  const [navState, setNavState] = useState({ canGoBack: false, onExpand: null, onBack: null });

  const handleNavigationStateChange = (state) => {
    setNavState(prev => ({ ...prev, ...state }));
  };

  return (
    <div className="app">
      {/* ... */}
      {isChatOpen && (
        <>
          <div className="chat-backdrop" onClick={handleChatClose} />
          <div className="chat-overlay-layout">
            <div className="widget-display-area">
              <BackButton
                visible={navState.canGoBack}
                onClick={navState.onBack}
              />
              {activeFormation && (
                <FormationRenderer
                  formation={activeFormation}
                  onWidgetClick={navState.onExpand}
                />
              )}
            </div>
            <div className="chat-area">
              <ChatPanel
                onClose={handleChatClose}
                onFormationReceived={setActiveFormation}
                onNavigationStateChange={handleNavigationStateChange}
                hideFormation={true}
              />
            </div>
          </div>
        </>
      )}
    </div>
  );
}
```

### 19. Validation

```bash
cd project/backend && go build ./...
cd project/backend && go test ./...
cd project/frontend && npm run build
cd project/frontend && npm run lint
```

## Validation Commands

```bash
cd project/backend && go build ./...
cd project/backend && go test ./...
cd project/frontend && npm run build
cd project/frontend && npm run lint
```

## Acceptance Criteria

### Preset System
- [ ] PresetProductDetail и PresetServiceDetail добавлены
- [ ] Новые AtomSlot: gallery, stock, description, tags, specs
- [ ] FieldGetters расширены для stockQuantity, tags, attributes, availability
- [ ] BuildFormation экспортирован и добавляет EntityRef к виджетам

### Backend Navigation
- [ ] Widget.EntityRef содержит type и id сущности
- [ ] ExpandUseCase: push в ViewStack, delta с step++, formation с detail preset
- [ ] BackUseCase: pop из ViewStack, delta с step++, formation с grid preset
- [ ] NavigationHandler: POST /expand, POST /back работают

### Frontend Templates
- [ ] ProductDetailTemplate: two-column layout, все слоты
- [ ] ServiceDetailTemplate: service-specific layout
- [ ] Templates зарегистрированы в WidgetRenderer

### Frontend Navigation
- [ ] Клик на виджет (с entityRef) вызывает expand
- [ ] BackButton показывается когда canGoBack=true
- [ ] Formation обновляется после expand/back
- [ ] sessionId остаётся в ChatPanel (не поднимается)

## Notes

**Delta Step — ВАЖНО:**
- При создании дельты ОБЯЗАТЕЛЬНО: `state.Step++` перед `delta.Step = state.Step`
- AddDelta не инкрементирует step автоматически

**sessionId остаётся в ChatPanel:**
- Не поднимаем sessionId в App.jsx
- ChatPanel передаёт функции `onExpand`/`onBack` через callback `onNavigationStateChange`
- App.jsx вызывает эти функции при кликах

**Сессия гарантированно существует:**
- Навигация возможна только когда виджеты отображены
- Виджеты появляются после pipeline запроса → state уже существует
- Нет необходимости проверять существование сессии

**EntityRef в Widget:**
- Бэкенд добавляет `entityRef: {type, id}` к каждому виджету
- Фронтенд использует для определения что expand'ить при клике
- Без entityRef виджет не кликабельный
