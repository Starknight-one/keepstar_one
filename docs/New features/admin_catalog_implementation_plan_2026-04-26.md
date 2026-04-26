# Admin Catalog — Implementation Plan

> **STATUS UPDATE 2026-04-26 13:13 UTC.** M1/M2/M3/M4a/M4b/M4c shipped. **Discovery agent verified end-to-end on the dev-store** (`dump-to-staging` + `discover` both return committed artifacts; logs in `docs/Updates/main-admin-catalog-m4abc-discovery-tested_2026-04-26_13-13.md`). **M4d (harvester orchestrator + cut-over legacy) intentionally deferred to the very end** — see "Order correction" at the bottom of this file. New ordering: **M6 → M7 → M8 → M9 → M10 → M11 → M4d (final polish)**.

## Context

Дизайн каталога админки закрыт в `docs/New features/admin_catalog_design_2026-04-23.md` (commit `c0e776c`). Сейчас в Neon Postgres лежат:
- 17 fashion-продуктов dev-store Shopify (синхронились через старый importer на пред-spec схему)
- 967 heybabes cosmetics-продуктов (legacy, с inline PIM-колонками skin_type/concern/ingredients и embeddings)

Нужно эволюционировать схему до spec'а **аддитивно** (без даунтайма), переписать Shopify-импорт на metadata-first flow с mapping artifact, добавить master_variants (parent-child), candidates-стейджинг, junk detection, audit log, curator service. Чат-движок V4 не должен сломаться в процессе.

**Decisions** (из Q&A 2026-04-26):
- Cosmetics PIM-колонки извлекаем в новую `master_cosmetics` (Tier 2)
- Dev-store 17 товаров: **wipe + resync** через новый pipeline; heybabes 967: **backfill-скрипт**
- Curator service: **standalone** (отдельная папка `curator/{backend,frontend}`, email+password login)

**Outcome**: production-ready каталог с master/listing/variants, metadata-first импортом, candidates promotion, junk detection. Ready под публичный launch.

---

## Critical files (где будем работать)

**Backend admin** (`project_admin/backend/`):
- `internal/adapters/postgres/catalog_migrations.go` — добавляем все новые миграции (registration pattern: append to `migrations []string`)
- `internal/adapters/postgres/catalog_adapter.go` — новые методы (UpsertMasterVariant, ListProductsCoalesce, etc.) + правка ListProducts на COALESCE
- `internal/adapters/postgres/integrations_adapter.go` — расширение для tenant_catalog_schema (mapping_artifact)
- `internal/adapters/postgres/audit_adapter.go` — НОВЫЙ
- `internal/adapters/postgres/candidates_adapter.go` — НОВЫЙ
- `internal/adapters/shopify/client.go` — добавить GraphQL bulk-operations (сейчас REST), metadata pulls
- `internal/usecases/shopify.go` + `shopify_mapper.go` — переписать на metadata-first flow
- `internal/usecases/mapping_artifact.go` — НОВЫЙ (agent normalizer, 1 LLM-вызов на тенанта)
- `internal/usecases/match_cascade.go` — НОВЫЙ (GTIN→vendor+SKU→fuzzy→embedding)
- `internal/usecases/junk_detector.go` — НОВЫЙ
- `internal/units/` — НОВАЯ папка (deterministic state-machine parser + canonical units, см. spec §5)
- `internal/domain/` — расширения (MasterVariant, AttributeCandidate, JunkVariant, AuditEntry, MappingArtifact)
- `internal/handlers/handler_products.go` — обновить ответ под новую модель
- `internal/handlers/handler_api_v1_*.go` — НОВЫЕ для public API (REST endpoints)
- `internal/handlers/middleware_apikey.go` — НОВЫЙ (X-API-Key auth)
- `cmd/server/main.go` — DI новых usecases/handlers
- `cmd/backfill-heybabes/main.go` — НОВЫЙ одноразовый скрипт

