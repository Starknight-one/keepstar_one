# Updates

Лог изменений проекта Keepstar.

---

## PIM Catalog Redesign — Structured Columns + Typed Search — 2026-02-18

Перевели каталог с JSONB-каши на нормальный PIM со структурированными колонками, справочником ингредиентов и типизированными фильтрами для агента.

**Ветка:** `feature/pim-catalog-redesign`
**Файлов:** 17, +1031 / -86 строк

### Database (auto-migrations)

- 19 новых колонок на `catalog.master_products`: `short_name`, `product_form`, `texture`, `routine_step`, `skin_type[]`, `concern[]`, `key_ingredients[]`, `target_area[]`, `free_from[]`, `marketing_claim`, `benefits[]`, `volume`, `inci_text`, `enrichment_version`, etc.
- 2 новые таблицы: `catalog.ingredients` (справочник INCI), `catalog.product_ingredients` (junction)
- 10 индексов: 5 B-tree + 5 GIN

### Enrichment v2 (admin backend)

Новый LLM промпт (18 структурированных полей из закрытых списков) + `EnrichFromDB` use case. Читает master_products по tenant → батчи по 10, 5 воркеров → Haiku API → парсит JSON → записывает в PIM-колонки + `enrichment_version = 2`.

- `POST /admin/api/catalog/enrich-v2` — принимает `{"tenantId": "..."}`
- Стоимость первого прогона: $1.81 за 962 товара

### Typed Search Filters (chat backend)

Заменены generic JSONB фильтры в `catalog_search` tool на типизированные: `product_form`, `skin_type`, `concern`, `key_ingredient`, `routine_step`, `texture`, `target_area` — все с enum-значениями в JSON schema.

SQL: `mp.product_form = $N`, `$N = ANY(mp.skin_type)`, etc.

### Embeddings (чистый текст)

`buildEmbeddingText()` для enrichment_version >= 2: ~30 токенов из PIM-полей вместо ~500-1000 из каши.

### CLI утилиты (новые)

- `cmd/seed-ingredients/` — парсинг INCI из attributes, создание справочника, LLM обогащение (name_ru + function)
- `cmd/rebuild-embeddings/` — пересборка векторов из чистых PIM-данных

### Статус

- [x] Миграции применены (19 колонок + 2 таблицы + 10 индексов)
- [x] Enrichment v2 endpoint работает, Haiku отвечает
- [ ] **Баг SKU matching:** 125/962 записались — LLM возвращает все продукты но SKU не матчатся при записи. Диагностика добавлена
- [ ] seed-ingredients
- [ ] rebuild-embeddings
- [ ] Проверка typed search

---

## Catalog Enrichment — LLM Product Classification — 2026-02-15

Enrichment пайплайн для 967 товаров из краулера heybabes. LLM (Claude Haiku 4.5) классифицирует каждый продукт по закрытым спискам: категория, форма, тип кожи, проблема, ключевые ингредиенты. Результат перезаписывается в crawl JSON → затем импорт в БД.

### Флоу

```
crawl JSON → POST /catalog/enrich (LLM, 2 мин) → enriched JSON → POST /catalog/import → БД → embed → digest
```

Enrichment работает на **файле**, не на БД. Читает JSON → батчит через LLM → перезаписывает JSON с добавленными атрибутами. В БД попадает уже обогащённый "эталонный" каталог.

### Seed категорий (`cmd/seed/`)

24 категории (4 корня + 20 листьев), deterministic UUID (uuid5 от slug), `ON CONFLICT DO NOTHING`.

```
face-care (10 дочерних: cleansing, toning, exfoliation, serums, moisturizing, suncare, masks, spot-treatment, essences, lip-care)
makeup (4: makeup-face, makeup-eyes, makeup-lips, makeup-setting)
body (3: body-cleansing, body-moisturizing, body-fragrance)
hair (3: hair-shampoo, hair-conditioner, hair-treatment)
```

### Enrichment (`POST /admin/api/catalog/enrich`)

