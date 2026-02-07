# Chore: Расширение БД + интеграционные тесты поиска

## ADW ID: chore-seed-and-search-tests

## Суть

Два deliverable:
1. **Seed-скрипт** — наполняет БД 4 тенантами и ~1000 товарами с embeddings
2. **Интеграционный тест** — 30+ запросов через полный pipeline (LLM → tool → DB → RRF), проверяет качество hybrid search

## Relevant Files

### New Files

- `project/backend/cmd/seedlarge/main.go` — скрипт генерации данных + embeddings
- `project/backend/internal/usecases/search_quality_test.go` — интеграционные тесты поиска

### Read-Only

- `project/backend/internal/adapters/postgres/catalog_seed.go` — паттерн существующего сида
- `project/backend/internal/tools/tool_catalog_search.go` — hybrid search tool
- `project/backend/internal/ports/catalog_port.go` — ProductFilter, CatalogPort
- `project/backend/internal/adapters/openai/embedding_client.go` — EmbeddingPort impl
- `project/backend/cmd/server/main.go` — runEmbedding паттерн

## Step by Step Tasks

### 1. Seed-скрипт: тенанты + категории

**Файл:** `project/backend/cmd/seedlarge/main.go`

Создать 3 новых тенанта (nike уже есть):

| Slug | Name | Type | Категории |
|------|------|------|-----------|
| `techzone` | TechZone Electronics | electronics | Smartphones, Laptops, Headphones, Tablets, Smartwatches, Accessories |
| `fashionhub` | FashionHub | fashion | Hoodies, Jackets, Jeans, Dresses, Bags, T-Shirts, Sneakers |
| `homemart` | HomeMart | home | Furniture, Lighting, Kitchenware, Textiles, Decor |

Паттерн: `INSERT ... ON CONFLICT (slug) DO NOTHING` — идемпотентный.

### 2. Seed-скрипт: master products (~300 на тенант)

Для каждого тенанта сгенерировать ~300 master products.

Формат каждого товара:
```go
type seedProduct struct {
    SKU         string
    Name        string            // "Samsung Galaxy S24 Ultra"
    Description string            // 2-3 предложения, семантически богатые
    Brand       string            // "Samsung"
    Category    string            // slug категории
    Price       int               // в копейках
    Rating      float64           // 3.5-5.0
    Images      []string          // 1-2 unsplash URL
    Attributes  map[string]string // {"color": "Black", "storage": "256GB", "ram": "12GB"}
}
```

**Важно для vector search:** Description должен содержать use-case слова:
- "идеально для бега" / "perfect for running"
- "подойдёт для офиса" / "great for office work"
- "тёплая, подходит для зимы" / "warm, suitable for winter"

Это позволит тестировать семантический поиск по intent.

**Данные для techzone (~300):**
- 40 смартфонов (Samsung, Apple, Xiaomi, Google, OnePlus — разные модели/цены/storage)
- 40 ноутбуков (Apple, Lenovo, Dell, ASUS, HP — gaming/office/ultrabook)
- 40 наушников (Sony, Apple, JBL, Bose, Sennheiser — TWS/over-ear/ANC/sport)
- 30 планшетов (Apple iPad, Samsung Tab, Lenovo, Xiaomi)
- 30 смарт-часов (Apple Watch, Samsung Galaxy Watch, Garmin, Fitbit)
- 30 аксессуаров (чехлы, зарядки, кабели, powerbank)
- 30 мониторов (Dell, LG, Samsung, ASUS)
- 30 клавиатур/мышей (Logitech, Razer, SteelSeries)
- 30 прочее (колонки, камеры, роутеры)

