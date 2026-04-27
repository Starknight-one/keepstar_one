# Catalog Completion — Phase A (foundations) shipped

- **Branch:** `main`
- **Date (UTC):** 2026-04-28 00:00 (local 03:00)
- **Parent commit:** `7c6f28c` (code quality audit doc)
- **Active plan:** `~/.claude/plans/synchronous-twirling-hoare.md`

## Context

После pivot 2026-04-27 (Stages 0-3 deployed) каталог в проде **не работал по-настоящему**:

- `catalog.products.raw_attributes` пустые — `_v2_variants/_v2_metafields/_v2_images/_v2_collections` не доходили до staging
- `master_attribute_candidates` / `master_category_candidates` мёртвые — никто не инкрементил
- `tenant_categories` пустые — harvester-lite вообще не писал в эту таблицу
- Promotion-через-ALTER-TABLE для Tier-2 — risky под прод-нагрузкой

Phase A — fixes для всего перечисленного. Без этой фазы Stage 4-5 (merge agent) тестировать невозможно.

## What landed

### A1. Bug #1: two-pass JSONL scanner

**Симптом**: 0 / 37 staging-продуктов имели `_v2_*` ключи.

**Root cause**: `streamJSONLToStaging` (project_admin/backend/internal/usecases/shopify_v2.go) был написан под допущение «children приходят ДО parent». Эмпирически в Shopify Admin API 2026-04 порядок **обратный** — parent сначала, потом его children. Старый код встречал product, флашил пустой accumulator, писал bare product в staging, а children приходили после и оседали в `pending` map без выгрузки.

**Fix**: переписал `streamJSONLToStaging` на two-pass подход:
- Pass 1: бакетим всё — top-level Products в `map[productGID]json.RawMessage` с сохранением порядка вставки; child rows в `map[parentGID][]childRow` с пред-классификацией kind по GID.
- Pass 2: для каждого product мерджим children и upsert в staging.

Robust к любому порядку, лишних branches на спекулятивный flush нет. Мемори-нота в комментарии: bulk op кэпится Shopify'ем (~1-5M lines), для 100k catalog × 3 children ≈ 300MB raw bytes — OK для Railway tier.

**Также**: новый CLI `cmd/dump-bulk-jsonl/main.go` для on-demand диагностики — подключается тем же путём что `cmd/sync-tenant-now` (decrypt token via secretbox, run/poll bulk op, fetch JSONL), пишет first N lines в файл + печатает гистограмму типов. Пригодится для будущих регрессий.

**Verification**: после fix sync-tenant-now на dev-store: `shopify_v2_jsonl_summary` лог `products=20, orphans=0` — все 20 продуктов получили детей. До фикса все children были orphan'ами.

### A3. Migration: `master_field_definitions` + `tier2` JSONB

Добавлено в `catalog_migrations.go`:

- `catalog.master_field_definitions` (id/vertical/key/label/type/enum_values/source/promoted_from_candidate/promoted_at/promoted_by, UNIQUE(vertical, key))
- `master_products.tier2 JSONB DEFAULT '{}'` + GIN-индекс
- Partial unique index `idx_tenant_categories_tenant_external` (M8-задизайнен, но фактически не зашиплен в миграции — добавлен сейчас)

**Naming note**: таблица `master_field_definitions`, не `field_definitions`. У V4 (`project_v4/backend`) уже есть `catalog.field_definitions` с другой семантикой (per-tenant rendering schema для chat-engine atom/slot meta). Две разные концепции в одной schema требуют разных имён, не ломая существующее. В коммент-блоке миграции явно расписано почему.

**Backwards-compat**: `master_cosmetics`, `master_laptops` остаются как были. ALTER-механика в curator пока не тронута — это Phase B2.

### A4. Wire candidates flow в harvester-lite

В `harvester_lite.go`:

- Добавлены опциональные поля `candidates ports.CandidatesPort`, `categories ports.CategoriesPort` + `SetSignals` setter.
- Новая helper `recordSignals(ctx, tenantID, view, counters)`:
  - `ClassifyVertical(productType, vendor, tags)` → cosmetics / furniture / footwear / unknown.
  - Для каждого metafield key (если не в Tier-1 reserved blacklist) → `UpsertAttributeCandidate(key, vertical, sampleValue, agentMeta)`.
  - Для каждой collection → `UpsertCategoryCandidate(name, "", vertical)` + `UpsertTenantCategory(externalID, slug, name, kind)` где `kind` = `ClassifyKind(name)`.
- Run-level лог обогащён счётчиками: `attribute_candidates_upserted`, `category_candidates_upserted`, `tenant_categories_upserted`.

**DI**: в `cmd/server/main.go` `candidatesAdapter` + `categoriesV2Adapter` подняты выше Shopify-блока (раньше создавались позже handlers); harvester-lite получает их через `SetSignals` сразу после `NewHarvesterLite`.