- Принимает `{"filePath": "/path/to/crawl.json"}`
- Батчи по 10 товаров, 5 параллельных горутин → ~97 API вызовов
- System prompt содержит дерево категорий + закрытые enum-списки (product_form: 20, skin_type: 7, concern: 15, key_ingredients: 25)
- LLM возвращает JSON array — парсится и мержится обратно в каждый продукт
- Прогресс и стоимость доступны через `GET /admin/api/catalog/enrich`

### Результат первого прогона (967 товаров)

| Метрика | Значение |
|---|---|
| Обогащено | 965/967 |
| Input tokens | 858K |
| Output tokens | 93K |
| Стоимость | **$1.06** |
| Время | ~2 мин |
| Модель | claude-haiku-4-5-20251001 |

### Расширения импорта

- `ImportItem.CategorySlug` — прямой lookup по slug из seed-дерева (без `slugify`)
- Embedding text расширен: + product_form, skin_type, concern, key_ingredients, benefits, how_to_use, ingredients, active_ingredients
- Поддержка `[]string` атрибутов (join через ", ")

### Конфигурация

```
ANTHROPIC_API_KEY=sk-ant-...   # включает enrichment
ENRICHMENT_MODEL=claude-haiku-4-5-20251001  # default
```

### Известные проблемы

- **path вместо leaf slug**: LLM иногда возвращает `face-care/toning` вместо `toning` (~30 из 967). Fallback на slugify(category) — создаёт мусорную категорию. Нужен фикс в промпте.
- **Digest неполный**: собирает только totalProducts, categories, brands. Не включает enriched фасеты (product_form, skin_type, concern, key_ingredients). Нужна доработка.
- **Стоимость выше ожидаемой**: $1.06 vs оценка $0.40. Причина — длинные INCI-списки ингредиентов. Оптимизация: обрезать ingredients до 200 символов, увеличить batch до 20.
- **Импорт медленный**: ~5 мин на 967 товаров (3 SQL запроса × 100ms RTT к Neon на каждый продукт). Нужен batch INSERT.

### Файлы

| Файл | Действие |
|---|---|
| `cmd/seed/main.go` | Создан — CLI seed категорий |
| `adapters/anthropic/enrichment_client.go` | Создан — HTTP клиент Anthropic Messages API |
| `ports/enrichment_port.go` | Создан — EnrichmentPort interface |
| `domain/enrichment.go` | Создан — EnrichmentInput/Output/Result/Job |
| `usecases/enrichment.go` | Создан — EnrichFile (файловый enrichment с прогресс-трекингом) |
| `handlers/handler_enrichment.go` | Создан — POST/GET /catalog/enrich |
| `config/config.go` | Изменён — + AnthropicAPIKey, EnrichmentModel, HasEnrichment() |
| `usecases/import.go` | Изменён — + CategorySlug, расширенный embedding text |
| `ports/catalog_port.go` | Изменён — + GetCategoryBySlug, GetMasterProductsForEnrichment, UpdateMasterProductEnrichment |
| `adapters/postgres/catalog_adapter.go` | Изменён — реализация 3 новых методов |
| `cmd/server/main.go` | Изменён — wiring enrichment client + handler + route |

---

## Web Crawler — Structured Product Extraction — 2026-02-15

Standalone Go crawler для heybabescosmetics.com. Парсит sitemap → продуктовые страницы → структурированный JSON для импорта в каталог.

### Crawler (`project_admin/backend/cmd/crawler/`)

- **Sitemap pipeline**: `sitemap.xml` → `sitemap-iblock-58.xml` → фильтрация `/catalog/` URLs
- **Concurrent crawling**: семафор + WaitGroup, настраиваемый concurrency/delay, progress bar с ETA
- **JSON-LD parsing**: `@type: Product` — name, SKU, brand, price, rating, images
- **HTML accordion parsing**: `row_PROP_*` (Состав/INCI), `row_USE` (Применение) — данные вне JSON-LD
- **Description splitting**: автоматическое разделение текста по маркерам на структурированные атрибуты
- **Output**: `ImportRequest` JSON, совместимый с `POST /admin/api/catalog/import`

