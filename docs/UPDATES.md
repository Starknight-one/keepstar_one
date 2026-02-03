# Updates

Лог изменений проекта Keepstar.

---

## 2026-02-03 15:30

### Entity Types and Preset System
- EntityType enum (product, service) for multi-entity support
- Service entity parallel to Product (duration, provider, availability)
- Preset system: FieldConfig → Slot → AtomType mapping
- PresetRegistry with 5 presets: product_grid, product_card, product_compact, service_card, service_list
- RenderProductPresetTool and RenderServicePresetTool for LLM
- ServiceCardTemplate.jsx with duration/provider chip display
- StateData extended with Services field
- StateMeta extended with ProductCount, ServiceCount

---

## 2026-02-03

### Chat Overlay with External Widget Rendering
- Backdrop overlay dims screen when chat open
- Chat positioned on the right side
- Widgets (Formation) render externally on the left
- Animations: backdrop-fade-in, chat-slide-in, widget-fade-in
- onFormationReceived callback from useChatSubmit to App
- hideFormation prop to prevent duplicate rendering in chat

### Universal ProductCardTemplate with Slot-based Atoms
- Template-based widget rendering (template field instead of type)
- Atom.Slot field for layout hints (hero, badge, title, primary, price, secondary)
- ProductCardTemplate.jsx groups atoms by slot
- ImageCarousel with navigation dots
- AtomChip renders text/rating/selector displays
- Expandable secondary attributes
- Auto-fill responsive grid layout

### Backend Template System
- AtomSlot enum: hero, badge, title, primary, price, secondary
- Widget.Template field for template name (ProductCard)
- applyWidgetTemplate generates atoms with slot hints
- Agent2 prompt prefers grid layout for 2-6 items

---

## 2026-02-02

### Two-Agent Pipeline - Frontend Rendering (Phase 4)
- FormationRenderer с режимами grid/carousel/single/list
- AtomRenderer со стилями для всех типов (text, number, price, image, rating, badge, button, icon, divider, progress)
- WidgetRenderer с размерами (tiny/small/medium/large)
- MessageBubble с поддержкой Formation (backward compatible)
- Pipeline API: `POST /api/v1/pipeline`
- Debug Console: `/debug/session/` с детальными метриками
- Метрики: время LLM/tool, токены in/out, стоимость USD, промпты, responses

### Two-Agent Pipeline - Template Builder (Phase 3)
- Agent2ExecuteUseCase: meta → LLM → FormationTemplate
- Agent2SystemPrompt с правилами выбора mode/size
- BuildAgent2Prompt() для генерации промпта из StateMeta
- PipelineExecuteUseCase: Agent 1 → Agent 2 → ApplyTemplate
- ApplyTemplate: шаблон + products → FormationWithData с widgets

### Two-Agent Pipeline - Tool Caller (Phase 2)
- Agent1ExecuteUseCase: query → LLM → tool call → state update
- Agent1SystemPrompt с правилами tool calling
- Tool Registry с search_products tool
- ChatWithTools в LLMPort для tool calling
- Delta creation и сохранение в state

### Two-Agent Pipeline - State Storage (Phase 1)
- StatePort interface для session state
- PostgreSQL adapter с JSONB storage
- Domain entities: SessionState, Delta, StateMeta, StateData
- Миграции для chat_session_state, chat_session_deltas
- Delta-based state management

### Multi-tenant Product Catalog
- Добавлены domain entities: Tenant, Category, MasterProduct
- Расширен Product с tenantId, masterProductId, priceFormatted
- Создан CatalogPort interface для операций с каталогом
- Реализован PostgreSQL adapter с merging master/tenant данных
- Добавлены миграции для catalog schema (tenants, categories, master_products, products)
- Seed data: Nike, Sportmaster + 8 кроссовок
- Use cases: ListProducts, GetProduct
- HTTP handlers с TenantMiddleware для резолва slug
- Frontend: getProducts(), getProduct() в apiClient
- Frontend: ProductGrid компонент с WidgetRenderer

### API Endpoints
```
GET /api/v1/tenants/{slug}/products
GET /api/v1/tenants/{slug}/products/{id}
```

---

## 2026-02-01

### Neon PostgreSQL Integration
- Добавлен PostgreSQL adapter (pgxpool)
- Реализован CachePort для сессий и сообщений
- Реализован EventPort для аналитики
- Auto-migrations для chat таблиц (users, sessions, messages, events)
- Session TTL 10 минут (sliding window)
- Graceful degradation если DATABASE_URL не задан

### Chat Hexagonal Migration
- Перенесён chat на hexagonal architecture
- SendMessageUseCase с persistence
- ChatHandler, SessionHandler
- Frontend: session persistence в localStorage
- Frontend: восстановление истории при загрузке

---

## 2025-01-29

### Архитектура
- Создана hexagonal структура backend (internal/domain, ports, adapters, usecases, handlers, prompts)
- Создана feature-sliced структура frontend (shared, entities, features, app)
- Старый рабочий код сохранён (main.go, App.jsx, Chat.jsx)
- Новая структура в stubs, готова к миграции

### Expert System
- Заполнены expertise.yaml для backend и frontend
- Обновлены self-improve.md с валидацией YAML и лимитами строк
- Обновлены question.md с примерами и контекстом
- Добавлен README для expert system (ACT → LEARN → REUSE)

### Инструменты
- Добавлен dev-inspector для отладки UI элементов

### Документация
- Создан драфт Product Manifesto (AI_docs/Manifesto)

---
