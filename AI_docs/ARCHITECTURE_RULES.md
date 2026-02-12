
# Keepstar Architecture Rules

> Этот документ — источник истины для архитектурных решений.
> Агент ОБЯЗАН прочитать его перед началом работы и сверяться при code review.
> **Обновлён: 2026-02-12** — соответствует текущей кодовой базе.

---

## 1. Философия

```
Код пишется для агентов, не только для людей.
Агент должен понять что делает файл за 10 секунд.
Агент должен изменить файл не сломав остальное.
```

**Главный принцип**: Изоляция > Переиспользование > DRY

**Архитектурный принцип**: Backend генерирует — Frontend рендерит. FE = тупой рендерер formations, никакой бизнес-логики на клиенте.

---

## 2. Два проекта

Keepstar состоит из двух независимых проектов в одном репо:

| Проект | Путь | Порты | Назначение |
|--------|------|-------|------------|
| **Chat** | `project/` | BE :8080, FE :5173 | Виджет-чат для конечных пользователей |
| **Admin** | `project_admin/` | BE :8081, FE :5174 | Админка для контрагентов (каталог, импорт, настройки) |

Оба проекта — Go backend + React frontend. Общая PostgreSQL база данных. Каждый проект имеет свою hexagonal architecture.

**Deploy**: Каждый проект собирается в свой Docker image (multi-stage: Node → Go → Alpine). Railway (два сервиса из одного repo).

---

## 3. Структура проекта

### Chat Backend (Go)

