# Admin Catalog — Curator-Driven Pivot

**Date:** 2026-04-27
**Status:** 🟢 Active plan — Этапы 0-3 shipped & deployed (см. секцию "Status snapshot" ниже)
**Owner:** Vlad
**Source session:** chat 2026-04-27 (Vlad + Claude)

> **STATUS SNAPSHOT 2026-04-29 02:30 UTC — Etap 4-5-6 shipped end-to-end на dev-store через прод UI**.
> Phase D1 (discovery agent extended), D2 (merge applier read-only), D3 (destructive applier + revert + curator endpoints), Phase 4 (curator UI MVP) — все имплементированы и shipped. Live smoke: wipe → reinstall → discovery ($0.40, 432k tokens) → merge (deterministic, ~20 сек) → apply 1 proposal → revert. Каталог формально работает.
>
> 🔴 **Известные пробелы перед первым клиентом** — собраны в [`docs/CATALOG_GAPS.md`](../CATALOG_GAPS.md). Критичные (E1 auto-discovery, G1 V4 chat читает tier2, I3 verify V4 search) **обязательны** перед onboarding'ом. Bug live-smoke'а #15-17 (vertical='unknown', tier2 пустой, gtins) — high-priority но не блокеры apply.
>
> Старый snapshot:
> Этапы **0/1/2.1/2.2/2.3/3 закрыты и задеплоены на прод** (deploy log: [stages0-3-deployed](../Updates/main-admin-catalog-curator-pivot-stages0-3-deployed_2026-04-27_19-15.md)).
>
> 🔴 **Post-deploy smoke выявил critical bug** ([postdeploy-issues](../Updates/main-admin-catalog-curator-pivot-postdeploy-issues_2026-04-27_19-23.md)) — Bulk JSONL от Shopify Admin API 2026-04 не возвращает nested children (variants/metafields/images/collections) в форме которую ожидает наш scanner. 0/37 staging-продуктов имеют `_v2_variants`. Без этого harvester-lite пишет пустые `raw_attributes`, ломая весь pipeline. **Этап 4 (merge agent) блокирован — сначала нужно разобраться с #1 в новой сессии.**
>
> Также корректировка: heybabes `catalog.products` (listings) **на русском** (962/1016 cyrillic в `name`), хотя `master_products` английские. Это не блокер — listings heybabes per pivot не трогаем. Осталось: **#1 fix + Этапы 4-7** (merge agent design + impl + e2e + heybabes cleanup).

> **Эта страница — единый источник правды для оставшейся работы по каталогу/PIM.**
>
> Старые документы (`admin_catalog_design_2026-04-23.md`, `admin_catalog_implementation_plan_2026-04-26.md`) **остаются актуальными по дизайну схемы и описанию реализованных M1-M3 / M4a/b/c / M6 / M8-M12**, но отменяют M4d (auto-harvester) и M7 (heybabes Russian backfill) в пользу подхода описанного здесь.

---

## 1. Зачем pivot

После сессии M1-M12 (см. `docs/Updates/main-admin-catalog-m6-m8-m9-m10-m11-m12_2026-04-26_14-21.md`) в системе остались два больших открытых пункта: **M4d — автоматический harvester orchestrator**, и **M7 — backfill 967 heybabes-продуктов с русскими названиями**. Оба требовали focused-сессии с пользователем у руля. На сессии 2026-04-27 архитектура была пересмотрена:

### 1.1 Что не нравилось в старом плане

1. **Параллельные пайплайны** — legacy ShopifyUseCase (REST + initial sync + webhook) и новый M4a-c (staging + discover) живут одновременно. Legacy пишет в `master_products` напрямую, новый — пока только генерит artifact. Это значит что после Connect в master_products у нового клиента уже куча мусора, ещё до того как curator успел что-то посмотреть. Дублирование, перезапись, рандомный vertical='cosmetics' default
2. **Auto-apply harvester** — по плану M4d harvester должен был применять mapping_artifact автоматически после discover'а. Это значит решения "мерджить ли в существующий мастер vs создавать новый" принимает агент в одиночку, без человеческого ревью. На малом числе тенантов это **дороже** ручного режима по последствиям (недомерженные дубли в master тяжело откатить)
3. **M7 — переводить 967 русских названий heybabes** через LLM batch. Дорого, рискованно, неприятно

