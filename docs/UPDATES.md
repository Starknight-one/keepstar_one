# Updates

Лог изменений проекта Keepstar.

---

## Alpha 0.0.6 — V2 Engine Critical Fixes — 2026-03-18

Результат тестирования 15 запросов на production. 4 критических бага найдены и исправлены.

### Fix 1: TenantSlug → tenant_id mismatch (fieldDefCount=0 ВСЕГДА)

**Проблема**: `field_definition_adapter.go` делал `WHERE tenant_id = $1` со slug строкой ("hey-babes-cosmetics"), а `tenant_id` — UUID. Результат: 0 field definitions → движок не знал поля каталога → detail view неполный (9 из 13 полей), preset не выбирался.

**Фикс**: SQL теперь делает JOIN на `catalog.tenants` и матчит `fd.tenant_id::text = $1 OR t.slug = $1`. Работает и с UUID и со slug.

**Файл**: `adapters/postgres/field_definition_adapter.go` — оба метода (List + Get)

### Fix 2: C1 Rule удаляла явно запрошенные show-поля

**Проблема**: Cross-widget constraint C1 удаляла поля присутствующие менее чем в 70% виджетов. Если у части продуктов нет rating/description (null), C1 удаляла их из ВСЕХ виджетов — даже когда Agent2 явно запросил через show. Пример: "топ-5 с рейтингом и описанием" → 4 поля вместо 6.

**Фикс**: `applyCrossWidgetV2Constraints` принимает `protectedFields` (show-поля агента). `normalizeFieldSetV2` всегда сохраняет protected поля. `engine_v2.go` прокидывает `input.Instructions.Show` в constraints.

**Файлы**: `engine/rules.go`, `engine/engine_v2.go`

### Fix 3: Agent2 промпт — show vs hide семантика

**Проблема**: Agent2 слал `show=["name","price"]` когда пользователь говорил "ТОЛЬКО name и price". Show additive (добавляет к дефолтам) → images/brand/rating оставались. Также "крупными карточками" → comparison+limit=3 вместо grid+size=large.

**Фикс**: Добавлен CRITICAL блок в оба промпта (v1/v2):
- "только X и Y" → hide всё остальное (не show)
- "крупнее/крупными карточками" → size: "large" (не comparison)
- Новые примеры для обоих кейсов

**Файл**: `prompts/prompt_compose_widgets.go` — Agent2ToolSystemPrompt + Agent2ToolSystemPromptV2

### Fix 4: Двойные hero-картинки в карточках