**Frontend admin** (`project_admin/frontend/src/features/`):
- `catalog/ProductsPage.jsx` + `ProductDetailPage.jsx` — переписать под master/listing разделение, COALESCE-рендер, "Master view" toggle
- `catalog/CategoryTree.jsx` + новая `CategoryEditor.jsx` — управление tenant_categories деревом
- `catalog/DetectedAddonsPage.jsx` — НОВЫЙ (junk variants triage)
- `import/ShopifyConnectPage.jsx` — добавить progress UI на длинный bulk-job (15-40 мин)
- `settings/ApiKeysPage.jsx` — НОВЫЙ (CRUD api keys для public API)

**Curator service** (НОВЫЕ папки):
- `curator/backend/cmd/server/main.go` + minimal Go service
- `curator/backend/internal/{handlers,usecases,adapters,domain}` — auth, candidates, match-reviews, junk
- `curator/frontend/src/features/{auth,candidates,matches,junk,master}` — React app

**V4 engine** (`project_v4/backend/`):
- `internal/adapters/postgres/postgres_catalog.go` — обновить `ListProducts` query на COALESCE (не сломать существующее)
- `internal/usecases/agent2_execute.go` — `loadFieldLabels` подсасывает Tier 2 labels через расширенный field_definitions

**Reuse** (что уже есть, не пишем заново):
- `internal/crypto/secretbox` — encryption envelope для api_keys (формат `v1.{nonce}.{ct}`)
- `internal/handlers/middleware_auth.go` — auth pattern для admin (JWT bearer)
- `internal/adapters/postgres/admin_migrations.go` — migration pattern (idempotent IF NOT EXISTS)
- `internal/adapters/postgres/integrations_adapter.go` — encrypted credentials pattern
- Существующий `catalog.master_products.embedding vector(384)` — для match cascade embedding-step
- Существующий `ports.FieldDefinition` (B7 sessions A/B/C) — для V4 engine read-path интеграции

---

## Milestones

Каждый milestone = atomic commit, оставляет систему работающей. Можно остановиться после любого если упрёмся.

### M1. Schema migrations (additive, все новые таблицы)

Один файл `catalog_migrations.go` с новыми SQL-блоками idempotent (IF NOT EXISTS). Старые таблицы не трогаем.

**Новые таблицы**:
- `catalog.master_variants` (id, master_product_id FK, sku, gtins TEXT[], image_url, weight_g, volume_ml, length_mm/width_mm/height_mm, color, size, material, axes JSONB, variant_kind ENUM, weight_raw, volume_raw, parse_status JSONB)
- `catalog.master_cosmetics` (master_variant_id PK FK, skin_type TEXT[], concern TEXT[], ingredients TEXT[], scent, spf)
- `catalog.master_categories` (id, parent_id, slug UNIQUE, name, vertical)
- `catalog.master_product_categories` (master_product_id, master_category_id) — M:N
- `catalog.tenant_categories` (id, tenant_id, parent_id, external_id, slug, name, kind ENUM)
- `catalog.category_mapping` (tenant_category_id PK, master_category_id, confidence, mapped_by)
- `catalog.tenant_listing_categories` (listing_id, tenant_category_id) — M:N
- `catalog.master_attribute_candidates` (id, key, vertical, seen_in_tenants, sample_values JSONB, proposed_type, embedding, status, promoted_to_column)
- `catalog.master_category_candidates`
- `catalog.tenant_variant_candidates_junk` (id, tenant_id, listing_id, master_variant_id, detected_reason JSONB, classification ENUM)
- `catalog.audit_log` (id BIGSERIAL, tenant_id, actor_kind, actor_id, entity_kind, entity_id, action, field_changes JSONB, aggregate_meta JSONB, created_at)
- `catalog.tenant_catalog_schema` (tenant_id PK, mapping_artifact JSONB, artifact_version, status, discovered_at, validated_at, re_discover_after)
- `catalog.unit_aliases` (id, tenant_id NULLABLE, raw_token, canonical_unit, confidence, source) + seed для глобальных алиасов
- `catalog.api_keys` (id, tenant_id, key_hash, label, last_used_at, revoked_at)

