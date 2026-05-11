# Catalog — known gaps before shipping

> Single source of truth для известных пробелов каталога после Phase D3 + live smoke (2026-04-29). Собрано из: разрозненных update logs, "Что НЕ входит" из pivot doc, "Out of scope" из плана `async-tumbling-wall.md`, кода (TODO маркеры), live smoke на dev-store.
>
> **Update 2026-04-29 (вечерняя сессия):** D1, A1, A2 закрыты. Добавлен async discovery в curator UI (fire-and-forget + spinner + polling). Pgxpool tuning под Neon autosuspend. Cleanup-tenant-stale теперь чистит и tenant_categories. HTML-unescape для vendor names в discovery. lookupPath теперь умеет `metafields.<ns>.<key>` форму. Live e2e: 3 dev-store proposals applied, vertical='furniture'/'footwear' стампится корректно, tier2 заполняется реальными значениями metafields. Подробности — `docs/Updates/main-catalog-finalize-flow_2026-04-29_05-30.md`.
>
> **Update 2026-05-11 (ре-валидация по живой БД + коду):** A4 ✅ (transforms применяются — `merge_apply.go:applyTransform` units/lowercase/shorten/split, commit `4e1c8d1`). G1 ✅ (V4 `postgres_catalog.go` и V5 `postgres_catalog.go`+`postgres_catalog_vector.go` оба читают `mp.tier2`, commit `8a3357d`). Остальные blockers (E1, A6, B-секция, C-секция, M7 миграция данных) — без изменений с 2026-05-07. Живые цифры БД совпали с аудитом 2026-05-07 → каталог не двигался 12 дней. Текущий снапшот: `docs/catalog-audit-2026-05-07.md`.

## Severity legend

- 🔴 **blocker** — мешает реальному клиенту
- 🟠 **high** — pipeline даёт неполные данные, нужен fix скоро
- 🟡 **medium** — UX недополирован, не блокер
- 🟢 **low** — деферрено сознательно

## A. Discovery / merge agent — данные неполные

