# Admin Catalog — M6 COALESCE read-path (admin + V4 engine)

- **Branch:** `main`
- **Date (UTC):** 2026-04-26 13:46
- **Parent commit:** `608bf55` (alpha-0.4 — M4 a/b/c shipped, discovery verified, M4d deferred)
- **Plan:** `/Users/starknight/.claude/plans/wise-foraging-walrus.md` (covers M6 → M8 → M9 → M10 → M11 → M12; M4d/M7 deferred)

## Context

После M4a/b/c discovery agent на dev-store работает end-to-end. M4d (harvester orchestrator + cut-over legacy) и M7 (heybabes 967 backfill) отложены: первый требует сидячей сессии с пользователем, второй — пересмотра кривых русских названий heybabes. План закрывает оставшиеся independent milestones (M6/M8/M9/M10/M11/M12) до финального M4 polish.

M6 — первый из них. Он переключает читалки каталога (admin `ListProducts/GetProduct/UpdateProduct` и V4 engine `ListProducts/GetProduct/VectorSearch`) с прямого `products → master_products` JOIN на двухпутевую резолюцию через `master_variants`. Heybabes продолжает работать через legacy fallback, потому что у его строк `products.master_variant_id IS NULL`, но `products.master_product_id IS NOT NULL`.

## Approach

### Two-path JOIN

```sql
LEFT JOIN catalog.master_variants mv ON mv.id = p.master_variant_id
LEFT JOIN catalog.master_products mp ON mp.id = COALESCE(p.master_product_id, mv.master_product_id)
```

