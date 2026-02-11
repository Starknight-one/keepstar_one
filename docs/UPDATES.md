# Updates

Лог изменений проекта Keepstar.

---

## Alpha 0.0.1 — 2026-02-11

### Embeddable Chat Widget — Shadow DOM (feat/embeddable-widget)

Фронтенд превращён из React SPA в встраиваемый виджет. Один `<script>` тег на сайте клиента → AI-чат с каталогом товаров. Shadow DOM, полная изоляция стилей.

**Использование:** `<script src="https://keepstar.one/widget.js" data-tenant="nike"></script>`

**Новые файлы:**
- `widget.jsx`: entry point — Shadow DOM shell, CSS injection, React mount
- `WidgetApp.jsx`: UI — trigger button + overlay + chat + formations
- `WidgetConfigContext.jsx`: React Context для `tenantSlug` + `apiBaseUrl`
- `widget.css`: Shadow DOM scoped стили (`:host` вместо `:root`)

**API Client — мультитенантность:**
- `setTenantSlug()` / `setApiBaseUrl()` — setter-функции
- `X-Tenant-Slug` header во всех fetch-запросах
- Backward compatible: без slug header не шлётся

**Build:**
- Один `npm run build` → `dist/widget.js` (IIFE, 72KB gzip)
- `shadowDomCss()` Vite plugin — глушит обычные CSS imports, всё через `?inline`
- Dev/prod parity: Shadow DOM в обоих режимах

**Удалено:** `App.jsx`, `App.css`, `main.jsx`, `index.css`, `vite.widget.config.js` — SPA больше не нужен

**Specs:** `ADW/specs/done/done-embeddable-widget.md`

---

## 2026-02-11 00:30

### Railway Deploy — Chat + Admin (main)

Два Railway service из одного GitHub repo. Каждый Go-сервер раздаёт свой React SPA + API.

**Backend (2 файла изменены):**
- `cmd/server/main.go` (chat + admin): SPA file server — catch-all `/` handler отдаёт static files из `./static/`, fallback на `index.html` для React Router
- `config/config.go` (admin): `PORT` → `getEnv("PORT", getEnv("ADMIN_PORT", "8081"))` для Railway

**DevOps (2 новых файла):**
- `project/Dockerfile`: multi-stage build (Node 22 frontend → Go 1.24 backend → Alpine 3.21 runtime), `VITE_API_URL=/api/v1`
- `project_admin/Dockerfile`: тот же паттерн, без VITE_API_URL

**Bugfix — silent embed error (2 файла):**
- `tool_catalog_search.go`: embedding ошибка глоталась молча → теперь `meta["embed_error"]`
- `postgres_trace.go`: добавлен вывод `embed: {ms}ms ERROR: {err}` и `results: keyword={n} vector={n} merged={n} type={type}` в pipeline trace logs

**Проблемы при деплое:** Railpack build fail (root directory), Admin 502 (порт), search 0 results (неверный OpenAI ключ в Railway), DATABASE_URL split на `&` (вставлять через JSON tab)

**Specs:** `ADW/specs/done/done-railway-deploy.md`

---

## 2026-02-10 19:30

### Session Init + Tenant Seed (feat/session-init)

При открытии чата — лёгкий init запрос создаёт сессию, резолвит тенант, сидит его в state, возвращает greeting. Убирает fallback на hardcoded "nike" при первом pipeline запросе.

**Backend (4 файла):**
- `handler_session.go`: `HandleInitSession` (POST /api/v1/session/init) — создаёт state + session, seeds tenant_slug в Aliases, возвращает `{ sessionId, tenant, greeting }`
- `routes.go`: роут `/api/v1/session/init` с `ResolveFromHeader` tenant middleware
- `middleware_cors.go`: `X-Tenant-Slug` в allowed headers
- `main.go`: `NewSessionHandler(cache, statePort)` — передан stateAdapter

**Frontend (2 файла):**
- `apiClient.js`: `initSession()` → POST /session/init
- `ChatPanel.jsx`: на mount без кэша → `initSession()` → показывает greeting как assistant message. Graceful fallback если init упал.

**Дубликации нет:** Pipeline и Agent1 используют get-or-create паттерн — если state/session уже есть, создание пропускается. Tenant seeding идемпотентен.