**В `cmd/sync-tenant-now/main.go`** тоже добавлены candidates+categories adapters + SetSignals + `RunCatalogMigrations` на старте (CLI теперь применяет миграции idempotent перед прогоном — чтобы dev branch с свежей миграцией работал без ручного дополнительного шага).

### A5. Category kind + vertical classifier

Новый файл `internal/usecases/category_classifier.go`. Две pure-функции, без IO:

- `ClassifyKind(name) → category|showcase|promo` — regex/keyword matcher на ru+en (`promoSignals`, `showcaseSignals`). `%` → promo всегда. Default = category.
- `ClassifyVertical(productType, vendor, tags) → cosmetics|furniture|footwear|unknown`. Default = unknown — курратор позже руками re-классифицирует через UI.

Обе функции keyword-based + детерминированы. LLM здесь не нужен — easy cases (`%` в названии = promo) не требуют семантики, для grey-zone есть merge agent в Stage 4-5.

### Side-fix: ON CONFLICT с partial index

В `categories_v2_adapter.go::UpsertTenantCategory` ON CONFLICT clause переписан на `(tenant_id, external_id) WHERE external_id IS NOT NULL` — чтобы соответствовать predicate'у нового partial unique index. Без `WHERE` Postgres даёт SQLSTATE 42P10 ("no unique or exclusion constraint matching").

## Files changed

| Scope | File | Action |
|---|---|---|
| backend | `internal/usecases/shopify_v2.go::streamJSONLToStaging` | EDIT — two-pass rewrite (~120 строк) |
| backend | `internal/usecases/category_classifier.go` | NEW (~80 строк) |
| backend | `internal/usecases/harvester_lite.go` | EDIT — `SetSignals` + `recordSignals` (~140 строк добавлено) |
| backend | `internal/adapters/postgres/catalog_migrations.go` | EDIT — `master_field_definitions` + `tier2` + partial index tenant_categories (~50 строк) |
| backend | `internal/adapters/postgres/categories_v2_adapter.go` | EDIT — fix ON CONFLICT clause for partial index |
| backend | `cmd/dump-bulk-jsonl/main.go` | NEW (~250 строк диагностики) |
| backend | `cmd/sync-tenant-now/main.go` | EDIT — RunCatalogMigrations on entry, SetSignals wire |
| backend | `cmd/server/main.go` | EDIT — candidates+categories adapters lifted above Shopify block |

## Verification

`go build && go vet && go test ./internal/...` — clean.

End-to-end на dev-store (live, прод Neon DB):
```
DumpToStaging: 20 products / 0 collections / 2 metafield_defs in 14s
shopify_v2_jsonl_summary: products=20, orphans=0
HarvesterLite: products_written=37 failures=0 attribute_candidates_upserted=22 category_candidates_upserted=22 tenant_categories_upserted=22 in 23s
```

37 listings = 20 fresh dev-store + 17 lingering snowboard'ов от вчерашнего M4 теста (Phase A2 cleanup'нет это в следующем коммите).

## Known gaps / next

- **Phase A2 (cleanup)** — DELETE 17 snowboard'ов в `catalog.products` + `catalog.shopify_raw_imports` для tenant_id=heybabes WHERE created_at от вчерашнего теста. Destructive, дёрну после approve пользователя.
- **Phase B1** — shared `pkg/catalog` (master-link JOIN дубл 7+ раз в admin/curator/V4). Не блокирует; план — после A2.
- **Phase B2** — переписать `curator/.../PromoteAttribute` с ALTER TABLE на `INSERT INTO master_field_definitions` + backfill через `jsonb_set(tier2, ...)`. Адаптер ещё не trampoline'нут.
- **Phase B3** — TODO marker на 30%-threshold в `tool_catalog_search.go`.
- **`shopify_v2_menu_unavailable`** — в первом запросе menu API даёт `Field 'menu' is missing required arguments: id`. Не блокирующее, прод всё равно запрашивает по handle="main-menu" и валится на первом запросе. Зафиксировано в логе, фикс отдельным коммитом (нужно добавить ID-resolve через `shop.menus`).

## State в БД на момент коммита

- `catalog.products` под heybabes: **37** (20 свежих + 17 lingering, все с `master_*_id = NULL`, теперь с **полными `raw_attributes` включая variants/metafields/collections**)
- `catalog.master_attribute_candidates`: 22 строки (свежие, vertical=cosmetics/furniture/footwear/unknown)
- `catalog.master_category_candidates`: 22 строки
- `catalog.tenant_categories` для heybabes: 22 строки (включая classified kind: showcase/promo/category)
- `catalog.master_field_definitions`: 0 строк (таблица создана, заполнится через curator promote после Phase B2)
- `catalog.master_products.tier2`: column добавлен, везде `'{}'::jsonb`
