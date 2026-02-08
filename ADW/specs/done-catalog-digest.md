# Done: Catalog Digest — мета-схема каталога для Agent1

**Дата:** 2026-02-08
**Ветка:** `fix/bug1-vector-search-relevance`
**Статус:** Реализовано, все 23 теста проходят

## Что сделано

### Step 1: Migration — колонка `catalog_digest`
**Файл:** `project/backend/internal/adapters/postgres/catalog_migrations.go`
- Добавлена `migrationCatalogDigest` в массив миграций
- `ALTER TABLE catalog.tenants ADD COLUMN IF NOT EXISTS catalog_digest JSONB DEFAULT NULL`

### Step 2: Domain entity
**Файл:** `project/backend/internal/domain/catalog_digest_entity.go` (NEW)
- Structs: `CatalogDigest`, `DigestCategory`, `DigestParam`
- `ToPromptText()` — генерация компактного текста для промпта:
  - Категории с count, price range (kopecks→rubles)
  - Params с хинтами `→ filter` / `→ vector_query` по кардинальности
  - Top-25 категорий при 30+, остальные в "...and N more"
  - Блок "Search strategy" в конце
- `ComputeFamilies()` — hardcoded маппинг цветов в семейства (Red/Blue/Green/...)
- `colorFamilyMap` — 100+ маппингов RU/EN цветов в 11 семейств

### Step 3: Port interface — 4 новых метода
**Файл:** `project/backend/internal/ports/catalog_port.go`
- `GenerateCatalogDigest(ctx, tenantID) → (*CatalogDigest, error)`
- `GetCatalogDigest(ctx, tenantID) → (*CatalogDigest, error)`
- `SaveCatalogDigest(ctx, tenantID, digest) → error`
- `GetAllTenants(ctx) → ([]Tenant, error)`

### Step 4: SQL-реализация
**Файл:** `project/backend/internal/adapters/postgres/postgres_catalog.go`
- `GetAllTenants()` — SELECT всех тенантов
- `GenerateCatalogDigest()` — два SQL-запроса:
  - Query 1: категории + бренды + цены (`GROUP BY c.name, c.slug`, `ARRAY_AGG(DISTINCT mp.brand)`)
  - Query 2: атрибуты с кардинальностью (`LATERAL jsonb_each_text(mp.attributes)`)
- Post-processing: brand как первый param, cardinality rules (≤15→values, 16-50→top+more, 50+→families)
- `buildDigestParam()` — применение правил кардинальности
- `isNumericValues()` / `numericRange()` / `stripNumericSuffix()` — детекция числовых диапазонов (size, ram, storage)
- `SaveCatalogDigest()` — `UPDATE catalog.tenants SET catalog_digest = $2`
- `GetCatalogDigest()` — `SELECT catalog_digest FROM catalog.tenants WHERE id = $1`

### Step 5: Prompt injection
**Файл:** `project/backend/internal/prompts/prompt_analyze_query.go`
- `BuildAgent1ContextPrompt` — новый параметр `digest *domain.CatalogDigest`, добавляет `<catalog>` блок перед `<state>`
- Правила 11-13 в `Agent1SystemPrompt`:
  - 11: использование digest для filter vs vector_query
  - 12: category strategy (specific → filter, broad → omit)
  - 13: high-cardinality attrs → vector_query

### Step 6: UseCase integration
**Файл:** `project/backend/internal/usecases/agent1_execute.go`
- Добавлен `catalogPort ports.CatalogPort` в `Agent1ExecuteUseCase` struct + конструктор
- В `Execute()`: загрузка digest через `GetCatalogDigest`, передача в `BuildAgent1ContextPrompt`
- Ошибка не критична — без digest Agent1 работает как раньше

**Файл:** `project/backend/internal/usecases/pipeline_execute.go`
- `NewPipelineExecuteUseCase` — добавлен параметр `catalogPort`, передаётся в Agent1

### Step 7: Admin endpoint
**Файл:** `project/backend/cmd/server/main.go`
- `POST /admin/generate-digests` — триггерит генерацию для всех тенантов в background
- `generateAllDigests()` — `GetAllTenants()` → для каждого `GenerateCatalogDigest` → `SaveCatalogDigest`
- Без автогенерации при старте — digest генерируется по запросу
- При старте сервер читает уже сохранённый digest из `tenants.catalog_digest` (может быть NULL — ок)

### Step 8: Tests — 23 кейса, все проходят

## Результаты тестов

### Unit tests (domain) — 12/12 PASS, 0.5s

