# Catalog — known gaps before shipping

> Single source of truth для известных пробелов каталога после Phase D3 + live smoke (2026-04-29). Собрано из: разрозненных update logs, "Что НЕ входит" из pivot doc, "Out of scope" из плана `async-tumbling-wall.md`, кода (TODO маркеры), live smoke на dev-store.

## Severity legend

- 🔴 **blocker** — мешает реальному клиенту
- 🟠 **high** — pipeline даёт неполные данные, нужен fix скоро
- 🟡 **medium** — UX недополирован, не блокер
- 🟢 **low** — деферрено сознательно

## A. Discovery / merge agent — данные неполные

| # | Sev | Issue |
|---|---|---|
| A1 | 🟠 | `proposed_master.tier2 = {}` всегда — discovery prompt пишет field_mapping target=`master_cosmetics.X` (M3 schema), `extractTier2` ищет префикс `tier2.*` или `master.tier2.*`. Нужен либо prompt update, либо translation в extractTier2 |
| A2 | 🟠 | `proposed_master.vertical = 'unknown'` для всех new_master — `resolveVertical` fallthrough; brand_mapping action=create_new не несёт vertical hint |
| A3 | 🟡 | `proposed_master.variant.gtins = []` — gtinsFromListing читает raw_attributes.variants[].barcode; на тестовых dev-store products barcode пустой, в реальных каталогах должно работать |
| A4 | 🟠 | Tier-2 transforms (`ml_from_string`, `units.weight` и т.д.) в `extractTier2` не применяются — данные пишутся 1:1 |
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
| D1 | 🟠 | Products в Admin Catalog после reinstall показываются **пустыми** (no stock / no attrs / no price) — read-path не разворачивает raw_attributes; БД полная, проблема только в admin UI рендере |
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

## F. Heybabes seed cleanup (E2 milestone)

| # | Sev | Issue |
|---|---|---|
| F1 | 🟢 | 1 master_product с кириллическим name + сломанным SKU — однострочный SQL fix |
| F2 | 🟢 | 0/979 master_products имеют description — `cmd/backfill-descriptions` через Haiku 4.5, ~$5 batch |

## G. Engine V4 chat — рендер не обновлён

| # | Sev | Issue |
|---|---|---|
| G1 | 🔴 | V4 чат не использует `master_products.tier2 JSONB` — рендеринг чата под новые поля **отложен явно**. После apply master_product получает tier2 атрибуты, но виджет в чате их не покажет до этой работы |
| G2 | 🟠 | Embedding-based search не работает для new verticals (furniture/footwear/lighting) — embeddings ещё не seed'ятся для new master_products (см. B3) |

## H. Refactor / quality

| # | Sev | Issue |
|---|---|---|
| H1 | 🟡 | Phase B refactor: shared `pkg/catalog` (master-link JOIN дублирован 7+ раз в admin/curator/V4) — отложено явно |
| H2 | 🟡 | Phase B refactor: `PromoteAttribute` через `master_field_definitions` вместо ALTER TABLE — отложено явно |
| H3 | 🟢 | Auth между curator-backend и admin-backend — сейчас X-Internal-Key (defense-in-depth, но не идеально). Долгоиграющая цель: shared session token / JWT pass-through |
| H4 | 🟢 | Refactor оставшихся 5 страниц curator на новые design tokens / shared components — постепенно |

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