```
project/backend/
├── cmd/
│   ├── server/main.go              # Bootstrap: config → adapters → usecases → handlers → routes
│   └── dbcheck/                    # DB diagnostic tools
│       ├── main.go
│       └── trace.go
│
├── internal/
│   ├── domain/                     # Чистые структуры, НОЛЬ зависимостей
│   │   ├── product_entity.go       # Product, ProductFilter
│   │   ├── master_product_entity.go # MasterProduct (shared catalog)
│   │   ├── service_entity.go       # Service entity
│   │   ├── category_entity.go      # Category
│   │   ├── tenant_entity.go        # Tenant, TenantConfig
│   │   ├── session_entity.go       # Session, SessionTTL
│   │   ├── message_entity.go       # Message, ContentBlock (Anthropic format)
│   │   ├── user_entity.go          # User
│   │   ├── state_entity.go         # SessionState, StateMeta, StateData, Delta, DeltaInfo
│   │   ├── widget_entity.go        # Widget, WidgetType, WidgetSize, WidgetTemplate
│   │   ├── atom_entity.go          # Atom, AtomType, AtomSlot
│   │   ├── formation_entity.go     # Formation, FormationWithData, FormationMode
│   │   ├── template_entity.go      # FormationTemplate
│   │   ├── preset_entity.go        # PresetConfig, FieldConfig, SlotMapping
│   │   ├── display_entity.go       # DisplayConfig, RenderConfig
│   │   ├── entity_type.go          # EntityType enum (product, service)
│   │   ├── catalog_digest_entity.go # CatalogDigest, DigestCategory, DigestParam
│   │   ├── tool_entity.go          # Tool, ToolResult, ToolContext
│   │   ├── trace_entity.go         # PipelineTrace, AgentTrace, DeltaTrace
│   │   ├── span.go                 # Span, SpanCollector (waterfall tracing)
│   │   ├── event_entity.go         # ChatEvent, SessionEvent
│   │   ├── domain_errors.go        # Domain error types
│   │   └── README.md
│   │
│   ├── ports/                      # Интерфейсы (контракты)
│   │   ├── llm_port.go             # LLMPort: Chat, ChatWithTools, ChatWithToolsCached
│   │   ├── catalog_port.go         # CatalogPort: ListProducts, VectorSearch, Embeddings, CatalogDigest
│   │   ├── state_port.go           # StatePort: CRUD + zone-writes (UpdateData/Template/View) + ViewStack
│   │   ├── cache_port.go           # CachePort: Sessions + Products cache
│   │   ├── embedding_port.go       # EmbeddingPort: Embed(texts) → vectors
│   │   ├── trace_port.go           # TracePort: RecordTrace, ListTraces, GetTrace
│   │   ├── event_port.go           # EventPort: TrackEvent
│   │   └── README.md
│   │
│   ├── adapters/                   # Реализации портов
│   │   ├── anthropic/              # LLMPort implementation
│   │   │   ├── anthropic_client.go # Chat, ChatWithTools, prompt caching, cache metrics
│   │   │   ├── cache_types.go      # CachedRequest/Response, cache_control types
│   │   │   └── README.md
│   │   ├── openai/                 # EmbeddingPort implementation
│   │   │   └── embedding_client.go # OpenAI text-embedding-3-small (384 dims)
│   │   ├── postgres/               # CatalogPort, StatePort, CachePort, TracePort, EventPort
│   │   │   ├── postgres_client.go  # pgxpool connection
│   │   │   ├── postgres_catalog.go # Products, MasterProducts, VectorSearch (pgvector), Categories
│   │   │   ├── postgres_state.go   # SessionState CRUD, zone-writes, deltas, ViewStack
│   │   │   ├── postgres_cache.go   # Session cache (Redis-like in Postgres)
│   │   │   ├── postgres_trace.go   # Pipeline traces + console waterfall printer
│   │   │   ├── postgres_events.go  # Event tracking
│   │   │   ├── retention.go        # Data retention service
│   │   │   ├── migrations.go       # Chat schema migrations
│   │   │   ├── catalog_migrations.go # Catalog schema (tenants, categories, master_products, products)
│   │   │   ├── state_migrations.go # State schema (session_state, session_deltas)
│   │   │   ├── trace_migrations.go # Trace schema (pipeline_traces)
│   │   │   └── README.md
│   │   ├── memory/                 # In-memory cache (заглушка под instant navigation)
│   │   │   ├── memory_cache.go
│   │   │   └── README.md
│   │   └── README.md
│   │
│   ├── usecases/                   # Бизнес-логика
│   │   ├── pipeline_execute.go     # Orchestrator: Agent1 → Agent2 → ApplyTemplate → Formation
│   │   ├── agent1_execute.go       # Tool Caller: query → LLM → tool calls → state update
│   │   ├── agent2_execute.go       # Template Builder: state meta → LLM → FormationTemplate
│   │   ├── template_apply.go       # ApplyTemplate: template + data → FormationWithData
│   │   ├── chat_send_message.go    # Legacy chat (pre-pipeline)
│   │   ├── catalog_list_products.go
│   │   ├── catalog_get_product.go
│   │   ├── navigation_expand.go    # Drill-down: grid → detail
│   │   ├── navigation_back.go      # Back: detail → previous view
│   │   ├── state_reconstruct.go    # Rebuild state at any step from deltas
│   │   ├── state_rollback.go       # Revert state to previous step
│   │   └── README.md
│   │
│   ├── handlers/                   # HTTP слой — ТОЛЬКО parse/validate/respond
│   │   ├── handler_pipeline.go     # POST /api/v1/pipeline — main entry point
│   │   ├── handler_chat.go         # POST /api/v1/chat (legacy)
│   │   ├── handler_session.go      # POST /api/v1/session/init — creates session + seeds tenant
│   │   ├── handler_navigation.go   # POST .../expand, POST .../back
│   │   ├── handler_catalog.go      # GET /api/v1/tenants/{slug}/products
│   │   ├── handler_trace.go        # GET /debug/traces — waterfall visualization
│   │   ├── handler_debug.go        # GET /debug/session/{id} — state inspector
│   │   ├── handler_health.go       # GET /health, GET /ready
│   │   ├── middleware_cors.go      # CORS (allow *, X-Tenant-Slug header)
│   │   ├── middleware_tenant.go    # Tenant resolution from X-Tenant-Slug header
│   │   ├── response.go            # JSON response helpers
│   │   ├── routes.go              # All route registration
│   │   └── README.md
│   │
│   ├── tools/                      # LLM Tools (Agent1 вызывает)
│   │   ├── tool_registry.go        # Registry: registers tools, executes by name
│   │   ├── tool_catalog_search.go  # Meta-tool: keyword SQL + vector pgvector + RRF merge
│   │   ├── tool_render_preset.go   # Render products/services via preset configs
│   │   ├── tool_freestyle.go       # Freestyle display overrides
│   │   ├── tool_search_products.go # Direct product search
│   │   └── README.md
│   │
│   ├── presets/                    # Preset configs (детерминированные, без LLM)
│   │   ├── preset_registry.go      # Registry: Get, GetByEntityType, List
│   │   ├── product_presets.go      # product_grid, product_card, product_compact, product_detail
│   │   ├── service_presets.go      # service_card, service_list, service_detail
│   │   └── README.md
│   │
│   ├── prompts/                    # LLM промпты — ОТДЕЛЬНО от бизнес-логики
│   │   ├── prompt_analyze_query.go # Agent1: system prompt + context builder (catalog digest, state)
│   │   ├── prompt_compose_widgets.go # Agent2: system prompt + tool prompt
│   │   └── README.md
│   │
│   ├── logger/
│   │   ├── logger.go               # Structured logger (slog-based), LLMUsage metrics
│   │   └── README.md
│   │
│   └── config/
│       ├── config.go               # Env vars: PORT, DATABASE_URL, ANTHROPIC_API_KEY, OPENAI_API_KEY, etc.
│       └── README.md
```

