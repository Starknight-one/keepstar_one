# Epic: Pipeline Works — всё должно работать

## Контекст
Базовый сценарий "пользователь пишет запрос → получает виджеты" не работает стабильно. Проблемы распределены по всему пайплайну. Нужно последовательно починить каждую.

## Часть 1: Первый запрос пользователя

### 1. Слой входа (Agent1 / Input Layer)

- **1.1** В чат нельзя писать сразу — пытается получить историю сообщений по сессии. Не хватает браузерного кэша.
- **1.2** Агент может не найти то что ожидается, или найти не то. Причина: тупейший SQL запрос, один тул `search_products` на все случаи. На деле пользователь может спросить очень разные штуки → нужна куча туллов. Чтобы не раздувать токены — мета-туллы (содержат кучу других). Агент оркестрирует и вызывает мета-туллы, передавая переменные и айдишки внутренних туллов. Реальная работа всегда на бэкенде.
- **1.3** Нет standardized output. Не видно: что отработало на бэке, какие результаты, что отработало в агенте, что в дельте, что улетело, как дельта обновит стейт, ждёт ли первый агент или второй. Мы слепые.
- **1.4** Тесты не проверяют ничего настоящего. Нужны реальные тесты, но их нужно проектировать руками, а не генерить.

### 2. State

- **2.1** Непонятно что хранится, когда и как обновляется, зачем. Нужен standardized output для стейта.
- **2.2** Нужно понимание кому что передавать и в каком виде, чтобы экономить токены/время/деньги при лучших результатах.

### 3. Слой выхода (Agent2 / Output Layer)

- **3.1** Нет standardized output — слепы.
- **3.2** Тесты — говнище. Нужно проектировать руками, говорить что тестировать.
- **3.3** Мало туллов.
- **3.4** Пресеты — должны быть механизмом, а не жёсткой штукой. Формочка со слотами: положи что угодно, отформатируй как угодно. Только фото, только название, группы данных в произвольном порядке — всё должно быть возможно.
- **3.5** Freestyle не работает вообще. Самая сложная штука: Agent2 должен понимать размер экрана, что хочет пользователь, что есть в данных, что сейчас отображено. Вызывает туллы с конкретными атрибутами для рендера.

## Часть 2: Когда уже есть история

_(Будет описано позже — добавляется история сообщений и действия пользователя на фронте)_

## Порядок работы

Берём по одной проблеме, решаем, проверяем, идём дальше. Не пытаемся чинить всё сразу.

---

## Что сделано

### 1.3 — Standardized output (Pipeline Trace) — DONE

**Дата:** 2026-02-06
**Ветка:** patch4
**Спека:** `ADW/specs/feature-patch4-pipeline-fixes.md`

Полная система трейсинга пайплайна. Каждый запрос записывается и доступен для анализа.

**Что создано:**
- `domain/trace_entity.go` — модель: PipelineTrace, AgentTrace, StateSnapshot, DeltaTrace, FormationTrace
- `ports/trace_port.go` — интерфейс: Record, List, Get
- `adapters/postgres/postgres_trace.go` — Postgres-хранилище + human-readable вывод в консоль
- `adapters/postgres/trace_migrations.go` — миграция таблицы `pipeline_traces`
- `handlers/handler_trace.go` — веб-UI на `/debug/traces/` (список + детальный просмотр + JSON)

**Что подключено:**
- `pipeline_execute.go` — trace создаётся, заполняется на каждом шаге, записывается
- `main.go` — миграции, адаптер, роуты `/debug/traces/`
- `.claude/commands/start.md` — обновлён URL дебаг-страницы

**Что видно в каждом trace:**
- **Agent1:** системный промпт (текст + размер в chars), кол-во messages, кол-во tool defs, модель, токены (input/output/cache read/write), cost, какой tool вызван, input, result, timing (llm ms / tool ms / total ms)
- **State:** products, services, fields, aliases, has template, детальная таблица дельт текущего turn (step, deltaType add/remove/update, path, actor, tool, count, fields)
- **Agent2:** prompt sent, raw response, tool, модель, токены, cost, timing
- **Formation:** mode, кол-во виджетов, cols, первый виджет
- **Итого:** total ms, total cost, ошибки