### 1.2 Что выяснилось в БД

Прямая проверка `catalog.master_products WHERE owner_tenant_id=hey-babes-cosmetics` (2026-04-27):

| Метрика | Значение |
|---|---|
| Всего master_products | 979 |
| Чистый английский в name | 978 / 979 (один сломанный — мусор в SKU) |
| Кириллица в description / display_name / original_name | 0 везде |
| С embedding | 979 / 979 |
| С brand | 978 / 979 |
| С images | 979 / 979 |
| С PIM (skin_type/concern/key_ingredients/benefits) | 961 / 979 (98%) |
| С description | 0 (все пустые) |
| Уникальных брендов | 10+ (MEDI-PEEL 57, COSRX 52, The Saem 45, Some By Mi 37...) |

**Вывод:** heybabes — не "проблема M7", а **готовый seed cosmetics master-каталога**. 979 продуктов с embeddings и PIM-обогащением, на которых можно проверять match cascade новых dev-store импортов.

---

## 2. Архитектурные решения 2026-04-27

| # | Решение | Что это значит |
|---|---|---|
| 1 | **Legacy полностью убираем.** | Удаляются `usecases/shopify.go`, `usecases/shopify_mapper.go`, периодический resync. OAuth + webhook регистрация остаются (через `shopify_v2`). |
| 2 | **После Connect клиент попадает только в `catalog.products`** — свои листинги, без `master_*`. Виджет работает сразу через **listing-only** режим поиска. Плашка в UI: "Search runs on basic schema. Full PIM enrichment in progress — usually within 24h". | Новый usecase `harvester_lite`: dump-to-staging → Tier1 deterministic mapping → write to `catalog.products`. Не трогает `master_products` / `master_variants` / candidates. |
| 3 | **Мерджинг в master — исключительно ручной**, через curator. | Никакого auto-apply. Все pre-existing M4b/M4c механики (match cascade + junk detector + discovery agent) переиспользуются, но обёрнуты в curator-flow. |
| 4 | **Merge-агент пишет отчёт, не БД.** | Новая таблица `catalog.merge_reports` (status='pending'). Curator UI рендерит per-product proposals с evidence. Курратор approve/reject per-row или batch'ем → детерминированный `merge_apply.go` выполняет. |
| 5 | **Поиск V4 в двух режимах.** | Listing-only (нет master coverage у тенанта) — keyword+pg_trgm по `original_name + raw_attributes::text + variants.sku`. Master-mode (≥30% master_link_coverage) — текущее поведение (vector + cross-tenant master facets). Переключение — динамическое per-request. |
| 6 | **Heybabes становится seed cosmetics master-каталога.** | Не трогаем `catalog.products` heybabes-листингов. Из `master_products` (979) — фиксим 1 сломанную запись + LLM batch для description backfill. |
| 7 | **Curator превращается в operations dashboard.** | Текущие Candidates/Junk/Audit остаются. Добавляются: Tenants list → drill-down в каталог тенанта → Run merge agent → Report review → Apply. Master Catalog browse (с фильтром по vertical). |
| 8 | **Рендеринг чата под новые поля — отложен** до момента когда каталог + curator стабилизируются. |

---

## 3. Этапы

### Этап 0 — Документация (этот файл + banner на старые) ✅ shipped
- Создать этот документ
- На `admin_catalog_design_2026-04-23.md` — banner "Pivot 2026-04-27" со ссылкой
- На `admin_catalog_implementation_plan_2026-04-26.md` — banner + краткое summary что отменяется (M4d, M7)

> **Лог:** [main-admin-catalog-curator-pivot-stage0_2026-04-27_17-09.md](../Updates/main-admin-catalog-curator-pivot-stage0_2026-04-27_17-09.md) · commit `823cd13`

