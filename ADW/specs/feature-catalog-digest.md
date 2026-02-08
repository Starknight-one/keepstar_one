# Feature: Catalog Digest — мета-схема каталога для Agent1

## ADW ID: feature-catalog-digest

## Контекст

**Проблема:** Agent1 формирует поисковые запросы "вслепую" — не знает какие категории, бренды, ценовые диапазоны и параметры есть у конкретного тенанта. Результат: нерелевантные фильтры, промахи по категориям, плохой recall.

**При масштабировании (10-30 тенантов):** загружать полный каталог в контекст LLM невозможно (40k SKU, тысячи параметров → overflow). Тюнить поиск per-tenant — дорого и не масштабируется.

**Проблема формата:** Наивный подход "выгрузить все бренды/модели/атрибуты" не работает на реальных каталогах. 50 брендов ноутбуков × 50 моделей × 200 параметров — не влезет в промпт, а если влезет — потратим кучу токенов и время ответа будет неприемлемым.

**Решение:** Pre-computed Catalog Digest — **мета-схема** каталога тенанта (~1-3kb). Не дамп данных, а описание структуры: дерево категорий, типы параметров, кардинальность, диапазоны, семейства значений. Генерируется из БД, инжектируется в system prompt Agent1. Один поисковый движок для всех тенантов, разный контекст.

## Архитектура

```
[Seed / Catalog Update] → [Background: GenerateCatalogDigest(tenantID)]
                                    ↓
                          catalog.tenants.catalog_digest (JSONB)
                                    ↓
[Request] → [Agent1 system prompt + digest] → [catalog_search tool] → [результат]
```

## Ключевой принцип: Digest — это не данные, а стратегия

Digest говорит Agent1 **как искать**, а не **что есть**:

- `brand(enum, 5 values: Nike, Adidas, Asics)` → кладу в SQL-фильтр `filters.brand`, точное совпадение
- `color(enum, 300 values, families: Red/Green/Blue/...)` → кладу в `vector_query`, пусть семантика разберётся
- `price(range: 6k-19k RUB)` → кладу в `filters.min_price/max_price`
- `category_name` из дерева → кладу в `filters.category`, точное имя

### Почему это работает

`catalog_search` tool имеет два типа входов:

**SQL-фильтры** (жёсткие, точное совпадение в WHERE):
- `filters.brand` → `WHERE mp.brand ILIKE '%Nike%'`
- `filters.category` → `WHERE c.name ILIKE '%Running Shoes%'`
- `filters.min_price`, `filters.max_price` → `WHERE p.price BETWEEN ...`
- `filters.color`, `filters.material`, `filters.size` → JSONB attribute ILIKE

**vector_query** (семантический, через embedding):
- `vector_query: "салатовые кроссовки для бега"` → текст → embedding → cosine distance

Digest подсказывает Agent1 для каждого аспекта запроса: куда его класть — в SQL-фильтр или в vector_query.

## Как работает полный pipeline поиска

### Пример 1: Точный запрос — "покажи кроссовки Nike до 15000"

**Agent1 видит digest:**
```
Footwear
  ├─ Running Shoes (45): 5990-18990 RUB
  │   brand(enum,5): Nike, Adidas, Asics, Hoka, New Balance
  │   color(enum,28, families: Black/White/Blue/Red/Green/Gray)
  │   size(enum,12): 36,37,38,...,47
  ├─ Basketball Shoes (20): 8990-24990 RUB
  │   brand(enum,3): Nike, Jordan, Adidas
```

**Agent1 формирует:**
```json
{
  "vector_query": "кроссовки Nike",
  "filters": {
    "brand": "Nike",
    "category": "Running Shoes",
    "max_price": 15000
  }
}
```

Логика: "кроссовки" → Running Shoes (видит точное имя в digest). Brand enum мало значений → фильтр. Цена в диапазоне → фильтр. Без digest Agent1 мог послать `category: "Sneakers"` (нет такой) → 0 результатов.

### Пример 2: Пространный запрос — "нужно что-то для бега до 12000"

**Agent1 видит digest и понимает:** "для бега" — это сценарий/активность, не конкретная категория. В каталоге есть Running Shoes, Sport Watches (с фитнесом), Running Apparel — все подходят.

**Agent1 формирует:**
```json
{
  "vector_query": "для бега спорт",
  "filters": {
    "max_price": 12000
  }
}
```