### Извлекаемые атрибуты

| Атрибут | Источник | Покрытие |
|---|---|---|
| `description` | JSON-LD / HTML itemprop | 99% |
| `ingredients` (INCI) | HTML accordion `row_PROP_*` / description | 97% |
| `how_to_use` | HTML accordion `row_USE` / description | 93% |
| `volume` | Description маркер / имя продукта | 92% |
| `skin_type` | Description маркер "Подходит для:" | 80% |
| `benefits` | Description маркер "Преимущества:" | 79% |
| `active_ingredients` | Description маркер "Основные компоненты:" | 78% |

Двойная стратегия: description splitting (маркеры в тексте) + accordion override (HTML-блоки перезаписывают). Volume fallback: description → имя продукта.

### Результат первого краула

967 товаров, 62 бренда, 30 категорий. ~5000 товаров всего на сайте (sitemap). 15 concurrency — полный краул за ~15 сек.

**Файлы:** `project_admin/backend/cmd/crawler/main.go` (1 файл, ~560 строк)

---

## Japanese Stepper — Chat + Stepper + Blur Backdrop — 2026-02-13

Степпер переехал из отдельного сайдбара в чат-колонку. Весь UI стал прозрачным (ghostly minimal). Новая toggle-кнопка с градиентом. По макету Pencil «V1 — Ghostly Minimal».

### UI/Layout

- **Blur backdrop** — `linear-gradient` заменён на `rgba(0,0,0,0.3)` + `backdrop-filter: blur(12px)`. Весь контент под оверлеем мягко размыт.
- **Chat column** — полностью прозрачная. Убран белый `background`, `box-shadow`, `border-radius` у `.chat-container`. Сообщения, инпут, степпер — на прозрачном фоне поверх блюра.
- **Vertical centering** — два `.chat-spacer` (flex: 1) выше и ниже контента. Инпут чата всегда по центру экрана по вертикали. `.chat-history` и `.stepper` ограничены `max-height: 35vh`, скроллятся внутри.
- **Layout** — `[widget-display-area] [chat-area]`, степпер убран из overlay-уровня, живёт внутри ChatPanel после `<ChatInput>`.

### Ghostly стиль (по макету Pencil)

- **Messages** — user bubble: `rgba(0,0,0,0.03)`, rounded `16px 16px 4px 16px`. Assistant: без фона, просто текст `rgba(0,0,0,0.67)`.
- **Input** — pill shape, `border: none`, `background: rgba(0,0,0,0.025)`, placeholder `rgba(0,0,0,0.27)`.
- **Stepper** — полупрозрачные цвета: past dots `rgba(0,0,0,0.53)`, future `rgba(0,0,0,0.09)`, active ring `rgba(0,0,0,0.4)`. Разделитель `rgba(0,0,0,0.06)`.
- **Scrollbars** — скрыты на `.chat-history` и `.stepper` (`scrollbar-width: none` + `::-webkit-scrollbar { display: none }`).

### Toggle-кнопка (макет Pencil «Chat Toggle Button»)

- **Открытие** — bubble «Спроси меня!» (белый, rounded 16px, shadow) + градиентный круг 56px (`linear-gradient(225deg, #5BA4D9, #F0924A)`) с белой SVG иконкой ⚡ (zap).
- **Закрытие** — такой же градиентный круг с SVG ✕ внутри, в правом верхнем углу чат-колонки. Большая синяя кнопка скрыта когда чат открыт.

### Рефакторинг

- **Stepper.jsx** — убрана обёртка `<div className="stepper-sidebar">`, рендерит `<nav>` напрямую.
- **ChatPanel.jsx** — импортирует `<Stepper>`, содержит `handleStepperGoTo` (trail-логика). `onNavigationStateChange` больше не передаёт `history`/`currentIndex`/`goTo` наверх.
- **WidgetApp.jsx** — убран `import Stepper`, убраны `history`/`currentIndex`/`goTo` из `navState`.
- **Overlay.css** — удалены `.stepper-sidebar`, `@keyframes stepper-fade-in`. `.chat-area` получил фиксированную ширину 360px.