| # | Sev | Issue |
|---|---|---|
| A1 | ✅ | ~~`proposed_master.tier2 = {}` всегда — discovery prompt пишет `master_cosmetics.X`~~ — fixed `30a59e6` (legacy block убран из системного промпта + sticky test). Дополнительно `8fd8e33` научил `lookupPath` форме `metafields.<ns>.<key>` (Sonnet её эмитит чаще чем bracket-форму) — иначе tier2 оставался пустым даже при чистом промпте |
| A2 | ✅ | ~~`proposed_master.vertical = 'unknown'` для всех new_master~~ — fixed `30a59e6` (`BrandMappingTarget.Vertical` поле + propose_brand_mapping требует vertical для create_new + `resolveVertical` rewrite без хардкода cosmetics). Hot-fix `8fd8e33` — `html.UnescapeString` для vendor (Sonnet иногда эмитит `Stone &amp; Steel`, lowercase-key не матчился к listing'у `Stone & Steel`) |
| A3 | 🟡 | `proposed_master.variant.gtins = []` — gtinsFromListing читает raw_attributes.variants[].barcode; на тестовых dev-store products barcode пустой, в реальных каталогах должно работать |
| A4 | ✅ | ~~Tier-2 transforms (`ml_from_string`, `units.weight` и т.д.) в `extractTier2` не применяются~~ — fixed `4e1c8d1` (`merge_apply.go:applyTransform` поддерживает `units.{weight,volume,length,count}`, `lowercase`, `shorten:N`, `split:delim`; unknown transforms — pass-through) |
| A5 | 🟡 | `ValidateArtifact` scoring coverage только на FieldMapping; не учитывает новые BrandMapping / JunkRules / MatchStrategyConfig (D1). Discovery agent забыл brand_mapping для vendor → coverage этого не зафиксирует |
| A6 | 🟠 | False-negative coverage: system fields (`id`, `createdAt`, `updatedAt`) считаются unmapped → status `needs_human_review` спускается с 73% до 56% впустую |
| A7 | 🟢 | Cost guard: нет rate-limit на discovery — курратор может в loop'е жать Re-run → каждый раз $0.40 |

## B. Apply / revert / consistency

| # | Sev | Issue |
|---|---|---|
| B1 | ✅ | ~~Revert SQL: `NULLIF($,'')` без `::uuid` cast → 42804~~ — fixed `1db672d` |
| B2 | 🟠 | Concurrent apply: две curator-вкладки → две Apply того же report → ApplyNewMaster использует ON CONFLICT (sku) DO UPDATE, но только частично safe; double master_products возможны на edge cases |
| B3 | 🟡 | Embedding NOT seed'ится для new master_products при apply — V4 chat не сможет матчить vector-search для нового vertical пока embedding job не прогонит. Сейчас deferred ("Что НЕ входит" §6) |
| B4 | 🟡 | `master_brands` table — упомянут в discovery prompt как "register Pacelab in master_brands", но apply создаёт `master_products.brand` как text, не FK на master_brands. Брендов как сущности нет |
| B5 | 🟡 | Tenant_categories ↔ master_categories — tenant_categories пишутся harvester_lite, но не связаны с master_categories tree. Filter в V4 чате по категории не сработает на нового тенанта |

## C. Curator UI — несделанное

| # | Sev | Issue |
|---|---|---|
| C1 | 🟡 | Candidates redesign (anatomy cards, AGENT SUGGESTS, дерево с inheritance) — спроектирован в Pencil 2026-04-29, не имплементирован |
| C2 | 🟡 | Sidebar v2 (3 группы WORK QUEUES / BROWSE / OBSERVE + counter badges) — спроектирован, не код |
| C3 | 🟡 | MergeReportPage: `alert()` вместо toast, no skeleton loaders, no virtualization для 10k+ proposals |
| C4 | 🟡 | TenantDetailPage не показывает owned master_products (только tenant listings) — курратор не видит "что у меня в master" |
| C5 | 🟡 | Edit-and-approve per-field — backend поддержка есть, UI dropdown'ы пустые если у proposal нет FieldDecisions (а сейчас агент их не пишет) |
| C6 | 🟢 | Drag-drop в category tree (M8) — TODO с момента M8 ship |
| C7 | 🟢 | Batch classify через LLM на junk page (M9) — placeholder с момента M9 ship |

## D. Admin UI — несделанное

| # | Sev | Issue |
|---|---|---|
| D1 | ✅ | ~~Products в Admin Catalog показываются пустыми~~ — fixed `30a59e6`. Корни: `formatPrice` хардкодил `₽` + INT-divide cents; harvester писал пустой Currency; UI хардкодил «kopecks»; size/color склеены в одно поле; не было визуального индикатора что листинг привязан к мастеру. Сейчас: `$14.99`, default USD, "Price (cents)", раздельные Size/Color, `MASTER` бейдж в строке таблицы. 8 unit-тестов на formatPrice |
| D2 | 🟢 | Frontend progress UI на ShopifyConnectPage — отложено явно |

## E. Pipeline / orchestration

| # | Sev | Issue |
|---|---|---|
| E1 | 🔴 | **Tenant onboarding auto-flow** — после install приложения discovery НЕ запускается автоматически. Сейчас курратор должен зайти в curator → жать Run discovery вручную для каждого нового тенанта. Это блокирует self-serve onboarding |
| E2 | 🟠 | Webhook product_update / product_delete: не верифицированы end-to-end. `app/uninstalled` мы видели в логах. Создание/изменение продукта в Shopify → должен дёргаться `harvester_lite.RunForOneListing` через webhook. Проверить что подписка регистрируется при OAuth callback и что handler есть |
| E3 | 🟠 | Stale cleanup: `cmd/cleanup-tenant-stale` есть, но никто его не дёргает периодически. Если в Shopify удалили продукт — наша БД хранит stale запись |
| E4 | 🟡 | Webhook hash-diff path: webhook просто перезаписывает листинг, без диффа (отложено явно — "быстрее проще, на текущих объёмах") |
| E5 | 🟡 | Bulk POST/DELETE через X-API-Key (M10 stubs) — отложено до первого реального запроса |
| E6 | 🟢 | Cursor pagination на /api/v1/products — offset работает на текущих объёмах |
| E7 | 🟠 | Cost monitoring — никто не агрегирует $0.40 × N tenants. 100 connect/нед = $40, не критично, но нет dashboard'а |

## E. Pipeline / orchestration (продолжение)

| # | Sev | Issue |
|---|---|---|
| E8 | 🟡 | seed-devstore: `imgCosmetic` URL из старого Shopify CDN устарел — Shopify не может скачать → 14 cosmetics в dev-store без картинок. Furniture/footwear (Unsplash URLs) приезжают ОК. Не блокер реального прода (тенанты сами заливают свои картинки), но мешает визуальному smoke на dev-store. Замена: подобрать актуальный публичный URL |
| E9 | 🟡 | seed-devstore: stock=0 для всех 20 продуктов даже с `InventoryQty: 200` в input. Корень — Shopify productSet без явных `inventoryQuantities: [{locationId, quantity}]` не активирует inventory tracking. Нужно дописать `cmd/seed-devstore` чтобы fetch'ил primary location и стэмпил per-variant inventory |
| E10 | 🟡 | Webhook regression: после `seed-devstore -reset` admin почему-то получил 20 события `products/create`, harvester отработал, но **между моментом sync-tenant-now и cleanup-tenant-stale** staging опустошился. Корень не до конца ясен (либо webhook запустил TruncateStaging логику где-то, либо race в DumpToStaging). Не блокер happy path, но оставлять нельзя |

## F. Heybabes seed cleanup (E2 milestone)

| # | Sev | Issue |
|---|---|---|
| F1 | 🟢 | 1 master_product с кириллическим name + сломанным SKU — однострочный SQL fix |
| F2 | 🟢 | 0/979 master_products имеют description — `cmd/backfill-descriptions` через Haiku 4.5, ~$5 batch |

## G. Engine V4 chat — рендер не обновлён

| # | Sev | Issue |
|---|---|---|
| G1 | ✅ | ~~V4 чат не использует `master_products.tier2 JSONB`~~ — fixed `8a3357d` (V4 `postgres_catalog.go:177/455/733`) + V5 mirror (`postgres_catalog.go:92`, `postgres_catalog_vector.go:144`). Оба движка читают `mp.tier2` через `COALESCE(mp.tier2, '{}'::jsonb)` |
| G2 | 🟠 | Embedding-based search не работает для new verticals (furniture/footwear/lighting) — embeddings ещё не seed'ятся для new master_products (см. B3) |

## H. Refactor / quality

| # | Sev | Issue |
|---|---|---|
| H1 | 🟡 | Phase B refactor: shared `pkg/catalog` (master-link JOIN дублирован 7+ раз в admin/curator/V4) — отложено явно |
| H2 | 🟡 | Phase B refactor: `PromoteAttribute` через `master_field_definitions` вместо ALTER TABLE — отложено явно |
| H3 | 🟢 | Auth между curator-backend и admin-backend — сейчас X-Internal-Key (defense-in-depth, но не идеально). Долгоиграющая цель: shared session token / JWT pass-through |
| H4 | 🟢 | Refactor оставшихся 5 страниц curator на новые design tokens / shared components — постепенно |
| H5 | 🟡 | Admin product detail page: «Additional information» блок (Social Links / Gallery / Stories / Reviews) — статичные disabled-инпуты, к данным не подвязаны. Под полную доделку нужен отдельный спек: что туда теннант кладёт, как UI это редактирует, как хранится (новая колонка vs JSONB extension). См. user-list 2026-04-29 #11 |
| H6 | 🟡 | Admin Catalog: при коннекте dev-store к существующему тенанту (`hey-babes-cosmetics`) UI показывает 999 items = 979 heybabes + 20 dev-store смешанно. По дизайну каждый коннект → отдельный tenant (per-store), но текущий OAuth flow вешает интеграцию на active session-tenant. Архитектурно — отдельное решение: либо forced-create-new-tenant per integration, либо UI tenant-switcher для оператора |
| H7 | 🟡 | Admin Catalog: sidebar категорий тоже показывает heybabes-категории + dev-store-коллекции миксом. Разрулится когда H6 закроется (per-tenant view) |

## I. Tests / verification — невыполненное

| # | Sev | Что не проверено |
|---|---|---|
| I1 | 🟠 | Apply на subset > 1 proposal с edit-and-approve (per-field decision dropdown) |
| I2 | 🟠 | Re-running merge agent после apply (report supersession + already_linked detection в свежем report) |
| I3 | 🔴 | V4 chat на dev-store с залинкованными master_products — продукты находятся? |
| I4 | 🟠 | Webhook update от Shopify после initial sync (E2) |
| I5 | 🟡 | Concurrent apply guard (B2) |
| I6 | 🟠 | Большие тенанты (5k+, 50k+ продуктов): cost discovery, время merge generate, RAM на staging |

---

## Recommended order перед первым реальным клиентом

1. **🔴 E1** — auto-trigger discovery после install (без этого новый тенант ждёт курратора вручную)
2. **🔴 G1** — V4 chat читает tier2 (без этого виджет показывает пустые карточки)
3. **🔴 I3** — verify V4 chat works on dev-store с merged products
4. **🟠 A1+A2** — discovery prompt fix → tier2 + vertical заполняются
5. **🟠 D1** — admin catalog UI правильно рендерит raw_attributes
6. **🟠 E2+E3** — webhook updates / stale cleanup verified
7. **🟠 A6** — coverage false-negative (status=needs_human_review при 73% в реальности)
8. **🟠 I1, I2** — multi-proposal apply, re-run merge
9. После этого можно подключать первого клиента
10. Остальные — итеративно по мере боли