### Chat Frontend (React/JS — Embeddable Widget)

```
project/frontend/src/
├── widget.jsx                      # Entry point: Shadow DOM shell, CSS injection, React mount
├── widget.css                      # Shadow DOM scoped styles (:host)
├── WidgetApp.jsx                   # Main UI: trigger button + overlay + chat + formations
├── index.css                       # Base styles
│
├── shared/
│   ├── api/
│   │   └── apiClient.js            # HTTP client: X-Tenant-Slug header, setTenantSlug(), setApiBaseUrl()
│   ├── config/
│   │   └── WidgetConfigContext.jsx  # React Context: tenantSlug + apiBaseUrl
│   ├── theme/                      # Theme system
│   │   ├── ThemeContext.js
│   │   ├── ThemeProvider.jsx
│   │   ├── ThemeSwitcher.jsx
│   │   ├── themeModel.js
│   │   └── themes/marketplace.css
│   ├── hooks/                      # (empty — reserved)
│   ├── lib/                        # (empty — reserved)
│   ├── logger/                     # (empty — reserved)
│   └── ui/                         # (empty — reserved for design kits)
│
├── entities/
│   ├── widget/
│   │   ├── WidgetRenderer.jsx      # Renders single widget by size (tiny/small/medium/large)
│   │   ├── widgetModel.js          # Widget type definitions
│   │   ├── Widget.css
│   │   └── templates/              # Template-based rendering
│   │       ├── templateUtils.js    # Shared: SLOTS, groupAtomsBySlot, normalizeImages
│   │       ├── ImageCarousel.jsx   # Shared carousel component
│   │       ├── ProductCardTemplate.jsx
│   │       ├── ProductDetailTemplate.jsx
│   │       ├── ServiceCardTemplate.jsx
│   │       ├── ServiceDetailTemplate.jsx
│   │       └── index.js
│   ├── atom/
│   │   ├── AtomRenderer.jsx        # Renders atom by type (text/number/image/rating/badge/button/icon/divider/progress)
│   │   ├── atomModel.js
│   │   └── Atom.css
│   ├── formation/
│   │   ├── FormationRenderer.jsx   # Renders formation by mode (grid/carousel/single/list)
│   │   ├── formationModel.js
│   │   ├── Formation.css
│   │   └── index.js
│   └── message/
│       ├── MessageBubble.jsx       # Chat message with optional Formation
│       └── messageModel.js
│
├── features/
│   ├── chat/
│   │   ├── ChatPanel.jsx           # Chat UI: messages + input + session init
│   │   ├── ChatInput.jsx
│   │   ├── ChatHistory.jsx
│   │   ├── chatModel.js
│   │   ├── useChatSubmit.js        # Pipeline submit + formation callback
│   │   ├── useChatMessages.js
│   │   ├── sessionCache.js         # localStorage session cache (30min TTL)
│   │   └── ChatPanel.css
│   ├── navigation/
│   │   ├── BackButton.jsx          # Back navigation
│   │   └── BackButton.css
│   ├── overlay/
│   │   ├── FullscreenOverlay.jsx   # Backdrop + external formation rendering
│   │   ├── useOverlayState.js
│   │   └── Overlay.css
│   ├── catalog/
│   │   ├── ProductGrid.jsx
│   │   ├── useCatalogProducts.js
│   │   ├── catalogModel.js
│   │   └── ProductGrid.css
│   └── canvas/                     # (reserved for future canvas mode)
```