**НЕ ставит category фильтр** — пусть vector search найдёт релевантное из всех категорий. Ставит только ценовой фильтр. Результат: кроссовки, часы, одежда — всё до 12к.

### Пример 3: Проблема "салатового" — "салатовые кроссовки"

Agent1 видит в digest: `color(enum, 28, families: Black/White/Blue/Red/Green/Gray)`. 28 значений, есть семейство Green — но точного "салатовый" может не быть (там "нежно-травяной").

**Agent1 формирует:**
```json
{
  "vector_query": "салатовые кроссовки зелёные",
  "filters": {
    "category": "Running Shoes"
  }
}
```

Цвет **не идёт в SQL-фильтр** (28 значений, точного совпадения не будет). Идёт в vector_query — embedding "салатовые" семантически близок к "нежно-травяной", "lime", "light green". Vector search найдёт.

### Механика поиска (keyword + vector + RRF)

**Путь A — Keyword (SQL):**
```sql
WHERE p.tenant_id = $1
  AND mp.brand ILIKE '%Nike%'           -- из фильтра
  AND c.name ILIKE '%Running Shoes%'    -- из фильтра
  AND p.price <= 1500000                -- kopecks
  AND (mp.name ILIKE '%кроссовки%' OR mp.description ILIKE '%кроссовки%')
```
Точный, но хрупкий. Не найдёт "Pegasus 41" если в названии нет слова "кроссовки".

**Путь B — Vector (семантика):**
```sql
WHERE p.tenant_id = $1
  AND mp.embedding IS NOT NULL
  AND mp.brand ILIKE '%Nike%'           -- pre-filter
  AND c.name ILIKE '%Running Shoes%'    -- pre-filter
ORDER BY mp.embedding <=> $embedding    -- cosine distance
LIMIT 20
```
Embedding "кроссовки" семантически близок к "running shoe", "Pegasus", "Air Zoom".

**RRF Merge (k=60):**
```
score = keywordWeight / (60 + rank_keyword + 1)  +  1.0 / (60 + rank_vector + 1)
```
- `keywordWeight = 2.0` если есть structured фильтры, иначе `1.5`
- Товар найден обоими путями → высокий score (двойной буст)
- Товар найден только одним → всё равно попадает в результаты

### Роль Digest в этой схеме

| Без Digest | С Digest |
|------------|----------|
| Agent1 угадывает имя категории → промах | Agent1 видит точные имена → попадание |
| Не знает кардинальность параметра → пытается фильтровать по цвету из 300 значений → 0 результатов | Видит `color(enum,300)` → кладёт в vector_query |
| Пространный запрос "для бега" → ставит случайную категорию | Видит несколько подходящих категорий → не ставит фильтр, доверяет вектору |
| Vector search без pre-filter → шум | Vector search с pre-filter → чистые результаты |

## Формат Digest

### JSON-структура (хранится в `catalog.tenants.catalog_digest`)

```json
{
  "generated_at": "2026-02-08T12:00:00Z",
  "total_products": 1245,
  "categories": [
    {
      "name": "Running Shoes",
      "slug": "running-shoes",
      "count": 45,
      "price_range": [599000, 1899000],
      "params": [
        { "key": "brand", "type": "enum", "cardinality": 5, "values": ["Nike", "Adidas", "Asics", "Hoka", "New Balance"] },
        { "key": "color", "type": "enum", "cardinality": 28, "families": ["Black", "White", "Blue", "Red", "Green", "Gray"] },
        { "key": "size", "type": "enum", "cardinality": 12, "range": "36-47" },
        { "key": "material", "type": "enum", "cardinality": 3, "values": ["Mesh", "Leather", "Synthetic"] }
      ]
    },
    {
      "name": "Laptops",
      "slug": "laptops",
      "count": 120,
      "price_range": [2999000, 49999000],
      "params": [
        { "key": "brand", "type": "enum", "cardinality": 50, "top": ["Apple", "Lenovo", "ASUS", "HP", "Dell"], "more": 45 },
        { "key": "ram", "type": "enum", "cardinality": 4, "values": ["8GB", "16GB", "32GB", "64GB"] },
        { "key": "storage", "type": "enum", "cardinality": 4, "values": ["256GB", "512GB", "1TB", "2TB"] },
        { "key": "screen", "type": "range", "range": "13-17 inch" },
        { "key": "color", "type": "enum", "cardinality": 15, "families": ["Silver", "Black", "White", "Blue"] }
      ]
    }
  ]
}
```

### Правила формирования params