**Доступ:**
- Консоль — каждый запрос выводит trace в stderr
- Браузер — `http://localhost:8080/debug/traces/`
- JSON API — `http://localhost:8080/debug/traces/?format=json`
- Детали — `http://localhost:8080/debug/traces/{id}`

---

### 1.1 — Браузерный кэш сессии + Kill Session — DONE

**Дата:** 2026-02-06
**Ветка:** patch4

Две проблемы: (1) при загрузке страницы чат блокировался сетевым запросом `getSession()`, пользователь не мог писать; (2) сессии живут 5 минут — для дебага нужна возможность убить вручную.

**Браузерный кэш:**
- `features/chat/sessionCache.js` — утилита: `saveSessionCache()`, `loadSessionCache()`, `clearSessionCache()`
- Сохраняет в localStorage: sessionId, messages, last formation
- Expire через 30 минут
- `ChatPanel.jsx` — на маунте моментальный restore из кэша (синхронно, без сети). Input **не блокируется**. Кэш обновляется после каждого ответа пайплайна.
- Удалена зависимость от `getSession()` API при маунте.

**Kill Session на trace-странице:**
- `cache_port.go` — добавлен `DeleteSession(ctx, id)` в интерфейс
- `postgres_cache.go` — имплементация: ставит `status='closed'` (traces и история сохраняются)
- `handler_trace.go` — `POST /debug/kill-session` endpoint, кнопка "Kill" в таблице трейсов
- Живая сессия → красная кнопка "Kill" с confirm-диалогом
- Мёртвая сессия → серая disabled кнопка "Dead"
- Убийство сессии НЕ удаляет traces — они остаются для анализа

**Находка при тестировании (для 1.2):**
Запрос `brand:"Nike" query:"кроссы Nike"` → strip brand → search="кроссы" → ILIKE на английских названиях → 0 результатов. Каталог полностью на английском, русские запросы не матчатся. Это проблема 1.2 — нужна нормальная обработка мультиязычных запросов или маппинг.

**Фикс при kill session (дополнение к 1.1):**
- `ChatPanel.jsx` — async-валидация кэша при маунте: `getSession()` → если `session.status !== 'active'` → `clearSessionCache()` + сброс всего UI. Input не блокируется (валидация в фоне).

---

### 1.2 — Meta-Tool `catalog_search` с нормализацией — CODE DONE, НЕ ПРОТЕСТИРОВАНО

**Дата:** 2026-02-06
**Ветка:** patch4
**Спека:** `ADW/specs/feature-catalog-search-metatool.md`

**Проблема (подтверждена трейсами из 1.3):**
- Agent1 вызывал `search_products(brand="Nike", query="кроссовки")` → strip brand → search="кроссовки" → ILIKE на английских именах → 0 результатов
- Один тул `search_products` на все случаи, тупой ILIKE-поиск
- Каталог полностью на английском, пользователи пишут по-русски
- Алиасы не обрабатывались (кроссы ≠ кроссовки ≠ sneakers)

**Что реализовано:**

1. **Meta-tool `catalog_search`** — заменяет `search_products` для Agent1
   - `tools/tool_catalog_search.go` — мета-тул с normalize + fallback cascade
   - `tools/normalizer.go` — `QueryNormalizer`: fast path (ASCII→0ms) / LLM path (Haiku→~300ms)
   - `prompts/prompt_normalize_query.go` — промпт с alias-таблицей и brand-транслитерациями
   - Flow: input → price conversion (руб→коп ×100) → normalize → SQL filter → fallback cascade → state write

2. **Расширенный каталог** — с ~14 до ~130 товаров
   - `adapters/postgres/catalog_seed.go` — `SeedExtendedCatalog()` (идемпотентно, проверяет count < 50)
   - 4 тенанта: nike, sportmaster, techstore (электроника), fashionhub (одежда)
   - 13+ категорий: sneakers, smartphones, laptops, headphones, tablets, hoodies, tshirts, jackets, pants, accessories
   - Бренды: Nike, Adidas, Puma, Levi's, The North Face, Apple, Samsung, Google, Sony, Dell, Lenovo

3. **Динамический ORDER BY** — sort по price/rating/name
   - `adapters/postgres/postgres_catalog.go` — whitelist switch (SQL injection safe)
   - `ports/catalog_port.go` — SortField, SortOrder в ProductFilter