### Admin Backend (Go)

```
project_admin/backend/internal/
├── domain/                         # Admin-specific entities
│   ├── admin_user.go               # AdminUser (email, password hash, tenant_id)
│   ├── product.go                  # Admin product view
│   ├── category.go
│   ├── tenant.go
│   ├── tenant_settings.go          # TenantSettings (geoCountry, geoRegion, enrichCrossData)
│   ├── import_job.go               # ImportJob (status, progress, error)
│   └── errors.go
│
├── ports/
│   ├── auth_port.go                # Signup, Login, GetUser
│   ├── catalog_port.go             # ListProducts, GetProduct, UpdateProduct, GetCategories
│   ├── import_port.go              # CreateJob, UpdateJob, UpsertMasterProduct, UpsertProductListing
│   └── embedding_port.go           # Embedding generation for imported products
│
├── adapters/
│   ├── postgres/
│   │   ├── postgres_client.go
│   │   ├── auth_adapter.go         # admin.admin_users table
│   │   ├── catalog_adapter.go      # Reads from catalog schema with master merge
│   │   ├── import_adapter.go       # Import: GetOrCreateCategory → UpsertMaster → UpsertListing → Embeddings
│   │   ├── admin_migrations.go     # admin schema
│   │   └── catalog_migrations.go   # catalog schema (shared with chat)
│   └── openai/
│       └── embedding_client.go     # Same OpenAI embeddings
│
├── handlers/
│   ├── handler_auth.go             # POST /admin/api/auth/signup, /login
│   ├── handler_products.go         # GET/PUT /admin/api/products
│   ├── handler_import.go           # POST /admin/api/import/upload, GET /progress
│   ├── handler_settings.go         # GET/PUT /admin/api/settings
│   ├── middleware_auth.go          # JWT middleware (24h, HS256)
│   ├── middleware_cors.go
│   └── response.go
│
├── usecases/
│   ├── auth.go                     # Signup (tenant + user + JWT), Login
│   ├── products.go                 # ListProducts, GetProduct, UpdateProduct
│   ├── import.go                   # JSON upload → async import → embeddings → digest regen
│   └── settings.go                 # Tenant settings CRUD
│
├── logger/
│   └── logger.go
└── config/
    └── config.go
```

### Admin Frontend (React/JS)

```
project_admin/frontend/src/
├── App.jsx                         # Router: auth guard → dashboard layout
├── main.jsx
│
├── features/
│   ├── auth/                       # Login/Signup → JWT localStorage
│   ├── layout/                     # DashboardLayout: sidebar (Catalog, Import, Settings, Widget)
│   ├── catalog/                    # PIM: products table + detail/edit page
│   ├── import/                     # JSON upload → progress bar → history
│   ├── settings/                   # Country, region, enrichment toggle
│   └── widget/                     # Widget embed code + copy button
│
└── shared/
    ├── api/apiClient.js            # Admin HTTP client (JWT auth header)
    └── ui/                         # UI kit: Button, Input, Table, Pagination, Badge, Spinner, Tabs
```