**Файлы:** 6 изменённых (Overlay.css, ChatPanel.css, ChatPanel.jsx, WidgetApp.jsx, Stepper.jsx, Stepper.css) + widget.css.

---

## Test Coverage — 5-Layer Strategy — 2026-02-13

Полное покрытие тестами chat backend. 5 слоёв, ~125 новых тестов, 13 новых файлов.

### Что реализовано

**Layer 1 — Domain Logic (49 тестов, 0 deps, <2 сек) — DONE, ALL PASS**
- `domain/llm_cost_test.go` (10) — все модели, cache multipliers, fallback
- `domain/span_test.go` (9) — concurrent safety, context round-trip
- `tools/formation_test.go` (20) — BuildFormation, BuildTemplateFormation, field getters
- `tools/rrf_merge_test.go` (10) — RRF merge, keyword weight, stable order

**Layer 2 — DB Integration (22+ тестов, DATABASE_URL) — DONE, ALL PASS**
- `postgres/postgres_catalog_integration_test.go` (15) — tenant CRUD, ListProducts (brand/price/search/sort/pagination), GetProduct, Digest generate+roundtrip
- `postgres/postgres_session_integration_test.go` (6) — FK constraint, session CRUD, delta steps, ViewStack push/pop, zone-write UpdateData

**Layer 3 — API Smoke (18 тестов, DATABASE_URL) — DONE, ALL PASS**
- `handlers/smoke_test.go` (9) — Health, Seed→GetSession, Expand→Back flow, error cases
- `handlers/middleware_test.go` (9) — CORS preflight, tenant from path/header, fallback

**Layer 4 — Usecase Integration (12 тестов, DATABASE_URL) — DONE, compiles, not yet run**
- `usecases/navigation_integration_test.go` (5) — DB-backed expand/back, deep stack (5 levels)
- `usecases/tool_execution_integration_test.go` (4) — catalog_search + render_product_preset с реальным registry
- `usecases/pipeline_mock_llm_test.go` (3) — MockLLMClient, TurnID grouping

**Layer 5 — LLM Integration — NOT TOUCHED (existing tests need rewrite)**

### Инфраструктура

- `internal/testutil/testutil.go` — TestDB, TestSession, TestStateWithProducts, SeedProducts, SeedServices, MockLLMClient
- `internal/adapters/postgres/shared_test.go` — TestMain с shared connection pool (один pool на весь пакет вместо per-test)
- `Makefile` — `test-unit`, `test-integration`, `test-usecase`, `test-llm`, `test-all` (автоподтяжка `.env`)

### Починки в существующем коде

- 7 сломанных `NewStateAdapter(client)` → `NewStateAdapter(client, log)` (usecases: state_rollback, cache, agent1_execute)
- 7 недостающих методов в mock'ах `tool_catalog_search_test.go` (CatalogPort расширился)

### Что осталось

1. **Скорость тестов** — `postgres_state_test.go` создаёт 14 отдельных DB connections (каждый ~4 сек TLS к Neon). Нужно перевести на shared client через `getSharedClient(t)` из `shared_test.go`. Паттерн уже готов, session/catalog тесты переведены.
2. **Layer 4 прогон** — написаны, компилируются, не прогнаны с DATABASE_URL
3. **Layer 5 переписать** — существующие LLM-тесты (agent1_execute_test, cache_test) логически слабые, нужен редизайн

### Аномалия Neon DB

На графиках Neon за ночь 12-13 февраля видна подозрительная активность:
- **Pooler client connections**: 2-6 активных соединений всю ночь (12:13 AM - 5:13 AM), хотя никто не работал
- **Rows inserted/updated**: всплески до 285 rows около 12:13 AM, затем периодическая активность (50-170 rows) до утра
- **Compute**: endpoint НЕ засыпал (autosuspend 5 min) — значит что-то держало соединения
- **Deadlocks**: 1 deadlock зафиксирован

**Расследование — причины найдены и устранены:**