4. **Registry и Agent1 обновлены**
   - `tools/tool_registry.go` — `NewRegistry()` принимает `llmPort`, регистрирует `catalog_search` вместо `search_products`
   - `usecases/agent1_execute.go` — фильтр тулов: `catalog_*` вместо `search_*`
   - `prompts/prompt_analyze_query.go` — Agent1SystemPrompt обновлён: `catalog_search`, цены в рублях, примеры
   - `cmd/server/main.go` — `llmClient` передаётся в Registry + `SeedExtendedCatalog` после `SeedCatalogData`
   - 3 call sites обновлены (main.go, agent1_execute_test.go, cache_test.go)

5. **Tool Breakdown в трейсах** — видимость внутренностей мета-тула
   - `domain/tool_entity.go` — `Metadata map[string]interface{}` в ToolResult
   - `domain/trace_entity.go` — `ToolBreakdown` в AgentTrace
   - `adapters/postgres/postgres_trace.go` — консольный вывод normalize/filter/fallback/sql_ms
   - `handlers/handler_trace.go` — веб-UI: колонка "Normalize" в списке + блок "Tool Breakdown" в деталях
   - `usecases/agent1_execute.go` — `ToolMetadata` в Agent1ExecuteResponse
   - `usecases/pipeline_execute.go` — проброс в trace

**Статус: go build ✓, go vet ✓, npm run build ✓**

---

### 1.2 — Первое тестирование на проде — ПРОБЛЕМЫ

**Дата:** 2026-02-07
**Что тестировали:** Отправлены запросы "Кроссы найик", "Ноуты дешевле 100000", "Наушники сони" через фронтенд-чат.

**Наблюдения из traces (`/debug/traces/`):**

1. **Agent1 вызывает `search_products`, а не `catalog_search`**
   - Все 6 трейсов показывают `search_products` как вызванный тул
   - Причина: **сервер запущен со старым кодом**. Бинарник не пересобран/не перезапущен после реализации 1.2
   - **Действие:** нужен `go build && restart` чтобы новый код заработал

2. **Formation = nil на 5 из 6 запросов**
   - Экран чата пустой — серые пузыри без карточек
   - Agent2 отрабатывает за 127-193ms но не производит formation
   - Единственный рабочий trace (самый старый): formation = grid/7w — Agent2 вызвал `render_product_preset`
   - На новых запросах Agent2 не вызывает render tool → formation = nil → фронт показывает пустоту
   - **Это существующая проблема 3.x** (Agent2/Output Layer), не связана с 1.2

3. **3 сессии вместо 1**
   - Браузер создал 3 разных session ID (ff58773c, 7503a1c9, 78195a28) для одного пользователя
   - Старые сессии (7503a1c9, 78195a28) уже Dead
   - Новая сессия ff58773c — все 3 последних запроса в ней
   - Причина: TTL сессии = 5 минут. Между сериями тестов прошло > 5 минут → новая сессия
   - Это ожидаемое поведение, не баг

4. **Колонка "Normalize" не появилась в таблице трейсов**
   - Потому что старый код — `search_products` не отдаёт `ToolBreakdown`
   - Появится после рестарта с новым кодом

**Итог тестирования:**
- 1.2 код написан и компилируется, но **не задеплоен** — сервер на старом бинарнике
- Formation=nil — **отдельная проблема** Agent2, нужно отдельное исследование (3.x)
- Для валидации 1.2 нужен рестарт сервера

---

### Проблема: 537MB в Neon — НЕТ RETENTION POLICY

**Дата:** 2026-02-07
**Обнаружено:** В Neon dashboard хранилище показывает 537MB.

**Причина — 3 таблицы без TTL/cleanup:**

| Таблица | Колонка | Почему растёт | Размер на сессию |
|---------|---------|--------------|-----------------|
| `chat_session_state` | `conversation_history` | **Append-only**, никогда не чистится. Каждый ход: user msg + assistant tool_call + tool_result. После 10 ходов = 100-300KB | 100-300KB |
| `chat_session_state` | `current_data` | Полный массив products[] из последнего поиска | 50-100KB |
| `pipeline_traces` | `trace_data` | Полный JSON трейса (system prompt, tool I/O, state snapshot) | 20-50KB на запрос |
| `chat_session_deltas` | `action`, `result`, `template` | Одна строка на каждое изменение стейта, растёт линейно | 300-500B на дельту |
| `chat_messages` | `widgets`, `formation` | Полная UI-структура каждого ответа | 5-20KB на сообщение |

