# Admin Catalog — Curator Pivot, Post-Deploy Smoke Findings (Open Issues)

- **Branch:** `main`
- **Date (UTC):** 2026-04-27 19:23
- **Parent commit:** `e702823` (deploy wrap-up)
- **Active plan:** `docs/New features/admin_catalog_curator_pivot_2026-04-27.md`

## Context

После деплоя пайплайна (Этапы 0-3) пользователь зашёл в прод-админ для smoke-проверки и нашёл **4 проблемы**. Этот лог — диагностика и приоритизация для следующей сессии.

## Issues

### 🔴 #1 — CRITICAL: harvester-lite не получает variants/metafields/images/collections

**Симптом**: продукт-карточка в админ UI пустая — price=0, stock=0, no image, no rating. Несмотря на то что seed-devstore создал продукты с variants и metafields.

**Root cause**: `0 / 37` staging-продуктов имеют `_v2_variants` или `_v2_metafields` в payload. Bulk JSONL от Shopify Admin API 2026-04 **не возвращает nested children** в формате который ожидает наш `streamJSONLToStaging` (одна product-row + children-rows с `__parentId`). Прислан только bare product node:

```json
{
  "id": "gid://shopify/Product/8288206880957",
  "title": "Gift Wrap Service",
  "tags": [...], "vendor": "Keepstar Store",
  "options": [{"name": "Size", "values": ["Small", "Large"]}],
  "descriptionHtml": "...",
  "featuredImage": null
  // NO variants, NO images, NO metafields, NO collections
}
```

**Impact**: ломает весь pipeline. Без variants нет SKU/price/inventory/barcode для match cascade, без metafields нет PIM-данных для merge, без collections нет fuzzy category match. Тестировать merge agent на этом нельзя.

**Hypothesis для расследования** (next session):
1. Bulk Operations API в 2026-04 поменял отдачу nested edges — возможно теперь нужно отдельные bulk queries на variants/metafields, или children приходят но scanner их не ловит
2. Наш GraphQL query `bulkProductsQueryBody` (`adapters/shopify/client.go:402`) либо устарел, либо children-edges нужно явно whitelist'ить через `pageInfo`
3. Scanner's `__parentId` matching сломан в новой schema (например `__parentId` теперь полное GID а не просто id)
4. Bulk operation вернула URL без children (нужно посмотреть raw JSONL что Shopify реально присылает)

**Diagnose first** — скачать raw JSONL ручкой `FetchBulkJSONL` и посмотреть что внутри. Если children там есть → bug в нашем scanner. Если нет → проблема query.

### 🟡 #2 — CORRECTION: Heybabes catalog.products **на русском** (978/1016 cyrillic в `name`)

**Что я сказал раньше** (ошибочно): heybabes 100% English, готовый seed master-каталога.

**Что в реальности**:
- `catalog.master_products` (heybabes-owned, 979 строк): 978/979 английских в `name` ✅
- `catalog.products` (heybabes listings, 1016 строк): **962/1016 русские** в `name`, `original_name` пустой у всех ❌

То есть **listings и masters в разных языках** — masters на английском (PIM-обогащены LLM ранее), listings на оригинальном русском.

**Impact**: minimal. Per pivot решено heybabes listings **не трогать** — они работают как есть. Master_products остаются английскими и пригодны как seed cosmetics master-каталога. Но если кому-то понадобится `display_name` на listings в админ UI heybabes — нужно отдельный backfill.

**Action**: обновить status snapshot в pivot doc — корректное понимание ситуации.

### 🟡 #3 — UI: Disconnect-кнопка недостижима

**Симптом**: пользователь не видит disconnect-кнопку.

**Root cause**: `IntegrationsPage.jsx` (с disconnect-кнопкой в actions колонке) **не имела ссылки в sidebar**. Пользователь смотрел `/import` (где disconnect не предусмотрен).

**Fix**: в `DashboardLayout.jsx` добавлен NavLink на `/integrations` (иконка Plug, под "Import" в sidebar). После следующего деплоя кнопка будет доступна.

### 🟢 #4 — BY DESIGN: tenant_categories пустые для heybabes / нет категорий в product detail

**Симптом**: в admin product detail "No categories yet — add some on the Categories page". Категорий тенанта нет вообще (`tenant_categories WHERE tenant_id=heybabes` → 0).

**Root cause**: harvester-lite кладёт `collections` только в `raw_attributes` (по дизайну), **не auto-link'ает** в `tenant_categories` / `tenant_listing_categories`. Это работа merge agent в Этапе 5 — он смотрит коллекции, мапит на master_categories с fuzzy match, и создаёт записи в tenant_categories.

**Impact**: до Этапа 5 категорий у тенанта не будет. Это ожидаемо.

**Note**: связано с #1 — даже если бы harvester-lite auto-link'ал коллекции, у нас в raw_attributes их сейчас нет (см. #1).

## Files changed in this commit

- `project_admin/frontend/src/features/layout/DashboardLayout.jsx` — добавлен NavLink на `/integrations` (#3 fix)

## Priority для next session

1. **Сначала #1** — без рабочих variants/metafields ничего нельзя тестировать дальше. 1-2 часа debug + fix
2. После #1 — re-run `cmd/sync-tenant-now`, проверить что catalog.products теперь имеет полные raw_attributes
3. Затем Этап 4 (merge agent design) уже будет на live данных
4. #2 — никаких действий, просто откорректировать status snapshot
5. #3 — fix уже есть в commit, запушим вместе с этим логом
6. #4 — естественно решится в Этапе 5

## State в БД на момент написания

- 37 листингов под heybabes / `source_system='shopify'` в catalog.products (20 seed + 17 lingering)
- Все с `master_*_id = NULL` (✅ pivot)
- raw_attributes содержит: `vendor, product_type, tags, handle` — но **НЕТ** variants/metafields/images/collections (см. #1)
- catalog.master_products created today: 0 (✅ legacy выпилен)
- tenant_categories для heybabes: 0