### Этап 1 — Curator UI (operations dashboard) ✅ shipped

> **Лог:** [main-admin-catalog-curator-pivot-stage1_2026-04-27_17-21.md](../Updates/main-admin-catalog-curator-pivot-stage1_2026-04-27_17-21.md) · commit `db02674`

**Backend** (`curator/backend/`):
- `internal/adapters/postgres.go` — добавить:
  - `ListTenants()` — JOIN `catalog.tenants` + counts `catalog.products` + JOIN `tenant_catalog_schema` (status, artifact_version)
  - `GetTenant(id)` — детали + integrations + master_link_coverage%
  - `ListTenantProducts(tenant_id, search, limit, offset)` — read-only листинг продуктов тенанта
  - `ListMasterProducts(vertical, search, limit, offset)` — глобальный browse master-каталога
  - `GetMasterProduct(id)` — карточка + variants + связанные tenant-листинги
  - `GetTenantSchema(tenant_id)` — читает `tenant_catalog_schema` (artifact, status, validation_report)
- `internal/handlers/handlers.go` — endpoints:
  - `GET /curator/tenants` → list
  - `GET /curator/tenants/:id` → detail
  - `GET /curator/tenants/:id/products?search=&limit=&offset=` → каталог
  - `GET /curator/tenants/:id/schema` → mapping artifact + validation
  - `GET /curator/master/products?vertical=&search=&limit=&offset=` → master browse
  - `GET /curator/master/products/:id` → карточка мастера
- `internal/domain/types.go` — `TenantSummary`, `MasterProductSummary`, `TenantProductRow`, `TenantSchemaSummary`

**Frontend** (`curator/frontend/src/`):
- `pages/TenantsPage.jsx` — таблица: name, slug, products_count, integrations[], master_link_coverage%, schema_status, last_action
- `pages/TenantDetailPage.jsx` — header (метрики тенанта) + табы: Catalog (read-only продукты), Schema (artifact + validation report), Reports (placeholder под Этап 5), Audit (фильтр по tenant)
- `pages/MasterCatalogPage.jsx` — search + vertical filter + таблица master_products с brand/name/PIM-coverage badge'ами
- `pages/MasterDetailPage.jsx` — карточка мастера: variants, embedding-status, связанные tenant-листинги (по `master_variant_id`/`master_product_id`)
- `App.jsx` — routing
- `app.css` — sidebar категоризован: **Tenants / Master Catalog / Curation queues (Candidates/Junk) / Audit**

**Verification:**
- Login → видим `hey-babes-cosmetics` в Tenants
- Click → 979 продуктов
- Master Catalog → vertical=cosmetics → 979 master_products, 10 брендов
- Drill into master_product → variants empty (heybabes ещё без master_variants), но PIM-поля + связанный листинг показаны

### Этап 2 — Cut legacy + harvester-lite + two-mode search ✅ shipped

> **Логи:** [2.1 (cut legacy)](../Updates/main-admin-catalog-curator-pivot-stage2-1_2026-04-27_17-28.md) commit `554feb1` · [2.2 (harvester-lite)](../Updates/main-admin-catalog-curator-pivot-stage2-2_2026-04-27_17-35.md) commit `73f8916` · [2.3 (two-mode search)](../Updates/main-admin-catalog-curator-pivot-stage2-3_2026-04-27_17-39.md) commit `918ff15`

**2.1 Cut legacy**:
- Удалить `project_admin/backend/internal/usecases/shopify.go`
- Удалить `project_admin/backend/internal/usecases/shopify_mapper.go`
- В `cmd/server/main.go` убрать `shopifyUC` и legacy parts of `shopifyHandler`. Сохранить OAuth + webhook регистрацию через `shopifyV2UC`

