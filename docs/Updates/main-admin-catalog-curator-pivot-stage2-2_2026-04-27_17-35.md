# Admin Catalog — Curator-Driven Pivot, Этап 2.2 (harvester-lite)

- **Branch:** `main`
- **Date (UTC):** 2026-04-27 17:35
- **Parent commit:** `554feb1` (Этап 2.1 — cut legacy)
- **Active plan:** `docs/New features/admin_catalog_curator_pivot_2026-04-27.md`

## Context

Этап 2.1 удалил legacy ShopifyUseCase и оставил `ShopifyV2UseCase.HandleWebhook` + `runInitialIngest` no-op'нутыми (с info-логом `harvester_not_wired`). После Connect клиента в `catalog.products` ничего не записывалось — данные доходили только до `catalog.shopify_raw_imports`.

Этап 2.2 — это та подсистема, которая закрывает цикл: **harvester-lite применяет Tier-1 deterministic mapping**: читает staging → пишет в `catalog.products` (без `master_*`). Виджет тенанта может работать сразу через listing-only поиск (Этап 2.3).

Принципы:
- **НЕ трогает `master_*`.** Все master-связи устанавливаются исключительно через curator merge agent (Этап 5)
- **Tier-1 без LLM.** Только детерминированные маппинги: title→original_name, body_html→description (HTML stripped), vendor/product_type/tags/handle→raw_attributes, variants[]/metafields[]/collections[]→raw_attributes (preserved structurally)
- **Идемпотентность.** Re-run перезаписывает листинг по `(tenant_id, source_system='shopify', source_id=<numeric Shopify ID>)` через unique partial index
- **Сохраняет master-link при re-run.** UPDATE не трогает `master_product_id` / `master_variant_id` — если курратор уже смерджил, repeated import не разрушит linkage

## What landed

### Schema migration (idempotent, авто-применяется на старт admin)

`catalog_migrations.go`:
```sql
CREATE UNIQUE INDEX IF NOT EXISTS idx_catalog_products_tenant_source_unique
ON catalog.products (tenant_id, source_system, source_id)
WHERE source_system IS NOT NULL AND source_id IS NOT NULL;
```

Существующий `idx_catalog_products_tenant_source` был **non-unique** — не годится как ON CONFLICT target. Новый partial index только для строк с source — манульно созданные продукты (CSV / admin UI) живут вне этой ограничения.

Применён в БД (idempotent: `CREATE INDEX` без падений).

### Domain + port + adapter

`internal/domain/product.go` — `ListingFromSource` struct: TenantID/SourceSystem/SourceID + Name/OriginalName/Description (plain text)/PriceCents (минимум по variants)/Currency/StockQuantity (сумма по variants)/Images flat list/Media jsonb/RawAttributes (vendor/product_type/tags/handle/variants[]/metafields[]/collections[])/PayloadHash sha256.

`internal/ports/catalog_port.go` — `UpsertListingFromSource(ctx, *ListingFromSource) (id, error)`.

`internal/adapters/postgres/catalog_adapter.go` — реализация `UpsertListingFromSource`. SQL:
```sql
INSERT INTO catalog.products (
  tenant_id, name, original_name, description,
  price, currency, stock_quantity, images, media, raw_attributes,
  source_system, source_id, payload_hash, created_at, updated_at)
VALUES (...)
ON CONFLICT (tenant_id, source_system, source_id) WHERE source_system IS NOT NULL AND source_id IS NOT NULL
DO UPDATE SET
  name = EXCLUDED.name, original_name = EXCLUDED.original_name, description = EXCLUDED.description,
  price = EXCLUDED.price, currency = EXCLUDED.currency, stock_quantity = EXCLUDED.stock_quantity,
  images = EXCLUDED.images, media = EXCLUDED.media, raw_attributes = EXCLUDED.raw_attributes,
  payload_hash = EXCLUDED.payload_hash, deleted_at = NULL, updated_at = NOW()
```

Заметка: `master_product_id` / `master_variant_id` НЕ в SET clause — re-import после merge'а сохраняет linkage.

### Use case `usecases/harvester_lite.go` (~330 строк)

`HarvesterLiteUseCase` имплементит `usecases.HarvesterLite` interface (объявлен в `shopify_v2.go` Этап 2.1):

| Метод | Что делает |
|---|---|
| `RunForTenant(ctx, tenantID)` | Стримит `shopify_raw_imports` (kind='product') через `IterateProducts`, парсит каждую строку через `parseBulkProduct`, вызывает `UpsertListingFromSource`. Возвращает count. Per-product failures логируются но не abort'ят run |
| `UpsertOne(ctx, tenantID, body)` | Парсит Shopify webhook payload (REST shape) через `parseWebhookProduct`, upsert |
| `SoftDeleteOne(ctx, tenantID, sourceID)` | Делегирует на `catalog.SoftDeleteProductBySource` |

Внутренние helper'ы:
- `shopifyListingView` — canonical intermediate shape between bulk и webhook парсерами
- `parseBulkProduct(payload)` — bulk JSONL: `id` (GID), `title`, `descriptionHtml`, `vendor`, `productType`, `tags` (array OR string), `featuredImage`, `_v2_variants[]`, `_v2_images[]`, `_v2_metafields[]`, `_v2_collections[]`
- `parseWebhookProduct(body)` — REST: `id` (numeric), `title`, `body_html`, `vendor`, `product_type`, `tags` (CSV), `images[]`, `variants[]` со snake_case ключами
- `canonicalizeBulkVariant(raw)` — flatten variant из bulk shape → канонические ключи: `id, sku, title, price_cents (int), compare_cents, barcode, inventory_qty, options{name→value}, weight_value, weight_unit, image`
- `aggregateVariants(variants) → (minPriceCents, totalStock)` — листинговые price/stock агрегаты
- `extractNumericID(gid)` — `gid://shopify/Product/12345` → `12345`
- `parsePriceCents(s)` — Shopify Money string `"12.34"` → `1234`
- `parseTagsField(raw)` — поддерживает обе формы (array из bulk, CSV из webhook)
- `stripHTML(html)` — наивный regex-стриппер `<[^>]+>` для description'а