---

## 4. Two-Agent Pipeline

Главный flow системы. Запрос пользователя проходит через два LLM-агента:

```
User Query
    ↓
[Pipeline] ─── creates SpanCollector, TurnID
    ↓
[Agent 1 — Tool Caller]
    ├── System prompt: catalog digest + state context + query
    ├── LLM decides which tools to call
    ├── Tools: catalog_search (hybrid: keyword + vector + RRF), render_preset, freestyle
    ├── Tool results → state update (zone-write: UpdateData)
    └── Returns: enriched query + state with data
    ↓
[Agent 2 — Template Builder]
    ├── Receives: state meta (product count, types, user intent)
    ├── LLM selects: mode (grid/list/carousel/single) + template + size
    ├── Returns: FormationTemplate
    └── Zone-write: UpdateTemplate + UpdateView
    ↓
[ApplyTemplate]
    ├── Template + state data → FormationWithData (widgets with atoms)
    ├── Deterministic (no LLM)
    └── Preset registry builds atoms from product/service fields
    ↓
[Response] → FormationWithData JSON → Frontend renders
```

**Prompt Caching**: Anthropic cache_control на tools, system prompt, conversation history. Cache hit rate ~90%+.

**Tracing**: Каждый pipeline run записывает PipelineTrace с waterfall spans (agent1, agent1.llm, agent1.tool, agent2, etc.).

---

## 5. Catalog Architecture

### Database Schema (shared between Chat and Admin)

```sql
catalog.tenants          -- Tenant (slug, name, type, settings JSONB, catalog_digest JSONB)
catalog.categories       -- Category (name, slug, parent_id) — hierarchical
catalog.master_products  -- Shared product data (sku, name, description, images, attributes, embedding vector(384))
catalog.products         -- Tenant-specific overlay (tenant_id, master_product_id, price, stock, custom fields)
```

**Master-Tenant Merge**: Product = master data + tenant overlay. Tenant может переопределить name, description, images поверх мастера. Fallback на мастер если поле пустое.

**Vector Search**: pgvector HNSW index на master_products.embedding. Hybrid: keyword (ILIKE) + vector (cosine) + RRF merge.

**Catalog Digest**: Компактный мета-schema каталога тенанта (категории, бренды, параметры). Используется в Agent1 system prompt для informed search decisions.

---

## 6. Правила именования

### КРИТИЧНО: Уникальность имён

```
ЗАПРЕЩЕНО повторять имена файлов, функций, переменных в проекте.
Перед созданием — проверь что такого имени ещё нет.
```

**Стратегия — контекстные префиксы:**

| Слой | Префикс | Пример |
|------|---------|--------|
| Handlers | `handler_` | `handler_pipeline.go`, `handler_session.go` |
| Usecases | `{domain}_` | `pipeline_execute.go`, `agent1_execute.go`, `navigation_expand.go` |
| Adapters | `{tech}_` | `postgres_catalog.go`, `anthropic_client.go` |
| Tools | `tool_` | `tool_catalog_search.go`, `tool_render_preset.go` |
| Prompts | `prompt_` | `prompt_analyze_query.go` |
| Logger | `logger_` | `logger.go` |
| Domain | `{entity}_entity.go` | `product_entity.go`, `formation_entity.go` |
| Ports | `{name}_port.go` | `llm_port.go`, `catalog_port.go` |
| Migrations | `{schema}_migrations.go` | `catalog_migrations.go`, `state_migrations.go` |
| Presets | `{entity}_presets.go` | `product_presets.go` |
| React components | `PascalCase.jsx` | `FormationRenderer.jsx`, `BackButton.jsx` |
| React hooks | `use{Feature}{Action}.js` | `useChatSubmit.js`, `useOverlayState.js` |
| React models | `{feature}Model.js` | `chatModel.js`, `widgetModel.js` |

### Файлы