**2.2 Harvester-lite** — новый файл `project_admin/backend/internal/usecases/harvester_lite.go`:
- После OAuth callback: вызвать `shopifyV2UC.DumpToStaging` → итерироваться по `shopify_raw_imports` (kind='product') → применить **только Tier1 deterministic mapping** (title→original_name, descriptionHtml→raw_attributes.description, vendor→raw_attributes.vendor, productType→raw_attributes.product_type, tags[]→raw_attributes.tags, featuredImage→media[0], variants[].sku/barcode/price/inventory/weight/options → variants array внутри `raw_attributes` или в отдельную колонку `variants JSONB` если её добавить)
- Upsert в `catalog.products` per product (idempotent по `(tenant_id, source_id)`)
- **НЕ трогает** `master_products` / `master_variants` / candidates
- Webhook `products/update` → harvester-lite на одном продукте

**2.3 Two-mode search в V4** — `project_v4/backend/internal/tools/tool_catalog_search.go`:
- На каждый запрос вычисляем `master_link_coverage` (можно кешировать в памяти на 5 мин per tenant)
- ≥30% → master mode (текущее поведение): vector + keyword + master facets
- <30% → listing-only: keyword (pg_trgm) по `original_name + raw_attributes::text + variants.sku`, ranking по rrf над несколькими полями. Vector skip
- Метадата для NLU (Agent1) — в режиме no-master возвращает field set из `raw_attributes` тенанта вместо master schema. Файл `project_v4/backend/internal/usecases/agent1_metadata.go` (или где сейчас `loadFieldLabels`)

**Verification:**
- `go build && go vet && go test` clean в admin + V4
- На dev-store (после wipe + reinstall): `SELECT COUNT(*) FROM master_products WHERE owner_tenant_id=<dev-store>` → 0
- `SELECT COUNT(*) FROM catalog.products WHERE tenant_id=<dev-store>` → 17
- V4 чат на dev-store → listing-only mode (coverage=0)
- V4 чат на heybabes → master mode (coverage 100%)

### Этап 3 — Новый dev-store с тестовыми данными ✅ shipped

> **Лог:** [main-admin-catalog-curator-pivot-stage3-code_2026-04-27_18-18.md](../Updates/main-admin-catalog-curator-pivot-stage3-code_2026-04-27_18-18.md) · commits `d81ed48` (write methods + seed) + `d3c7ac2` (productSet schema 2026-04 fixes). Live: 20 продуктов в Shopify dev-store, 4 collections, 17 lingering snowboard listings от вчерашнего M4 теста (cleanup в Этапе 7). После деплоя на прод запущен `cmd/sync-tenant-now` который через DumpToStaging + harvester-lite заполнил `catalog.products` (37 листингов, все без master-link).

**Зависит от пользователя.** Нужно: релизнуть Shopify-приложение в Partners с scopes `write_products + write_inventory + write_publications + write_collections` → переустановить в `keepstar-neaqpan1.myshopify.com`.

**После того как scopes есть:**
- Расширить `project_admin/backend/internal/adapters/shopify/client.go` write-методами (`ProductCreate`, `ProductVariantsBulkCreate`, `CollectionCreate`)
- Создать `cmd/seed-devstore/main.go` — Go-скрипт, заливает 25-30 продуктов покрывающих 8 сценариев:

| # | Сценарий | Состав | Что проверяем |
|---|---|---|---|
| 3.1 | Полный матч с heybabes-мастером | 3 cosmetics с GTIN/SKU существующих heybabes (COSRX, MEDI-PEEL) | match cascade: GTIN exact → существующий master_variant |
| 3.2 | Матч + обогащение | 3 cosmetics с теми же GTIN, но с metafield'ами которых у heybabes нет (scent, spf, packaging) | merge agent предлагает обогатить master |
| 3.3 | Матч, у тенанта меньше данных | 3 cosmetics с GTIN, но без description/images/metafield'ов | merge agent предлагает линк, не предлагает downgrade master |
| 3.4 | Категория переименована | 3 продукта в коллекции "Skincare Solutions" вместо "Face Care" | fuzzy category match |
| 3.5 | Нет матча, новая vertical | 5 furniture (chairs, tables) с уникальной структурой metafield'ов | merge agent предлагает new master_template |
| 3.6 | Multi-axis variants | 1 product (shoes) с осями Color × Size × Material | master_variants с axes JSONB |
| 3.7 | Junk variants | 1 product со sub-variants "Sample Sachet $2", "Gift Wrap $5" | junk detector ловит |
| 3.8 | Edge cases | 1 без GTIN/SKU, 1 с пустыми metafield'ами, 1 с очень длинным title | graceful handling |

