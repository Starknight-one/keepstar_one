# Updates

Лог изменений проекта Keepstar.

---

## Frontend Polish (попытка) — 2026-02-21

Бэкенд-часть работает, фронт — сломан. Нужна полная переработка CSS виджетов.

### Что сделано (бэкенд — ок):
- **Comparison preset:** `product_comparison` — таблица бок-о-бок (макс 4 товара), ComparisonTemplate.jsx + CSS
- **Catalog search limit:** default 10 → 50 (0 = no limit, safety cap 200)
- **FieldName в атомах:** buildAtoms прокидывает `field.Name` → `atom.FieldName`
- **Agent2 prompt:** добавлен `product_comparison` в примеры и описание пресетов
- **Lazy loading:** FormationRenderer батчи по 12 + IntersectionObserver

### Что сломано (фронт — критические баги):
1. **Грид карточек:** убрали фиксированные width/min-width/max-width из Widget.css и ProductCardTemplate.css чтобы карточки заполняли грид → карточки раздулись на весь экран, текст огромный, выглядит ужасно. Нужно найти баланс: карточки должны заполнять грид-ячейки но иметь разумные max-width (220-280px)
2. **Comparison не работает:** бэкенд отдаёт mode=comparison, но фронт рендерит как обычный list — гигантская фотка на весь экран вместо таблицы. Вероятно mode не доходит или condition в FormationRenderer не срабатывает
3. **Sticky счётчик "13 товаров"** — технически работает, но выглядит отвратительно. Нужен редизайн: либо встроить в контекст красиво, либо убрать
4. **Overlay `widget-display-area > *` получил `width: 100%`** — это может быть причиной раздувания. Нужно аккуратнее: max-width на контейнере, а не безлимитный width
5. **Фотки в comparison:** нет ограничения, рендерятся на полный размер контейнера

### Текущее состояние файлов с проблемами:
- `Widget.css` — убраны фиксированные размеры, нужно вернуть разумные max-width
- `ProductCardTemplate.css` — width 220px → 100%, нужно ограничить
- `Formation.css` — грид auto-fill minmax(200px,1fr) + sticky counter
- `Overlay.css` — `width: 100%` на дочерних, может нужен max-width
- `FormationRenderer.jsx` — formation-wrapper + formation-status + lazy load

---

## Параллелизация catalog_search — 2026-02-19

DB-запросы в `catalog_search` через `errgroup`: embedding, keyword и vector теперь параллельно.

- **catalog_search:** 7200ms → **1194ms** (×6)
- **Pipeline total:** ~15s → **2949ms** (×5, включая LLM)
- 3 фазы: embedding+keyword параллельно → vector параллельно → RRF merge
- Каждая горутина пишет в свою переменную, SpanCollector thread-safe

Коммит: `03f704a`

---

## Compact Digest + One-Time Delivery — 2026-02-19

Дайджест каталога: ~2000 токенов → ~650 токенов. Доставка один раз при старте сессии, кешируется Anthropic.

- Новая структура: CategoryTree + SharedFilters + TopBrands(30) + TopIngredients(30)
- `ToPromptText()` — ультракомпактный формат
- Вставка `<catalog>` блока в `conversation_history` при `session/init`
- Убрана per-turn загрузка дайджеста из agent1

---

## PIM Catalog Redesign — 2026-02-18

Каталог переведён с JSONB на структурированные PIM-колонки + справочник ингредиентов + типизированные фильтры.

- 19 новых колонок на `master_products` (product_form, texture, skin_type[], concern[], key_ingredients[] и т.д.)
- 2 таблицы: `ingredients` (4705 записей) + `product_ingredients` (27318 связей)
- Enrichment V2: Haiku → 18 структурированных полей, 961/962 продуктов, $1.81
- Typed search filters в `catalog_search` вместо generic JSONB
- Embeddings пересобраны из чистых PIM-данных

---

## Catalog Enrichment — 2026-02-15

