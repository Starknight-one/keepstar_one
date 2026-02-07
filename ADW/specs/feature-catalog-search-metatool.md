# Feature: 1.2 — Meta-Tool `catalog_search` с `normalize_query`

## Feature Description
Замена тупого `search_products` тула на мета-тул `catalog_search` с нормализацией запросов. Пользователь пишет на любом языке (включая сленг и алиасы) — нормализатор разрешает алиасы, переводит, нормализует бренды. Фильтры — декларативные параметры, собираются мгновенно. Единственная async-операция — вызов LLM Haiku для нормализации (~300ms).

Дополнительно: расширение ProductFilter (sort), конвертация цен (копейки), массивное расширение каталога для тестирования.

## Objective
После реализации:
- "кроссы Найк" → normalize → `{query: "sneakers", brand: "Nike"}` → SQL → результаты
- "Nike Air Max" → fast path (уже English) → SQL → результаты
- "дешёвые худи" → normalize → `{query: "hoodie"}` + `max_price` из filters → SQL → результаты
- "ноутбуки дешевле 50000" → normalize → `{query: "laptops"}` + `max_price: 5000000` (kopecks) → SQL
- "покажи телефоны по цене" → catalog_search(query="телефоны", sort_by="price", sort_order="asc")
- Fallback-каскад: brand+search → brand → search → empty
- Каталог: 100+ товаров в 5+ категориях для реального тестирования

## Expertise Context
Expertise used:
- **backend-pipeline**: `ToolExecutor` interface, `Registry` pattern, `ToolContext` с SessionID/TurnID/ActorID, tool naming `tool_{name}.go`
- **backend-ports**: `CatalogPort.ListProducts(ctx, tenantID, ProductFilter)` — tenantID это UUID (не slug!), `ProductFilter` поля: CategoryID, Brand, MinPrice, MaxPrice, Search, Limit, Offset. Фильтры комбинируются через AND. **НЕТ SortField/SortOrder — нужно добавить**
- **backend-domain**: Price в КОПЕЙКАХ (int). Seed: Nike Air Max 90 = 1,299,000 kopecks = 12,990 руб. formatPrice() делит на 100. `ToolResult.Content` возвращает строку "ok"/"empty"
- **backend-adapters**: Postgres ILIKE search: multi-word → OR между словами, AND с остальными фильтрами. Brand — ILIKE с wildcards. SQL hardcoded `ORDER BY p.created_at DESC` — нужно сделать динамическим
- **backend-usecases**: `Agent1ExecuteUseCase.getAgent1Tools()` фильтрует по префиксу `search_*` и `_internal_*`. Нужно менять на `catalog_*`
- **backend-ports** (LLM): `LLMPort.ChatWithUsage(ctx, systemPrompt, userMessage)` — простой вызов для normalize (не нужны tools/history)

## Relevant Files

### Existing Files (модифицируются)
- `project/backend/internal/ports/catalog_port.go` — добавить SortField, SortOrder в ProductFilter
- `project/backend/internal/adapters/postgres/postgres_catalog.go` — динамический ORDER BY из SortField/SortOrder
- `project/backend/internal/adapters/postgres/catalog_seed.go` — массивное расширение каталога
- `project/backend/internal/tools/tool_registry.go` — регистрация нового тула, новый параметр llmPort
- `project/backend/internal/usecases/agent1_execute.go` — изменить фильтр тулов: `search_*` → `catalog_*`
- `project/backend/internal/prompts/prompt_analyze_query.go` — обновить `Agent1SystemPrompt` под новый тул
- `project/backend/cmd/server/main.go` — передать llmPort в NewRegistry

### Existing Files (read-only)
- `project/backend/internal/tools/tool_search_products.go` — legacy, НЕ удаляется, НЕ меняется
- `project/backend/internal/ports/llm_port.go` — LLMPort.ChatWithUsage (read-only)