1. **Retention loop на Railway** (главный виновник) — тикал каждые 30 мин, 3-4 DELETE/UPDATE запроса за тик. Neon видел активность → не засыпал. Фикс: `CleanupInterval: 30min → 6h`.
2. **Connection pool config** — `MinConns=2` + `HealthCheckPeriod=1min` держали 2 idle соединения и пинговали БД каждую минуту. Фикс: `MinConns=0`, `MaxConnIdleTime=5min`, `HealthCheckPeriod=5min`.
3. **Зомби-пулы от тестов** — `postgres_state_test.go` создаёт 14 отдельных пулов; при панике `defer client.Close()` не выполняется → пулы живут с health check. Фикс: перевод на shared client (TODO).
4. **Admin backend** — LogAdapter всегда on, писал каждый HTTP запрос в `request_logs`. Фикс: `PERSIST_LOGS=true` opt-in (аналогично chat backend).

**Правки:**
- `postgres_client.go` — MinConns=0, MaxConnIdleTime=5min, HealthCheckPeriod=5min
- `retention.go` — CleanupInterval=6h
- `project/backend/cmd/server/main.go` — PERSIST_LOGS opt-in для LogAdapter
- `project_admin/backend/cmd/server/main.go` — PERSIST_LOGS opt-in для LogAdapter

**Файлы:** 13 новых + 6 изменённых + Makefile + 4 hotfix

---

## Logging — Full Project Coverage — 2026-02-12

Полное покрытие логами всего проекта: chat backend, admin backend, оба фронтенда. Каждый HTTP запрос = waterfall trace с таймингами на каждом стыке. Логи хранятся в Postgres, чистятся раз в 3 дня.

### Что реализовано (7 фаз)

1. **Postgres `request_logs`** — таблица для персистентного хранения логов запросов. Оба бэкенда пишут в одну таблицу, различаясь полем `service` (chat/admin). Индексы на timestamp, session_id, errors, service.

2. **Logger extension** — `With()`, `FromContext()`, context keys (`request_id`, `session_id`, `tenant_slug`). Автоматическое обогащение логов полями из context.

3. **HTTP Middleware** — каждый запрос получает UUID `request_id`, `SpanCollector`, response capture. По завершении: JSON stdout log + async persist в Postgres. Health → Debug, errors → Error, rest → Info.

4. **Span инструментация всех слоёв** — 7 handler spans, 3 usecase spans, ~20 adapter spans (DB, LLM). Полный waterfall для любого запроса — от HTTP до каждого DB query. 16 raw `slog.Warn()` → injected logger.

5. **Retention** — `RequestLogMaxAge: 72h` в RetentionConfig. Автоматическая чистка в cleanup loop.

6. **Admin backend** — logger + spans в 5 handlers (auth, products, import, settings, stock) + 2 adapters (catalog, import). Логирование: signup/login, import start, settings update, stock bulk update.

7. **Frontend** — `logger.js` с localStorage debug toggle, API timing через `performance.now()`. Замена `console.log/error` и silent catches.

### Naming Convention

Spans: `http` → `handler.pipeline` → `usecase.expand` → `db.get_state`. Admin: `db.admin.list_products`.

**Файлы:** 8 новых + ~24 изменённых = ~32 файла.

**Specs:** `ADW/specs/done/done-logging-full-coverage.md`

---

## Adjacent Templates — 2026-02-12 (feature/instant-navigation)

Оптимизация instant expand: N formations → 1 template + raw entities. ~68% payload reduction, меньше CPU на бэке.

### Механика

- **BuildTemplateFormation** (tool_render_preset.go): новая функция — создаёт FormationWithData-шаблон с `value: null` и `fieldName` на каждом атоме. Currency sentinel `__ENTITY_CURRENCY__`. 1 вызов на тип entity вместо N вызовов `BuildFormation`.
- **Atom.FieldName** (atom_entity.go): новое поле `FieldName string` в Atom struct (`omitempty` — backward compatible).
- **fillFormation.js** (NEW): фронт-утилита — заполняет template данными entity при клике. Зеркалит Go field getters. ~90 строк.
- **Pipeline response**: `adjacentFormations` → `adjacentTemplates` (1 per entity type) + `entities` (raw StateData).
- **ChatPanel**: expand lookup по entityType → find entity → `fillFormation()` → instant render. Templates + entities в sessionCache (переживает F5).