| Что | Формат | Пример |
|-----|--------|--------|
| Go файл | `prefix_snake_case.go` | `tool_catalog_search.go` |
| Go тест | `prefix_snake_case_test.go` | `agent1_execute_test.go` |
| React компонент | `PascalCase.jsx` | `ProductCardTemplate.jsx` |
| React hook | `use{Feature}{Action}.js` | `useChatSubmit.js` |
| CSS | `{Component}.css` или `{feature}.css` | `ChatPanel.css`, `widget.css` |

---

## 7. Правила размера

| Метрика | Лимит | Что делать при превышении |
|---------|-------|---------------------------|
| Строк в файле | **1000 max** | Отметить для рефакторинга |
| Параметров функции | **5 max** | Создать struct для параметров |
| Вложенность (if/for) | **3 уровня max** | Early return, выделить функцию |
| Импортов | **20 max** | Файл делает слишком много |

---

## 8. Правила зависимостей

### Go: Направление зависимостей

```
handlers → usecases → ports ← adapters
               ↓         ↑
            domain    tools/presets/prompts
```

- `domain/` — НОЛЬ импортов из проекта
- `ports/` — только `domain/`
- `usecases/` — только `domain/`, `ports/`, `tools/`, `presets/`, `prompts/`
- `adapters/` — `domain/`, `ports/`, внешние библиотеки
- `handlers/` — всё выше + http библиотеки
- `tools/` — `domain/`, `ports/` (вызывает порты для работы с данными)
- `presets/` — только `domain/`
- `prompts/` — только `domain/` (для типов)
- `logger/` — ноль импортов из проекта

### React: Направление зависимостей

```
WidgetApp → features → entities → shared
```

- `shared/` — ноль импортов из проекта (только внешние)
- `entities/` — только `shared/`
- `features/` — `shared/` и `entities/`
- `WidgetApp.jsx` — всё

---

## 9. Промпты (LLM)

**Промпты ВСЕГДА лежат отдельно от бизнес-логики** в `prompts/`.

Текущие промпты:
- `prompt_analyze_query.go` — Agent1: system prompt + `BuildAgent1ContextPrompt()` (catalog digest + state + query)
- `prompt_compose_widgets.go` — Agent2: system prompt + tool prompt (mode/template/size selection)

### Правила

1. Один файл = один промпт (или связанная группа)
2. System prompt и context builder раздельно
3. Функция-билдер для подстановки контекста
4. Catalog digest встраивается в Agent1 prompt

---

## 10. Логирование

Структурированный slog-based logger. Передаётся через DI во все компоненты.

```go
// Использование
uc.log.Info("pipeline_started", "session_id", sessionID, "tenant", tenant)
uc.log.LLMUsageWithCache(stage, usage) // Вывод: tokens, cost, cache hit rate
```

### Правила

1. **Каждое действие логируется** — начало, конец, ошибка
2. **Структурированные логи** — key-value через slog, не строки
3. **Уровни**: Debug (детали), Info (события), Warn (проблемы), Error (ошибки)
4. **Никогда не логировать PII** — пароли, токены, персональные данные
5. **Нет inline логов** — `log.Printf()` и `fmt.Println()` запрещены

---

## 11. Error Handling

### Go: Стратегия ошибок

```go
// domain/domain_errors.go — предопределённые ошибки
var (
    ErrTenantNotFound    = &Error{Code: "TENANT_NOT_FOUND", Message: "Tenant not found"}
    ErrSessionExpired    = &Error{Code: "SESSION_EXPIRED", Message: "Session expired"}
    ErrLLMUnavailable    = &Error{Code: "LLM_UNAVAILABLE", Message: "AI service unavailable"}
)

// В adapters — wrap с контекстом, errors.Is для pgx.ErrNoRows
// В usecases — бизнес-контекст
// В handlers — логируем и возвращаем клиенту (domain error → user-facing, internal → "Internal error")
```

---

## 12. Конфигурация