**Расчёт:**
- ~100 сессий × 10 запросов × (300KB history + 100KB data + 50KB traces + 20KB messages) ≈ **~500MB**
- `conversation_history` — основной виновник (~60% объёма)

**Что нужно:**
1. **TTL для `pipeline_traces`** — удалять трейсы старше 24-48ч (они нужны только для дебага)
2. **Лимит `conversation_history`** — хранить последние N ходов, а не все. Сейчас хранится ВСЁ для prompt caching, но prompt caching TTL = 5 минут. После закрытия сессии history бесполезна
3. **Cleanup для dead sessions** — удалять state/deltas/messages для сессий со status='closed' старше 1 часа
4. **SQL для проверки:** `SELECT relname, pg_size_pretty(pg_total_relation_size(c.oid)) FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname IN ('public','catalog') AND c.relkind='r' ORDER BY pg_total_relation_size(c.oid) DESC;`

---

### 1.2 — Рестарт сервера + Валидация catalog_search — DONE

**Дата:** 2026-02-07
**Ветка:** patch4

Пересобран бинарник с новым кодом. Agent1 теперь вызывает `catalog_search`.

**Результат тестирования:**
- "Покажи кроссовки Nike" → `catalog_search` → 7 products → formation grid/7w → карточки на экране
- "Наушники сони" → normalize: "наушники"→"headphones", "сони"→"Sony" (LLM, 707ms) → 0 results
  - Причина: tenant="nike", а Sony headphones в "techstore" tenant. Cross-tenant search — отдельная задача.

---

### 3.x — Formation=nil: Agent2 не вызывает render tool — FIXED

**Дата:** 2026-02-07
**Ветка:** patch4

**Корневая причина:** Anthropic API без параметра `tool_choice` позволяет модели ответить текстом вместо вызова инструмента. Промпт "ONLY call tools" — hint, а не гарантия. Claude отвечал текстом → `ToolCalls=[]` → цикл не выполнялся → formation=nil.

**Фикс:**
- `ports/llm_port.go` — добавлен `ToolChoice string` в `CacheConfig` ("auto"/"any"/"tool:name")
- `adapters/anthropic/cache_types.go` — `toolChoiceConfig` struct + поле в `anthropicCachedRequest`
- `adapters/anthropic/anthropic_client.go` — маппинг `CacheConfig.ToolChoice` → `tool_choice` в JSON-запросе
- `usecases/agent2_execute.go` — `ToolChoice: "any"` для Agent2 (must always call render tool)

**Результат:** "Покажи кроссовки Nike" → formation grid/7w → 7 карточек с image, title, brand, price, rating.

---

### DB Retention Policy — DONE

**Дата:** 2026-02-07
**Ветка:** patch4

**Проблема:** 537MB в Neon. conversation_history append-only (~60%), traces без TTL, dead sessions не чистятся.

**Что создано:**
- `adapters/postgres/retention.go` — `RetentionService` с тремя cleanup-операциями:
  1. **Traces TTL** — DELETE FROM pipeline_traces WHERE timestamp < 48h ago
  2. **Dead sessions** — DELETE state/deltas/messages/traces/session для closed sessions > 1h
  3. **Conversation trim** — JSONB array → keep last 20 messages (CTE + CASE для защиты от scalar values)

**Подключение:**
- `cmd/server/main.go` — фоновая горутина, запускается при старте, тикает каждые 30 минут
- Graceful shutdown через context cancellation

**Первый запуск:**
- 14 dead sessions удалено
- 2 conversation_history trimmed
- Traces: все свежие, ничего удалять

**Конфигурация по умолчанию:**
| Параметр | Значение |
|----------|----------|
| TraceMaxAge | 48h |
| DeadSessionMaxAge | 1h |
| ConversationMaxMsgs | 20 |
| CleanupInterval | 30min |

---

### Пустой экран при 0 результатах + Configurable Tenant + DB Seed Fix — DONE

**Дата:** 2026-02-07
**Ветка:** patch4