LLM-классификация 967 товаров heybabes. Claude Haiku по закрытым спискам: категория, форма, тип кожи, проблема, ингредиенты.

- Флоу: crawl JSON → LLM enrichment (батчи по 10, 5 воркеров) → enriched JSON → import → БД
- 965/967 обогащено, $1.06, ~2 мин
- 24 категории (4 корня + 20 листьев), deterministic UUID
- Embedding text расширен enriched полями

---

## Web Crawler — 2026-02-15

Standalone Go crawler для heybabescosmetics.com. Sitemap → продуктовые страницы → структурированный JSON.

- JSON-LD parsing + HTML accordion parsing + description splitting
- 967 товаров, 62 бренда, 30 категорий за ~15 сек
- Атрибуты: description, ingredients, how_to_use, volume, skin_type, benefits

---

## Japanese Stepper — 2026-02-13

Степпер переехал в чат-колонку. Весь UI стал прозрачным (ghostly minimal) по макету Pencil.

- Blur backdrop: `backdrop-filter: blur(12px)`
- Chat column полностью прозрачная, вертикальное центрирование
- Toggle-кнопка с градиентом (открытие/закрытие)
- Stepper рендерит `<nav>` напрямую внутри ChatPanel

---

## Test Coverage — 5 слоёв — 2026-02-13

~125 новых тестов, 13 новых файлов. 4 из 5 слоёв реализованы.

- **Layer 1** — Domain (49 тестов): cost, spans, formation, RRF — всё проходит
- **Layer 2** — DB Integration (22+ тестов): tenant CRUD, products, sessions — проходит
- **Layer 3** — API Smoke (18 тестов): health, session flow, CORS, middleware — проходит
- **Layer 4** — Usecase Integration (12 тестов): написаны, компилируются, не прогнаны
- **Layer 5** — LLM Integration: не тронут
- Фикс ночной активности Neon: retention 30min→6h, MinConns=0, PERSIST_LOGS opt-in

---

## Logging — Full Coverage — 2026-02-12

Полное покрытие логами: chat backend, admin backend, оба фронтенда. Каждый HTTP запрос = waterfall trace.

- Postgres `request_logs` + retention 72h
- Logger: `With()`, `FromContext()`, context keys
- HTTP Middleware: UUID request_id, SpanCollector, response capture
- Span инструментация всех слоёв (~20 adapter spans)
- Admin backend: 5 handlers + 2 adapters
- Frontend: `logger.js` с API timing

---

## Adjacent Templates — 2026-02-12

N formations → 1 template + raw entities. ~68% payload reduction.

- `BuildTemplateFormation`: шаблон с `fieldName` на атомах, 1 вызов вместо N
- `fillFormation.js`: фронт заполняет template данными при клике
- `adjacentFormations` → `adjacentTemplates` + `entities` в response
- Bugfix: `buildDetailFormation` не ставил Config → agent1 не видел detail view

---

## Instant Navigation — 2026-02-12

Back и Expand без round-trip к серверу.

- `useFormationStack` hook: push/pop для instant back
- `backgroundSync`: fire-and-forget POST для sync backend state
- `sessionCache`: formationStack в localStorage, переживает F5
- **Метрики:** Back/Expand 100-300ms → <16ms

---

## Catalog Evolution — 2026-02-12

Три структурных изменения: stock table, services tables, tags.

- **Stock:** отдельная таблица `catalog.stock`, bulk update API
- **Services:** `master_services` + `services`, full CRUD, vector search, RRF merge
- **Tags:** JSONB + GIN на products и services
- Найдено и исправлено 4 бага при верификации на живой БД

---

## Alpha 0.0.2 — 2026-02-11

Widget auto-detection fix + Admin Widget page.

- Фикс: `document.currentScript` = null для динамических скриптов → fallback поиск по `src`
- Админка: страница `/widget` с embed code и кнопкой Copy
- Backend: `GET /admin/api/tenant`, `GET /admin/api/widget-config`

