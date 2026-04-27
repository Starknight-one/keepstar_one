# Admin Catalog — Curator-Driven Pivot, Этап 2.1 (cut legacy ShopifyUseCase)

- **Branch:** `main`
- **Date (UTC):** 2026-04-27 17:28
- **Parent commit:** `db02674` (Этап 1 — curator UI)
- **Active plan:** `docs/New features/admin_catalog_curator_pivot_2026-04-27.md`

## Context

До этого этапа в системе **одновременно жили два пайплайна импорта**:
- **Legacy** `usecases.ShopifyUseCase` — REST-пагинация после OAuth, инициирует `runInitialSync` который пишет напрямую в `catalog.master_products` + `catalog.products` через `ImportUseCase`. Активен в DI, пишет с дефолтным `vertical='cosmetics'` независимо от типа товара.
- **V2** `usecases.ShopifyV2UseCase` — `dump-to-staging` + `discover` (M4a/c). Только пишет в `catalog.shopify_raw_imports` + `tenant_catalog_schema`. **Никто не применяет artifact** — harvester orchestrator не написан.

Это обернулось мусорными данными: 17 dev-store snowboard'ов попали в `master_products` под owner=hey-babes-cosmetics с `vertical='cosmetics'`. Discovery агент это видел, но коррекции не было — потому что master_products уже был испорчен legacy-импортером _до_ запуска discovery.

Этап 2.1 закрывает первую часть pivot: **legacy полностью удалён**, OAuth + webhook теперь идут только через V2 UC. На месте legacy initial-sync — пока no-op (harvester-lite появится в Этапе 2.2). Webhook на products/upsert тоже no-op'нут до 2.2 (с info-логом что harvester не подключён). Это намеренно: цель этапа — выпилить legacy без регрессий, не вводить harvester параллельно.

## What landed

### Удалено
- `project_admin/backend/internal/usecases/shopify.go` (312 строк)
- `project_admin/backend/internal/usecases/shopify_mapper.go`

Вместе с ними ушли:
- `runInitialSync` — REST-пагинация и прямые записи в master_products
- `FullResync` + endpoint `/admin/api/integrations/shopify/{id}/resync` — заменено на curator-triggered re-run (через `/dump-to-staging`)
- `StartPeriodicResync` — 6h ticker (был placeholder, фактически no-op потому что `IntegrationsPort.ListByKindAndStatus` не реализован). Заменён комментарием в main.go
- `webhookTopics` const + `randomState` — переехали в `shopify_v2.go`
- `shopifyProductToImportItem` mapper — больше не нужен, без `ImportUseCase` пути

### Добавлено в `usecases/shopify_v2.go` (≈220 строк)

- **`HarvesterLite` interface** — контракт под Этап 2.2:
  ```go
  RunForTenant(ctx, tenantID) (productCount int, err error)
  UpsertOne(ctx, tenantID, body []byte) error
  SoftDeleteOne(ctx, tenantID, sourceID string) error
  ```
- **`SetHarvester(h HarvesterLite)`** — late-binding wire setup в main.go (избегает import-cycle, harvester будет жить в `usecases` package)
- **`StartOAuth`** — minted state nonce, returns redirect URL (поведение идентично legacy)
- **`CompleteOAuth`** — verify HMAC, consume state, exchange code → token, persist integration со статусом `syncing`, register webhooks. **Вместо `runInitialSync` запускает `runInitialIngest` фоном**
- **`runInitialIngest`** — `DumpToStaging` → если `harvester != nil` зовёт `RunForTenant` → status='connected'. Если harvester nil (текущее состояние, до 2.2) — staging заполняется, status='connected', но листинги не пишутся. Лог `shopify_v2_harvester_not_wired_skipping_apply` чётко это сигнализирует
- **`HandleWebhook`** — диспатчер 5 топиков:
  - `products/create`/`products/update` → `harvester.UpsertOne` (no-op если harvester nil)
  - `products/delete` → `harvester.SoftDeleteOne` (no-op если nil)
  - `inventory_levels/update` → log-only (как было в legacy)
  - `app/uninstalled` → status=disconnected
- **`Client()`** accessor + helper `randomState()`

### Обновлено

