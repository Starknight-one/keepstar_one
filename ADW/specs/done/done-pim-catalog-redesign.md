# Done: PIM Catalog Redesign — Structured Columns + Ingredients + Typed Search

**Дата:** 2026-02-18
**Ветка:** `feature/pim-catalog-redesign`
**Статус:** Миграции применены, enrichment v2 запущен (125/962 — баг с SKU matching, требует фикса)

## Проблема

Каталог 962 товара, всё в одной таблице `master_products` с JSONB `attributes` куда свалено: маркетинговый текст, INCI составы, enriched enum-поля. Названия 86 символов в среднем (маркетинг + бренд + product name в одном поле). 74% товаров в 2 из 25 категорий. Фильтрация по JSONB неэффективна. Embeddings строятся из каши (name + description + INCI + marketing) — 500-1000 токенов шума.

## Что сделано

### 1. Миграция БД — новые колонки и таблицы

**19 PIM-колонок на `catalog.master_products`:**
- Идентификация: `short_name`, `original_name`, `product_line`
- Enum-поля: `product_form`, `texture`, `routine_step`, `routine_time`, `application_method`
- Массивы (TEXT[] + GIN): `skin_type`, `concern`, `key_ingredients`, `target_area`, `free_from`
- Текст: `marketing_claim`, `benefits`, `how_to_use`, `volume`, `inci_text`
- Версионирование: `enrichment_version SMALLINT DEFAULT 0`

**Новые таблицы:**
- `catalog.ingredients` — справочник INCI (inci_name, name_ru, slug, function)
- `catalog.product_ingredients` — junction (master_product_id, ingredient_id, position, is_key)

**Индексы:** 5 B-tree (`product_form`, `routine_step`, `brand`, `short_name`, `(category_id, product_form)`) + 5 GIN (`skin_type`, `concern`, `key_ingredients`, `target_area`, `free_from`)

**Файлы:**
- `project/backend/internal/adapters/postgres/catalog_migrations.go`
- `project_admin/backend/internal/adapters/postgres/catalog_migrations.go`

### 2. Domain Entities — новые поля

**MasterProduct:** +19 PIM-полей (ShortName..EnrichmentVersion)
**Product:** +11 PIM-полей (ShortName..Volume)

**Файлы:**
- `project/backend/internal/domain/product_entity.go`
- `project/backend/internal/domain/master_product_entity.go`
- `project_admin/backend/internal/domain/product.go`

### 3. Enrichment V2 — новый LLM промпт

Новый `EnrichProductsV2` метод с расширенным системным промптом:
- 18 структурированных полей из закрытых списков
- `short_name` без бренда и маркетинга (2-3 слова)
- `marketing_claim` max 150 символов
- Enum-ы: product_form (20 вариантов), texture (9), routine_step (7), skin_type (7), concern (15), key_ingredients (25), target_area (8), free_from (10)

**EnrichFromDB flow:** читает master_products по tenant → строит inputs → батчи по 10, 5 воркеров → Haiku API → парсит JSON → записывает в PIM-колонки + enrichment_version=2

**Файлы:**
- `project_admin/backend/internal/domain/enrichment.go` — EnrichmentOutputV2
- `project_admin/backend/internal/adapters/anthropic/enrichment_client.go` — EnrichProductsV2 + systemPromptV2
- `project_admin/backend/internal/usecases/enrichment.go` — EnrichFromDB
- `project_admin/backend/internal/adapters/postgres/catalog_adapter.go` — UpdateMasterProductPIM, GetAllMasterProducts
- `project_admin/backend/internal/ports/catalog_port.go` — интерфейс
- `project_admin/backend/internal/ports/enrichment_port.go` — интерфейс

### 4. API Endpoint — enrich-v2

`POST /admin/api/catalog/enrich-v2` — принимает `{"tenantId": "..."}`, запускает EnrichFromDB в фоне.

**Файлы:**
- `project_admin/backend/internal/handlers/handler_enrichment.go` — HandleEnrichV2
- `project_admin/backend/cmd/server/main.go` — регистрация роута

### 5. Typed Search Filters (chat backend)

**ProductFilter и VectorFilter:** заменены generic JSONB фильтры на типизированные — `ProductForm`, `SkinType`, `Concern`, `KeyIngredient`, `TargetArea`, `RoutineStep`, `Texture`.

**SQL:** `mp.product_form = $N`, `$N = ANY(mp.skin_type)`, etc.

**Tool definition:** enum-ы вместо freetext в JSON schema для агента.

**Display:** если `short_name` заполнен → показывается как "short_name (brand)".

**Файлы:**
- `project/backend/internal/ports/catalog_port.go`
- `project/backend/internal/adapters/postgres/postgres_catalog.go`
- `project/backend/internal/tools/tool_catalog_search.go`

### 6. Embeddings — чистый текст

`buildEmbeddingText()` — для enrichment_version >= 2 строит из PIM-полей (~30 токенов):
`short_name + brand + category + product_form + texture + marketing_claim + skin_type + concern + key_ingredients + routine_step`

Для legacy — fallback на `name + brand + category`.

**Файлы:**
- `project_admin/backend/internal/usecases/import.go`
- `project_admin/backend/cmd/rebuild-embeddings/main.go` (новый CLI)

### 7. Seed Ingredients CLI

`cmd/seed-ingredients/main.go` — парсит INCI из attributes, создаёт справочник, линкует через junction, обогащает через Haiku (name_ru + function).

## Известные проблемы

### SKU Matching Bug (enrichment v2)
97/97 батчей обработаны Haiku ($1.81), но только 125/962 продуктов записались. LLM возвращает все продукты (293K output tokens), но SKU из ответа не матчатся с SKU в `skuToID` map. Вероятные причины:
- Haiku возвращает числовые SKU как JSON numbers (`158321` вместо `"158321"`), Go json.Unmarshal для `string` поля тихо фейлит
- Haiku модифицирует SKU (добавляет префиксы/форматирование)

**Диагностика добавлена:** логирование `enrichment_v2_sku_miss` с первыми 10 промахами + итоговая статистика `enrichment_v2_sku_stats`.

**Не запущено:**
- seed-ingredients
- rebuild-embeddings

## Стоимость

| Операция | Токены | Стоимость |
|----------|--------|-----------|
| Enrichment v2 (attempt 1) | 794K in / 293K out | $1.81 |

## Верификация

- [x] Оба бэкенда компилируются: `go build ./...`
- [x] Миграции применены: 19 колонок + 2 таблицы + 10 индексов
- [x] Enrichment v2 endpoint работает, Haiku отвечает
- [ ] SKU matching fix → re-run enrichment
- [ ] seed-ingredients
- [ ] rebuild-embeddings
- [ ] Проверка typed search filters