---

## Alpha 0.0.1 — Embeddable Widget — 2026-02-11

Фронтенд превращён в встраиваемый виджет. Один `<script>` тег → AI-чат.

- Shadow DOM, полная изоляция стилей
- `<script src="keepstar.one/widget.js" data-tenant="nike">`
- API Client: `X-Tenant-Slug` header, `setTenantSlug()`/`setApiBaseUrl()`
- Build: `widget.js` IIFE, 72KB gzip

---

## Railway Deploy — 2026-02-11

Два Railway service из одного GitHub repo. Go раздаёт React SPA + API.

- Multi-stage Dockerfile: Node 22 → Go 1.24 → Alpine 3.21
- SPA file server: catch-all с fallback на `index.html`
- Фикс: embedding ошибка глоталась молча → `meta["embed_error"]`

---

## Session Init + Tenant Seed — 2026-02-10

При открытии чата — init запрос создаёт сессию, резолвит тенант, возвращает greeting.

- `POST /api/v1/session/init` — создаёт state + session, seeds tenant
- Frontend: `initSession()` на mount → greeting как assistant message
- Pipeline и Agent1: get-or-create, дубликации нет

---

## Admin Panel MVP — 2026-02-10

Отдельный проект `project_admin/` — админка для загрузки каталогов. Go + React, гексагоналка, общая Postgres БД.

- **Auth:** signup → tenant + user + JWT, login, middleware
- **Catalog CRUD:** ListProducts, GetProduct, UpdateProduct, GetCategories
- **Import:** JSON upload → async background → embedding → digest
- **Settings:** TenantSettings в JSONB
- **Frontend:** Login/Signup, PIM-таблица, Import с progress, Settings

---

## Technical Debt Cleanup — 2026-02-10

- `errors.Is(err, pgx.ErrNoRows)` вместо string matching
- `mergeProductWithMaster()` helper — дедупликация ~70 строк
- Все `json.Marshal/Unmarshal` с обработкой ошибок
- `templateUtils.js` + `ImageCarousel.jsx` — дедупликация фронта
- Удалено 891 строка мёртвого кода

---

## Search Relevance — Digest + RRF Tuning — 2026-02-08

Catalog Digest для Agent1 + VectorFilter (brand/category) + RRF keyword weight boost.

- `CatalogDigest`: pre-computed мета-схема каталога (категории, параметры, бренды)
- Agent1 получает `<catalog>` + `<state>` блоки вокруг запроса
- RRF: keyword weight 1.5× default, 2.0× при structured filters
- VectorSearch pre-filter по brand/category перед cosine ranking

---

## Pipeline Span Waterfall — 2026-02-07

Waterfall tracing для всего pipeline.

- `SpanCollector` (thread-safe), dot-separated naming
- Anthropic adapter: `{stage}.llm`, `{stage}.llm.ttfb` через `httptrace`
- CatalogSearch: `{stage}.tool.embed`, `{stage}.tool.sql`, `{stage}.tool.vector`
- Debug page: горизонтальный waterfall timeline

---

## Vector Search — Hybrid — 2026-02-07

Keyword SQL + semantic pgvector + RRF merge.

- `EmbeddingPort` → OpenAI adapter
- `embedding vector(384)` + HNSW index на `master_products`
- `CatalogSearchTool`: hybrid search мета-тул
- Normalizer удалён — vector embeddings покрывают мультиязычность

---

## Design System Integration — 2026-02-06

6 типов атомов + freestyle tool + Agent2 rework. **UNSTABLE.**

- Atom types: text, number, image, icon, video, audio
- `tool_freestyle.go` — стиль и display overrides
- Agent1/Agent2 tool isolation: search_* vs render_*
- ToolContext вместо bare sessionID

---

## Bugfix: E2E Pipeline — 2026-02-04