### Bugfix: Expand → RenderConfig

- `navigation_expand.go`: `buildDetailFormation` не ставил `Config` на formation → Agent1 не видел что мы на detail view → запускал поиск заново вместо передачи Agent2.
- Фикс: добавлен `formation.Config` с Preset, Mode, Size, Fields после expand.

**Payload:** 6 products: ~7.2KB → ~2.3KB. Backend CPU: 6x BuildFormation → 1x BuildTemplateFormation.

**Файлы:** 1 новый (FE), 8 изменённых (5 BE + 3 FE).

**Specs:** `ADW/specs/done/done-adjacent-templates.md`

---

## Instant Navigation — 2026-02-12 (feature/instant-navigation)

Back и Expand без round-trip к серверу. Decision tree: каждый ответ бэкенда = узел с предсобранными потомками.

### Phase 1: Formation Stack (Back = instant, FE only)

- **useFormationStack hook** (NEW): React hook — `push/pop/clear/canGoBack/stack`. Хранит предыдущие formations для instant back.
- **backgroundSync module** (NEW): `syncExpand()` + `syncBack()` — fire-and-forget POST с `keepalive: true` для sync backend state.
- **ChatPanel rewrite**: handleBack = `stack.pop()` → render → syncBack (убран `await goBack()`). handleExpand = push перед API + rollback при ошибке. canGoBack из стека, не из backend.
- **sessionCache**: `formationStack` персистируется в localStorage — переживает F5.
- **apiClient**: `getHeaders()` экспортирован для backgroundSync.

### Phase 2: Adjacent Formations (Expand = instant, BE + FE) — replaced by Adjacent Templates

- ~~buildAdjacentFormations~~ → см. "Adjacent Templates" выше.
- **Navigation handlers**: `?sync=true` → `{"ok": true}` без formation (экономия сериализации).

**Метрики:** Back 100-300ms → <16ms. Expand 100-300ms → <16ms.

**Файлы:** 2 новых (FE), 10 изменённых (5 FE + 5 BE).

**Specs:** `ADW/specs/done/done-instant-navigation.md`

---

## Catalog Evolution — 2026-02-12 (feature/catalog-evolution)

Три структурных изменения в каталоге для подготовки к пилоту.

### 1. Stock Table — изоляция стоков

Отдельная таблица `catalog.stock` с PK `(tenant_id, product_id)`. Stock больше не колонка в products.

**Chat backend:** `ListProducts`, `GetProduct`, `VectorSearch` переведены на `LEFT JOIN catalog.stock` → `COALESCE(s.quantity, 0)`. Новый метод `GetStock`.

**Admin backend:** `POST /admin/api/stock/bulk` — массовое обновление стоков по SKU. `StockUseCase` + `StockHandler` + `BulkUpdateStock` в адаптере.

Seed-миграция переносит существующие `stock_quantity` из products в stock. Колонка не удалена (backward compat).

### 2. Services Tables — услуги в БД

`catalog.master_services` + `catalog.services` — полный аналог products.

**Chat backend:** `ListServices`, `GetService`, `VectorSearchServices`, `SeedServiceEmbedding`. RRF merge для hybrid search. `tool_catalog_search` расширен полем `entity_type` (product/service/all).

**Admin backend:** Full CRUD (UpsertMasterService, UpsertServiceListing, ListServices, GetService, UpdateService). Import расширен: поле `type` = "product"/"service", отдельные `processProductItem` / `processServiceItem`. Post-import `embedServices`.

### 3. Tags — JSONB + GIN

`tags JSONB DEFAULT '[]'` на `products` и `services` с GIN индексами. Чтение/запись в адаптерах, поддержка в импорте.