### Wire-up в main.go

```go
harvesterLite := usecases.NewHarvesterLite(shopifyStagingAdapter, catalogAdapter, log)
shopifyV2UC.SetHarvester(harvesterLite)
```

Late-bind через `SetHarvester` (объявлен в Этапе 2.1) избегает import-cycle: harvester живёт в `usecases`, V2 UC тоже в `usecases`, но V2 не должна знать про конкретную имплементацию harvester'а.

### Unit-тесты `harvester_lite_test.go`

8 test'ов, все проходят:
- `TestParsePriceCents` — corner cases (empty, "12", "12.3", "12.345" → truncate, "0.99")
- `TestExtractNumericID` — GID parsing
- `TestStripHTML` — basic tags + multiple + whitespace collapse
- `TestParseCSVTags` / `TestParseTagsField` — обе формы tags из Shopify
- `TestParseBulkProduct` — реалистичная JSONL row с variants/metafields/collections, проверяет aggregation
- `TestParseWebhookProduct` — REST webhook body (snake_case), variants с option1/option2 + grams
- `TestParseWebhookProduct_MissingID` — error path

## Files changed

| Scope | File | Action |
|---|---|---|
| backend | `project_admin/backend/internal/usecases/harvester_lite.go` | NEW (~330 строк) |
| backend | `project_admin/backend/internal/usecases/harvester_lite_test.go` | NEW (~220 строк) |
| backend | `project_admin/backend/internal/domain/product.go` | EDIT (+`ListingFromSource` struct, ~20 строк) |
| backend | `project_admin/backend/internal/ports/catalog_port.go` | EDIT (+`UpsertListingFromSource` метод) |
| backend | `project_admin/backend/internal/adapters/postgres/catalog_adapter.go` | EDIT (+`UpsertListingFromSource` реализация, ~50 строк) |
| backend | `project_admin/backend/internal/adapters/postgres/catalog_migrations.go` | EDIT (+unique partial index migration) |
| backend | `project_admin/backend/cmd/server/main.go` | EDIT (DI: `NewHarvesterLite` + `SetHarvester`) |

## Verification

```
$ cd project_admin/backend && go build ./...
clean

$ go vet ./...
clean

$ go test ./...
ok  	keepstar-admin/internal/units
ok  	keepstar-admin/internal/usecases  (8 new harvester_lite_test PASS + existing tests pass)
```

### Live state в БД

`catalog.shopify_raw_imports` сейчас содержит 17 dev-store snowboard'ов под `tenant_id=hey-babes-cosmetics` (артефакты от M4abc test session). После applying migration → если повторно запустить Connect для dev-store через новый pipeline:
- `runInitialIngest` фоном вызовет `DumpToStaging` (overwrite staging)
- Затем `harvesterLite.RunForTenant` запишет 17 новых строк в `catalog.products` с `source_system='shopify'`, `source_id=<numeric>`, и оригинальным title в `original_name`. БЕЗ `master_*_id`.

Этого пока не делаем — `master_products` всё ещё содержит мусорные snowboard'ы от legacy (Этап 7 cleanup). Реальный live-test произойдёт в Этапе 6 после release нового dev-store с тестовыми данными.

### Behavior после wire-up

- **Connect (новый тенант)** → OAuth → DumpToStaging → `harvesterLite.RunForTenant` → `catalog.products` наполнен листингами с source_system='shopify'. status='connected'. master_*_id = NULL для всех новых строк.
- **Webhook products/create или products/update** → `harvesterLite.UpsertOne` → upsert одного листинга. Existing master_*_id (если был установлен curator'ом) сохраняется.
- **Webhook products/delete** → `harvesterLite.SoftDeleteOne` → flip `deleted_at`.
- **Webhook app/uninstalled** → status=disconnected (без изменений с Этапа 2.1).

## Known gaps / next steps

- **Этап 2.3** — двухрежимный search в V4 (listing-only когда master_link_coverage<30%, master+listing когда ≥30%). Без него virджет нового тенанта будет искать по существующему master-режиму V4 и ничего не найдёт.
- **Curator-triggered re-run** — кнопка "Re-run harvester" на TenantDetailPage. Сейчас единственный trigger — Connect. Когда merchant обновляет каталог (без webhook'а — например пакетная заливка), курратор должен мочь руками передёрнуть. Добавим в Этапе 5 параллельно с merge-agent UI.
- **Per-tenant DELETE staging при reinstall** — `staging.DeleteByTenant` есть, но `runInitialIngest` его не вызывает. На fresh reinstall старые staging-строки могут содержать продукты которых уже нет в Shopify — тогда после re-dump в staging-овом DELETE поможет. Сейчас рискуем "stale" staging-rows. Опционально add'нем в Этап 6.
- **Webhook payload может приходить ДО того как metafields подтянуты** — REST webhook body не содержит metafields. Раньше legacy делал доп. fetch (`GetProductMetafields`). Сейчас webhook UpsertOne пишет без metafields. Курратор после merchant'ского обновления может re-trigger Run для синка metafields через bulk pull. Acceptable trade-off.
- **HTML stripper наивный** — regex `<[^>]+>` не парсит вложенные структуры, не декодирует entities. Для description'ов в чате приемлимо; если будем рендерить description в карточке/чате — нужен proper parser (например `golang.org/x/net/html`).