| Кардинальность | Тип в digest | Что отдаём | Стратегия поиска |
|----------------|-------------|------------|------------------|
| 1-15 значений | `values` | Полный список | SQL-фильтр (точное совпадение) |
| 16-50 значений | `top` + `more` | Top-5 + "and N more" | SQL-фильтр если значение в top, иначе vector_query |
| 50+ значений | `families` | Группы/семейства | Только vector_query |
| Числовой диапазон | `range` | min-max | SQL range фильтр |

### Brand — особый случай

Brand всегда выносится отдельно в params каждой категории, потому что это ключевой фильтр. Правила:
- До 15 брендов → полный список в `values` → SQL-фильтр
- 16-50 брендов → `top` 5 самых популярных + `more: N` → SQL-фильтр если бренд в top, для остальных полное имя тоже пойдёт в SQL (Agent1 может попробовать, ILIKE найдёт)
- 50+ брендов → `top` 10 + `more: N` → SQL-фильтр всегда (бренды — это точные имена, ILIKE работает)

### Текстовое представление для system prompt

`ToPromptText()` генерирует компактный текст (~30-80 строк):

```
<catalog>
Tenant catalog: 1245 products

Footwear
  Running Shoes (45): 5990-18990 RUB
    brand(5): Nike, Adidas, Asics, Hoka, New Balance → filter
    color(28, families: Black/White/Blue/Red/Green/Gray) → vector_query
    size(12): 36-47 → filter
    material(3): Mesh, Leather, Synthetic → filter
  Basketball Shoes (20): 8990-24990 RUB
    brand(3): Nike, Jordan, Adidas → filter
    ...

Electronics
  Laptops (120): 29990-499990 RUB
    brand(50, top: Apple/Lenovo/ASUS/HP/Dell +45) → filter
    ram(4): 8GB, 16GB, 32GB, 64GB → filter
    storage(4): 256GB, 512GB, 1TB, 2TB → filter
    screen(range: 13-17") → filter
    color(15, families: Silver/Black/White/Blue) → vector_query
  Smartphones (150): 12990-179990 RUB
    brand(12, top: Apple/Samsung/Xiaomi/Google/OnePlus +7) → filter
    storage(5): 64GB, 128GB, 256GB, 512GB, 1TB → filter
    color(40, families: Black/White/Blue/Red/Green/Gold/Pink) → vector_query
  ...

Services (no price — "от...")
  Cleaning (30): from 1500 RUB
    type(4): standard, deep, window, post-construction → filter
  ...

Search strategy:
- param marked "→ filter": use in filters.{param} (exact SQL match)
- param marked "→ vector_query": include in vector_query text (semantic match)
- broad/activity queries ("для бега", "в подарок"): do NOT set category filter, use only vector_query + price
- if unsure about category: omit category filter, let vector search find across all categories
</catalog>
```

### Лимиты prompt-представления

| Каталог | Категорий | Примерный размер | Поведение |
|---------|-----------|-----------------|-----------|
| Маленький (до 500 SKU) | 5-10 | ~1kb, 30 строк | Полный вывод |
| Средний (500-5000 SKU) | 10-30 | ~2-3kb, 50-80 строк | Полный вывод |
| Большой (5000+ SKU) | 30+ | ~3-4kb | Top-25 категорий по count, остальные в "... and N more categories" |

## Пагинация результатов поиска

### Проблема
При запросе "покажи ноутбуки" может найтись 200+ товаров. Грузить все в стейт бессмысленно и дорого.

### Решение
`catalog_search` tool возвращает **страницу** результатов:
- `limit` уже есть (default 10) — это размер страницы
- Добавить `total_count` в ответ тула (уже есть в `ListProducts` — возвращается вторым значением)
- Agent1 сообщает пользователю: "Нашлось 247, показываю топ-10 по релевантности"

**Что НЕ делаем сейчас** (follow-up):
- Offset/cursor пагинация ("покажи ещё")
- Lazy load на фронте
- Хранение полного result set в стейте

Текущее поведение: top-N самых релевантных (RRF sort) → в стейт → на рендер. Достаточно для MVP.

## Step-by-Step

### Step 1: Миграция — добавить колонку `catalog_digest`

**Файл:** `project/backend/internal/adapters/postgres/catalog_migrations.go`

Добавить миграцию в существующий список:
```sql
ALTER TABLE catalog.tenants ADD COLUMN IF NOT EXISTS catalog_digest JSONB DEFAULT NULL;
```