**Specs:** `ADW/specs/done/done-session-init.md`

---

### Admin Panel MVP (feat/admin-panel-mvp)

Отдельный проект в `project_admin/` — админка для самостоятельной загрузки каталогов клиентами. Go backend (порт 8081) + React frontend (порт 5174), своя гексагоналка, общая Postgres БД.

**Backend (34 файла):**
- **Auth**: signup (email+password+companyName → tenant + user + JWT), login, JWT middleware (24h, HS256), bcrypt
- **Catalog CRUD**: ListProducts (search/filter/pagination/merge master→product), GetProduct, UpdateProduct (partial), GetCategories
- **Import**: JSON upload → async background goroutine (GetOrCreateCategory → UpsertMasterProduct ON CONFLICT sku → UpsertProductListing ON CONFLICT tenant+master) → embedding generation (batch 100, pgvector) → catalog digest regeneration. Progress polling
- **Settings**: TenantSettings в catalog.tenants.settings JSONB (geoCountry, geoRegion, enrichCrossData)
- **DB**: admin.admin_users, admin.import_jobs + unique index catalog.products(tenant_id, master_product_id)

**Frontend (25 src файлов):**
- Login/Signup → JWT localStorage → AuthProvider
- Dashboard: sidebar (Catalog, Import, Settings) + protected routes
- PIM: таблица товаров (image, name, brand, category, price, stock) + search + category filter + pagination 25/page + detail/edit page
- Import: file input (.json) → preview 5 items → upload → progress bar (polling 2s) → history table
- Settings: country dropdown, region input, enrichment toggle
- UI kit: Button, Input, Table, Pagination, Badge, Spinner, Tabs

**DevOps:**
- Claude commands: `/start_admin`, `/stop_admin`, `/start_all`, `/stop_all`
- Shell scripts: `scripts/start_admin.sh`, `scripts/stop_admin.sh`, `scripts/start_all.sh`, `scripts/stop_all.sh`

**Порты:** chat :8080/:5173, admin :8081/:5174 — без конфликтов

**Specs:** `ADW/specs/done/done-admin-panel-mvp.md`

---

### Technical Debt Cleanup (chore/technical-debt-cleanup)

**Reliability:**
- `postgres_catalog.go`: 4× `err.Error() == "no rows..."` → `errors.Is(err, pgx.ErrNoRows)` (robust error matching)
- `postgres_catalog.go`: extracted `mergeProductWithMaster()` helper — deduplicated ~70 lines across ListProducts, GetProduct, VectorSearch
- `postgres_state.go`: all 16 `json.Marshal` calls now return errors; all 12 `json.Unmarshal` calls now `slog.Warn` + continue; AddDelta step sync uses `slog.Warn`
- `postgres_catalog.go`: 2 additional `_ = json.Unmarshal` → `slog.Warn` (GetMasterProductsWithoutEmbedding, GetAllTenants)

**Logging unification:**
- `tool_catalog_search.go`: VectorSearch error captured in `meta["vector_error"]` instead of silently dropped
- 8× `log.Printf` → structured logger: handler_chat, handler_catalog, chat_send_message get `*logger.Logger` field; anthropic_client uses `slog.Warn`; main.go passes `appLog` to all constructors

**Frontend deduplication:**
- Extracted `templateUtils.js` (groupAtomsBySlot + normalizeImages) — shared by 4 template files
- Extracted `ImageCarousel.jsx` — shared by ProductCard + ServiceCard templates
- Detail templates keep local ImageGallery (different UI with thumbnails)

**Dead code removal (−891 lines):**
- Deleted `mock_tools.go` (414 lines of cache padding tools) + removed `GetCachePaddingTools()` call from registry
- Deleted `adapters/json_store/` directory (legacy MVP stub)
- Removed deprecated `DefaultSessionTTL` constant
- Deleted empty FE directories: `src/app/`, `src/styles/`, `src/entities/atom/atoms/`

**Specs:** `ADW/specs/done/done-technical-debt-cleanup.md`, reorganized specs into `done/` and `todo/` subdirectories

---

## 2026-02-08 16:00