**Данные для fashionhub (~300):**
- 50 худи/свитшотов (Nike, Adidas, Puma, Uniqlo, H&M)
- 40 курток (North Face, Columbia, Patagonia, Zara)
- 40 джинсов (Levi's, Wrangler, Diesel, Calvin Klein)
- 30 платьев (Zara, H&M, Mango)
- 30 сумок (Michael Kors, Coach, Guess, Nike)
- 30 футболок (Nike, Adidas, Uniqlo, Ralph Lauren)
- 30 кроссовок (Nike, Adidas, New Balance, Puma, Reebok)
- 30 аксессуаров (шапки, шарфы, ремни, очки)
- 20 спортивная одежда (леггинсы, шорты, топы)

**Данные для homemart (~300):**
- 50 мебель (IKEA, West Elm — столы, стулья, шкафы, кровати)
- 40 освещение (Philips, IKEA — люстры, лампы, торшеры, LED)
- 40 посуда (Villeroy & Boch, IKEA — тарелки, кружки, кастрюли)
- 40 текстиль (постельное, полотенца, шторы, пледы)
- 40 декор (вазы, свечи, картины, часы настенные)
- 30 кухонная техника (Bosch, Philips — чайники, блендеры, кофемашины)
- 30 хранение (корзины, органайзеры, полки)
- 30 ванная (зеркала, полотенцедержатели, коврики)

### 3. Seed-скрипт: tenant products

Для каждого master product создать tenant product (привязка к тенанту с ценой и stock).

```sql
INSERT INTO catalog.products (tenant_id, master_product_id, price, currency, stock_quantity, rating)
VALUES ($1, $2, $3, 'RUB', $4, $5)
ON CONFLICT DO NOTHING
```

### 4. Seed-скрипт: embeddings

После вставки всех товаров — вызвать `runEmbedding()` (тот же паттерн что в main.go):
- `GetMasterProductsWithoutEmbedding()` — вернёт новые товары
- `Embed()` батчами по 100
- `SeedEmbedding()` для каждого

### 5. Запуск seed-скрипта

```bash
cd project/backend && go run ./cmd/seedlarge/
```

Ожидаемый вывод:
```
Tenants: 3 created (techzone, fashionhub, homemart)
Categories: 21 created
Master products: 900 created
Tenant products: 900 created
Embeddings: 900 generated (9 batches × 100)
Total time: ~30s
```

### 6. Интеграционный тест поиска

**Файл:** `project/backend/internal/usecases/search_quality_test.go`

```bash
go test -v -run TestSearchQuality -timeout 300s ./internal/usecases/
```

Формат: каждый тест-кейс = полный pipeline вызов через `CatalogSearchTool.Execute()`.

**Setup:**
- Подключение к реальной БД (DATABASE_URL)
- Инициализация EmbeddingClient (OPENAI_API_KEY)
- Создание CatalogSearchTool с реальными адаптерами
- StateAdapter для zone-write

**Структура теста:**
```go
type searchTestCase struct {
    Name        string
    Tenant      string                 // tenant slug
    Input       map[string]interface{} // tool input (vector_query, filters, sort_by, etc.)
    ExpectType  string                 // "hybrid", "vector", "keyword"
    ExpectMin   int                    // minimum results count
    ExpectMax   int                    // maximum results count (0 = no limit)
    ExpectBrand []string               // at least one of these brands in results
    ExpectCat   []string               // at least one of these categories in results
    ExpectInTop5 []string              // substrings that must appear in top-5 product names
}
```

### 7. Тест-кейсы

#### A. Семантический поиск (vector_query only, no filters)

```
1.  tenant=nike     query="кроссы для бега"           → min=3, brands=["Nike"], cats=["Running"]
2.  tenant=nike     query="что-то для бега"            → min=3, cats=["Running"]
3.  tenant=techzone query="lightweight laptop for work" → min=3, cats=["Laptops"]
4.  tenant=techzone query="наушники с шумодавом"       → min=3, top5 contains "ANC" or "Noise"
5.  tenant=fashionhub query="тёплая куртка на зиму"   → min=3, cats=["Jackets"]
6.  tenant=homemart query="уютный свет в спальню"      → min=2, cats=["Lighting"]
7.  tenant=techzone query="что подарить геймеру"       → min=3 (gaming peripherals)
8.  tenant=fashionhub query="для йоги"                 → min=2 (leggings, tops)
9.  tenant=homemart query="организовать пространство"  → min=2 (хранение, мебель)
10. tenant=techzone query="фоткать в путешествии"      → min=2 (камеры, телефоны)
```

#### B. Hybrid search (vector_query + filters)

```
11. tenant=nike      query="кроссы" filters={brand:"Nike"}              → all Nike, cats=["Running","Sneakers"]
12. tenant=techzone  query="телефон" filters={brand:"Samsung",max_price:30000} → Samsung phones under 30k
13. tenant=techzone  query="наушники" filters={color:"Black"}           → black headphones
14. tenant=techzone  query="ноутбук" filters={ram:"16GB"}               → laptops with 16GB
15. tenant=fashionhub query="худи" filters={brand:"Adidas",color:"Black"} → specific combo
16. tenant=homemart  query="кофе" filters={brand:"Bosch"}               → Bosch coffee machines
17. tenant=techzone  query="часы" filters={brand:"Apple"}               → Apple Watch
18. tenant=fashionhub query="кроссовки" filters={brand:"New Balance"} sort=price asc → NB sneakers cheap first
19. tenant=techzone  query="монитор" filters={size:"27 inch"}           → 27" monitors
20. tenant=homemart  query="постельное" filters={material:"Cotton"}     → cotton bedding
```

#### C. Edge cases

```
21. tenant=nike      query=""                          → min=0 or fallback to all (graceful)
22. tenant=techzone  query="asdfghjkl"                 → count=0 (gibberish)
23. tenant=techzone  query="Найк"                      → vector finds Nike (кириллица → семантика)
24. tenant=techzone  query="iphon"                     → vector may find iPhone (typo tolerance)
25. tenant=techzone  query="дешёвые" sort=price asc    → sorted by price, vector semantic
26. tenant=nike      query="ноутбук"                   → count=0 (nike doesn't sell laptops)
27. tenant=fashionhub query="ноутбук"                  → count=0 (fashion doesn't sell laptops)
28. tenant=homemart  query="кроссовки"                 → count=0 (home doesn't sell shoes)
```

#### D. Cross-tenant isolation

```
29. tenant=techzone  query="наушники Sony"             → Sony headphones (ONLY from techzone)
30. tenant=nike      query="наушники"                  → 0 or only sport earbuds if exist
```

#### E. RRF merge quality

```
31. tenant=fashionhub query="чёрные кроссы Nike" filters={brand:"Nike",color:"Black"}
    → keyword+vector, merged, all Nike+Black
32. tenant=techzone query="бюджетный смартфон" filters={max_price:15000}
    → keyword filters price, vector ranks by "budget", merged
33. tenant=homemart query="подарок на новоселье" filters={max_price:5000}
    → vector finds housewarming gifts, keyword filters price
```

### 8. Метрики в отчёте теста

Каждый тест-кейс выводит:
```
[OK]  #1 "кроссы для бега" (nike)
      type=vector | keyword=0 vector=15 merged=10 | embed=800ms sql=50ms vec=30ms
      top5: Nike Zoom Fly 5, Nike Pegasus 40, Nike Vaporfly, ...
```

В конце — сводка:
```
SEARCH QUALITY REPORT
=====================
Total: 33 tests
Passed: 30
Failed: 3
  #24 "iphon" — expected iPhone in results, got 0 (typo too strong)
  #26 "ноутбук" on nike — expected 0, got 2 (vector matched "tech" descriptions)
  ...

Avg timing:
  embed: 650ms
  sql: 45ms
  vector: 28ms
  total tool: 750ms
```

## Acceptance Criteria

- [ ] 4 тенанта в БД: nike (53 existing), techzone (~300), fashionhub (~300), homemart (~300)
- [ ] ~950 master products с embeddings (vector(384))
- [ ] Seed-скрипт идемпотентный (повторный запуск не дублирует)
- [ ] 30+ интеграционных тест-кейсов
- [ ] Тесты проходят: `go test -v -run TestSearchQuality -timeout 300s`
- [ ] Каждый тест валидирует: count, brands, categories, search_type
- [ ] Cross-tenant isolation: товары одного тенанта не появляются в другом
- [ ] Graceful degradation: gibberish → 0 results без ошибки
- [ ] Timing report в stdout

## Notes

- Тесты интеграционные — требуют DATABASE_URL и OPENAI_API_KEY
- Skip если env vars не заданы (как agent1_execute_test.go)
- Seed-скрипт отдельный от тестов — запускается один раз
- Данные статические (hardcoded в Go), не генерируются LLM — воспроизводимость