**Проблема:** Запрос "покажи наушники сони" → пустой экран. Три причины:
1. Agent2 возвращал `nil` при 0 результатах → фронт получал JSON без `formation` → пустой bubble
2. Tenant hardcoded "nike" — нельзя протестировать другие каталоги (techstore с Sony, Apple и т.д.)
3. Extended seed не работал — `ON CONFLICT (slug)` падал, т.к. нет UNIQUE constraint на `catalog.categories(slug)`

**Что сделано:**

1. **DB Migration fix** — `catalog_migrations.go`
   - Добавлена миграция `migrationCatalogCategorySlugUnique`: `CREATE UNIQUE INDEX IF NOT EXISTS idx_catalog_categories_slug_unique ON catalog.categories(slug)`
   - Теперь `SeedExtendedCatalog` отрабатывает: `ON CONFLICT (slug)` работает, электроника/одежда засеиваются

2. **"Ничего не найдено" в Agent2** — `agent2_execute.go`
   - При `ProductCount == 0 && ServiceCount == 0` вместо `nil` возвращается `FormationWithData` с text_block виджетом
   - Текст: "К сожалению, по вашему запросу ничего не найдено"
   - LLM не вызывается — экономия токенов (~130ms вместо ~2000ms)

3. **Frontend: text_block inline в чате** — `MessageBubble.jsx`, `useChatSubmit.js`
   - `MessageBubble`: text_block-only formations показываются inline в chat bubble даже при `hideFormation=true` (formation рендерится отдельно от чата для product cards, но текстовые сообщения должны быть в бабле)
   - `useChatSubmit`: не показывает "Нашёл N товаров" для text_block-only formations

4. **Configurable tenant** — `config.go`, `routes.go`, `main.go`, `apiClient.js`
   - `Config.TenantSlug` из env `TENANT_SLUG` (default: "nike")
   - `SetupRoutes` принимает `defaultTenant` вместо хардкода `"nike"`
   - Frontend: `API_BASE_URL` из `VITE_API_URL` env (для тестирования на другом порте)

**Тестирование:**
- "покажи наушники сони" (tenant=nike) → text_block "ничего не найдено" в чате (ранее — пустой экран)
- "покажи кроссовки Nike" → formation с карточками (работает как раньше)
- Наблюдение: "покажи кроссовки Nike" вернул ВСЮ продукцию Nike (не только кроссовки). Фильтрация по категории работает слабо — Agent1 передаёт `query="кроссовки"`, normalize → "sneakers", но SQL ищет по product name ILIKE, не по category. Нужна отдельная задача на улучшение category matching.

---

## Search Layer — целевая архитектура

**Дата:** 2026-02-07
**Статус:** проектирование, реализация сегодня

### Продуктовый контекст

Продукт — виртуальный консультант в магазине. Пользователь общается в чате, консультант понимает что нужно найти и показывает результаты на экране. Быстро, точно, на любом языке.

БД мультитенантная. Сейчас ~130 товаров, но в перспективе недель — сотни тысяч позиций (товары + услуги), каждая с атрибутами. Масштаб нужно закладывать сразу.

### Что сейчас сломано (1.2 текущая реализация)

1. **Два LLM вызова** — Agent1 (Sonnet) + отдельный normalizer (Sonnet). Дорого, медленно, бессмысленно — Agent1 и есть нормализатор.
2. **Category matching мёртв** — "кроссовки" нормализуется в "sneakers", ищется через ILIKE по product name. CategoryID поле не используется потому что Agent1 не знает UUID категорий.
3. **Single-tenant** — поиск только в одном тенанте (hardcoded/env). "наушники Sony" → 0 results потому что Sony в другом тенанте.
4. **ILIKE — тупой поиск** — нет category matching, нет fuzzy, нет synonyms. Один SQL на все случаи.
5. **Fallback теряет intent** — если brand+search → 0, показывает всё по brand. Пользователь просил кроссовки, получил всю продукцию Nike.
6. **Alias/brand таблицы захардкожены** в промпте нормализатора.

### Целевая архитектура

