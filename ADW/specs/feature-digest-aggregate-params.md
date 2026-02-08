# Feature: Digest — агрегация общих параметров (Global Params)

## ADW ID: feature-digest-aggregate-params

## Контекст

**Проблема:** Текущий `ToPromptText()` генерирует ~6300 символов для Sportmaster (254 продукта, 18 категорий). Причина — каждая категория дублирует одни и те же параметры: `color` в 18/18 категориях, `brand` в 18/18, `material` в ~15. Это ~80-100 строк, ~400 лишних токенов в system prompt Agent1.

**Решение:** Механическая агрегация — если параметр встречается в 2+ категориях, выносим на верхний уровень с объединённым диапазоном значений. Из per-category удаляем. Уникальные параметры (`ram`, `storage`, `sole`) остаются per-category.

**Ожидаемый эффект:** ~6300 → ~2500-3000 символов, ~100 строк → ~40 строк, экономия ~200 токенов на каждый запрос.

## Алгоритм

### Шаг 1: Построить per-category digest (как сейчас)

Без изменений — два SQL-запроса, `buildDigestParam`, cardinality-правила.

### Шаг 2: Подсчитать частоту параметров

```go
// paramKey → в скольких категориях встречается
freq := map[string]int{}
for _, cat := range categories {
    for _, p := range cat.Params {
        freq[p.Key]++
    }
}
```

### Шаг 3: Вынести global params (порог >= 2)

Для каждого `param.Key` с `freq >= 2`:
1. Собрать **все уникальные values** со всех категорий где встречается
2. Применить те же cardinality-правила (`buildDigestParam`)
3. Добавить в `GlobalParams []DigestParam`
4. Удалить этот param из каждой категории

### Шаг 4: Обновить ToPromptText

Вывести Global params перед категориями:

```
Tenant catalog: 254 products

Global params:
  brand(14): Adidas, Apple, Asics, Nike, Puma, ... → filter
  color(families: Black/White/Red/Blue/Green/Gray/Yellow/Pink/Brown) → filter
  material(7): Canvas, Leather, Mesh, Nylon, Suede, Synthetic, Wool → filter

Running Shoes (40): 9891-24291 RUB
  sole(27, top: Boost/DNA Flash/DNA Loft v2/... +22) → filter
Casual Shoes (30): 5990-17091 RUB
Laptops (36): 64990-124990 RUB
  ram(range: 8GB-64GB) → filter
  storage(6): 256GB, 512GB, 1TB, ... → filter
  display(range: 13.3 inch-18 inch) → filter
Smartphones (40): 20990-91990 RUB
  ...

Search strategy:
  ...
```

## Изменения в domain entity

**Файл:** `project/backend/internal/domain/catalog_digest_entity.go`

Добавить поле в `CatalogDigest`:

```go
type CatalogDigest struct {
    GeneratedAt   time.Time        `json:"generated_at"`
    TotalProducts int              `json:"total_products"`
    GlobalParams  []DigestParam    `json:"global_params,omitempty"` // NEW
    Categories    []DigestCategory `json:"categories"`
}
```

Новая функция агрегации:

```go
// AggregateGlobalParams extracts params appearing in threshold+ categories
// into GlobalParams. Removes them from per-category Params.
// Values are merged across all categories, then cardinality rules applied.
func (d *CatalogDigest) AggregateGlobalParams(threshold int) {
    // 1. Count frequency of each param key across categories
    // 2. For freq >= threshold: collect all values, merge, buildDigestParam
    // 3. Remove from per-category params
}
```

## Изменения в adapter

**Файл:** `project/backend/internal/adapters/postgres/postgres_catalog.go`

В конце `GenerateCatalogDigest`, перед return:

```go
digest.AggregateGlobalParams(2) // threshold = 2
```

Никаких изменений в SQL. Агрегация — чистая Go post-processing.

## Изменения в ToPromptText

**Файл:** `project/backend/internal/domain/catalog_digest_entity.go`

Обновить `ToPromptText()`:

```go
func (d *CatalogDigest) ToPromptText() string {
    // ... header ...

    // Global params block (NEW)
    if len(d.GlobalParams) > 0 {
        b.WriteString("Global params:\n")
        for _, p := range d.GlobalParams {
            b.WriteString("  ")
            b.WriteString(formatParam(p))
            b.WriteString("\n")
        }
        b.WriteString("\n")
    }

    // Per-category (оставшиеся уникальные params)
    for _, cat := range cats {
        priceStr := formatPriceRange(cat.PriceRange[0], cat.PriceRange[1])
        b.WriteString(fmt.Sprintf("%s (%d): %s\n", cat.Name, cat.Count, priceStr))
        for _, p := range cat.Params {
            b.WriteString("  ")
            b.WriteString(formatParam(p))
            b.WriteString("\n")
        }
    }

    // ... search strategy ...
}
```

## Graceful degradation

- Если у товара нет `color` → просто не попадёт в vector search по цвету, но остальные фильтры (brand, category, price) найдут его
- Если у товара нет `size` → аналогично, не помешает остальному поиску
- Global params — подсказка для Agent1 "такие фильтры есть", а не жёсткое ограничение

## Тесты

**Файл:** `project/backend/internal/domain/catalog_digest_test.go`

Новые unit-тесты:

```go
// TestAggregateGlobalParams_Basic — color+brand в 3 категориях → выносятся
// TestAggregateGlobalParams_Threshold — param в 1 категории → остаётся per-category
// TestAggregateGlobalParams_MergedValues — values из разных категорий объединяются
// TestAggregateGlobalParams_CardinalityRules — агрегированные values подчиняются тем же правилам
// TestAggregateGlobalParams_EmptyAfterExtract — категория без оставшихся params → только header line
// TestToPromptText_WithGlobalParams — проверить формат вывода с global блоком
```

**Файл:** `project/backend/internal/adapters/postgres/catalog_digest_test.go`

Обновить integration-тесты:

```go
// TestGenerateCatalogDigest_Sportmaster — проверить что GlobalParams не пусты
// TestGenerateCatalogDigest_ToPromptText_Integration — проверить размер < 4000 chars
```

## Файлы

| Файл | Действие |
|------|----------|
| `domain/catalog_digest_entity.go` | EDIT — добавить GlobalParams, AggregateGlobalParams(), обновить ToPromptText() |
| `domain/catalog_digest_test.go` | EDIT — добавить unit-тесты агрегации |
| `adapters/postgres/postgres_catalog.go` | EDIT — вызвать AggregateGlobalParams(2) в GenerateCatalogDigest |
| `adapters/postgres/catalog_digest_test.go` | EDIT — обновить assertions (GlobalParams, размер prompt) |

## Что НЕ входит

- Изменения SQL-запросов (агрегация только в Go)
- Изменения в search логике (фильтры работают как раньше)
- Настраиваемый threshold per-tenant (хардкод 2, достаточно)
- Группировка категорий по parent (отдельная задача)

## Verification

```bash
# Unit tests
go test ./internal/domain/ -run TestAggregate -v
go test ./internal/domain/ -run TestToPromptText_WithGlobal -v

# Integration: digest size check
DATABASE_URL=... go test ./internal/adapters/postgres/ -run TestGenerateCatalogDigest -v
# Ожидаем: ToPromptText < 4000 chars (было 6300)

# E2E: restart → Agent1 видит Global params в system prompt
# "покажи чёрные кроссовки" → Agent1 использует color из Global params
```
