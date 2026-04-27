# Admin Catalog — Curator-Driven Pivot, Этап 2.3 (two-mode V4 search)

- **Branch:** `main`
- **Date (UTC):** 2026-04-27 17:39
- **Parent commit:** `73f8916` (Этап 2.2 — harvester-lite)
- **Active plan:** `docs/New features/admin_catalog_curator_pivot_2026-04-27.md`

## Context

После Этапа 2.2 harvester-lite пишет листинги тенантов в `catalog.products` без master-привязок. Для нового тенанта виджет должен находить эти листинги в чате — но V4 keyword search в `ListProducts` искал только по `p.name + mp.name + mp.brand`. Без master-link `mp.*` пусто → search возвращает почти ничего, кроме случайных совпадений в `p.name` (а оно у новых тенантов = title).

Цель Этапа 2.3: расширить keyword-предикат в `ListProducts` чтобы поиск работал в **обоих режимах**:
- **Listing-only** (нет master coverage) — match через `p.original_name`, `p.display_name`, `p.raw_attributes::text` (вытягивает vendor / tags / variants[].sku / variants[].barcode которые harvester-lite кладёт в JSONB)
- **Master-mode** (есть coverage) — продолжает работать как раньше: `mp.name + mp.brand` плюс `p.*`

Это **минимальное** решение без runtime-mode-switch'а. SQL естественно деградирует: для тенанта без master `mp.* IS NULL`, поиск идёт по листинговым полям; для heybabes (100% master coverage) `mp.*` доминирует, плюс листинговые поля добавляют запас.

Vector path не трогаем — он `INNER JOIN master_products` (embedding на master), для тенанта без master вернёт 0 результатов. Это корректное поведение: нет embedding'а — нет семантического поиска.

## What landed

### `project_v4/backend/internal/adapters/postgres/postgres_catalog.go::ListProducts`

Расширен Search clause:

```diff
- (p.name ILIKE $X OR mp.name ILIKE $X OR mp.brand ILIKE $X)
+ (
+   p.name ILIKE $X OR p.display_name ILIKE $X OR p.original_name ILIKE $X OR
+   mp.name ILIKE $X OR mp.brand ILIKE $X OR
+   p.raw_attributes::text ILIKE $X
+ )
```

Multi-word path использует тот же предикат через `fmt.Sprintf` — каждое слово получает свой positional argument.

`raw_attributes::text` — JSONB сериализуется в текстовое представление; ILIKE ловит и значения, и ключи. False positive по ключам приемлим на per-product blob'е (~1-5 KB у Shopify-листинга): vs выгода покрытия `vendor`, `tags`, `variants[].sku`, `variants[].barcode`, `metafields[].value`. Альтернатива — отдельные индексы под каждое вложенное поле — слишком тяжело для unblock'а.

### Что НЕ менялось

- Vector path (`VectorSearch`) — `INNER JOIN master_products` остаётся, embedding fundamentally on master. Тенант без master → vector returns nothing → keyword path несёт всю нагрузку (что и должно быть)
- Master facet filters (skin_type, concern, key_ingredient, и т.д.) — продолжают использовать `$X = ANY(mp.skin_type)` etc. На tenant'е без master они вернут 0 результатов — это корректно: если LLM вызвал `skin_type=oily`, а каталог не классифицирован по skin_type, лучше показать "нет данных" чем guess'ить
- Tool definition `catalog_search` — без изменений. LLM не нужно знать про режимы; просто вызывает search с filters/vector_query, дальше backend сам разбирается
- `agent1_metadata` / `loadFieldLabels` — не трогаю. Когда понадобится дать Agent1'у per-tenant поля (вместо master schema), это будет отдельная задача после первого реального тенанта без master

## Files changed

| File | Action |
|---|---|
| `project_v4/backend/internal/adapters/postgres/postgres_catalog.go` | EDIT (~20 строк) |

## Verification

```
$ cd project_v4/backend && go build ./...
clean

$ go vet ./...
clean

$ go test ./...
ok  	keepstar_v4/internal/engine_v4
ok  	keepstar_v4/internal/tools
ok  	keepstar_v4/internal/usecases
```

### SQL smoke на реальной БД

| Query | Old hits | New hits | Note |
|---|---|---|---|
| `tenant=heybabes, search='cosrx'` | 52 | 52 | No regression — master.brand ловит COSRX как раньше |
| `tenant=heybabes, search='toner'` | 270 (new) | 270 | Listing-level поля бы добавили в режиме без master, но heybabes уже с master — числа одинаковые |

Тестовые expectations при наличии листинг-only тенанта (после Stage 3 dev-store reset):
- `search='snowboard'` хитнет через `p.original_name` или `p.raw_attributes->>product_type`
- `search='SKU-12345'` хитнет через `p.raw_attributes::text` (variants[].sku)
- `search='vanilla'` хитнет через `p.raw_attributes::text` (vendor / metafields)

Этого live-test'а сейчас не делаем (dev-store пока не сброшен). Будет в Этапе 6.

## Known gaps / next steps

- **Master_link_coverage в tool meta** — не делал; для production observability полезно бы вернуть `master_link_coverage: 0.0` в результат tool'а. Пока курратор видит coverage в TenantsPage. Добавим при реальной необходимости в дебаге.
- **Per-tenant Agent1 field set** — для тенанта без master Agent1 предлагает skin_type/concern facets которые ничего не дают. Идеально — генерить per-tenant tool definition (динамически снимать facets если master coverage 0). Отложено: realistic после первой реальной retail'ной интеграции с не-косметическим каталогом.
- **`raw_attributes::text` ILIKE** — перформит O(N×size). Для каталога 1000+ продуктов пойдёт; для 100K+ нужен trigram GIN индекс на `(raw_attributes::text gin_trgm_ops)`. Добавим когда увидим bottleneck.
- **JSONB key matching false positives** — поиск по `raw_attributes::text` совпадёт с ключом "tags" если query="tag". Acceptable cost для покрытия vendor/sku в значениях.
