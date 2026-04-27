# Admin Catalog — Curator-Driven Pivot, Этапы 0-3 deployed (session wrap-up)

- **Branch:** `main`
- **Date (UTC):** 2026-04-27 19:15
- **Parent commit:** `d3c7ac2` (Этап 3 fix — productSet schema)
- **Active plan:** `docs/New features/admin_catalog_curator_pivot_2026-04-27.md`
- **Per-stage logs:** см. таблицу ниже

## Context

Сегодня (2026-04-27) сделан pivot от старого M4d-плана (auto-harvester) к curator-driven flow. Этот лог — **финальный wrap-up сессии**: что отшиплено, что задеплоено на прод, что осталось.

Вчерашняя сессия (2026-04-26) закрыла M1-M12 — это был дизайн каталога админки. Сегодняшний pivot **отменяет M4d (auto-harvester) и M7 (heybabes Russian backfill)** в пользу принципиально другой архитектуры:

- Legacy `ShopifyUseCase` (REST initial-sync, писавший напрямую в `master_products`) **полностью удалён**
- После Connect Shopify клиент попадает **только в `catalog.products`** через harvester-lite (Tier-1 deterministic). Никаких master_* записей до явного действия curator'а
- Поиск в V4 работает в **двух режимах** — listing-only когда нет master coverage, master+listing когда есть
- Curator получил **operations dashboard** — список тенантов, master-каталог browse, drill-down в каталог тенанта (placeholder под Run merge agent + Reports)
- Heybabes из "проблемы M7" превратился в **seed cosmetics master-каталога** (DB-проверка показала 100% English, 979/979 с embeddings, 961/979 с PIM)

## Commits в этой сессии (9 шт., запушены в `origin/main`)

```
d3c7ac2 fix(admin-shopify): seed-devstore — productSet schema for Admin API 2026-04
d81ed48 feat(admin-shopify): pivot Этап 3 code — seed-devstore + write methods
cf7b837 fix(curator): image URL extraction handles both jsonb shapes
312a79a fix(curator): re-render flicker on login + simpler dev creds
918ff15 feat(v4-search): pivot Этап 2.3 — two-mode keyword search
73f8916 feat(admin-shopify): pivot Этап 2.2 — harvester-lite (staging → catalog.products)
554feb1 refactor(admin-shopify): pivot Этап 2.1 — cut legacy ShopifyUseCase
db02674 feat(curator): pivot Этап 1 — operations dashboard (tenants + master catalog)
823cd13 docs(admin-catalog): curator-driven pivot — Этап 0 (docs)
```

## Этапы — что сделано / что осталось