**Расширение существующих**:
- `ALTER TABLE catalog.products ADD COLUMN master_variant_id UUID, original_name TEXT, display_name TEXT, raw_attributes JSONB DEFAULT '{}', media JSONB DEFAULT '[]', source_system TEXT, source_id TEXT, payload_hash TEXT, deleted_at TIMESTAMPTZ`
- `ALTER TABLE catalog.master_products ADD COLUMN vertical TEXT NOT NULL DEFAULT 'cosmetics', tier3 JSONB DEFAULT '{}', confidence TEXT DEFAULT 'unverified'`

Индексы: GIN на `gtins`, ivfflat на новых embedding-колонках где появятся, btree на FK + tenant_id.

**Verification**: `go build && go vet`, deploy → Neon применяет → проверить `\d+ catalog.master_variants` через psql, проверить что V4 engine + admin frontend работают как раньше (старые колонки не тронуты).

### M2. Domain types + порты + skeleton-адаптеры

Новые структы в `internal/domain/`:
- `MasterVariant`, `MasterCosmetics`
- `AttributeCandidate`, `CategoryCandidate`
- `JunkCandidate`
- `AuditEntry`
- `MappingArtifact` (с FieldMapping/CategoryMapping/MatchStrategy полями)
- `MasterCategory`, `TenantCategory`

Новые ports в `internal/ports/`:
- `MasterVariantsPort`, `CandidatesPort`, `AuditPort`, `MappingArtifactPort`, `CategoriesPort`

Skeleton-адаптеры в `internal/adapters/postgres/` (CRUD методы, но без сложной логики). Билд должен пройти, но методы пока возвращают NotImplemented для всего кроме базовых insert/select.

**Verification**: `go build && go vet` — всё компилируется, никаких регрессий.

### M3. Units parser + alias-таблица

`internal/units/` (новая папка):
- `units.go` — канонические единицы (mL, g, mm, pcs), constant-table конверсий
- `parser.go` — deterministic state machine (tokenize → alias lookup → pattern classify → store)
- `aliases.go` — adapter wrapper для unit_aliases table lookup с tenant→global fallback
- `units_test.go` — 50+ unit-тестов на реальные строки (`30 ml`, `2x30ml`, `30ml/100ml`, bare `60`, junk `large`)

Seed глобальных алиасов в M1 миграции (мл, ml, milliliter, г, g, gram, etc.).

**Verification**: `go test ./internal/units/...` — все 50+ кейсов pass.

### M4. New Shopify metadata-first import

> **Status: 4a/b/c ✅ shipped, 4d 🔴 deferred to end.** End-to-end test на dev-store пройден (51 сек / 13 turns / commit_artifact / 13 mappings + 3 categories + 1 master_template `winter_sports` / ~$0.15). Лог: `docs/Updates/main-admin-catalog-m4abc-discovery-tested_2026-04-26_13-13.md`. M4d (harvester orchestrator + cut-over legacy ShopifyUseCase + embedding job + hash-diff webhooks + frontend progress UI + wipe/resync 17 dev-store) делается **последним milestone'ом** — после M6/M7/M8/M9/M10/M11. См. "Order correction" внизу.