```go
// Env vars (основные):
PORT              // Chat: 8080, Admin: 8081
DATABASE_URL      // Neon PostgreSQL
ANTHROPIC_API_KEY // Claude Haiku 4.5
OPENAI_API_KEY    // Embeddings (text-embedding-3-small)
LLM_MODEL         // default: claude-haiku-4-5-20251001
WIDGET_BASE_URL   // URL для embed code в админке
JWT_SECRET        // Admin auth
```

---

## 13. API Routes

### Chat Backend

```
POST /api/v1/pipeline              # Main: query → Agent1 → Agent2 → Formation
POST /api/v1/session/init          # Init session + seed tenant
POST /api/v1/session/{id}/expand   # Drill-down navigation
POST /api/v1/session/{id}/back     # Back navigation
POST /api/v1/chat                  # Legacy chat (pre-pipeline)
GET  /api/v1/tenants/{slug}/products        # Catalog API
GET  /api/v1/tenants/{slug}/products/{id}
GET  /debug/traces                 # Pipeline traces (waterfall)
GET  /debug/session/{id}           # Session state inspector
GET  /health                       # Health check
```

### Admin Backend

```
POST /admin/api/auth/signup        # Register (creates tenant + user)
POST /admin/api/auth/login         # Login → JWT
GET  /admin/api/products           # List products (search, filter, pagination)
GET  /admin/api/products/{id}      # Get product
PUT  /admin/api/products/{id}      # Update product
GET  /admin/api/categories         # List categories
POST /admin/api/import/upload      # JSON file upload → async import
GET  /admin/api/import/progress/{id} # Import progress
GET  /admin/api/import/history     # Import history
GET  /admin/api/settings           # Tenant settings
PUT  /admin/api/settings           # Update settings
GET  /admin/api/tenant             # Tenant info
GET  /admin/api/widget-config      # Widget embed URL
```

---

## 14. Тестирование

Тесты рядом с кодом (`_test.go`). Текущее покрытие:

| Область | Тесты |
|---------|-------|
| Domain | `catalog_digest_test.go`, `state_entity_test.go` |
| Usecases | `agent1_execute_test.go`, `agent2_execute_test.go`, `navigation_test.go`, `state_rollback_test.go`, `cache_test.go` |
| Adapters | `postgres_state_test.go`, `catalog_digest_test.go`, `catalog_search_relevance_test.go` |
| Tools | `tool_catalog_search_test.go`, `tool_render_preset_test.go` |
| Prompts | `prompt_analyze_query_test.go` |

---

## 15. Widget Embedding

Frontend собирается в один IIFE бандл `widget.js` (~72KB gzip). Встраивается на любой сайт:

```html
<script src="https://keepstar.one/widget.js" data-tenant="nike"></script>
```

- Shadow DOM — полная изоляция стилей
- `data-tenant` → `X-Tenant-Slug` header
- Auto-detection API URL из origin скрипта
- Go backend раздаёт `widget.js` как static file

---

## 16. Anti-patterns (ЗАПРЕЩЕНО)

- Промпты в коде usecases (только через `prompts/`)
- Inline логи (`fmt.Println`, `log.Printf`)
- Бизнес-логика на фронте (FE = рендерер)
- Повторяющиеся имена файлов
- God object (один сервис на всё)
- `err.Error() == "no rows..."` (использовать `errors.Is`)
- `_ = json.Marshal()` (всегда обрабатывать ошибки)

---

## 17. Чеклист перед коммитом

- [ ] Файл < 1000 строк?
- [ ] Нет циклических зависимостей?
- [ ] Domain не импортит ничего из internal?
- [ ] Имя файла УНИКАЛЬНО в проекте?
- [ ] Промпты в `prompts/`, не в usecases?
- [ ] Логи через logger, не inline?
- [ ] Ошибки wrapped с контекстом?
- [ ] Тест рядом с кодом?
- [ ] README папки обновлён?

---

## Версия документа

- **v1.0** — Initial version (2025-01-29)
- **v2.0** — Added naming, prompts, logging, error handling, configs, API, tests (2025-01-29)
- **v3.0** — Full rewrite: actual project structure, two-agent pipeline, catalog architecture, widget embedding, admin panel (2026-02-12)