### Search Relevance: Catalog Digest + RRF Tuning (fix/bug1-vector-search-relevance)
- **catalog_digest_entity.go** (new): `CatalogDigest`, `DigestCategory`, `DigestParam` — pre-computed meta-schema of tenant catalog for Agent1 system prompt. `ToPromptText()` generates compact text with search strategy hints (→ filter / → vector_query). `ComputeFamilies()` groups ~100 RU/EN color names into 11 families via `colorFamilyMap`
- **CatalogPort extensions**: `GenerateCatalogDigest`, `GetCatalogDigest`, `SaveCatalogDigest`, `GetAllTenants`. `VectorSearch` now accepts optional `*VectorFilter` (brand/category pre-filtering)
- **catalog_migrations**: `catalog_digest JSONB` column on `catalog.tenants` table
- **Agent1 enriched context**: `BuildAgent1ContextPrompt(meta, currentConfig, query, digest)` prepends `<catalog>` block (from digest) + `<state>` block (with RenderConfig) around user query
- **Agent1SystemPrompt**: catalog-aware rules — exact category names from digest, category strategy (specific→filter, broad→vector_query only), high-cardinality params (families) → vector_query not filter
- **RRF merge tuning**: keyword weight boost 1.5× default, 2.0× when structured filters (brand/category) are present
- **VectorFilter**: `VectorSearch` pre-filters by brand/category before cosine ranking
- **Agent1ExecuteUseCase**: now depends on `CatalogPort`, loads digest, passes `EnrichedQuery` to response
- **Large seed data**: 6 category-specific seed files (clothing, shoes, phones, audio, home electronics, services)
- **Tests**: catalog_digest_test.go (domain), prompt_analyze_query_test.go, tool_catalog_search_test.go, catalog_search_relevance_test.go, catalog_digest_test.go (adapter)

### Stone: Expert + README Sync
- Expert sync: 5 of 9 updated (backend-domain, backend-ports, backend-adapters, backend-usecases, backend-pipeline)
- README sync: 6 updated (domain, ports, adapters/postgres, usecases, tools, prompts)

---

## 2026-02-07 22:00

### Pipeline Span Waterfall Tracing (feature/pipeline-span-waterfall)
- **domain/span.go** (new): `Span` struct, `SpanCollector` (thread-safe), context helpers (`WithSpanCollector`, `SpanFromContext`, `WithStage`, `StageFromContext`), dot-separated naming convention
- **PipelineTrace.Spans**: `[]Span` field for waterfall data
- **Anthropic adapter**: span instrumentation — `{stage}.llm`, `{stage}.llm.ttfb` (via `httptrace.GotFirstResponseByte`), `{stage}.llm.body`, slow TTFB warning (>10s)
- **CatalogSearchTool**: sub-operation spans — `{stage}.tool.embed`, `{stage}.tool.sql`, `{stage}.tool.vector`
- **Agent1**: span `agent1` + `WithStage`, `agent1.tool`, `agent1.state`; tool filter changed `search_*` → `catalog_*`
- **Agent2**: span `agent2` + `WithStage`, `agent2.tool`; `ToolChoice='any'` for forced tool calls
- **Pipeline**: creates `SpanCollector`, `pipeline` span, records `trace.Spans = sc.Spans()`
- **CacheConfig.ToolChoice**: `auto`/`any`/`tool:name` support
- **Trace handler**: waterfall visualization with horizontal timeline bars, TTFB column in list; template funcs (spanDepth, spanLabel, spanColor, spanPercent, maxTTFB)
- **postgres_trace**: WATERFALL section in console `printTrace`

### Stone: Expert + README Sync
- Expert sync: 6 of 9 updated (backend-domain, backend-ports, backend-adapters, backend-usecases, backend-handlers, backend-pipeline)
- README sync: 10 updated (root, AI_docs, .claude/experts, backend×7), 18 unchanged

---

## 2026-02-07 19:00

### Vector Search — Hybrid Keyword + Semantic (feature/vector-search)
- **EmbeddingPort**: new port interface (`Embed(ctx, texts) → [][]float32`)
- **OpenAI adapter**: `openai/embedding_client.go` — implements EmbeddingPort via OpenAI embeddings API
- **CatalogPort extensions**: `VectorSearch`, `SeedEmbedding`, `GetMasterProductsWithoutEmbedding` methods
- **ProductFilter extensions**: `CategoryName` (ILIKE), `SortField`/`SortOrder`, `Attributes` (JSONB ILIKE)
- **pgvector integration**: `embedding vector(384)` column on `master_products`, HNSW index, cosine distance
- **CatalogSearchTool rewrite**: hybrid search meta-tool — keyword SQL + vector pgvector + RRF merge
- **Normalizer removed**: deleted `normalizer.go` and `prompt_normalize_query.go` — vector embeddings handle multilingual matching
- **Registry update**: `NewRegistry` now accepts `embeddingPort` (nil = keyword-only mode)
- **Agent1 prompt update**: `vector_query` in original language, `filters` in English, style request handling