### New Files
- `project/backend/internal/tools/tool_catalog_search.go` — мета-тул `catalog_search`, implements `ToolExecutor`
- `project/backend/internal/tools/normalizer.go` — `QueryNormalizer`: LLM-based normalize + fast path + alias table
- `project/backend/internal/prompts/prompt_normalize_query.go` — промпт для Haiku normalizer

## Step by Step Tasks
IMPORTANT: Execute strictly in order.

### 1. Расширить ProductFilter — добавить Sort

**Файл:** `project/backend/internal/ports/catalog_port.go`

Добавить два поля в ProductFilter:
```go
type ProductFilter struct {
    CategoryID string
    Brand      string
    MinPrice   int
    MaxPrice   int
    Search     string
    SortField  string // "price", "rating", "name", "" (default: created_at)
    SortOrder  string // "asc", "desc" (default: "desc")
    Limit      int
    Offset     int
}
```

Существующий код передаёт ProductFilter без SortField/SortOrder → zero values ("") → fallback на `created_at DESC` — обратная совместимость сохранена.

### 2. Обновить SQL — динамический ORDER BY

**Файл:** `project/backend/internal/adapters/postgres/postgres_catalog.go`

Найти строку ~225: `baseQuery += fmt.Sprintf(" ORDER BY p.created_at DESC LIMIT ...`

Заменить на логику:
```go
// Dynamic ORDER BY
orderClause := "p.created_at DESC" // default
if filter.SortField != "" {
    sortOrder := "ASC"
    if strings.ToUpper(filter.SortOrder) == "DESC" {
        sortOrder = "DESC"
    }
    switch filter.SortField {
    case "price":
        orderClause = fmt.Sprintf("p.price %s", sortOrder)
    case "rating":
        orderClause = fmt.Sprintf("p.rating %s", sortOrder)
    case "name":
        orderClause = fmt.Sprintf("COALESCE(p.name, mp.name) %s", sortOrder)
    }
}
baseQuery += fmt.Sprintf(" ORDER BY %s LIMIT $%d OFFSET $%d", orderClause, argNum, argNum+1)
```

**CRITICAL:** НЕ использовать user input напрямую в ORDER BY — только whitelist значений через switch. Предотвращает SQL injection.

### 3. Массивное расширение каталога

**Файл:** `project/backend/internal/adapters/postgres/catalog_seed.go`

Текущее состояние: 2 тенанта (Nike, Sportmaster), ~14 товаров, только кроссовки.

**Расширить до:**

**Тенанты (добавить):**
- `techstore` — "TechStore" (retailer) — электроника
- `fashionhub` — "FashionHub" (retailer) — одежда мультибренд

**Категории (добавить к существующим sneakers/running/basketball/lifestyle):**
- `smartphones` — "Smartphones"
- `laptops` — "Laptops"
- `headphones` — "Headphones"
- `tablets` — "Tablets"
- `tshirts` — "T-Shirts"
- `hoodies` — "Hoodies"
- `jackets` — "Jackets"
- `pants` — "Pants"
- `accessories` — "Accessories"

**Master Products (~60-80 штук, примерный набор):**

Электроника:
- iPhone 15 Pro, iPhone 15, iPhone 14 — Apple
- Samsung Galaxy S24 Ultra, S24, S23, A54 — Samsung
- Google Pixel 8 Pro, Pixel 8 — Google
- MacBook Air M3, MacBook Pro 14 M3 — Apple
- Dell XPS 15, Dell Inspiron 16 — Dell
- Lenovo ThinkPad X1, IdeaPad 5 — Lenovo
- AirPods Pro 2, AirPods Max — Apple
- Sony WH-1000XM5, WF-1000XM5 — Sony
- Samsung Galaxy Buds3 Pro — Samsung
- iPad Pro M4, iPad Air M2 — Apple
- Samsung Galaxy Tab S9 — Samsung