- `internal/handlers/handler_integrations_shopify.go` — переписано (~110 строк): берёт `*usecases.ShopifyV2UseCase` вместо legacy. Endpoints `/install`, `/callback`, `/webhooks/shopify` без изменений в API. `/resync` метод удалён.
- `cmd/server/main.go`:
  - Убран `var shopifyUC *usecases.ShopifyUseCase` и его DI block
  - `NewShopifyV2UseCase` получил доп. параметр `cfg.PublicBaseURL`
  - `shopifyHandler = handlers.NewShopifyHandler(shopifyV2UC, log)` (через V2 UC)
  - `/resync` route убран из integrations switch
  - `StartPeriodicResync` block заменён на comment

## Files changed

| Scope | File | Action |
|---|---|---|
| backend | `project_admin/backend/internal/usecases/shopify.go` | **DELETE** (312 строк) |
| backend | `project_admin/backend/internal/usecases/shopify_mapper.go` | **DELETE** |
| backend | `project_admin/backend/internal/usecases/shopify_v2.go` | EDIT (+220 строк: OAuth, webhook, harvester-lite interface, init helpers) |
| backend | `project_admin/backend/internal/handlers/handler_integrations_shopify.go` | REWRITE (через V2 UC) |
| backend | `project_admin/backend/cmd/server/main.go` | EDIT (legacy DI выпилен, V2 принимает publicURL) |

## Verification

```
$ cd project_admin/backend && go build ./...
clean

$ go vet ./...
clean

$ go test ./internal/usecases/...
ok  	keepstar-admin/internal/usecases	1.006s
```

### Behavior after this commit

- Connect Shopify (любой тенант): OAuth пройдёт → webhook'и зарегаются → DumpToStaging запустится фоном → staging наполнится → status='connected'. **`master_products` ничего не получит**. **`catalog.products` тоже ничего не получит** (до Этапа 2.2). Это намеренное промежуточное состояние.
- Webhook products/create или products/update: HMAC verified → лог `shopify_v2_webhook_upsert_skipped_no_harvester`. Никаких записей в БД.
- Webhook products/delete: лог skip, никаких записей.
- Webhook app/uninstalled: status → disconnected (работает как раньше).
- Curator UI (Этап 1): `Tenants` page продолжает показывать heybabes / test-electronics / 10 пустых; `master_products coverage` остаётся 100% для heybabes (старые данные — Этап 7 cleanup пока не запускали).

### Что **НЕ** сломано

- V4 чат heybabes — не менялся, читает `master_products` через двухпутевой COALESCE-JOIN (M6). Все 979 продуктов на месте.
- Curator UI (Этап 1) — endpoints без изменений, UI работает.
- Admin UI ProductsPage / Detail / Categories / Detected add-ons / API keys / Audit — независимы от Shopify пайплайна.
- M10 public REST API (`/api/v1/products`, `/api/v1/categories`) — независимо.

## Known gaps / next steps

- **Этап 2.2** — harvester-lite (немедленно следующим): новый файл `internal/usecases/harvester_lite.go` имплементит `HarvesterLite` interface. Wire через `shopifyV2UC.SetHarvester(harvester)` в main.go. После этого Connect → staging → `catalog.products` без `master_*`
- **Этап 2.3** — двухрежимный поиск V4 (listing-only / master-mode по master_link_coverage)
- **Cleanup от legacy-мусора** (отдельно, в Этапе 7 или раньше): удалить 17 snowboard master_products которые были созданы под owner=hey-babes-cosmetics с `vertical='cosmetics'`. Команда:
  ```sql
  DELETE FROM catalog.products WHERE id IN (
    SELECT p.id FROM catalog.products p
    JOIN catalog.master_products mp ON mp.id=p.master_product_id
    WHERE mp.brand IN ('Keepstar', 'Multi-managed Vendor')
      AND p.tenant_id IN (SELECT id FROM catalog.tenants WHERE slug='hey-babes-cosmetics')
  );
  DELETE FROM catalog.master_products WHERE owner_tenant_id IN (
    SELECT id FROM catalog.tenants WHERE slug='hey-babes-cosmetics')
    AND brand IN ('Keepstar', 'Multi-managed Vendor');
  ```
  Не выполняем сейчас — оставим до Этапа 7 cleanup, чтобы видеть в curator UI как работает мастер-каталог с "грязным" состоянием.
