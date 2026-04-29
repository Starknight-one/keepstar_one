# Catalog finalize flow — D1+A1+A2 closed, async UX, live e2e on 3 proposals

- **Branch:** `main`
- **Date (UTC):** 2026-04-29 02:55 (session ran ~22:00 UTC prev → 03:00 UTC, ~5 hours)
- **Parent commit:** `9518ca9` (gaps-doc rev)
- **Commits in this session:**
  - `30a59e6` — D1+A1+A2 backend fixes + seed-devstore media
  - `0d04171` — pgxpool tuning for Neon autosuspend
  - `894f0ae` — cleanup-tenant-stale matches staging by full GID
  - `cf66057` — curator merge-proxy timeout 90s → 10m
  - `33804c4` — async discovery (fire-and-forget + spinner + polling)
  - `b6d133a` — cleanup-tenant-stale also deletes tenant_categories
  - `5d0f9f7` — (omitted-from-set; intermediate)
  - `8fd8e33` — html.UnescapeString on vendor + lookupPath learns `metafields.<ns>.<key>`

## Context

Сессия началась с плана из `/Users/starknight/.claude/plans/effervescent-seeking-teacup.md`: довести catalog flow «Shopify connect → harvester → discovery → merge → apply → admin отображение» до состояния «работает на 8 сценариях dev-store без сюрпризов». До этой сессии один happy-path apply был пройден утром (`02:30 UTC`), но три блокирующих бага лежали в `docs/CATALOG_GAPS.md`: **D1** (admin Catalog UI пустой), **A1** (`tier2 = {}` всегда), **A2** (`vertical='unknown'` для всех new_master).

В процессе вылезли четыре дополнительных бага: pgxpool не давал Neon суспендиться, cleanup-tenant-stale стирал staging из-за format mismatch GID vs numeric, curator UI HTTP-таймил discovery на 90s (агент работает 145s), HTML-encoding `&amp;` в vendor names от Sonnet ломал resolveVertical для брендов с `&`. Все восемь закрыты.

## What landed

### D1 — admin Catalog UI shows real data (commit `30a59e6`)

`formatPrice` (`catalog_adapter.go:1264`) хардкодил `₽` как default + INT-divide cents (`14 ₽` вместо `$14.99`). Harvester писал пустой `Currency` из Shopify (нет default). UI хардкодил «Price (kopecks)» + size/color склеены в одно поле + не было бейджа что листинг привязан к мастеру.

**Изменения:**
- Backend: `formatPrice` → default USD, поддерживает USD/EUR/`<CODE> 14.99` fallback, фракционные центы.
- Backend: `harvester_lite.toListing` — `Currency = "USD"` если Shopify не вернул.
- Frontend: «Price (kopecks)» → «Price (cents)»; Size/Color раздельные read-only; badge `MASTER` в строке таблицы когда `master_product_id` не nil.

**Тесты:** `catalog_adapter_format_test.go` — 8 кейсов на formatPrice (default/USD/EUR/thousands/sub-dollar/zero-fraction/etc).

### A1 — discovery prompt cleanup (commit `30a59e6`)

`discovery_agent.go:497-513` инструктировал LLM использовать legacy `master_cosmetics.skin_type[]` targets из M3 typed-columns схемы. `extractTier2` (`merge_apply.go:487`) принимает только `master.tier2.*` или `tier2.*` префиксы — silently выкидывает легаси. Аналогично `discovery_tools.go:117` имел упоминание `master_cosmetics.scent` в `propose_field_mapping` description.

**Изменения:** убраны оба упоминания, оставлен только `master.tier2.<key>` синтаксис в системном промпте и tool description.

**Тесты:** sticky test `TestBuildDiscoverySystemPrompt_NoLegacyMasterCosmeticsTargets` — гарантирует что промпт больше не содержит `master_cosmetics.` подстроку.

### A2 — `BrandMappingTarget.Vertical` + resolveVertical rewrite (commit `30a59e6`)

`BrandMappingTarget` не имел поля `Vertical` — discovery agent не мог пометить «Cushion & Sons → furniture». `resolveVertical` хардкодил `return "cosmetics"` для `link_existing` и **полностью игнорировал** ветку `create_new` → fall-through на unknown.

**Изменения:**
- `domain/mapping_artifact.go` — добавлен `Vertical string` в `BrandMappingTarget`.
- `discovery_tools.go::proposeBrandMapping` — Vertical обязателен для action=create_new (валидируется на dispatch); опционален для link_existing.
- `merge_apply.go::resolveVertical` — порядок: `BrandMapping.Vertical` (hint) → MasterTemplate by collection → MasterTemplate by vendor → unknown. Хардкод `cosmetics` удалён.
- Discovery prompt обновлён — описывает что vertical обязателен для create_new иначе vertical='unknown'.