### Step 2: Domain entity

**Файл:** `project/backend/internal/domain/catalog_digest_entity.go` (NEW)

```go
type CatalogDigest struct {
    GeneratedAt   time.Time              `json:"generated_at"`
    TotalProducts int                    `json:"total_products"`
    Categories    []DigestCategory       `json:"categories"`
}

type DigestCategory struct {
    Name       string          `json:"name"`
    Slug       string          `json:"slug"`
    Count      int             `json:"count"`
    PriceRange [2]int          `json:"price_range"` // kopecks [min, max]
    Params     []DigestParam   `json:"params"`
}

type DigestParam struct {
    Key         string   `json:"key"`
    Type        string   `json:"type"`         // "enum" | "range"
    Cardinality int      `json:"cardinality"`
    Values      []string `json:"values,omitempty"`    // полный список (cardinality <= 15)
    Top         []string `json:"top,omitempty"`       // top-N (cardinality 16-50)
    More        int      `json:"more,omitempty"`      // сколько ещё значений (cardinality 16-50)
    Families    []string `json:"families,omitempty"`   // семейства (cardinality 50+)
    Range       string   `json:"range,omitempty"`      // "36-47", "13-17 inch"
}

// ToPromptText returns compact text for LLM system prompt.
// Includes search strategy hints (→ filter / → vector_query).
func (d *CatalogDigest) ToPromptText() string { ... }
```

**Логика `ToPromptText()`:**
- Группирует категории по parent (если есть parent_id в categories)
- Для каждой категории: имя, count, price range в рублях (kopecks / 100)
- Для каждого param: формат зависит от кардинальности
- Добавляет `→ filter` или `→ vector_query` хинт по правилу:
  - `values` или `top` → `→ filter`
  - `families` → `→ vector_query`
  - `range` → `→ filter`
- В конце: блок "Search strategy" с правилами

### Step 3: CatalogPort — новые методы

**Файл:** `project/backend/internal/ports/catalog_port.go`

Добавить к существующему `CatalogPort` interface:

```go
// GenerateCatalogDigest computes a compact catalog meta-schema for a tenant.
// Aggregates categories, brands, price ranges, attribute cardinality.
GenerateCatalogDigest(ctx context.Context, tenantID string) (*domain.CatalogDigest, error)

// GetCatalogDigest returns the pre-computed digest from tenants.catalog_digest.
GetCatalogDigest(ctx context.Context, tenantID string) (*domain.CatalogDigest, error)

// SaveCatalogDigest persists the computed digest to the tenants table.
SaveCatalogDigest(ctx context.Context, tenantID string, digest *domain.CatalogDigest) error
```

### Step 4: SQL-агрегат — `GenerateCatalogDigest`

**Файл:** `project/backend/internal/adapters/postgres/postgres_catalog.go`

Новый метод. Два SQL-запроса:

**Запрос 1 — категории + бренды + цены:**
```sql
SELECT
    c.name AS category_name,
    c.slug AS category_slug,
    COUNT(DISTINCT mp.id) AS product_count,
    ARRAY_AGG(DISTINCT mp.brand) AS brands,
    MIN(p.price) AS min_price,
    MAX(p.price) AS max_price
FROM catalog.products p
JOIN catalog.master_products mp ON p.master_product_id = mp.id
JOIN catalog.categories c ON mp.category_id = c.id
WHERE p.tenant_id = $1
GROUP BY c.name, c.slug
ORDER BY product_count DESC
```

**Запрос 2 — атрибуты с кардинальностью (per category):**
```sql
SELECT
    c.name AS category_name,
    attr.key AS attr_key,
    COUNT(DISTINCT attr.value) AS cardinality,
    ARRAY_AGG(DISTINCT attr.value ORDER BY attr.value) AS all_values
FROM catalog.products p
JOIN catalog.master_products mp ON p.master_product_id = mp.id
JOIN catalog.categories c ON mp.category_id = c.id,
LATERAL jsonb_each_text(mp.attributes) AS attr(key, value)
WHERE p.tenant_id = $1
GROUP BY c.name, attr.key
ORDER BY c.name, cardinality DESC
```

**Пост-обработка в Go:**
- Brand добавляется как первый param каждой категории (из запроса 1)
- Для каждого attr из запроса 2:
  - cardinality <= 15 → `DigestParam{Type: "enum", Values: all_values}`
  - cardinality 16-50 → `DigestParam{Type: "enum", Top: top5, More: cardinality - 5}`
  - cardinality 50+ → `DigestParam{Type: "enum", Families: computeFamilies(all_values)}`
  - Числовые значения (size, screen) → попытка парсинга в range