Переписать `internal/usecases/shopify.go` + `shopify_mapper.go`:
1. **Metadata pull** (sync GraphQL) — `metafieldDefinitions(ownerType: PRODUCT|VARIANT|COLLECTION)`, `menu(handle:)`, `shop.{productVendors, productTypes, productTags}`
2. **Bulk Operations API** — `bulkOperationRunQuery` для полного каталога, polling, JSONL download (новый метод в `client.go`)
3. **Metadata harvest** (чистый код) — frequency, type inference, value distribution per field
4. **Auto-mapping Tier 1** — Vendor/Brand/Manufacturer → `master.brand`, GTIN/Barcode → `master_variants.gtins`, Weight → `master_variants.weight_g` через unit parser
5. **Agent normalizer** (`mapping_artifact.go`) — 1 LLM-вызов на тенанта с meta-report → mapping artifact JSON, max 5 tool-calls
6. **Validation** — прогон mapping на 10-20 батчевых товарах
7. **Harvester** — применяет artifact, вызывает match cascade, создаёт listings + master + variants

Старый `usecases/shopify.go` удаляется. Frontend `ShopifyConnectPage.jsx` показывает progress bar на bulk-job ("Analyzing your catalog... ~30 min").

**Verification**:
- Wipe 17 dev-store products + integration record (через `Disconnect` button + SQL cleanup)
- Reinstall App → новый flow → 17 products через metadata-first → проверить что `master_variants` созданы, `payload_hash` записан, `mapping_artifact` сохранён
- Re-install (повторный) → hash-diff skip, < 1 sec, 0 LLM calls

### M5. Match cascade + junk detection

`internal/usecases/match_cascade.go`:
- `MatchVariant(payload, tenantID) (matchResult, error)` — GTIN exact → vendor+SKU → fuzzy (similarity > 0.85 + axes) → embedding (threshold 0.92) → новый master с `confidence='unverified'`
- Конфликты в `match_review_queue` (новая таблица в M1 если упустил, иначе reuse `tenant_variant_candidates_junk` с другой `kind`)

`internal/usecases/junk_detector.go`:
- `IsJunkCandidate(variant) []signal` — axis_name regex patterns (gift wrap/engraving/warranty/add-on), no_identifiers, no_dimensions, small_price_delta
- Сигналы ≥2 → запись в `tenant_variant_candidates_junk` со `classification='pending'`

Wired в harvester (M4): каждый Shopify variant прогоняется через MatchVariant + IsJunkCandidate.

**Verification**:
- Создать в dev-store товар "Gift wrap" с axis_name=Gift wrap, без barcode, цена $5 → resync → должно появиться в `tenant_variant_candidates_junk`
- Создать второй тенант (test-tenant-2) с GTIN-совпадающим товаром → должен слинковаться с тем же master_variant (verify SQL)

### M6. COALESCE-render: admin + V4 engine

**Admin adapter** (`catalog_adapter.go.ListProducts`):
```sql
SELECT
  COALESCE(p.display_name, p.original_name, mp.name) AS name,
  COALESCE(p.original_name, mp.name) AS full_name,
  COALESCE(jsonb_array_length(p.media) > 0, false) AS has_listing_media,
  COALESCE(p.media, mv.image_url, mp.images) AS image,
  p.price, p.currency, p.stock_quantity,
  mp.brand, mv.gtins, mv.sku, mv.size, mv.color,
  p.raw_attributes
FROM catalog.products p
JOIN catalog.master_variants mv ON mv.id = p.master_variant_id
JOIN catalog.master_products mp ON mp.id = mv.master_product_id
WHERE p.tenant_id = $1 AND p.deleted_at IS NULL
```
Добавить query parameter `?view=master` для curator-debug.

**V4 engine** (`postgres_catalog.go.ListProducts`): обновить аналогично, осторожно — не сломать V4 чат на проде. Подход: добавить новые колонки в SELECT с `COALESCE(новое_поле, старое_поле)`, чтобы для старых записей (master_variant_id NULL) fallback на текущее поведение. Тест на heybabes (после M7 backfill). Если что-то сломается — `git revert` только этого коммита.

**Verification**:
- Admin `/catalog` показывает товары через COALESCE
- V4 chat на heybabes продолжает выдавать продукты
- Smoke-test: dev-store товар через V4 (если test-tenant подключен)