| Этап | Описание | Статус | Per-stage log |
|---|---|---|---|
| 0 | Pivot doc + banners на старых docs | ✅ shipped | [main-admin-catalog-curator-pivot-stage0_2026-04-27_17-09.md](./main-admin-catalog-curator-pivot-stage0_2026-04-27_17-09.md) |
| 1 | Curator UI — Tenants page, Master Catalog page, Tenant detail с tabs | ✅ shipped | [main-admin-catalog-curator-pivot-stage1_2026-04-27_17-21.md](./main-admin-catalog-curator-pivot-stage1_2026-04-27_17-21.md) |
| 2.1 | Cut legacy ShopifyUseCase (delete shopify.go + shopify_mapper.go) | ✅ shipped | [main-admin-catalog-curator-pivot-stage2-1_2026-04-27_17-28.md](./main-admin-catalog-curator-pivot-stage2-1_2026-04-27_17-28.md) |
| 2.2 | Harvester-lite (staging → catalog.products, без master_*) | ✅ shipped | [main-admin-catalog-curator-pivot-stage2-2_2026-04-27_17-35.md](./main-admin-catalog-curator-pivot-stage2-2_2026-04-27_17-35.md) |
| 2.3 | Two-mode keyword search в V4 | ✅ shipped | [main-admin-catalog-curator-pivot-stage2-3_2026-04-27_17-39.md](./main-admin-catalog-curator-pivot-stage2-3_2026-04-27_17-39.md) |
| 3 | Dev-store seed (20 продуктов в Shopify по 8 сценариям) | ✅ shipped | [main-admin-catalog-curator-pivot-stage3-code_2026-04-27_18-18.md](./main-admin-catalog-curator-pivot-stage3-code_2026-04-27_18-18.md) |
| 4 | Merge agent design (focused-сессия с user'ом) | ⬜ next | — |
| 5 | Merge agent implementation + curator review UI | ⬜ | — |
| 6 | E2E тесты на dev-store | ⬜ | — |
| 7 | Heybabes master cleanup (1 broken record + description backfill) | ⬜ | — |
| 8 | Забытое (когда user вспомнит) | ⬜ | — |

## Verification на проде после деплоя

### Railway redeploy
- Last-Modified prod: `Mon, 27 Apr 2026 19:04:13 GMT` (после push'а 9 коммитов)
- Health: `https://admin-production-4ae4.up.railway.app/health` → 200 OK

### State в общей БД (после ручного `cmd/sync-tenant-now`)
| Таблица | Состояние | Ожидание |
|---|---|---|
| `catalog.products` (heybabes / source=shopify) | **37 листингов** (20 seed + 17 lingering от M4 yesterday), **все с master_*_id = NULL** | ✅ pivot работает: harvester-lite пишет, master не трогается |
| `catalog.master_products` (created today) | **0** записей | ✅ legacy realmly выпилен, никто не пишет в master без curator merge |
| `catalog.shopify_raw_imports` (heybabes) | 37 product rows + 4 metadata | ✅ staging заполнен |

### Sample листингов
- "Generic Lip Balm" (vendor=Keepstar, no product_type)
- "Lavender Hand Cream — A Very Long Title…" (vendor=Boutique de Provence, type=Hand Cream) — edge case scenario 3.8
- "Minimal Listing No Identifiers" (vendor=Anonymous) — edge case 3.8
- "Sample Sachet" (Keepstar Store, Sample) — junk scenario 3.7
- "Gift Wrap Service" (Keepstar Store, Service) — junk scenario 3.7

`raw_attributes` корректно заполнен: `vendor`, `product_type`, `tags`, `variants[]` (с SKU/price/barcode), `metafields[]`.

## Files added для CLI-инструментов (могут пригодиться, не в проде)

- `cmd/seed-devstore/main.go` — наливает 25-30 тестовых продуктов в Shopify dev-store по 8 сценариям. Запуск: `SHOPIFY_SHOP=... SHOPIFY_TOKEN=... go run ./cmd/seed-devstore -reset`
- `cmd/decrypt-shopify-token/main.go` — достаёт OAuth offline token из `admin.tenant_integrations` через secretbox. Запуск: `ADMIN_ENCRYPTION_KEY=... DATABASE_URL=... go run ./cmd/decrypt-shopify-token -shop X.myshopify.com`
- `cmd/sync-tenant-now/main.go` — replay'ит pipeline (DumpToStaging → harvester-lite) для одного тенанта. Используется когда webhook'и пропустили окно (HMAC mismatch / deploy lag). Запуск: `ADMIN_ENCRYPTION_KEY=... DATABASE_URL=... go run ./cmd/sync-tenant-now -shop X.myshopify.com`

Эти CLI **не запускаются в проде**, не светятся в UI клиентам. Это инструменты для dev'а.

## Известные нюансы / open issues

1. **Webhook'и из Shopify не доходят** до прод-админа. Гипотеза — HMAC mismatch (`SHOPIFY_API_SECRET` в Railway env vars не совпадает с актуальным секретом v3 app). Не блокирует pivot — в Этапе 5 curator-triggered re-run заменит зависимость от webhook'ов; на ручную синхронизацию используется `cmd/sync-tenant-now`.
2. **Disconnect-кнопка** в `/integrations` админки — код её рендерит, но user не видит. Возможно браузер кэширует старый JS. Хард-рефреш или DevTools-инспекция должны прояснить.
3. **17 lingering snowboard listings** в `catalog.products` heybabes — артефакты от вчерашнего M4 теста, удалены из Shopify но остались в нашей staging→products. Не блокер, удаляются в Этапе 7.
4. **`integration.updated_at` всё ещё 2026-04-26** — за сегодня не был обновлён. Значит "Install app" из Dev Dashboard не вызвал OAuth callback / не написал в этот row. Не критично — токен из вчерашнего install'а работает (write_products + write_inventory etc. подтверждены через GraphQL mutations).

## Next session (новый терминал)

User планирует:
- Открыть новый терминал, скормить мне свежий контекст
- Заново разобраться в общей архитектуре каталога
- Затем перейти к Этапу 4 (merge agent design) — focused-сессия

Этот лог — **точка возврата**. Любая последующая сессия может начать с него + pivot doc.