Одежда (бренды: Nike, Adidas, Puma, The North Face, Levi's):
- Nike Sportswear Club Hoodie, Nike Tech Fleece Hoodie — Nike
- Adidas Essentials Hoodie, Adidas Trefoil Hoodie — Adidas
- Nike Dri-FIT T-Shirt, Nike Air T-Shirt — Nike
- Adidas Originals Trefoil Tee, Adidas Run Tee — Adidas
- Puma Essential Tee, Puma Logo Hoodie — Puma
- The North Face Thermoball Jacket, TNF Nuptse — The North Face
- Nike Windrunner Jacket — Nike
- Adidas Track Pants, Nike Tech Fleece Pants — Nike/Adidas
- Levi's 501 Original, Levi's 511 Slim — Levi's

Аксессуары:
- Nike Heritage Backpack, Adidas Linear Backpack — Nike/Adidas
- Apple Watch Series 9, Apple Watch SE — Apple
- Samsung Galaxy Watch 6 — Samsung

**Listings по тенантам:**
- `nike` — все Nike товары (кроссовки + одежда Nike) ~20 listings
- `sportmaster` — микс всех брендов (кроссовки + одежда + аксессуары) ~40 listings
- `techstore` — вся электроника (смартфоны, ноутбуки, наушники, планшеты, часы) ~30 listings
- `fashionhub` — вся одежда всех брендов + аксессуары ~35 listings

**Итого:** ~120-130 listings, 4 тенанта, 13+ категорий

**Цены (в копейках!):**
- Смартфоны: 3,499,000 — 14,999,000 (34,990 — 149,990 руб)
- Ноутбуки: 7,999,000 — 24,999,000 (79,990 — 249,990 руб)
- Наушники: 999,000 — 6,999,000 (9,990 — 69,990 руб)
- Планшеты: 4,999,000 — 12,999,000 (49,990 — 129,990 руб)
- Часы: 2,499,000 — 4,999,000 (24,990 — 49,990 руб)
- Худи: 399,000 — 899,000 (3,990 — 8,990 руб)
- Футболки: 199,000 — 499,000 (1,990 — 4,990 руб)
- Куртки: 999,000 — 2,999,000 (9,990 — 29,990 руб)
- Штаны: 399,000 — 999,000 (3,990 — 9,990 руб)
- Рюкзаки: 299,000 — 599,000 (2,990 — 5,990 руб)

**Важно:** `SeedCatalogData` проверяет `count > 0` и скипает если данные есть. Для обновления нужно: (A) дропнуть старые данные перед seed, или (B) добавить отдельную функцию `SeedExtendedCatalog` которая проверяет по количеству (если < 50 — досыпать). Вариант B безопаснее — не ломает существующие сессии.

**Решение:** Вариант B — `SeedExtendedCatalog(ctx, client)`, вызов из main.go после `SeedCatalogData`. Проверяет `SELECT COUNT(*) FROM catalog.products` — если < 50, добавляет недостающее.

### 4. Создать промпт нормализатора

**Файл:** `project/backend/internal/prompts/prompt_normalize_query.go`

**Содержимое:**
- Константа `NormalizeQueryPrompt` — system prompt для Haiku
- Функция `BuildNormalizeRequest(query, brand string) string` — формирует user message

**Промпт должен содержать:**
- Инструкцию: "Ты нормализатор поисковых запросов для e-commerce каталога на английском языке"
- Alias-таблицу (hardcoded, расширяемую):
  ```
  кроссы, кроссовки → sneakers
  худи → hoodie
  кеды → sneakers (canvas)
  футболка, футболки, футболки → t-shirt
  штаны, брюки → pants
  куртка, куртки → jacket
  ботинки → boots
  шорты → shorts
  ноутбук, ноут, ноутбуки → laptop
  телефон, телефоны, смартфон, смартфоны, мобильник → smartphone
  наушники → headphones
  планшет, планшеты, таблет → tablet
  часы, часики → watch
  рюкзак, рюкзаки → backpack
  ```
- Brand-таблицу транслитераций:
  ```
  Найк, найк, найки → Nike
  Адидас, адидас → Adidas
  Пума, пума → Puma
  Рибок, рибок → Reebok
  Самсунг, самсунг → Samsung
  Эпл, эпл, Апл, апл → Apple
  Сони, сони → Sony
  Леново, леново → Lenovo
  Делл, делл → Dell
  Левайс, левайс, левис → Levi's
  ```
- Правила:
  1. Если query уже на английском и не содержит алиасов → вернуть as-is
  2. Разрешить алиасы ПЕРЕД переводом (кроссы → кроссовки → sneakers)
  3. Brand нормализовать отдельно от query (Найк → Nike)
  4. Вернуть JSON: `{"query": "...", "brand": "...", "source_lang": "...", "alias_resolved": bool}`
  5. Если brand пустой на входе → brand пустой на выходе
  6. Если query пустой на входе → query пустой на выходе

### 5. Создать normalizer

**Файл:** `project/backend/internal/tools/normalizer.go`

**Struct:**
```go
type QueryNormalizer struct {
    llm ports.LLMPort
}
```

**Конструктор:** `NewQueryNormalizer(llm ports.LLMPort) *QueryNormalizer`

**Метод:**
```go
func (n *QueryNormalizer) Normalize(ctx context.Context, query, brand string) (*NormalizeResult, error)
```

**NormalizeResult struct:**
```go
type NormalizeResult struct {
    Query         string `json:"query"`
    Brand         string `json:"brand"`
    SourceLang    string `json:"source_lang"`
    AliasResolved bool   `json:"alias_resolved"`
}
```

**Fast path логика (NO LLM call):**
- Если query и brand оба ASCII-only (a-z, A-Z, 0-9, space, punctuation) → вернуть as-is с `source_lang: "en"`, `alias_resolved: false`
- Это экономит ~300ms и деньги на каждый английский запрос

**LLM path:**
- Вызвать `llm.ChatWithUsage(ctx, NormalizeQueryPrompt, BuildNormalizeRequest(query, brand))`
- Распарсить JSON из ответа через `json.Unmarshal` на `NormalizeResult`
- Перед Unmarshal: strip markdown code fences (```json ... ```) если LLM обернул ответ
- Если parse error → fallback: вернуть input as-is

**Import правило:** normalizer импортирует `ports/` и `prompts/` — это допустимо для слоя tools (tools импортируют domain/, ports/).

### 6. Создать мета-тул `catalog_search`

**Файл:** `project/backend/internal/tools/tool_catalog_search.go`

**Struct:**
```go
type CatalogSearchTool struct {
    statePort   ports.StatePort
    catalogPort ports.CatalogPort
    normalizer  *QueryNormalizer
}
```

**Конструктор:** `NewCatalogSearchTool(statePort, catalogPort, normalizer) *CatalogSearchTool`

**Definition() — tool schema для LLM:**
```go
{
    Name: "catalog_search",
    Description: "Search product catalog. Handles any language, slang, aliases. Pass user text as-is in query/brand fields.",
    InputSchema: {
        "type": "object",
        "properties": {
            "query":      {"type": "string", "description": "Search text in ANY language (e.g. 'кроссы', 'sneakers', 'ноутбук'). Normalized automatically."},
            "brand":      {"type": "string", "description": "Brand in ANY language/transliteration (e.g. 'Найк', 'Nike', 'Самсунг'). Normalized automatically."},
            "category":   {"type": "string", "description": "Category filter (optional)"},
            "min_price":  {"type": "number", "description": "Minimum price in RUBLES (optional). Example: 10000 means 10,000 rubles."},
            "max_price":  {"type": "number", "description": "Maximum price in RUBLES (optional). Example: 50000 means 50,000 rubles."},
            "sort_by":    {"type": "string", "enum": ["price", "rating", "name"], "description": "Sort field (optional)"},
            "sort_order": {"type": "string", "enum": ["asc", "desc"], "description": "Sort direction (optional, default asc)"},
            "limit":      {"type": "integer", "description": "Max results (default 10)"}
        },
        "required": ["query"]
    }
}
```

**Execute() flow:**
1. Parse input (query, brand, category, min_price, max_price, sort_by, sort_order, limit)
2. **Конвертация цен: рубли → копейки (×100)**
   ```go
   minPriceKopecks := minPrice * 100
   maxPriceKopecks := maxPrice * 100
   ```
   LLM думает в рублях (пользователь говорит "дешевле 10000"), БД хранит в копейках.
3. Вызвать `normalizer.Normalize(ctx, query, brand)` → `NormalizeResult`
4. Собрать `ProductFilter` из NormalizeResult + декларативных filters:
   ```go
   filter := ports.ProductFilter{
       Search:     normalizeResult.Query,
       Brand:      normalizeResult.Brand,
       CategoryID: category,
       MinPrice:   minPriceKopecks,
       MaxPrice:   maxPriceKopecks,
       SortField:  sortBy,
       SortOrder:  sortOrder,
       Limit:      limit,
   }
   ```
5. Resolve tenant (паттерн из search_products: state → tenant_slug → GetTenantBySlug → UUID)
6. `catalogPort.ListProducts(ctx, tenantID, filter)`
7. **Fallback каскад** (если 0 результатов):
   - Retry 1: убрать Search (только Brand + price/category)
   - Retry 2: убрать Brand (только Search + price/category)
   - Если всё ещё 0 → "empty"
8. Записать результат в state через `statePort.UpdateData()` (паттерн из search_products)
9. Вернуть `"ok: found N products"` или `"empty: 0 results, previous data preserved"`

**Скопировать из search_products:** `extractProductFields()` — скопировать в этот файл, не ломать legacy.

**CRITICAL gotchas:**
- `ListProducts(tenantID)` — tenantID это UUID, не slug. Resolve через `GetTenantBySlug`
- Price: LLM передаёт в РУБЛЯХ → конвертировать ×100 в копейки перед SQL
- Aliases из state.Current.Meta.Aliases должны сохраняться (tenant_slug)
- DeltaInfo.Tool = "catalog_search" (не "search_products")

### 7. Зарегистрировать тул в Registry

**Файл:** `project/backend/internal/tools/tool_registry.go`

**Изменения в `NewRegistry()`:**
- Добавить `llmPort ports.LLMPort` в параметры конструктора
- Создать normalizer: `normalizer := NewQueryNormalizer(llmPort)`
- Заменить `r.Register(NewSearchProductsTool(statePort, catalogPort))` на:
  ```go
  r.Register(NewCatalogSearchTool(statePort, catalogPort, normalizer))
  ```
- Оставить SearchProductsTool НЕ зарегистрированным (legacy, не exposed к LLM)

**Также добавить `llmPort` в struct Registry:**
```go
type Registry struct {
    tools          map[string]ToolExecutor
    statePort      ports.StatePort
    catalogPort    ports.CatalogPort
    presetRegistry *presets.PresetRegistry
    llmPort        ports.LLMPort
}
```

### 8. Обновить ВСЕ call sites NewRegistry (3 штуки)

**Breaking change:** `NewRegistry()` теперь принимает 4 параметра вместо 3.

**Call site 1:** `project/backend/cmd/server/main.go` (строка 119)
```go
// Было:
toolRegistry = tools.NewRegistry(stateAdapter, catalogAdapter, presetRegistry)
// Стало:
toolRegistry = tools.NewRegistry(stateAdapter, catalogAdapter, presetRegistry, llmClient)
```
`llmClient` уже создан на строке 87.

Также добавить вызов `SeedExtendedCatalog` после `SeedCatalogData` (строка ~80):
```go
if err := postgres.SeedExtendedCatalog(seedCtx, dbClient); err != nil {
    appLog.Error("extended_catalog_seed_failed", "error", err)
}
```

**Call site 2:** `project/backend/internal/usecases/agent1_execute_test.go` (строка 71)
```go
// Было:
toolRegistry := tools.NewRegistry(stateAdapter, catalogAdapter, presetRegistry)
// Стало:
toolRegistry := tools.NewRegistry(stateAdapter, catalogAdapter, presetRegistry, llmClient)
```
`llmClient` уже создан в `setupIntegration` на строке 66.

**Call site 3:** `project/backend/internal/usecases/cache_test.go` (строка 76)
```go
// Было:
toolRegistry := tools.NewRegistry(stateAdapter, catalogAdapter, presetRegistry)
// Стало:
toolRegistry := tools.NewRegistry(stateAdapter, catalogAdapter, presetRegistry, llmClient)
```
`llmClient` уже создан в том же тесте.

### 9. Обновить фильтр тулов Agent1

**Файл:** `project/backend/internal/usecases/agent1_execute.go`

**Изменить `getAgent1Tools()` (строки 236-244):**
```go
func (uc *Agent1ExecuteUseCase) getAgent1Tools() []domain.ToolDefinition {
    allTools := uc.toolRegistry.GetDefinitions()
    var agent1Tools []domain.ToolDefinition
    for _, t := range allTools {
        if strings.HasPrefix(t.Name, "catalog_") || strings.HasPrefix(t.Name, "_internal_") {
            agent1Tools = append(agent1Tools, t)
        }
    }
    return agent1Tools
}
```

Заменить `"search_"` на `"catalog_"`.

### 10. Обновить Agent1 System Prompt

**Файл:** `project/backend/internal/prompts/prompt_analyze_query.go`

**Заменить `Agent1SystemPrompt` на:**
```go
const Agent1SystemPrompt = `You are Agent 1 - a data retrieval agent for an e-commerce chat.

Your job: call catalog_search when user needs NEW data. If the user is asking about STYLE or DISPLAY (not new data), do nothing.

Rules:
1. If user asks for products/services → call catalog_search
2. Pass user text AS-IS in query and brand fields — normalization is automatic
3. Prices are in RUBLES. "дешевле 10000" → max_price: 10000
4. Use filters (min_price, max_price, category, sort_by, sort_order) for structured constraints
5. If user asks to CHANGE DISPLAY STYLE (bigger, smaller, hero, compact, grid, list, photos only, etc.) → DO NOT call any tool. Just stop.
6. Do NOT explain what you're doing.
7. Do NOT ask clarifying questions - make best guess.
8. Tool results are written to state. You only get "ok" or "empty".
9. After getting "ok"/"empty", stop. Do not call more tools.

Available tools:
- catalog_search: Search product catalog. Handles any language, slang, transliteration automatically.

Examples:
- "покажи кроссы Найк" → catalog_search(query="кроссы", brand="Найк")
- "Nike shoes under 10000" → catalog_search(query="shoes", brand="Nike", max_price=10000)
- "дешевые телефоны Samsung" → catalog_search(query="телефоны", brand="Samsung", sort_by="price", sort_order="asc")
- "покажи худи" → catalog_search(query="худи")
- "ноутбуки дешевле 50000" → catalog_search(query="ноутбуки", max_price=50000)
- "покажи с большими заголовками" → DO NOT call tool (style request)
- "покажи только фотки" → DO NOT call tool (display change)
- "сделай покрупнее" → DO NOT call tool (style request)
`
```

### 11. Экспортировать helper из search_products

**Файл:** `project/backend/internal/tools/tool_search_products.go`

**Решение:** Скопировать `extractProductFields()` в `tool_catalog_search.go` — проще, не ломает legacy код.

### 12. Validation

```bash
cd project/backend && go build ./...
cd project/frontend && npm run build
```

Frontend не меняется в этой фиче.

## Validation Commands
```bash
# Backend build (required)
cd project/backend && go build ./...

# Frontend build (required — verify no breakage)
cd project/frontend && npm run build
```

## Acceptance Criteria
- [ ] `catalog_search` тул зарегистрирован и доступен Agent1
- [ ] `search_products` тул НЕ доступен Agent1 (не в `catalog_*` prefix)
- [ ] English query "Nike shoes" → fast path, NO LLM normalize call, direct SQL
- [ ] Russian query "кроссы Найк" → LLM normalize → "sneakers" + "Nike" → SQL → results
- [ ] Alias resolution: "кроссы" → "кроссовки" → "sneakers"
- [ ] Brand transliteration: "Найк" → "Nike"
- [ ] Price conversion: LLM передаёт 10000 (рубли) → filter.MaxPrice = 1000000 (копейки)
- [ ] Sort: sort_by="price" → SQL ORDER BY p.price ASC/DESC
- [ ] Fallback cascade: brand+search → brand → search → empty
- [ ] Empty result НЕ перезаписывает state.Data (existing products preserved)
- [ ] DeltaInfo.Tool = "catalog_search"
- [ ] tenant_slug в Meta.Aliases сохраняется
- [ ] Agent1 промпт обновлён, цены в рублях, примеры показывают query as-is
- [ ] Каталог: 100+ товаров, 4+ тенанта, 10+ категорий
- [ ] `go build ./...` — OK
- [ ] main.go передаёт llmPort в NewRegistry
- [ ] Все 3 call sites NewRegistry обновлены (main.go + 2 test files)

## Notes

### Gotcha: Price — КОПЕЙКИ!
`domain.Product.Price` = int в КОПЕЙКАХ. Seed: Nike Air Max 90 = 1,299,000 копеек = 12,990 руб.
`formatPrice(kopecks, currency)` делит на 100. Тест: `float64(p.Price)/100`.
LLM/пользователь думает в РУБЛЯХ → `catalog_search.Execute()` конвертирует ×100.

### Gotcha: Registry signature change — 3 call sites, не 1
`NewRegistry()` вызывается в:
1. `cmd/server/main.go:119`
2. `usecases/agent1_execute_test.go:71`
3. `usecases/cache_test.go:76`
Все три нужно обновить. В тестах `llmClient` уже создаётся в `setupIntegration`.

### Gotcha: extractJSON() НЕ существует
Нигде в codebase нет `extractJSON()`. Normalizer парсит JSON через `json.Unmarshal`.
Перед парсингом — strip markdown code fences (``` ```json ... ``` ```) если LLM обернул.

### Gotcha: Sort SQL injection prevention
ORDER BY clause собирается через switch/whitelist, НЕ через string concatenation из user input.

### Gotcha: Normalizer LLM model
`ChatWithUsage` использует default model из `LLM_MODEL` env. Для normalizer нужен Haiku (дешёвый, быстрый). Если env = Sonnet — normalize будет дороже и медленнее. Phase 1 — используем default model, оптимизируем позже.

### Gotcha: SeedExtendedCatalog идемпотентность
Проверяет `COUNT(*) FROM catalog.products` — если >= 50, не досыпает. Не дропает существующие данные. Безопасно вызывать повторно.

### Gotcha: ProductFilter.Search + Brand AND
Postgres адаптер комбинирует Search и Brand через AND. Если normalize вернул brand="Nike" и query="sneakers", SQL будет:
`WHERE brand ILIKE '%Nike%' AND (name ILIKE '%sneakers%' OR ...)` — корректно.

### Gotcha: Fallback cascade и state writes
Каждый fallback retry — повторный SQL, но НЕ повторный LLM call. Normalize вызывается один раз. Fallback просто меняет filter и перезапрашивает SQL. Максимум 3 SQL-запроса в worst case (~150ms total).

### Hexagonal Architecture Compliance
- **Domain layer** — без изменений
- **Ports layer** — ProductFilter получает 2 новых поля (SortField, SortOrder). Интерфейс CatalogPort НЕ меняется. Обратная совместимость: zero values → default sort
- **Tools layer** (pipeline) — новые файлы `tool_catalog_search.go`, `normalizer.go`. Импортируют только `domain/`, `ports/`, `prompts/`
- **Usecases layer** — одна строка в agent1_execute.go (prefix filter)
- **Adapters layer** — postgres_catalog.go: динамический ORDER BY. catalog_seed.go: расширенный каталог
- **Prompts layer** — новый файл `prompt_normalize_query.go`, изменение `prompt_analyze_query.go`
- **main.go** — composition root: llmPort в Registry + SeedExtendedCatalog

Стрелки зависимостей не нарушены. Tools → ports (interfaces). Adapters → ports (implements). Никаких циклов.