**Функция `computeFamilies`:**
- Для цветов: маппинг в цветовые семейства (Red, Blue, Green, Black, White, ...)
  - "Красный", "Бордовый", "Алый" → "Red"
  - "Салатовый", "Зелёный", "Травяной" → "Green"
- Для прочих: top-10 самых частых значений
- Hardcoded маппинг цветов на первую итерацию, позже можно вынести в конфиг

**Сохранение:**
```sql
UPDATE catalog.tenants SET catalog_digest = $2, updated_at = NOW() WHERE id = $1
```

### Step 5: Инжекция в Agent1 prompt

**Файл:** `project/backend/internal/prompts/prompt_analyze_query.go`

Обновить `BuildAgent1ContextPrompt`:
- Новый параметр: `digest *domain.CatalogDigest`
- Если digest != nil → добавляет `digest.ToPromptText()` перед `<state>` блоком
- Если digest == nil → работает как сейчас (без `<catalog>` блока)

**Файл:** `project/backend/internal/usecases/agent1_execute.go`

В `Execute()`, после определения tenantSlug:
```go
// Load pre-computed catalog digest for tenant context
var digest *domain.CatalogDigest
if uc.catalog != nil {
    digest, _ = uc.catalog.GetCatalogDigest(ctx, tenantID)
    // Ошибка не критична — Agent1 просто работает без digest
}
// ... передать digest в BuildAgent1ContextPrompt
```

### Step 6: Agent1 system prompt — правила использования digest

**Файл:** `project/backend/internal/prompts/prompt_analyze_query.go`

Добавить правила в `Agent1SystemPrompt`:

```
11. When <catalog> block is present, use it to form precise search filters:
    - Params marked "→ filter": use EXACT values in filters.{param}
    - Params marked "→ vector_query": include descriptive text in vector_query (semantic match)
    - Use EXACT category names from the catalog tree
    - Use price_range to validate min_price/max_price make sense
    - Translate user terms to catalog terms: "Найк" → "Nike", "кроссы" → look at Running Shoes category

12. Category strategy:
    - Specific product request ("кроссовки Nike") → set category filter to exact name from catalog
    - Broad/activity request ("для бега до 12000", "в подарок маме") → do NOT set category filter, use only vector_query + price filter. Vector search will find relevant items across all categories.
    - Ambiguous ("обувь") → if multiple categories match, omit category filter

13. High-cardinality attributes (colors, models, etc.):
    - If a param shows "families" or cardinality > 15, do NOT try exact filter — put it in vector_query
    - Example: user says "салатовые" → vector_query: "салатовые зелёные кроссовки", NOT filter.color: "салатовый"
```

### Step 7: Генерация digest при старте

**Файл:** `project/backend/cmd/server/main.go`

После seed + embedding, запустить digest generation:

```go
// Generate catalog digest for all tenants (after seed + embeddings)
if catalogAdapter != nil {
    go func() {
        digestCtx, digestCancel := context.WithTimeout(context.Background(), 30*time.Second)
        defer digestCancel()
        if err := generateAllDigests(digestCtx, catalogAdapter, appLog); err != nil {
            appLog.Error("digest_generation_failed", "error", err)
        } else {
            appLog.Info("digest_generation_completed")
        }
    }()
}
```

```go
func generateAllDigests(ctx context.Context, catalog *postgres.CatalogAdapter, log *logger.Logger) error {
    tenants, err := catalog.GetAllTenants(ctx)
    if err != nil {
        return fmt.Errorf("get tenants: %w", err)
    }
    for _, t := range tenants {
        digest, err := catalog.GenerateCatalogDigest(ctx, t.ID)
        if err != nil {
            log.Error("digest_generate_failed", "tenant", t.Slug, "error", err)
            continue
        }
        if err := catalog.SaveCatalogDigest(ctx, t.ID, digest); err != nil {
            log.Error("digest_save_failed", "tenant", t.Slug, "error", err)
            continue
        }
        log.Info("digest_generated", "tenant", t.Slug, "categories", len(digest.Categories), "total_products", digest.TotalProducts)
    }
    return nil
}
```

### Step 8: Тесты

**Файл:** `project/backend/internal/domain/catalog_digest_test.go` (NEW)