### Этап 4 — Merge agent: проектирование (с пользователем)

Совместная focused-сессия (~1 час):
- Промпт agent'а — стиль пользователя
- JSON-схема `MergeReport`:
  ```
  {
    tenant_id, generated_at, agent_meta: { model, input_tokens, output_tokens, turns_used },
    proposals: [
      {
        listing_id,
        action: "link_to_existing" | "create_new_master" | "create_variant_of" | "skip" | "needs_human_review",
        target_master_id?, target_variant_id?,
        evidence: {
          match_score: 0..1,
          reason: "gtin_exact" | "vendor_sku" | "fuzzy_title" | "embedding" | "no_match",
          fields_to_inherit_from_master: [...],
          fields_to_propagate_to_master: [...],
          fields_conflicting: [{field, master_value, listing_value}],
          agent_reasoning: "..."
        },
        agent_confidence: "high" | "medium" | "low",
        status: "pending" | "approved" | "rejected"
      }
    ]
  }
  ```
- Решение: один батч на тенанта vs per-product. Default: батч (дешевле, consistency)
- Storage: новая таблица `catalog.merge_reports (id BIGSERIAL, tenant_id UUID, generated_at, status TEXT, report JSONB, agent_input_tokens, agent_output_tokens, applied_at)`
- Миграция

### Этап 5 — Merge agent: implementation + curator review UI

**Backend admin** (`project_admin/backend/`):
- `internal/usecases/merge_agent.go` — agent loop (Sonnet 4.6 + tool-use), переиспользует `adapters/anthropic/agent_client.go` (от M4c)
- `internal/usecases/merge_apply.go` — детерминированный applier: получает approved proposals → выполняет insert/update в `master_products` / `master_variants` / `products.master_variant_id`. Использует существующий `match_cascade.go` для финального linking + `junk_detector.go` для маркировки junk variants
- Миграция `catalog.merge_reports` (создаётся в Этапе 4)
- Endpoints (требуют curator-auth, новый middleware либо проксируется через curator backend):
  - `POST /admin/api/curator/tenants/:id/merge-agent/run` → возвращает `report_id`
  - `GET /admin/api/curator/tenants/:id/merge-reports` → список prior reports
  - `GET /admin/api/curator/merge-reports/:id` → полный report
  - `POST /admin/api/curator/merge-reports/:id/approve` → весь батч
  - `POST /admin/api/curator/merge-reports/:id/reject` → весь батч
  - `POST /admin/api/curator/merge-reports/:id/approve-partial` → body=`{proposal_ids: [...]}`

**Curator frontend**:
- `pages/TenantDetailPage.jsx` (Reports tab) — кнопка "Run merge agent" + список prior reports + drill-down
- `pages/MergeReportPage.jsx` — таблица proposals с per-row checkbox approve/reject. Каждая строка показывает: action, target master (если есть), match_score, evidence summary, conflicts. Bulk actions: Approve selected, Reject all, Approve all confident (agent_confidence=high)

### Этап 6 — End-to-end тесты на dev-store

- Reset dev-store (delete всё что налили) и повторить seed
- Прогнать через UI curator'а:
  1. Tenants page → click dev-store
  2. Run dump-to-staging (или auto-trigger в новом install flow)
  3. Run discover (M4c) → видим artifact + validation report
  4. Run merge agent → видим report
  5. Review proposals по 8 сценариям → approve по кейсам
  6. Verify в БД что master_products / master_variants / products.master_variant_id заполнились корректно