### M7. Heybabes backfill script

`cmd/backfill-heybabes/main.go` — одноразовый Go-скрипт:
1. Читает все 967 master_products (heybabes tenant)
2. Для каждого создаёт 1 default master_variant (sku из mp.sku, без gtin поскольку нет, image_url из mp.images[0], etc.)
3. Cosmetics PIM-колонки (skin_type, concern, etc.) копируются в `master_cosmetics(master_variant_id, ...)` — это тот самый Tier 2 extract
4. `catalog.products` rows получают `master_variant_id` через UPDATE
5. Existing `attributes JSONB` копируется в `listing.raw_attributes`
6. Embedding пересчитывать НЕ нужно — уже есть на master_products

После успешного backfill: миграция дропа inline cosmetics-колонок из master_products (`ALTER TABLE ... DROP COLUMN skin_type, concern, ...`) — отдельным коммитом, после verification.

**Verification**:
- Запустить `go run ./cmd/backfill-heybabes` локально с DATABASE_URL=Neon
- SQL: `SELECT count(*) FROM catalog.master_variants WHERE master_product_id IN (SELECT id FROM catalog.master_products WHERE owner_tenant_id=...)` → 967
- Heybabes V4 chat продолжает работать (smoke-test 5 запросов)
- После DROP колонок второй прогон V4 chat — не сломалось

### M8. Categories M:N + tree editor

Backend:
- `categories_adapter.go` методы: дерево с CTE для product counts per category, M:N upsert/delete
- `handler_categories.go` — расширить (CRUD tenant_categories, GET с count)
- Заполнение `tenant_categories` в harvester (M4) при импорте Shopify collections (с приоритетом nav menu > handle prefix > parent metafield)

Frontend:
- `catalog/CategoryEditor.jsx` — drag-and-drop tree (можно использовать `react-arborist` или native HTML5 dnd)
- `catalog/ProductDetailPage.jsx` — multi-select для категорий товара

**Verification**:
- В dev-store создать collection "Best Sellers" (kind=showcase), reimport → tenant_categories обновляется
- В админке зайти в /catalog/categories → дерево, count рядом с каждой
- Перетащить товар в две разные категории → SQL `SELECT * FROM tenant_listing_categories WHERE listing_id=...` → 2 строки

### M9. Detected add-ons page (junk triage)

Frontend `features/catalog/DetectedAddonsPage.jsx`:
- Видна в сайдбаре только если есть `pending` записи в `tenant_variant_candidates_junk`
- Список с product preview, detected_reason badges, кнопки "Mark as add-on" / "Mark as real" / "Send batch to agent"
- "Send batch to agent" вызывает LLM (Claude Haiku) для классификации, обновляет `classification` колонку

Backend handler `handler_junk.go` + usecase `usecases/junk_classify.go`.

**Verification**: см. M5 — junk record уже создан → страница показывает его → нажимаем Mark as add-on → SQL `classification='confirmed_addon'`.

### M10. Public API + api_keys management

Backend:
- `handlers/handler_api_v1_products.go` — REST endpoints (GET list/get/PATCH/DELETE, POST bulk push, GET imports)
- `middleware_apikey.go` — `X-API-Key: <plain key>` header, lookup hash
- `handlers/handler_api_keys.go` — CRUD для tenant API keys (admin-protected)

Frontend:
- `settings/ApiKeysPage.jsx` — список ключей, generate (показ plain text один раз), revoke

OpenAPI spec в `docs/api/v1/openapi.yaml` (минимальный).

**Verification**: `curl -H 'X-API-Key: kp_xxx' https://admin-production-4ae4.up.railway.app/api/v1/products?limit=10` → 200 + список товаров. С невалидным ключом → 401.

### M11. Curator service (standalone)

Новые папки `curator/backend/` + `curator/frontend/`:

Backend (`curator/backend/`):
- Go service на отдельном порту (8082)
- Отдельная DB schema `curator.users` (email + bcrypt password)
- Прямой доступ к `catalog.*` через service-account, тенантский middleware не применяется
- Endpoints:
  - `POST /curator/auth/login` — JWT
  - `GET /curator/candidates?vertical=&status=pending` — список + counts
  - `POST /curator/candidates/{id}/promote` — ALTER TABLE + backfill (см. M12)
  - `GET /curator/match-reviews` — спорные match'и
  - `POST /curator/match-reviews/{id}/link` — линк к выбранному master_variant
  - `GET /curator/junk-classification?status=pending`
  - `POST /curator/junk-classification/batch` — массовый классификатор через агента

Frontend (`curator/frontend/`):
- React Vite (отдельный package.json)
- Pages: `/curator/login`, `/curator/candidates`, `/curator/match-reviews`, `/curator/junk-classification`
- Минималистичный UI (utility-focused, не fancy)

Деплой: новый Railway service `Curator` с env `CURATOR_DB_URL` (та же Neon что и admin).

**Verification**:
- `curl POST /curator/auth/login` → JWT
- `curl GET /curator/candidates` → список (после M5 должен быть junk-detected запись)
- `curl POST /curator/candidates/{id}/promote` → `ALTER TABLE master_cosmetics ADD COLUMN scent TEXT` происходит → mapping artifacts помечаются stale → следующий import пересобирается с типизированной колонкой

### M12. Audit log + promotion mechanics

`internal/usecases/audit.go`:
- `LogSystem(batch_id, count, source)` — для bulk-операций (1 запись с aggregate_meta)
- `LogHuman(actor, entity, field_changes)` — детально per-field

Wire в:
- Harvester (M4) → один `LogSystem` на batch
- Admin product update endpoint → `LogHuman` per-field diff
- Curator promote endpoint → `LogHuman` с описанием миграции

Promotion mechanics в curator (M11) usecase `promote_attribute.go`:
1. Transactional `ALTER TABLE master_cosmetics ADD COLUMN ...`
2. Update `master_attribute_candidates.status='promoted'`
3. Background job: backfill значения из listing.raw_attributes в новую колонку
4. Mark mapping_artifact как stale в `tenant_catalog_schema`

**Verification**: SQL `SELECT * FROM catalog.audit_log WHERE entity_kind='listing' ORDER BY created_at DESC LIMIT 10` — записи появляются после правок и импортов.

---

## Verification — end-to-end после всех milestones

1. **Polished install flow** — новый тенант:
   - `/auth/signup` → workspace create
   - `/integrations/shopify` → install → bulk job (progress UI) → mapping artifact created → harvester
   - `/catalog` показывает товары (COALESCE-rendered, master+listing)
   - `/catalog/categories` показывает дерево из nav menu
   - `/catalog/detected-addons` показывает junk-варианты если есть

2. **Re-import** — товар в Shopify меняем (price), webhook прилетает:
   - hash-diff видит изменение → updates listing.price
   - Latency < 1s, 0 LLM calls

3. **Cross-tenant match** — тестовый второй тенант с GTIN-совпадением → автоматически линкуется к существующему master_variant

4. **Curator workflow** — заходим в `/curator`, видим candidates, promote → ALTER TABLE → backfill → re-import тенанта показывает новое поле в типизированной колонке

5. **V4 chat** — heybabes prod чат продолжает работать как до миграции (нет регрессии в реальных запросах)

6. **Public API** — внешний клиент пушит обновление через `POST /api/v1/products`, результат виден в admin

7. **Audit log** — все ключевые действия записаны (импорты с aggregate_meta, ручные правки per-field)

---

## Implementation order rationale (original)