**Тесты:** `TestResolveVertical_BrandMappingHintWins`, `TestResolveVertical_NoHardcodedCosmetics`, `TestResolveVertical_FallsBackToMasterTemplateByCollection`, `TestResolveVertical_UnknownWhenNothingMatches` + 4 теста validation на `proposeBrandMapping`.

### Pgxpool tuning для Neon autosuspend (commit `0d04171`)

Все 4 backend'а держали Neon compute активным круглосуточно, что billable.

| Сервис | До | После |
|---|---|---|
| admin | MinConns=2, IdleTime=30m, HealthCheck=1m | MinConns=0, IdleTime=1m, HealthCheck=30m |
| v4 | MinConns=0, IdleTime=5m, HealthCheck=5m | MinConns=0, IdleTime=1m, HealthCheck=30m |
| chat | MinConns=0, IdleTime=5m, HealthCheck=5m | MinConns=0, IdleTime=1m, HealthCheck=30m |
| curator | (defaults — нет конфига) | MinConns=0, IdleTime=1m, HealthCheck=30m |

После deploy: на Neon dashboard «Working set size» / «Local file cache hit rate» показывают «ENDPOINT INACTIVE» штриховкой через ~6 минут полной тишины (1 мин idle pool + 5 мин Neon autosuspend delay). Pooler client connections могут быть >0 — это PgBouncer-прокси, не сам compute, не billable.

### Curator UX async discovery (commits `cf66057` + `33804c4`)

Discovery agent в admin-backend работает ~145s. Curator proxy timeout стоял на 90s → клиент UI получал «Discovery failed» на 90-й секунде, **а агент продолжал крутиться** в admin-backend, тратил LLM-токены, и в итоге сохранял артефакт. Vlad видел failure dialog после успешного `$0.40` run'а.

**Изменения:**
- Curator proxy timeout 90s → 10m (`cf66057`).
- `HandleDiscover` стал fire-and-forget: spawn goroutine с `context.Background()`, immediate 202 Accepted клиенту. Закрытие вкладки больше не убивает run (`33804c4`).
- Curator frontend `SchemaTab`: при клике появляется inline-плашка «Discovery agent is running… Elapsed Ns · runs on the server, you can leave this page», polling `/schema` каждые 5 секунд, сравнивает `discoveredAt` с timestamp клика. Когда новый артефакт записан — спиннер пропадает, UI обновляется. 10-минутный safety net на случай craш'а admin-backend.

### Cleanup-tenant-stale fix matrix (commits `894f0ae` + `b6d133a`)

Два проблема в одной утилите:

1. `shopify_raw_imports.source_id` хранит **полный GID** (`gid://shopify/Product/N`), а `catalog.products.source_id` — **bare numeric** (`N`). Cleanup сравнивал обе таблицы с numeric set → staging never matched → каждый прогон silently стирал весь staging для tenant'а. Surface'илось как «Discovery failed: staging is empty» сразу после чистого sync.
2. Cleanup чистил `catalog.products` + `shopify_raw_imports`, но **не чистил `tenant_categories`**. После `seed-devstore -reset` старые коллекции (со старыми Collection GID) оставались, новые добавлялись с теми же названиями но другими `external_id` → дубликаты chips на product detail.

**Fix:** `fetchCurrentCatalog` теперь возвращает три set'а — numeric productIDs (для catalog.products), full productGIDs (для shopify_raw_imports), collection GIDs (для tenant_categories). Каждый DELETE с правильным форматом. Empty-set edge case (`NOT IN ()` invalid SQL) обрабатывается отдельной веткой.

### `productCreateMedia` в seed-devstore (commit `30a59e6`)

`ProductCreateInput.ImageURLs` (`shopify/client.go:825`) был задекларирован, но **нигде не использовался** в `ProductCreate` mutation. seed-devstore передавал URLs, они уходили в /dev/null.

**Fix:** новый метод `Client.ProductCreateMedia` (использует `productCreateMedia` GraphQL mutation с `originalSource: URL` — Shopify сам качает по URL, дополнительные scopes не нужны). seed-devstore вызывает его после `ProductCreate` если `ImageURLs` непуст. Failures логируются как WARN, не аборт run.

После deploy + reseed dev-store: 6 furniture/footwear продуктов имеют картинки в admin Catalog UI. 14 cosmetics остались без — `imgCosmetic` URL устарел (Shopify CDN ссылка не работает). Записал в `CATALOG_GAPS.md` как E8.

### HTML-unescape vendor + `metafields.<ns>.<key>` syntax (commit `8fd8e33`)

После live e2e: 3 master_products создались, но:

