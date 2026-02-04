# Updates

Лог изменений проекта Keepstar.

---

## 2026-02-04 22:00

### Documentation Sync
- Expert sync: 8 expertise.yaml files updated to match codebase
  - backend-domain: added DeltaInfo, TurnID, SentAt/ReceivedAt/Timestamp, CreatedAt/UpdatedAt fields
  - backend-ports: added 4 zone-write methods + AppendConversation to StatePort
  - backend-adapters: added turn_id column, test files
  - backend-usecases: TurnID in requests, zone-writes in navigation, AppendConversation, fixed SessionTTL
  - backend-handlers: added /debug/seed, SetupNavigationRoutes(), HealthHandler struct
  - backend-pipeline: Agent2 dual prompts, PresetRegistry methods, BuildFormation exports
  - frontend-entities: WidgetSize, full legacy types, FormationType duplicate note
  - frontend-features: navState + BackButton in App.jsx
- README sync: 10 backend/frontend READMEs updated
  - domain: DeltaInfo, TurnID, Message timestamps, Session timestamps
  - ports: zone-write methods (UpdateData/UpdateTemplate/UpdateView/AppendConversation)
  - adapters/postgres: test files, turn_id column, conversation_history
  - usecases: zone-writes in navigation, TurnID in requests, imports fix, SessionTTL 5min
  - handlers: /debug/seed, SetupNavigationRoutes(), HealthHandler
  - prompts: Agent2 dual prompts (text + tool), mode rule update
  - presets: GetByEntityType(), List() methods
  - tools: BuildFormation export, getter function types
  - frontend/widget: enums section (WidgetType, WidgetTemplate, FormationType, WidgetSize)
  - frontend/formation: onWidgetClick prop
  - frontend/features: App.jsx navState integration

---

## 2026-02-04 20:00

### Zone-based State Management (ADW-z8v4q1w)
- `DeltaInfo` struct + `Delta.TurnID` — дельты группируются по Turn'ам
- 4 zone-write метода в StatePort: `UpdateData`, `UpdateTemplate`, `UpdateView`, `AppendConversation`
- Postgres adapter: zone-write реализация (UPDATE зоны + INSERT delta), `zoneWriteWithDelta` helper
- `turn_id` колонка в `chat_session_deltas` (миграция + AddDelta + scanDeltas)
- Pipeline генерирует TurnID, передаёт в Agent1/Agent2
- Agent1: дельта через `DeltaInfo.ToDelta()`, conversation через `AppendConversation` (не UpdateState)
- Agent2: создаёт дельту на render path и empty path через `AddDelta`
- Expand/Back: zone-write (`UpdateView` + `UpdateTemplate`) вместо `AddDelta` + `UpdateState`
- Navigation handler: генерирует TurnID для Expand/Back
- Fix: search_products при total==0 очищает stale data, сохраняет Aliases
- `UpdateState` остаётся только в: rollback (легитимный blob), tools (промежуточно), debug seed
- Тесты: 3 unit (domain), 6 usecase (mock), 6 integration (Postgres) — все PASS

**E2E тесты с LLM не проводились** — полный pipeline flow (Agent1 → tool → Agent2 → render) не тестировался. Возможны баги на стыке LLM ↔ zone-write. Требуется smoke test через `/api/v1/pipeline` + проверка дельт в `/debug/session/{id}`.

**TODO**: полное покрытие тестами кодовой базы — нужна стратегия тестирования (моки для LLMPort, contract tests для API, regression suite).

---

## 2026-02-04 17:30

### Activate Prompt Caching — Phase 2 (ADW-r4w8n3k)
- `conversation_history JSONB` column in `chat_session_state` (migration + CreateState/GetState/UpdateState)
- Padding tools expanded: 8 → 10 tools (~3200 → ~4000 tokens), safely above 4096 threshold for Haiku
- Confirmed: Go `encoding/json.Marshal` sorts map keys deterministically — no cache instability
- Confirmed: Prompt caching is GA (Dec 2024), no beta header needed
- `cache_test.go`: upgraded WARNING to `t.Error` for zero cache hits
- Expertise synced: backend-adapters, backend-pipeline, backend-domain, backend-handlers, frontend-shared, frontend-features