**Проблема**: GenericCardV2Template рендерил ImageCarousel (фото #1), затем LayoutTreeRenderer рендерил тот же hero node из layout tree (фото #2). skipHero фильтр не срабатывал надёжно.

**Фикс**: ImageCarousel рендерится ТОЛЬКО в fallback (без layout tree). При наличии layout tree — LayoutTreeRenderer единственный источник hero. Like кнопка вынесена как absolute overlay. skipHero механизм удалён.

**Файлы**:
- `frontend/src/entities/widget/templates/GenericCardV2Template.jsx`
- `frontend/src/entities/widget/templates/LayoutTreeRenderer.jsx`

### Документация

- `docs/V2_ENGINE_ISSUES.md` — Known Issues: MaxFields как концепция, Agent2 интерпретация, Comparison preset

### Как тестировать

Сессия из 15 запросов (одна цепочка):

**Базовый поиск + авторезолв:**
1. "Привет, покажи кремы для лица" → grid, small, images+name+price
2. "Покажи их списком" → list
3. "Покажи детально первый товар" → single, large, 9-13 полей

**Show/Hide:**
4. "Покажи только названия и цены" → name+price БЕЗ images
5. "Добавь рейтинг" → name+price+rating
6. "Убери цены" → name+rating
7. "Покажи всё как было" → дефолты

**Визуальные примитивы:**
8. "Покажи крупными карточками" → size=large, НЕ comparison
9. "Покажи каруселью" → carousel с картинками
10. "Покажи таблицей" → table

**Сложные комбинации:**
11. "Покажи топ-5 с рейтингом, брендом и описанием, крупно" → limit=5, 6 полей, без двойных фоток
12. "Сравни первые 3" → comparison

**Фильтрация + бренд:**
13. "Покажи только COSRX" → state_filter, 3-5 товаров
14. "Покажи их с составом и типом кожи" → show добавляет keyIngredients+skinType

**Стресс:**
15. "Сравни два самых дешёвых — только цена, бренд и состав" → comparison, price+brand+keyIngredients

---

## Alpha 0.0.5 — Pipeline Traces + Admin UI + Engine V2 Bugfixes — 2026-03-18

### Часть 1: Обогащение трейсов (chat backend)

Agent2 трейсы были неполными — не было tool input, system prompt, engine breakdown. Теперь записывается полная цепочка.

**Новые поля в `AgentTrace` (agent2):**
- `SystemPrompt` / `SystemPromptChars` — какой промпт использовался (v1/v2)
- `ToolInput` — JSON параметров visual_assembly (THE KEY DATA)
- `ToolBreakdown` — что решил движок (preset, layout, size, entityCount, warnings)
- `MessageCount` / `ToolDefCount`

**Обогащённая FormationTrace:**
- `Widgets []WidgetTrace` — ID, template, size, atomCount, entityRef для каждого виджета
- `FullJSON json.RawMessage` — полная formation (до 100KB)

**Engine Metadata:**
- V1 `writeFormation()` и V2 `executeV2()` теперь возвращают `ToolResult.Metadata` с breakdown (engineVersion, preset, layout, size, entityType, entityCount, widgetCount, warnings)

**Файлы:**
- `domain/trace_entity.go` — `WidgetTrace`, `FormationTrace.FullJSON`
- `usecases/agent2_execute.go` — новые поля + `json.Marshal(toolCall.Input)` + `result.Metadata`
- `usecases/pipeline_execute.go` — wire всех новых полей Agent2 + widget details + fullJSON
- `tools/tool_visual_assembly.go` — Metadata в return для v1 и v2

### Часть 2: Admin API для трейсов

Админ-бэкенд читает ту же таблицу `pipeline_traces` (shared Neon DB).

**Endpoints:**
- `GET /admin/api/traces?limit=50&offset=0` — список трейсов (пагинация + total)
- `GET /admin/api/traces/{id}` — полный trace JSON
- `GET /admin/api/sessions` — список chat sessions (active/closed)
- `POST /admin/api/sessions/kill` — убить сессию `{"sessionId": "..."}`

**Файлы:**
- `project_admin/backend/internal/adapters/postgres/postgres_trace.go` — TraceAdapter (List, Get, ListSessions, KillSession)
- `project_admin/backend/internal/handlers/handler_traces.go` — TracesHandler
- `project_admin/backend/internal/adapters/postgres/catalog_migrations.go` — idempotent pipeline_traces migration
- `project_admin/backend/cmd/server/main.go` — wiring + routes

### Часть 3: Admin Frontend — Traces UI

**TracesPage** (`/traces`):
- Sessions panel наверху — active sessions с кнопкой Kill, closed в `<details>`
- Таблица трейсов: Time, Query, Agent1/Agent2 Tool, Mode, Widgets, Duration, Cost, Status
- Клик → детальный вид

**TraceDetail** (`/traces/:id`):
- Hero-карточка с запросом и метриками
- Цветные collapsible секции (синяя=Agent1, фиолетовая=Agent2, зелёная=Formation)
- Tokens bar (визуальная полоска input/output)
- JSON блоки: тёмная тема, collapsible, auto-prettyprint, Tool Input + Engine Breakdown раскрыты по умолчанию
- Таблицы: deltas, widget details
- Waterfall: цветные span bars

**Файлы:**
- `project_admin/frontend/src/features/traces/TracesPage.jsx`
- `project_admin/frontend/src/features/traces/TraceDetail.jsx`
- `project_admin/frontend/src/features/traces/traces.css`
- `project_admin/frontend/src/App.jsx` — routes
- `project_admin/frontend/src/features/layout/DashboardLayout.jsx` — nav link (Activity icon)

### Часть 4: Bugfixes

**Лайки/корзина не очищались между сессиями:**
- `actionStateCache` (24h TTL) жил дольше сессии (30min TTL)
- Новая сессия подхватывала старые лайки/корзину
- Фикс: `clearSessionCache()` теперь вызывает `clearActionCache()` автоматически
- Файлы: `sessionCache.js`, `ChatPanel.jsx`

**V2 Engine — 3 критических бага:**

1. **`TenantSlug` не передавался в ToolContext** (agent2_execute.go строка 301) → `fieldDefCount: 0` ВСЕГДА, field definitions никогда не загружались из БД. Фикс: добавлен `TenantSlug: req.TenantSlug`.

2. **Size всегда `small` для detail view**: `AutoResolve` вызывался с полным entityCount (23), даже когда agent послал `limit: 1`. Получалось 23 → small (MaxFields=3). Фикс: V2 engine теперь использует `effectiveCount = min(limit, entityCount)` перед AutoResolve. Один товар → `large`, 10 полей.

3. **MaxFields обрезал явные show поля**: Agent просил `show: ["name","price","rating","brand"]` (4 поля), но MaxFields=3 (для 23 items) молча обрезал до 3. Фикс: если agent явно указал `show`, `MaxFields` поднимается до `len(show)`.

**Также:** `CHAT_API_URL` добавлен в `.env` (embed code генерировал placeholder `YOUR_CHAT_SERVER`).

### Как тестировать V2 движок

Убедись что в `.env` стоит `ENGINE_VERSION=v2` и `AGENT2_PROMPT_VERSION=v2`.

**Базовые запросы (проверка авторезолва):**
1. "покажи кремы для лица" → grid, small cards, 3 поля (images, name, price)
2. "покажи их списком" → list layout, те же данные
3. "покажи детально первый товар" → single, LARGE card, 10+ полей (описание, ингредиенты, рейтинг...)
4. "сравни первые 3" → comparison layout

**Show/hide/order (проверка override-ов агентом):**
5. "покажи только названия и цены" → show: [name, price], NO images
6. "добавь рейтинг и бренд" → show добавляет поля к текущим
7. "убери фотки" → hide: [images]
8. "покажи цену первой, потом название" → order: [price, name]

**Direction/size/layout (визуальные примитивы):**
9. "покажи горизонтальными карточками" → direction: horizontal (image left, content right)
10. "покажи крупными карточками" → size: large
11. "покажи каруселью" → layout: carousel
12. "покажи таблицей" → layout: table

**Кастомные комбинации (стресс-тест агента):**
13. "покажи топ-5 по цене с рейтингом, брендом и описанием, крупно" → limit:5, show:[price,rating,brand,description], size:large
14. "сравни два самых дешёвых крема — только цена, бренд и состав" → comparison, limit:2, show:[price,brand,keyIngredients]
15. "покажи все кремы COSRX одной строкой — имя и цена" → layout:list, show:[name,price], filter by brand

**На что смотреть в трейсах:**
- Agent2 → Tool Input: что агент реально послал движку (layout, show, size, limit)
- Agent2 → Engine Breakdown: что решил движок (engineVersion, preset, size, widgetCount, fieldDefCount)
- Formation → Widgets table: сколько виджетов, какой size, сколько атомов
- Formation → Full JSON: полная структура (для отладки)
- Waterfall: сколько времени на LLM vs tool execution

### Известные ограничения
- Трейсы показываются все (без фильтрации по тенанту) — для мультитенант нужна колонка `tenant_slug` в `pipeline_traces`
- Старые трейсы (до Alpha 0.0.5) не содержат Agent2 toolInput/systemPrompt/breakdown — показывают null
- `fieldDefCount: 0` до редеплоя chat backend (TenantSlug фикс)

---

## Alpha 0.0.4 — Engine V2: Metadata-Driven Visual Assembly — 2026-03-18

Полная замена visual assembly engine. 6 фаз, ~3500 LOC нового кода (backend + frontend).
Активируется через `ENGINE_VERSION=v2` + `AGENT2_PROMPT_VERSION=v2` в `.env`. По умолчанию — v1.
Полная спецификация: `docs/ENGINE_V2_IMPLEMENTATION.md`.

### Что решает:
- Хардкод 14 полей → DB-driven `field_definitions` (22 поля, расширяемо per-tenant)
- Единый `display` string → раздельные `TextStyle` (типография) + `WrapperConfig` (контейнер)
- Плоские `Zone[]` → рекурсивное `LayoutNode` дерево (row/column/flow/span)
- Пиксели в промптах → семантические токены (`fontSize: "lg"`, `fontWeight: "bold"`)
- Switch-case field access → `ProductToMap`/`ServiceToMap` + `GenericFieldGetter`

### Фазы:
- **Phase 0**: `catalog.field_definitions` таблица + seed, `FieldDefinitionPort`, postgres adapter, generic field access
- **Phase 1**: `AtomV2` (textStyle/wrapper), `Rigidity` (locked/preferred/flexible), `LayoutNode`, `DesignTokensV2`
- **Phase 2**: `EngineV2.Execute()` — 10-step pipeline, `AutoLayout`, `BudgetDown`/`NeedsUp`, 15 правил на 5 стыках
- **Phase 3**: `executeV2` в visual_assembly tool, `convertV1ParamsToV2`, `PresetV2Registry` (5 пресетов)
- **Phase 4**: `Agent2ToolSystemPromptV2` (токены+лейблы), `BuildAgent2ToolPromptV2(fieldLabels)`, feature flag
- **Phase 5**: `AtomV2Renderer.jsx`, `LayoutTreeRenderer.jsx`, `GenericCardV2Template.jsx`, v2 routing
- **Phase 6**: `NewRegistryV2`, wiring в `main.go`, env-based activation

### Ключевые файлы (куда смотреть для доработки):

**Backend engine (бизнес-логика V2):**
- `engine/engine_v2.go` — 10-step pipeline, `DisplayToTextStyleWrapper`
- `engine/auto_layout.go` — `AutoLayout` (группировка атомов в layout tree)
- `engine/rules.go` — 15 constraint rules (atom, widget, cross-widget, junction)
- `engine/layout_pass.go` — `BudgetDown`/`NeedsUp` (двупроходный layout)
- `engine/tokens.go` — `DesignTokensV2` (fontSize, fontWeight, spacing)
- `engine/compat.go` — v2→v1 конвертеры (AtomV2ToLegacy, LayoutToZones)

**Backend tool/prompt:**
- `tools/tool_visual_assembly.go` — `executeV2()` (строки ~645+), `convertV1ParamsToV2`
- `prompts/prompt_compose_widgets.go` — `Agent2ToolSystemPromptV2`, `BuildAgent2ToolPromptV2`
- `usecases/agent2_execute.go` — `NewAgent2ExecuteUseCaseV2`, `loadFieldLabels`

**Backend domain/infra:**
- `domain/atom_entity.go` — `AtomV2`, `Rigidity`, `TextStyle`, `WrapperConfig`
- `domain/layout_entity.go` — `LayoutNode`, `LayoutChild`, `ActionDef`, `WidgetStates`
- `domain/preset_v2_entity.go` — `PresetV2`, `PresetV2Field`
- `presets/preset_v2.go` — `PresetV2Registry` (5 пресетов)
- `ports/field_definition_port.go` — `FieldDefinitionPort`
- `adapters/postgres/field_definition_adapter.go` — postgres impl

**Frontend V2 rendering:**
- `entities/atom/AtomV2Renderer.jsx` — textStyle→CSS, wrapper→component (10 wrapper types)
- `entities/atom/AtomV2.css` — стили для всех wrappers + layout tree
- `entities/widget/templates/LayoutTreeRenderer.jsx` — рекурсивный рендер LayoutNode
- `entities/widget/templates/GenericCardV2Template.jsx` — v2 card template
- `entities/widget/WidgetRenderer.jsx` — v2 routing (`widget.layout || widget.atomsV2`)
- `widget.jsx` — **ВАЖНО**: CSS для Shadow DOM, все `?inline` импорты здесь

### Известные проблемы (исправлено):
- CSS `AtomV2.css` не попадал в Shadow DOM бандл → добавлен `?inline` импорт в `widget.jsx`
- Двойная hero-картинка → `LayoutTreeRenderer` теперь пропускает hero nodes через `skipHero`
- Hover ломал фон → убран `var(--hover-background, inherit)`

### Тесты:
- 12 тестов в `engine/engine_v2_test.go`
- Pre-existing failures в tools/usecases (UpdateActions mock) — НЕ связано с V2

---

## Alpha 0.0.4a — Actions System + Widget Templates Polish — 2026-03-18

Actions (like/cart) + улучшения шаблонов виджетов.

### Что сделано:
- **Actions system:** like, cart — `ActionHandler`, `ActionContext` (frontend), `action_execute.go`
- **Widget templates:** favorite button, cart button, image carousel во всех карточках
- **State:** `UpdateActions` port + postgres adapter + migration
- **Agent1:** minor prompt + test adjustments
- **Frontend:** backgroundSync, sessionCache, apiClient improvements

### Файлы:
- `handlers/handler_action.go`, `usecases/action_execute.go`, `usecases/action_view.go`
- `frontend/src/features/actions/` — ActionContext, ActionToolbar, CartView, LikedView, actionCache

---

## Admin Widget Embed Fix — 2026-03-18

Embed code в админке генерировался без `data-api` и с относительным путём `/widget.js`.

### Что исправлено:
- Admin config: добавлен `CHAT_API_URL` env var
- Widget-config endpoint возвращает `chatApiUrl`
- WidgetPage генерирует JS-сниппет с `createElement` + полными URL + `data-api`
- **ВАЖНО**: На Railway для admin нужна переменная `CHAT_API_URL=https://chat-production-005e.up.railway.app/api/v1`

### Файлы:
- `project_admin/backend/internal/config/config.go` — `ChatAPIURL`
- `project_admin/backend/cmd/server/main.go` — widget-config endpoint
- `project_admin/frontend/src/features/widget/WidgetPage.jsx` — embed code generation

---

## Alpha 0.0.3 — Engine Layer Extraction — 2026-03-01

Рефакторинг: вынос бизнес-логики из `tools/` в `internal/engine/`. Гексагональная архитектура восстановлена — tools/ содержит только LLM tool адаптеры.

### Что сделано:
- **Новый пакет `internal/engine/`** (6 файлов, ~1400 LOC) — чистая бизнес-логика, импортирует только `domain/`
  - `defaults.go` — AutoResolve, field ranking, display validation, CalcGridConfig, GetDisplayMeta, BuildFieldConfigs
  - `constraints.go` — ApplyAtomConstraints, ApplyWidgetConstraints, ApplyCrossWidgetConstraints (30+ правил, 4 уровня)
  - `layout.go` — CalculateZones, DesignTokens (zone-based widget layout)
  - `field_types.go` — FieldTypeMap (field→AtomType/Subtype)
  - `formation.go` — BuildFormation, BuildTemplateFormation, BuildAtoms, ProductFieldGetter, ServiceFieldGetter
  - `assembly.go` — BuildVisualWidgets, BuildComposedFormation, ApplyPostProcessing, conditional styling
- **tools/ облегчён** — удалены 3 файла целиком (defaults_engine.go, constraints.go, layout_engine.go), tool_visual_assembly.go и tool_render_preset.go делегируют в engine/
- **Удалены дубли** — 90 LOC дублированных `productFieldGetter`/`serviceFieldGetter`/`nonEmpty` из `navigation_expand.go` + `productFieldGetterDebug` из `handler_debug.go`
- **Обновлены 8 consumer-файлов** — handlers, usecases, prompts теперь импортируют engine/ вместо tools/
- **Тесты перенесены** — formation_test.go и formation_fuzz_test.go (315k+ комбинаций) в engine/

### Граф зависимостей после:
```
domain/ ←── engine/ (только domain)
              ↑
       tools/ (domain + engine + ports + presets)
              ↑
   usecases/ (domain + engine + tools[Registry] + ports)
              ↑
   handlers/ (domain + engine + usecases + ports)
```

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