| # | Тест | Статус |
|---|------|--------|
| 1 | `TestDigestParam_ValuesFormat_LowCardinality` | PASS |
| 2 | `TestDigestParam_TopFormat_MediumCardinality` | PASS |
| 3 | `TestDigestParam_FamiliesFormat_HighCardinality` | PASS |
| 4 | `TestDigestParam_RangeFormat_Numeric` | PASS |
| 5 | `TestCatalogDigest_ToPromptText_Basic` | PASS |
| 6 | `TestCatalogDigest_ToPromptText_Empty` | PASS |
| 7 | `TestCatalogDigest_ToPromptText_LargeCategories` | PASS |
| 8 | `TestCatalogDigest_ToPromptText_PriceFormatting` | PASS |
| 9 | `TestCatalogDigest_ToPromptText_BrandHandling` | PASS |
| 10 | `TestCatalogDigest_ToPromptText_FilterHints` | PASS |
| 11 | `TestCatalogDigest_ToPromptText_ServicesNoPrice` | PASS |
| 12 | `TestComputeFamilies_ColorGrouping` | PASS |

### Integration tests (postgres) — 8/8 PASS, ~46s

| # | Тест | Статус | Примечания |
|---|------|--------|------------|
| 13 | `TestGenerateCatalogDigest_Nike` | PASS | 15 products, 8 categories |
| 14 | `TestGenerateCatalogDigest_Sportmaster` | PASS | 24 products, 8 categories, multi-brand |
| 15 | `TestGenerateCatalogDigest_TechStore` | PASS | Smartphones/Laptops/Headphones, numeric ranges |
| 16 | `TestGenerateCatalogDigest_EmptyTenant` | PASS | 0 products, 0 categories |
| 17 | `TestGenerateCatalogDigest_AttributeCardinality` | PASS | Cardinality rules verified |
| 18 | `TestSaveCatalogDigest_RoundTrip` | PASS | Save → Get JSONB roundtrip |
| 19 | `TestGetCatalogDigest_Exists` | PASS | Generate → Save → Get |
| 20 | `TestGetCatalogDigest_NotGenerated` | PASS | nil, no error |

### Prompt tests — 3/3 PASS

| # | Тест | Статус |
|---|------|--------|
| 21 | `TestBuildAgent1ContextPrompt_WithDigest` | PASS |
| 22 | `TestBuildAgent1ContextPrompt_WithoutDigest` | PASS |
| 23 | `TestBuildAgent1ContextPrompt_DigestAndState` | PASS |

## Итого

| Группа | Кейсов | Прошло | OpenAI cost | DB cost |
|--------|--------|--------|-------------|---------|
| Unit (domain) | 12 | 12 | $0 | $0 |
| Integration (postgres) | 8 | 8 | $0 | ~46s |
| Unit (prompts) | 3 | 3 | $0 | $0 |
| **TOTAL** | **23** | **23** | **$0** | **~46s** |

## Файлы (изменённые и созданные)

| Файл | Действие |
|------|----------|
| `adapters/postgres/catalog_migrations.go` | EDIT — migration |
| `domain/catalog_digest_entity.go` | **NEW** — entity + ToPromptText + ComputeFamilies |
| `domain/catalog_digest_test.go` | **NEW** — 12 unit tests |
| `ports/catalog_port.go` | EDIT — 4 новых метода |
| `adapters/postgres/postgres_catalog.go` | EDIT — SQL-реализация Generate/Get/Save/GetAllTenants |
| `adapters/postgres/catalog_digest_test.go` | **NEW** — 8 integration tests |
| `prompts/prompt_analyze_query.go` | EDIT — BuildAgent1ContextPrompt + правила 11-13 |
| `prompts/prompt_analyze_query_test.go` | **NEW** — 3 prompt tests |
| `usecases/agent1_execute.go` | EDIT — catalogPort + digest loading |
| `usecases/pipeline_execute.go` | EDIT — catalogPort parameter |
| `cmd/server/main.go` | EDIT — admin endpoint + generateAllDigests |

## Verification commands

```bash
# Build
cd project/backend && go build ./...

# Unit tests
go test ./internal/domain/ -run "TestDigest|TestCatalogDigest|TestComputeFamilies" -v
go test ./internal/prompts/ -run TestBuildAgent1ContextPrompt -v

# Integration tests (нужен DATABASE_URL)
go test ./internal/adapters/postgres/ -run "TestGenerateCatalogDigest|TestSaveCatalogDigest|TestGetCatalogDigest" -v

# E2E: POST /admin/generate-digests → генерация digest для всех тенантов
```