- **M1-M3** (миграции, домен, юниты) — фундамент, ничего не меняет в работающей системе
- **M4** (новый импорт) — первое наблюдаемое изменение, тестируется на dev-store wipe+resync (минимальный риск, 17 товаров)
- **M5** (cascade+junk) — нужен для M4, но логически отделим
- **M6** (COALESCE) — read-path обновление, **здесь риск регрессии V4 чата** (главное не сломать heybabes)
- **M7** (backfill) — превращает 967 heybabes в новую модель
- **M8-M10** — пользовательские фичи (UI categories, junk triage, public API)
- **M11-M12** — curator + audit (production polish)

После M7 продукт уже может приниматься как "production-ready Shopify integration с master/variants/listing" — M8-M12 это фичи поверх.

---

## Order correction (2026-04-26 13:13 UTC)

**Что поменялось.** M4 был задуман как один FAT-коммит. По ходу его разбили на 4 коммита (4a/b/c/d). 4a/b/c **полностью отшиплены и протестированы end-to-end на dev-store** — discovery agent работает: bulk pull → staging → meta-report → Tier1 auto-map → Sonnet 4.6 loop с 8 tools → mapping artifact → validation. Полный лог: `docs/Updates/main-admin-catalog-m4abc-discovery-tested_2026-04-26_13-13.md`.

**Что осталось от M4 = M4d:**
- Harvester orchestrator (стримит staging → applies artifact → match cascade → junk detector → пишет master/listing/cosmetics/tier3)
- Embedding job (parent + per-variant через OpenAI text-embedding-3-small)
- Hash-diff webhook re-write (заменить legacy `usecases/shopify.go::HandleWebhook`)
- DI cut-over в `cmd/server/main.go` (удаление `usecases.NewShopifyUseCase` + `usecases/shopify_mapper.go`)
- Frontend progress UI на `ShopifyConnectPage.jsx` (стадии install)
- Wipe + resync 17 dev-store products через новый pipeline
- Полировка из "Known gaps" лога (validation threshold для system fields, conditional field mapping для Color-axis, expansion dev-store через `seed-dev-products` endpoint)

**Почему отложено в самый конец.** Discovery агент работает, но понять что именно агент сделал и почему (transcript / validation report / proposed mappings) требует фокусированной сессии — пользователь должен сесть и пройти end-to-end вместе с тем кто кодит, не параллельно с другой работой. Делать cut-over legacy importer'а параллельно с UX/curator работой = риск регрессии без активного review.

**Новый порядок:**

| Phase | Milestones | Зачем |
|---|---|---|
| **Сейчас → следующие сессии** | M6 → M7 → M8 → M9 → M10 → M11 | Продвигаем всё что независимо от harvester'а. Read-path COALESCE, backfill heybabes, categories tree, junk triage UI, public API, curator service + promotion. После M7 чат-движок V4 уже работает на новой модели. |
| **Финальная сессия M4 polish** | M4d (всё что выше) | В одну сидячую сессию с пользователем у руля: агент → harvester → embedding → cut-over → UI → wipe/resync. Можно параллельно подкрутить validation, conditional mapping, расширить test catalog. |

**Риски нового порядка.**

- M6/M7 могут вскрыть мелкие нужды в artifact format или staging shape, которые хотелось бы прошить через discovery agent. **Принимаем** — фиксим в финальной M4 сессии.
- **M8-M11 не блокированы** — они работают с уже существующими `master_products`/`products`. M11 (curator) тем более независим — у него собственная схема и собственный сервис.
- Production-quality install flow (полировка) появляется только в финальной M4 сессии. Дев-store продолжает работать через legacy auto-sync (как и сейчас).

**Что должно быть готово до финальной M4 сессии:**

1. Все M6-M11 закрыты или хотя бы доведены до точки "не блокируют M4d"
2. Расширенный test catalog в dev-store (`seed-dev-products`) — cosmetics-дубли brand'ов heybabes + furniture + junk + collections + custom metafields. Нужен write_products scope (instructions в логе).
3. Пользователь свободен на 1-2 часа сидячей работы, не отвлекаясь на параллельные задачи.