Unit tests:
- `TestDigestParam_ValuesFormat` — cardinality <= 15 → values list
- `TestDigestParam_TopFormat` — cardinality 16-50 → top + more
- `TestDigestParam_FamiliesFormat` — cardinality 50+ → families
- `TestCatalogDigest_ToPromptText` — полный digest → текст с хинтами → filter / → vector_query
- `TestCatalogDigest_ToPromptText_Empty` — пустой digest → пустая строка
- `TestCatalogDigest_ToPromptText_LargeCategories` — 30+ категорий → top-25 + "and N more"

**Файл:** `project/backend/internal/adapters/postgres/catalog_digest_test.go` (NEW)

Integration tests (requires DB):
- `TestGenerateCatalogDigest` — digest для тенанта с данными: проверить categories, brands, price ranges, params cardinality
- `TestGenerateCatalogDigest_EmptyTenant` — тенант без товаров → пустой digest
- `TestGenerateCatalogDigest_AttributeCardinality` — проверить что cardinality считается корректно

## Файлы

| Файл | Действие |
|------|----------|
| `adapters/postgres/catalog_migrations.go` | EDIT — добавить миграцию catalog_digest |
| `domain/catalog_digest_entity.go` | CREATE — CatalogDigest + DigestParam + ToPromptText |
| `domain/catalog_digest_test.go` | CREATE — unit tests ToPromptText |
| `ports/catalog_port.go` | EDIT — 3 новых метода |
| `adapters/postgres/postgres_catalog.go` | EDIT — реализация Generate/Get/SaveCatalogDigest |
| `adapters/postgres/catalog_digest_test.go` | CREATE — integration тесты |
| `prompts/prompt_analyze_query.go` | EDIT — BuildAgent1ContextPrompt + правила 11-13 |
| `usecases/agent1_execute.go` | EDIT — загрузка digest, передача в prompt |
| `cmd/server/main.go` | EDIT — generateAllDigests при старте |

## Стоимость

| Что | Стоимость |
|-----|-----------|
| Генерация digest | 2 SQL запроса per tenant (~10-20ms) |
| Хранение | 1 JSONB колонка (~1-3kb per tenant) |
| Runtime | 1 доп. SELECT при старте сессии (из tenants row, уже загружается) |
| LLM tokens | +200-400 tokens в system prompt |
| Масштаб | 30 тенантов × 3kb = 90kb. Без изменений кода |

## Edge Cases

| Кейс | Поведение |
|------|-----------|
| Digest ещё не сгенерирован | Agent1 работает без `<catalog>` блока (как сейчас) |
| Каталог обновился, digest устарел | Digest пересчитывается при рестарте. Позже: webhook/cron |
| Тенант без товаров | Пустой digest, Agent1 не добавляет `<catalog>` |
| 30+ категорий | ToPromptText показывает top-25 по count, остальные в "... and N more categories" |
| Цены = 0 (услуги "от...") | price_range показывает "from N RUB" |
| 200+ результатов поиска | Возвращаем top-N (limit), отображаем total_count |
| Неизвестный цвет ("салатовый") | Не в SQL-фильтре, в vector_query → семантический поиск |
| Кросс-категорийный запрос ("для бега") | Без category фильтра, только vector_query + price |
| Param без значений | Не включается в digest |

## Что НЕ входит в эту задачу

- Offset/cursor пагинация "покажи ещё" (follow-up)
- Lazy load на фронте (follow-up)
- Per-tenant agent tuning (не нужен — один движок, разный контекст)
- Кэширование digest в Redis/memory (overkill для 30 тенантов)
- Real-time обновление digest при каждом изменении каталога (позже, через events)
- Per-tenant system prompt шаблоны (digest подставляется в единый шаблон)
- Автоматический маппинг цветов в семейства через LLM (hardcoded маппинг на первую итерацию)

## Verification

```bash
# Build
cd project/backend && go build ./...

# Unit test: digest format + ToPromptText
go test ./internal/domain/ -run TestDigest -v

# Integration test: digest generation
DATABASE_URL=... go test ./internal/adapters/postgres/ -run TestGenerateCatalogDigest -v

# E2E: restart server → check logs for "digest_generation_completed"
# Then:
#   "покажи кроссовки Nike до 15000" → Agent1 uses category=Running Shoes, brand=Nike
#   "нужно что-то для бега до 12000" → Agent1 uses only price filter, no category
#   "салатовые кроссовки" → Agent1 puts color in vector_query, not filter
```