---

## 2026-02-04 15:00

### Anthropic Prompt Caching — Phase 1 (ADW-k7x9m2p)
- `ChatWithToolsCached` method in Anthropic adapter with cache_control on tools, system, conversation
- Cache types: `cache_types.go` (request/response with cache metrics)
- `CacheConfig` struct in LLMPort (CacheTools, CacheSystem, CacheConversation)
- `LLMUsage` extended with `CacheCreationInputTokens`, `CacheReadInputTokens`
- `CalculateCost()` accounts for cache pricing (write x1.25, read x0.1)
- Agent1 builds messages from `ConversationHistory` for multi-turn cache hits
- Agent2 refactored: tool-based preset selection with `ChatWithToolsCached`
- `AddDelta` auto-increment step via `MAX(step)+1` (no manual step management)
- Logger: `LLMUsageWithCache` method with cache hit rate
- Debug page: cache metrics (CacheCreationInputTokens, CacheReadInputTokens, CacheHitRate)
- Padding tools (8 dummy `_internal_*` tools, ~3200 tokens) for cache threshold
- Integration test: `cache_test.go` (10 queries, 1 session)

---

## 2026-02-04 00:15

### Drill-Down Navigation (k3m9x2p)
- Navigation usecases: `ExpandUseCase` (drill-down to detail), `BackUseCase` (navigate back)
- Navigation handler: `POST /api/v1/session/{id}/expand`, `POST /api/v1/session/{id}/back`
- Detail presets: `product_detail`, `service_detail` for full entity views
- Frontend templates: `ProductDetailTemplate`, `ServiceDetailTemplate`
- Frontend navigation: `BackButton` component for back navigation
- ViewStack integration: push current view on expand, pop on back
- Tests: navigation scenarios (expand, back, stack depth)

---

## 2026-02-03 22:30

### Delta State Management (x7k9m2p)
- Extended Delta with source tracking: `Source` (user/llm/system), `ActorID`, `DeltaType`, `Path`
- Added ViewStack for back/forward navigation: `ViewMode`, `EntityRef`, `ViewSnapshot`, `ViewState`
- Extended SessionState with `View` and `ViewStack` fields
- Extended StatePort with `GetDeltasUntil`, `PushView`, `PopView`, `GetViewStack`
- Database migration: new columns in `chat_session_state` (view_mode, view_focused, view_stack) and `chat_session_deltas` (source, actor_id, delta_type, path)
- New usecases: `ReconstructStateUseCase` (rebuild state at any step), `RollbackUseCase` (revert to previous step)
- Agent1 now populates new delta fields (Source=llm, ActorID=agent1, etc.)
- Integration tests: 10 tests covering delta tracking, ViewStack, reconstruct, rollback scenarios

---

## 2026-02-03 19:30

### Session TTL Fix
- Fixed "eternal sessions" bug: sessions now properly expire after 5 min inactivity
- Added `domain.SessionTTL` constant (5 minutes) as single source of truth
- `handler_session.go` now checks TTL on read and marks expired sessions as closed
- Synced TTL in `chat_send_message.go` (was 10 min, now 5 min)
- Frontend sees `status: "closed"` → clears localStorage → shows fresh welcome

---

## 2026-02-03 17:00

### Architecture Refactoring
- Remove unused SearchPort (search via CatalogPort.ListProducts)
- Deduplicate convertToFormation (agent2 uses shared function from pipeline)
- Deduplicate tool_render_preset.go: 386→320 lines
  - Generic buildFormation() with FieldGetter/CurrencyGetter
  - Shared buildAtoms() for Product and Service
- Remove ExecuteLegacy from Agent2 (unused code path)
- Add tenant middleware for pipeline (X-Tenant-Slug header)
- Proper tenant context flow: Handler → Pipeline → Agent1 → State → Tool

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