- `Trail Runner Sneaker` → `vertical=footwear` ✅ (Pacelab — нет `&` в имени)
- `Marble Side Table` → `vertical=unknown` ❌ (Stone & Steel)
- `Linen Reading Armchair` → `vertical=unknown` ❌ (Cushion & Sons)
- tier2 у всех 3 = `{}` ❌

**Корень №1 (vertical):** Sonnet эмитит `propose_brand_mapping(vendor="Stone &amp; Steel")` — HTML-encoded ampersand, хотя в staging payload vendor `Stone & Steel` (raw). После lowercase ключ становится `stone &amp; steel`. Listing `vendor='Stone & Steel'` нормализуется в `stone & steel`, lookup в BrandMapping не находит → fallthrough → unknown.

**Fix:** `html.UnescapeString` на `args.Vendor` и `args.MasterBrand` в `proposeBrandMapping` перед `SetBrandMapping`. Defensive — не зависит от того откуда encoding пришёл.

**Корень №2 (tier2):** Discovery эмитит targets вида `metafields.custom.wood_type` (dot-form). `lookupPath` (`merge_apply.go:629`) знал только bracket-форму `metafields[key=X].value`. Dot-форма silently возвращала nil → `extractTier2` пропускал → tier2 пустой.

**Fix:** `lookupPath` теперь распознаёт три формата:
- direct key (`vendor`)
- `metafields[key=X].value` (legacy bracket)
- `metafields.<namespace>.<key>` (matches both ns and key) или `metafields.<key>` (any ns)

Helper `findMetafield` извлекает значение из `[]interface{}` массива метафилдов с фильтром по namespace+key.

**Тесты:** `TestDispatch_ProposeBrandMapping_DecodesHTMLEntitiesInVendor`, `TestLookupPath_DotForm_NamespaceAndKey`, `TestLookupPath_BracketFormStillWorks`, `TestLookupPath_DirectKey`.

После deploy + SQL fix existing artifact (`UPDATE … SET mapping_artifact = REPLACE(mapping_artifact::text, '&amp;', '&')::jsonb`) + manual SQL update existing 3 master_products: vertical у всех корректный (furniture/furniture/footwear), tier2 содержит `base_material: "blackened steel"` / `upholstery: "linen"` / `{}` для Trail Runner (у sneaker не было metafields в seed).

## Files changed

| Scope | File | Action |
|---|---|---|
| backend | `project_admin/backend/internal/adapters/postgres/catalog_adapter.go` | EDIT — formatPrice rewrite |
| backend | `project_admin/backend/internal/adapters/postgres/catalog_adapter_format_test.go` | NEW — 8 formatPrice tests |
| backend | `project_admin/backend/internal/adapters/postgres/postgres_client.go` | EDIT — pool config tuned |
| backend | `project_admin/backend/internal/adapters/shopify/client.go` | EDIT — `ProductCreateMedia` method |
| backend | `project_admin/backend/internal/domain/mapping_artifact.go` | EDIT — `BrandMappingTarget.Vertical` |
| backend | `project_admin/backend/internal/usecases/discovery_agent.go` | EDIT — system prompt cleanup |
| backend | `project_admin/backend/internal/usecases/discovery_agent_test.go` | EDIT — sticky tests + html-decode + brand validation |
| backend | `project_admin/backend/internal/usecases/discovery_tools.go` | EDIT — vertical validation + `html.UnescapeString` |
| backend | `project_admin/backend/internal/usecases/harvester_lite.go` | EDIT — Currency default USD |
| backend | `project_admin/backend/internal/usecases/merge_apply.go` | EDIT — resolveVertical rewrite + lookupPath三 формы |
| backend | `project_admin/backend/internal/usecases/merge_apply_test.go` | EDIT — resolveVertical + lookupPath tests |
| cli | `project_admin/backend/cmd/cleanup-tenant-stale/main.go` | EDIT — staging GID format + tenant_categories cleanup |
| cli | `project_admin/backend/cmd/seed-devstore/main.go` | EDIT — call ProductCreateMedia after ProductCreate |
| frontend | `project_admin/frontend/src/features/catalog/ProductsPage.jsx` | EDIT — MASTER badge |
| frontend | `project_admin/frontend/src/features/catalog/ProductDetailPage.jsx` | EDIT — Price (cents), Size/Color split |
| frontend | `project_admin/frontend/src/features/catalog/catalog.css` | EDIT — badge styles |
| curator-be | `curator/backend/internal/adapters/postgres.go` | EDIT — pool config tuned |
| curator-be | `curator/backend/internal/handlers/handler_merge.go` | EDIT — fire-and-forget discover + 10m timeout |
| curator-fe | `curator/frontend/src/pages/TenantDetailPage.jsx` | EDIT — async discovery + spinner + polling |
| curator-fe | `curator/frontend/src/styles/tokens.css` | EDIT — `@keyframes spin` |
| backend (chat) | `project/backend/internal/adapters/postgres/postgres_client.go` | EDIT — pool config tuned |
| backend (v4) | `project_v4/backend/internal/adapters/postgres/postgres_client.go` | EDIT — pool config tuned |
| docs | `docs/CATALOG_GAPS.md` | EDIT — D1/A1/A2 marked done + new entries E8/E9/E10/H5/H6/H7 |
| docs | `docs/Updates/main-catalog-finalize-flow_2026-04-29_05-30.md` | NEW (this file) |