- Search: ILIKE разбивает запрос на слова с OR
- Conversation history: добавлен `tool_result` (Anthropic требует user→tool_use→tool_result)
- Cache control: исправлена потеря полей при конвертации contentBlock
- Cache threshold: 10→20 padding tools, cache hit rate 91.6%, LLM 2685ms→698ms

---

## Zone-based State Management — 2026-02-04

Дельты по зонам вместо full-state UpdateState.

- 4 zone-write метода: UpdateData, UpdateTemplate, UpdateView, AppendConversation
- `Delta.TurnID` — группировка по Turn'ам
- Agent1/Agent2 через zone-write, UpdateState только для rollback
- 15 тестов (unit + usecase + integration)

---

## Prompt Caching — 2026-02-04

Anthropic prompt caching Phase 1 + Phase 2.

- `ChatWithToolsCached` с cache_control на tools, system, conversation
- `conversation_history JSONB` в state для multi-turn cache
- Cache pricing: write ×1.25, read ×0.1
- Padding tools для порога 4096 токенов Haiku
- Cache hit rate 91.6%

---

## Drill-Down Navigation — 2026-02-04

- Expand/Back usecases + handlers
- Detail presets: `product_detail`, `service_detail`
- ViewStack: push on expand, pop on back
- Frontend: `BackButton` component

---

## Delta State Management — 2026-02-03

- Delta: Source, ActorID, DeltaType, Path
- ViewStack для back/forward навигации
- Reconstruct state at any step, Rollback to previous step
- 10 integration тестов

---

## Session TTL Fix — 2026-02-03

- Sessions expire после 5 мин inactivity
- `domain.SessionTTL` — single source of truth
- Frontend: `status: "closed"` → clear localStorage

---

## Architecture Refactoring — 2026-02-03

- Remove unused SearchPort
- Deduplicate convertToFormation, tool_render_preset (386→320 lines)
- Remove ExecuteLegacy from Agent2
- Tenant middleware + proper context flow

---

## Entity Types + Preset System — 2026-02-03

- EntityType: product, service
- PresetRegistry: product_grid, product_card, product_compact, service_card, service_list
- RenderProductPresetTool, RenderServicePresetTool
- ServiceCardTemplate.jsx

---

## Chat Overlay + Widget Rendering — 2026-02-03

- Backdrop overlay + chat справа + widgets слева
- Animations: backdrop-fade-in, chat-slide-in, widget-fade-in
- ProductCardTemplate: slot-based atoms (hero, badge, title, price, secondary)
- ImageCarousel, AtomChip, expandable secondary

---

## Two-Agent Pipeline — 2026-02-02

4 фазы: State Storage → Tool Caller → Template Builder → Frontend Rendering.

- **State:** SessionState, Delta, StateMeta в PostgreSQL JSONB
- **Agent1:** query → LLM → tool call → state update
- **Agent2:** meta → LLM → FormationTemplate → ApplyTemplate
- **Frontend:** FormationRenderer (grid/carousel/single/list), AtomRenderer
- **Debug:** `/debug/session/` с метриками (время, токены, стоимость)
- **Pipeline API:** `POST /api/v1/pipeline`

---

## Multi-tenant Product Catalog — 2026-02-02

- Domain: Tenant, Category, MasterProduct, Product
- CatalogPort + PostgreSQL adapter с master/tenant merging
- Миграции: catalog schema (tenants, categories, master_products, products)
- Seed: Nike, Sportmaster + 8 кроссовок
- Frontend: getProducts(), ProductGrid

---

## Neon PostgreSQL + Chat Hexagonal — 2026-02-01

- PostgreSQL adapter (pgxpool): CachePort + EventPort
- Auto-migrations, session TTL 10 мин, graceful degradation
- Hexagonal architecture: domain, ports, adapters, usecases, handlers
- Frontend: session persistence в localStorage

---

## Initial Architecture — 2025-01-29

- Hexagonal backend + feature-sliced frontend (stubs)
- Expert system: expertise.yaml для backend и frontend
- Dev-inspector для отладки UI
- Product Manifesto драфт