**Файлы:** 7 изменённых + 3 новых (chat), 7 изменённых + 4 новых (admin).

**Верификация на живой БД** — найдено и исправлено 4 бага:
1. Admin `ListProducts`/`GetProduct` читали `stock_quantity` из products вместо stock таблицы — добавлен `LEFT JOIN catalog.stock`
2. `UpsertProductListing` не писал в stock таблицу при импорте — добавлен upsert
3. `UpdateProduct` не синхронизировал stock таблицу — добавлен upsert
4. Chat `ProductResponse` не содержал поле `Tags` — добавлено

**Specs:** `ADW/specs/done/done-catalog-evolution.md`

---

## Plan — 2026-02-12 (Pre-Pilot Sprint)

Цель дня: подготовка к пилоту. Три направления, работа параллельная.

### 1. Каталог — архитектура мультитенантной БД

**Проблема:** Текущий каталог рабочий (master_products + products per tenant), но нужно продумать архитектуру под пилот: как загружать каталоги клиентов, как работает shared master data, как это масштабируется.

**Ключевые вопросы:**
- Формат импорта каталога для пилот-клиента (JSON/CSV через админку уже есть)
- Master products как shared knowledge base (ультрамного инфо → лучший vector search)
- Tenant overlay — клиент дополняет/переопределяет данные мастера
- Мультитенантность: изоляция данных, но shared catalog benefits для МСБ

**Результат:** Архитектурное решение + готовый каталог для пилота. → **Реализовано** в "Catalog Evolution" выше.

### 2. Кейсы фронта — UX и бизнес-сценарии

**А) Навигационная панель:**
- История ходов (forward/back по turns)
- Лайки / сохранение товаров
- Возможно: share, compare

**Б) Мгновенная отрисовка:**
- Предгенерация дефолтных пресетов (instant transitions)
- Кэш готовых formations на клиенте
- Спека: `feature-instant-navigation.md`

**В) Бизнес-кейсы:**
- Поиск товаров/услуг (core)
- "Покажи похожие" / рекомендации
- Триггеры от контрагента (промо, рекомендации)
- Элементарная поддержка (FAQ, ответы по товару)

**Г) Пресеты и freestyle:**
- Больше пресетов для разных сценариев
- Freestyle mode — кастомный рендеринг по запросу пользователя

### 3. Логирование — полное покрытие

**Цель:** Видеть всё что происходит в системе, отлавливать плохое поведение.

**Что логировать:**
- Pipeline flow (уже есть traces, нужно расширить)
- LLM quality metrics (relevance, latency, cost per query)
- User behavior (что ищут, что кликают, где бросают)
- Errors и anomalies
- Tenant-level aggregation

**Результат:** Можно открыть логи и понять "система работает хорошо/плохо" без угадывания.

---

## Alpha 0.0.2 — 2026-02-11

### Widget Auto-Detection Fix + Admin Widget Page

**Баг-фикс:** Виджет не работал при кросс-доменном встраивании — `document.currentScript` = null для динамически вставленных скриптов, API запросы шли на хост-сайт вместо бэкенда.

**Фикс** (`widget.jsx`): Fallback поиск `<script>` по `src*="widget.js"`, автоматическое вычисление API URL из origin скрипта. Цепочка: `data-api` attr → origin из `src` + `/api/v1` → `window.__KEEPSTAR_WIDGET__`.

**Админка — раздел "Widget":**
- Новая страница `/widget` с готовым embed code и кнопкой Copy
- Информация о tenant (slug, name)
- Инструкция "How It Works" для клиентов
- Иконка `<Code>` в sidebar

**Backend (admin):**
- `GET /admin/api/tenant` — возвращает tenant info (slug, name, type, settings)
- `GET /admin/api/widget-config` — возвращает `{ widgetUrl }` из env `WIDGET_BASE_URL`
- `WIDGET_BASE_URL` в config — указывает на chat-сервис для генерации embed code

**Новые файлы:**
- `project_admin/frontend/src/features/widget/WidgetPage.jsx`
- `project_admin/frontend/src/features/widget/widget.css`

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