### Stone: Expert + README Sync
- Expert sync: 4 of 9 updated (backend-ports, backend-adapters, backend-usecases, backend-pipeline)
- README sync: 20 of 30 updated (root, AI_docs, .claude, backend×9, frontend×8)

---

## 2026-02-07 14:00

### Housekeeping: Spec Archive + Expert Sync
- Archived 25 completed/superseded spec files from `ADW/specs/` → `ADW/specs/archive/`
- Added `project/backend/bin/server` (compiled binary)
- Expert sync: 6 of 9 experts updated
  - backend-domain: added PipelineTrace, AgentTrace, StateSnapshot, DeltaTrace, FormationTrace types
  - backend-ports: added TracePort interface, CachePort.DeleteSession method
  - backend-adapters: added TraceAdapter, RetentionService, trace_migrations, pipeline_traces table
  - backend-handlers: added TraceHandler (debug traces list/detail, kill-session endpoint)
  - backend-pipeline: added CatalogSearchTool (meta-tool with normalizer, fallback cascade), QueryNormalizer, NormalizeQueryPrompt
  - frontend-features: added sessionCache.js (localStorage session cache with 30min TTL)
- README sync: backend (added trace/navigation endpoints, TracePort), frontend (added navigation feature, expand/goBack API)
- Changelog updated

---

## 2026-02-06 20:00

### Design System Integration (UNSTABLE)
- **Design system atoms**: 6 atom types (text, number, image, icon, video, audio) + subtype + display model
- **Freestyle tool**: `tool_freestyle.go` — Agent2 tool for style aliases and display overrides
- **ToolContext**: Registry.Execute now receives ToolContext (SessionID+TurnID+ActorID) instead of bare sessionID
- **Agent2 rework**: receives view state, user query, and data delta; filters render_* + freestyle tools
- **Agent1 rework**: filters search_* and _internal_* tools; re-reads state after tool zone-write
- **Prompt updates**: Agent2ToolPrompt with view context, user intent, data change signal
- **Frontend**: useChatSubmit adjustments, theme system foundation
- **New specs**: agent-tool-isolation, session-state-flow patches
- **New tests**: tool_render_preset_test.go, tool_search_products_test.go

### Stone: Expert + README Sync
- Expert sync: 7 expertise.yaml files updated (backend-adapters, backend-usecases, backend-pipeline, backend-handlers, backend-domain, frontend-shared, frontend-entities)
- README sync: tools/README.md updated with freestyle tool, ToolContext, test files

---

## 2026-02-04 23:00

### Bugfix: E2E Pipeline Smoke Test
- **Search fix**: `postgres_catalog.go` — ILIKE `%Nike shoes%` не матчил "Nike Air Max 90". Поиск теперь разбивает запрос на слова с OR (`%Nike%` OR `%shoes%`)
- **Conversation history fix**: `agent1_execute.go` — не сохранялся `tool_result` в history. Anthropic API требует `[user → assistant:tool_use → user:tool_result]`. Второе сообщение в чат вызывало 500
- **Cache control fix**: `anthropic_client.go` `markMessageCacheControl()` — при конвертации `[]contentBlock → []contentBlockWithCache` терялись поля `id`, `name`, `input`. Добавлен `contentBlockFullCache` тип с полным набором полей
- **Cache threshold fix**: `mock_tools.go` — 10 → 20 padding tools. Input tokens 2985 → 5512, выше минимума 4096 для Haiku 4.5. Cache hit rate: 91.6%, LLM latency 2685ms → 698ms

**Известные ограничения (debug page):**
- `/debug/session/{id}` показывает метрики только последнего turn'а (MetricsStore перезаписывает). Нет истории по шагам агентов
- Состояние (стейт) отображается слабо — нет визуализации зон и дельт по turn'ам

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
