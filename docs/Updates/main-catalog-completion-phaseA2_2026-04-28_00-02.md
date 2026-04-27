# Catalog Completion — Phase A2 (cleanup) shipped

- **Branch:** `main`
- **Date (UTC):** 2026-04-28 00:02
- **Parent commit:** `90b0e0c` (Phase A foundations)
- **Active plan:** `~/.claude/plans/synchronous-twirling-hoare.md`

## Context

После Phase A foundations в проде осталось 17 lingering snowboard listings в `catalog.products` и 37 stale rows в `catalog.shopify_raw_imports` — артефакты от M4 теста 2026-04-27 (старая dev-store наливка), которые потом были удалены из Shopify но продолжали жить у нас. Это шум, который сбил бы будущий merge agent (Phase C/D): он бы пытался match'ить snowboards с heybabes-cosmetics master'ами.

## What landed

Новый CLI `cmd/cleanup-tenant-stale/main.go` — surgical cleanup стейлд Shopify-listings для одного тенанта:

**Алгоритм** (idempotent, dry-run by default):
1. Запускает свежий Bulk Operations query на текущий каталог тенанта.
2. Собирает set текущих numeric source_id'ов.
3. Считает (а с `-apply` — удаляет) `catalog.products` где `tenant_id=X AND source_system='shopify' AND source_id NOT IN (current_set)`.
4. То же для `catalog.shopify_raw_imports` (`source_kind='product'`).
5. Защита: refuse если current_set пустой (0 продуктов в Shopify) — иначе скрипт удалил бы всё.

Не трогает manually-created products / CSV imports — они keyed по другим (tenant, manual id) cardinality, фильтр по `source_system='shopify'` их пропускает.

**Применено к heybabes / dev-store на проде:**
```
current Shopify catalog: 20 products
stale rows to delete: catalog.products=17, shopify_raw_imports=37
sample stale catalog.products:
  source_id=8286373511357 name="The Collection Snowboard: Liquid"
  source_id=8286373478589 name="The Collection Snowboard: Oxygen"
  source_id=8286373445821 name="The Multi-managed Snowboard"
  ... (остальные 14 — все Snowboard'ы)
APPLIED: catalog.products deleted=17, shopify_raw_imports deleted=37
```

## Verification

После cleanup'а прогон `cmd/sync-tenant-now`:
```
products=20 (DumpToStaging, чистое наполнение staging — fresh)
products_written=20 failures=0
attribute_candidates_upserted=22
category_candidates_upserted=22
tenant_categories_upserted=22
```

20 свежих dev-store листингов — никаких lingering snowboards. Candidates flow стабильно работает.

## Files changed

| File | Action |
|---|---|
| `project_admin/backend/cmd/cleanup-tenant-stale/main.go` | NEW (~270 строк) |

## Why this is reusable, not throwaway

CLI остаётся в репо потому что:
- Same flow понадобится курратору/devops при будущей очистке любого тенанта (мердж двух стораджей/тестовый импорт случайно вылил мусор/etc.)
- Dry-run + sample print защищает от misfire
- Логика «keep only what's currently in Shopify» — правильный invariant: source-of-truth это Shopify, наша БД это derivative

## Next

Phase C — interactive design сессия по merge agent с пользователем. Через AskUserQuestion итерации, без pre-baked решений. Артефакт — переписанный Stage 4-5 раздел в `docs/New features/admin_catalog_curator_pivot_2026-04-27.md`.