- Smoke V4 чата на dev-store (5-10 запросов: cosmetics, furniture, edge cases)

### Этап 7 — Heybabes master cleanup (минимальный)

- SQL fix: один master_product с cyrillic name + сломанным SKU
- LLM batch (Haiku 4.5, ~$5): backfill `description` для всех 979. Скрипт `cmd/backfill-descriptions/main.go`
- НЕ трогаем `catalog.products` heybabes-листингов

### Этап 8 — Забытое

Заглушка под пункт который пользователь забыл и вспомнит позже.

---

## 4. Порядок и зависимости

```
Этап 0 (docs)              ─┬─→ может стартовать сразу
Этап 1 (curator UI)        ─┘   самостоятельный, 4-6 часов
Этап 2 (cut+harvester+2mode)    самостоятельный, 3-4 часа, обязателен ДО Этапа 6
Этап 3 (dev-store seed)    ←── BLOCKED on user (Shopify app release)
Этап 4 (agent design)           совместная сессия после 1+2
Этап 5 (agent impl)             после 4
Этап 6 (e2e)                    после 2+3+5
Этап 7 (heybabes cleanup)       независим, 30 мин когда удобно
```

**Реалистичный график:**
- Этапы 0+1+2 — одна рабочая сессия (4-6 часов) **сегодня**
- Этап 3 — когда у пользователя готов app (отдельно)
- Этапы 4+5 — отдельная focused-сессия (4-6 часов)
- Этап 6 — 1 час
- Этап 7 — 30 минут

---

## 5. Reuse — что переиспользуем

| Компонент | Файл | Зачем |
|---|---|---|
| Tool-use Anthropic client | `project_admin/backend/internal/adapters/anthropic/agent_client.go` | Merge agent loop |
| Match cascade (GTIN/SKU/fuzzy/embedding) | `project_admin/backend/internal/usecases/match_cascade.go` | merge_apply applier |
| Junk detector | `project_admin/backend/internal/usecases/junk_detector.go` | merge_apply на variants |
| Discovery механика | `project_admin/backend/internal/usecases/discovery_run.go` (`MetadataHarvest`, `AutoMapTier1`, `ValidateArtifact`) | Запускается из curator'а отдельной кнопкой |
| Staging adapter | `project_admin/backend/internal/adapters/postgres/shopify_staging_adapter.go` | Без изменений |
| Curator promote pattern | `curator/backend/internal/adapters/postgres.go::PromoteAttribute` | Pattern для transactional whitelisted ALTER TABLE |
| Audit log | `catalog.audit_log` + `auditAdapter` | Все merge-actions пишут audit |
| HybridSearch | `project_v4/backend/internal/tools/tool_catalog_search.go` | Расширяем двумя режимами, не переписываем |

---

## 6. Что НЕ входит в этот pivot

- **Полностью автоматический harvester** (без curator'а) — отложено до момента когда ручной режим перестанет масштабироваться (>50 тенантов в неделю)
- **Embedding job для новых master_products** — добавим когда будет реально много новых vertical'ей. Heybabes уже с embeddings, dev-store furniture — без embeddings на старте, vector search для них просто не активен в master-mode
- **Webhook hash-diff path** — webhook просто перезаписывает листинг через harvester-lite, без диффа (быстрее проще, на текущих объёмах)
- **Frontend progress UI на ShopifyConnectPage** — в новом флоу клиент не ждёт agent'а, виджет работает сразу через listing-only
- **Bulk POST/DELETE через X-API-Key** (M10 stubs) — отложено до первого реального запроса
- **Cursor pagination на /api/v1/products** — offset работает на текущих объёмах
- **Переписывание рендеринга чата** под новые поля — отложено явно

---

## 7. Changelog

- **2026-04-27** — pivot создан, M4d auto-harvester и M7 heybabes Russian backfill отменены в пользу curator-driven manual merge flow. DB-проверка показала heybabes 100% English (979 master_products, 961 с PIM, 979 с embeddings) → переходит в seed cosmetics master-каталога.