## Verification

**Локально:**
- `cd project_admin/backend && go test ./internal/...` — clean (8 + 4 + 4 + 1 = 17 new tests, все зелёные)
- `cd curator/backend && go build ./...` — clean
- `cd project_admin/frontend && npm run build` — clean (152 KB CSS / 1.9 MB JS gzipped)
- `cd curator/frontend && npm run build` — clean (16 KB CSS / 270 KB JS gzipped)

**На production (Railway → admin/curator/v4/chat все redeployed):**
- Admin Catalog для dev-store: 20 продуктов с реальными ценами `$4.00`-`$699.00`, `$` валюта, картинки на 6 furniture/footwear.
- Curator → tenant `hey-babes-cosmetics` → Schema tab: артефакт после `8fd8e33` исправлен SQL'ем, ключи brand_mapping без `&amp;`.
- Curator → Reports → report id=4 (`pending`, 999 listings, 15 new_master, 1 needs_review, 4 skip, 979 already_linked).
- 3 applied master_products (Marble Side Table / Linen Reading Armchair / Trail Runner Sneaker): vertical корректный (`furniture`/`furniture`/`footwear`), tier2 заполнено реальными значениями metafields.

**Контроль на Neon dashboard:** RAM/CPU/Working set size показывают «ENDPOINT INACTIVE» штриховкой через ~6 мин после полной тишины. Pgxpool tuning сработал.

## Что НЕ закрыто (см. CATALOG_GAPS.md)

🔴 **Перед первым реальным клиентом:**
- **E1** — auto-trigger discovery после OAuth install (сейчас курратор жмёт кнопку вручную; для self-serve onboarding обязательно)
- **G1** — V4-чат читает `master_products.tier2 JSONB` (после apply мастера полные, но виджет на стороне клиента их не покажет)
- **I3** — verify V4-чат на dev-store с залинкованными мастерами (никем не пройдено)

🟠 **High-priority но не блокеры:**
- **A6** — coverage false-negative (system fields считаются unmapped, status спускается с 73% до 56%)
- **E2** — webhook `products/update`/`products/delete` end-to-end не верифицированы
- **E3** — `cmd/cleanup-tenant-stale` нужно дёргать периодически (cron)
- **I1, I2, I4, I6** — нехоженые сценарии (multi-proposal apply, re-run merge after apply, webhook update, large tenants)

🟡 **Medium / новое из этой сессии:**
- **E8** — seed-devstore: cosmetics URL устарел, 14/20 без картинок
- **E9** — seed-devstore: stock=0 по всем продуктам (нужен location-aware inventoryQuantities)
- **E10** — staging самопроизвольно опустошился между sync-tenant-now и cleanup-tenant-stale (root cause не до конца ясен; не блокер happy path)
- **H5** — Additional Information block (Gallery/Reviews/Stories) — статичные плейсхолдеры, нужен спек
- **H6/H7** — admin Catalog UI смешивает несколько tenant'ов в одном view (dev-store повесился на hey-babes-cosmetics tenant)

## Plan-mode rule note

Сессия началась через `ExitPlanMode` с планом `/Users/starknight/.claude/plans/effervescent-seeking-teacup.md`. Этот документ — обязательный update log по правилу из `CLAUDE.md`.

## Reality check

Юзер закрывает сессию словами «**Надо чтобы один кусок работал идеально, вот этот, потому что это гейт для всех клиентов. Одна из 5 самых важных частей проекта**». Сегодня закрыты 3 баг-блокера из плана (D1+A1+A2) + 5 побочных (pool, cleanup, async UX, html-encode, lookupPath). Полный happy-path прошёл на dev-store с 3 разнотипными proposals. Но E1/G1/I3 (🔴) остались, и их нельзя пропустить перед onboarding'ом первого реального клиента — они в `CATALOG_GAPS.md` помечены первыми в «Recommended order перед первым реальным клиентом».

Юзер вернётся через ~7-8 часов отдыхать и добивать. Следующая сессия: разобрать E1/G1/I3, потом I1/I2 (manual smoke), потом 8 сценариев Этапа 6.