- **Heybabes (legacy)**: `mv` пуст, `mp` резолвится через `p.master_product_id`. Никаких изменений в данных не требуется.
- **Новые тенанты (после M4d)**: `mv` заполнен, `mp` резолвится через `mv.master_product_id`.
- В V4 `VectorSearch` master_products остаётся INNER JOIN (semantic search требует embedding'а на master), но резолюция тоже двухпутевая.

### COALESCE для name

Имя для UI определяется в Go в `mergeProductFromJoins`/`mergeProductWithMaster` по приоритету:
1. `listing.display_name` (override)
2. `listing.original_name` (raw из импорта)
3. `listing.name` (legacy)
4. `master.name`

Картинки: `listing.media[]` (новое) → `listing.images[]` (legacy) → `variant.image_url` → `master.images[]`.

SKU: `variant.sku` → legacy `master.sku`.

### Domain extension

`Product` struct (admin + V4) расширен полями `MasterVariantID, DisplayName, OriginalName, SKU, GTINs, Size, Color, WeightG, VolumeML, RawAttributes, Media`. Старые поля и hardcoded `fieldName`'ы в presets остались как есть — V4 `ProductToMap` дополнительно эмитит новые ключи (`sku, gtins, size, color, weightG, volumeMl, originalName`) для будущих presets'ов.

### `ProductUpdate` для listing-overrides

Расширен `DisplayName *string` и `RawAttributes *map[string]interface{}`. Frontend `ProductDetailPage` теперь шлёт `displayName` (а не `name`) — пишется в `products.display_name` через `NULLIF($n, '')` чтобы пустая строка превращалась в NULL и COALESCE падал на следующий уровень.

### V4 миграция-страховка

В `project_v4/backend/internal/adapters/postgres/catalog_migrations.go` добавлена `migrationCatalogProductsM4Columns`:
- `ALTER TABLE catalog.products ADD COLUMN IF NOT EXISTS master_variant_id, display_name, original_name, raw_attributes, media, deleted_at`
- `CREATE TABLE IF NOT EXISTS catalog.master_variants (...)` минимально под нужды V4 SQL

На production'е admin migrations уже создали эти объекты — но V4 standalone (CI/тестовая БД) теперь тоже бутстрапится сам.

### Deleted_at filter

Все три SQL'я (admin/V4 ListProducts/GetProduct/VectorSearch) теперь фильтруют `WHERE p.deleted_at IS NULL`. До M6 soft-delete существовал только на тенант-уровне через `SoftDeleteProductBySource`, но `ListProducts` его не уважал.

## Files changed

| Scope | File | Change |
|---|---|---|
| admin domain | `project_admin/backend/internal/domain/product.go` | `Product` +9 fields; `ProductUpdate` +DisplayName, +RawAttributes |
| admin adapter | `project_admin/backend/internal/adapters/postgres/catalog_adapter.go` | `ListProducts`/`GetProduct` SELECT/JOIN переписаны на two-path; `mergeProductFromJoins` helper; `UpdateProduct` пишет `display_name` и `raw_attributes` |
| admin frontend | `project_admin/frontend/src/features/catalog/ProductsPage.jsx` | колонка SKU читает `row.sku`; под именем показывается `originalName` если отличается |
| admin frontend | `project_admin/frontend/src/features/catalog/ProductDetailPage.jsx` | секция "Listing overrides" (display_name editable) + "Master fields (read-only)" (brand, sku, gtins, size, color, weight, volume); hero показывает sku/size/color чипами |
| admin frontend | `project_admin/frontend/src/features/catalog/catalog.css` | `.product-cell-original` |
| admin frontend | `project_admin/frontend/src/features/catalog/productDetail.css` | `.pd-section-hint`, `.pd-hero-original` |
| V4 domain | `project_v4/backend/internal/domain/product_entity.go` | `Product` +9 variant/listing fields |
| V4 adapter | `project_v4/backend/internal/adapters/postgres/postgres_catalog.go` | `ListProducts`/`GetProduct`/`VectorSearch` SELECT/JOIN переписаны; `masterProductRow` расширена variant-полями; `mergeProductWithMaster` теперь COALESCE'ит display_name/original_name/name |
| V4 миграции | `project_v4/backend/internal/adapters/postgres/catalog_migrations.go` | `migrationCatalogProductsM4Columns` (idempotent ALTER + CREATE master_variants) для standalone deploy |
| V4 tools | `project_v4/backend/internal/tools/tool_visual_assembly.go` | `ProductToMap` эмитит новые ключи (sku, gtins, size, color, weightG, volumeMl, originalName) |

## Verification

### Local builds (clean)

- `cd project_admin/backend && go build ./... && go vet ./... && go test ./...` — clean (units + usecases tests pass cached)
- `cd project_admin/frontend && npm run build` — `built in 3.35s`, no errors
- `cd project_v4/backend && go build ./... && go vet ./... && go test ./...` — clean (engine_v4, tools, usecases tests pass)

### Что проверить руками после деплоя

1. **Heybabes V4 chat (prod)**: 5-10 реальных запросов через виджет — пресеты `product_card_grid` / `product_detail` рисуются как до M6 (нет регрессии в количестве/качестве выдачи).
2. **Admin /catalog (prod)**: открыть страницу, увидеть heybabes 967 продуктов с превью, ценами, SKU-фолбэком на `master_product_id.slice(0,8)` (новый SKU поле пустое до M4d harvester).
3. **Admin product detail**: правка "Display name" сохраняет в `products.display_name`. Очистка поля → null, имя в каталоге переключается обратно на master.name.
4. **Dev-store (test-tenant с M4 импортом)**: после того как harvester запустится, `master_variant_id` заполнится — продукты должны корректно резолвиться через `mv → mp.master_product_id`.

### Heybabes embedding lookup

`VectorSearch` в V4 продолжает требовать `mp.embedding IS NOT NULL`. Heybabes уже имеет embeddings на `master_products` — резолюция через `p.master_product_id` сохранена. INNER JOIN с master не сменён на LEFT, чтобы semantic search не возвращал записи без embedding'а.

## Known gaps / caveats

1. **`?view=master` query parameter** в плане упоминался, но в M6 не реализован — `ProductDetailPage.jsx` уже показывает оба набора (master read-only + listing editable) одновременно, что покрывает curator-debug use-case на admin-стороне. Для standalone curator UI (M11) тот же endpoint вернёт оба блока в JSON, и curator-frontend сам отрисует master-view.
2. **`UpdateProduct` пишет в `display_name`, не в `name`**. Старый legacy `name` остаётся для совместимости с импортами, которые могут писать туда напрямую. Если разные пути перезапишут оба поля одновременно — в UI по приоритету выиграет `display_name`. Это OK для текущей модели "tenant overrides master", но при M4d cut-over harvester должен писать в `original_name`, а не `name`, чтобы listing override не перетирал raw данные.
3. **Search filter** в admin теперь матчит `mp.name`, `mp.sku`, `mv.sku`, `mp.brand`, `p.display_name`, `p.original_name` — раньше было только `mp.name/sku/brand`. На больших каталогах это лишний overhead; индексы на новые колонки не создавались (можно добавить в M4 polish если перформанс упадёт).
4. **`VectorSearch` без harvester'а** на новых тенантах останется пустым (нет embedding'ов) — это нормально, M4d backfill заполнит.
5. **`ProductsPage` фронт-фильтр по категориям** всё ещё использует legacy `catalog.categories` через `mp.category_id`. M8 переключит на `tenant_categories` через `tenant_listing_categories` М:N.

## Next

M8 — categories M:N + tree editor (`handler_categories.go` новый, DI `categories_v2_adapter`, `CategoryEditor.jsx` drag-drop tree).