```
Пользователь: "красные найки 44 размера подешевле, хочу видеть фотки и цены"
                    │
                    ▼
            ┌──────────────┐
            │   Agent1     │  — LLM (Sonnet)
            │  (Router +   │  — Понимает intent
            │  Normalizer) │  — Нормализует данные (Найк→Nike, кроссы→sneakers)
            └──────┬───────┘  — Игнорирует display-запросы (это для Agent2)
                   │
                   │ catalog_search(
                   │   brand="Nike",
                   │   category="sneakers",
                   │   color="red",
                   │   size=44,
                   │   price_mode="percentile",
                   │   price_value=0.75,
                   │   sub_tools=["brand_filter","category_filter",
                   │              "color_filter","size_filter","price_percentile"]
                   │ )
                   ▼
            ┌──────────────┐
            │ catalog_search│  — Программная метатула, БЕЗ LLM внутри
            │  (meta-tool)  │  — Выполняет только указанные sub-tools
            └──────┬───────┘  — Собирает SQL, fallback при 0 results
                   │
                   ├── brand_filter:      WHERE brand = 'Nike'
                   ├── category_filter:   WHERE category = 'sneakers'
                   ├── color_filter:      WHERE color = 'red'
                   ├── size_filter:       WHERE size = 44
                   └── price_percentile:  WHERE price <= median * 0.75
                   │
                   ▼
            ┌──────────────┐
            │  Fallback     │  При 0 результатов — ослабление фильтров
            │  (relaxation) │  по приоритету
            └──────────────┘
```

### Ключевые принципы

1. **Agent1 = единственный LLM в search flow.** Никаких отдельных normalizer-LLM. Agent1 сам нормализует (Найк→Nike), сам определяет какие sub-tools вызвать, сам ставит параметры.

2. **Meta-tool = программный оркестратор.** Без LLM. Получил инструкции от Agent1 → собрал SQL из sub-tools → выполнил → fallback если нужно → вернул результат.

3. **Sub-tools = модульные фильтры.** Каждый отвечает за одну вещь. Можно вызывать любое подмножество. Примеры:
   - `brand_filter` — точное совпадение бренда
   - `category_filter` — по категории (slug или ID)
   - `size_filter` — по размеру
   - `color_filter` — по цвету
   - `price_range` — явный диапазон (min/max в рублях)
   - `price_percentile` — "подешевле"/"подороже" через медиану (считается динамически по текущему каталогу)
   - `text_search` — полнотекстовый/fuzzy поиск по названию и описанию

4. **Приоритеты фильтров задаёт пользователь через формулировку, а Agent1 это интерпретирует.** "Обязательно красные" → color = hard. "Может красные" → color = soft. "Подешевле" — всегда soft. Agent1 передаёт в метатулу приоритет (hard/soft/preference) для каждого фильтра. При fallback ослабляются только soft/preference фильтры.

5. **Display-запросы пролетают мимо.** "Хочу видеть фотки и цены" — Agent1 игнорирует, это работа Agent2 (output layer).

### Fallback / Relaxation

При 0 результатов метатула ослабляет фильтры по приоритету:

| Приоритет | Что | Когда дропается |
|-----------|-----|-----------------|
| hard | Явно и жёстко указанное пользователем ("обязательно Nike", конкретный товар) | Никогда |
| soft | Конкретное но не жёсткое (размер, цвет без "обязательно") | В последнюю очередь |
| preference | Размытое/субъективное ("подешевле", "из последней коллекции", "может красные") | В первую очередь |

Цикл: убрать все preference → retry → если всё ещё 0, убрать soft по одному → retry → если 0 после всех ослаблений → "ничего не найдено".

Metadata фиксирует что ослаблено → Agent2 может объяснить пользователю.

### Будущее (не сейчас)

- **Векторный поиск** — для семантических запросов ("что-нибудь удобное для бега по утрам"). Абсолютно точно будет нужен при росте каталога. pgvector или отдельный vector store.
- **pg_trgm** — fuzzy matching для опечаток и частичных совпадений.
- **Таблица синонимов в БД** — детерминированный маппинг (кроссы=кроссовки=sneakers=trainers), подстраховка если LLM нормализует неточно.
- **Cross-tenant search** — выбор тенанта по контексту запроса или multi-tenant search.

---

## Дальнейшие шаги

### Следующие задачи
- **1.2v2 — Search Layer переделка** — реализация целевой архитектуры: убрать normalizer LLM, sub-tools, fallback с приоритетами, category matching
- **Cross-tenant search** — Agent1 ищет только в одном tenant. Нужна логика выбора tenant или multi-tenant
- **1.4** — Реальные тесты
- **3.x** — Agent2 output layer (freestyle, пресеты как гибкий механизм)
