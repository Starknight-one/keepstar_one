# Admin Catalog — M6 / M8 / M9 / M10 / M11 / M12 (one session, six milestones)

- **Branch:** `main`
- **Date (UTC):** 2026-04-26 14:21
- **Parent commit:** `608bf55` (alpha-0.4 — M4a/b/c shipped, discovery verified)
- **Commits in this session (oldest first):**
  - `a200f4a` — feat(catalog): M6 — COALESCE read-path (admin + V4), heybabes legacy fallback
  - `7887d02` — feat(catalog): M8 — categories M:N + tenant tree editor
  - `6aba054` — feat(catalog): M9 — detected add-ons triage UI
  - `7779fa9` — feat(catalog): M10 — public REST API + tenant API key management
  - `0c70871` — feat(curator): M11 — standalone curator service
  - `413f77b` — feat(catalog): M12 — audit log integration (admin + history UI)
- **Plan file:** `~/.claude/plans/wise-foraging-walrus.md`
- **Per-milestone session logs (детально):**
  - `docs/Updates/main-admin-catalog-m6-coalesce-readpath_2026-04-26_13-46.md`
  - `docs/Updates/main-admin-catalog-m8-categories_2026-04-26_13-52.md`
  - `docs/Updates/main-admin-catalog-m9-detected-addons_2026-04-26_13-56.md`
  - `docs/Updates/main-admin-catalog-m10-public-api_2026-04-26_14-03.md`
  - `docs/Updates/main-curator-m11-standalone-service_2026-04-26_14-10.md`
  - `docs/Updates/main-admin-catalog-m12-audit-log_2026-04-26_14-15.md`

## Context

После M4a/b/c discovery agent работал end-to-end на dev-store. **M4d (harvester orchestrator + cut-over legacy)** и **M7 (heybabes 967 backfill)** были осознанно отложены в самый конец — оба требуют focused-сессии с пользователем у руля. План этой сессии — закрыть всё, что **не** зависит от harvester'а и от heybabes-backfill'а: read-path, categories, junk-triage UI, public API, curator-сервис, audit-wiring.

Все шесть milestones отшиплены атомарными коммитами. Между каждым код собирается чисто и тесты проходят.

## What landed (high level)

| Milestone | Что закрылось | Где смотреть |
|---|---|---|
| **M6** | COALESCE read-path в admin + V4 engine. Двухпутевой JOIN: `LEFT JOIN master_variants mv ON mv.id = p.master_variant_id`, потом `LEFT JOIN master_products mp ON mp.id = COALESCE(p.master_product_id, mv.master_product_id)`. Heybabes (master_variant_id NULL) продолжает работать через legacy `master_product_id`. domain.Product расширен displayName/originalName/sku/gtins/size/color/weightG/volumeMl/rawAttributes/media. ProductsPage показывает SKU + originalName под именем. ProductDetailPage — секции "Listing overrides" (editable) + "Master fields" (read-only). | `a200f4a` + лог |
| **M8** | Categories M:N + tenant tree editor. handler_categories.go (CRUD tenant + read master + N:1 mapping + M:N listing↔category). categories_v2_adapter заведён в DI (раньше был только реализован, но не зарегистрирован). Frontend CategoryEditor.jsx с tree + create/move/delete + kind-toggle (category/showcase/promo). На ProductDetailPage chip-multiselect категорий. Sidebar sub-link под "Catalog → Categories". | `7887d02` + лог |
| **M9** | Detected add-ons triage page. handler_junk.go (list/count/classify), DetectedAddonsPage.jsx с 3 tabs (Pending/Confirmed/False positive), sidebar count badge poll каждые 60 сек. Empty state до тех пор пока harvester не наполнит таблицу. | `6aba054` + лог |
| **M10** | Public REST API + tenant API key management. Plain token format `kp_<base64url(32 bytes)>`, в БД хранится `bcrypt(plain) + key_prefix` для индексированного lookup'а. `/admin/api/api-keys` (JWT) — CRUD; `/api/v1/products` + `/api/v1/categories` (X-API-Key) — read/PATCH публичный API. Bulk POST + DELETE возвращают 501 с TODO, потому что требуют harvester pipeline. Frontend `/settings/api-keys` с generate/copy-once/revoke flow. OpenAPI 3.0.3 sketch в `docs/api/v1/openapi.yaml`. | `7779fa9` + лог |
| **M11** | Standalone curator-сервис в новом repo root `curator/{backend,frontend}/`. Отдельный Go-модуль `keepstar-curator` (не импортирует project_admin). Port 8082. Auth: `curator.users + curator.sessions` с opaque token'ом `cs_<base64url>`, sha256 для O(1) session resolve. Транзакционный `PromoteAttribute` с paranoid-валидацией (whitelist verticals, whitelist column types, snake_case ident regex для candidate.key). Vite-frontend на 5175 dev: Login/Candidates/Junk/Audit pages. Scripts `start_curator.sh/stop_curator.sh`, Dockerfile для backend и frontend. CLI `seed-curator` для bootstrap'а первого user'а. | `0c70871` + лог |
| **M12** | Audit log wiring. `auditAdapter` теперь в DI (раньше был только реализован). Все state-changing endpoints пишут `LogHuman`: products.HandleUpdate (snapshot+diff over displayName/price/stock/rating/description), junk.HandleClassify, api_keys.HandleCreate+HandleRevoke, categories.HandleCreateTenant+HandleDeleteTenant. Best-effort — failures warn but don't fail request. Новый `GET /admin/api/audit?entity_kind=&entity_id=`. Frontend "History" секция на ProductDetailPage показывает per-field diff'ы (`price: 1290 → 1500`). Curator UI `/audit` (M11) теперь видит unified ленту: admin edits + curator promotes. | `413f77b` + лог |

## Files changed (cumulative this session)

### project_admin/backend
- `internal/domain/product.go` — Product +displayName/originalName/sku/gtins/size/color/weightG/volumeMl/rawAttributes/media; ProductUpdate +displayName/rawAttributes
- `internal/domain/audit.go` — +EntityKindAPIKey, +AuditActionClassify
- `internal/domain/api_key.go` — NEW
- `internal/domain/master_category.go` — +TenantCategoryWithCount
- `internal/domain/errors.go` — +ErrAPIKeyInvalid
- `internal/ports/categories_port.go` — +ListTenantCategoriesWithCounts, +DeleteTenantCategory
- `internal/ports/api_keys_port.go` — NEW
- `internal/adapters/postgres/catalog_adapter.go` — ListProducts/GetProduct/UpdateProduct под двухпутевой JOIN; mergeProductFromJoins helper
- `internal/adapters/postgres/categories_v2_adapter.go` — +ListTenantCategoriesWithCounts (LEFT JOIN с counts), +DeleteTenantCategory (tx с re-parent)
- `internal/adapters/postgres/catalog_migrations.go` — +ALTER api_keys ADD COLUMN key_prefix + indexed
- `internal/adapters/postgres/api_keys_adapter.go` — NEW (bcrypt cost 10, prefix index lookup)
- `internal/handlers/handler_products.go` — ProductsHandler.audit + diffProductUpdate helper, audit calls в HandleUpdate
- `internal/handlers/handler_categories.go` — NEW (7 endpoints) + audit calls
- `internal/handlers/handler_junk.go` — NEW + audit calls
- `internal/handlers/handler_api_keys.go` — NEW + audit calls
- `internal/handlers/handler_api_v1_products.go` — NEW (X-API-Key gated)
- `internal/handlers/handler_api_v1_categories.go` — NEW (X-API-Key gated)
- `internal/handlers/handler_audit.go` — NEW
- `internal/handlers/middleware_apikey.go` — NEW
- `cmd/server/main.go` — DI всех новых adapters/handlers + routes (admin protected + public sub-mux for /api/v1)

### project_admin/frontend
- `src/features/catalog/ProductsPage.jsx` — колонка SKU читает row.sku, оригинальное имя под displayName
- `src/features/catalog/ProductDetailPage.jsx` — Listing overrides + Master fields + Categories multi-select + History секция
- `src/features/catalog/CategoryEditor.jsx` — NEW (tree editor)
- `src/features/catalog/DetectedAddonsPage.jsx` — NEW (3 tabs + classify)
- `src/features/catalog/categoryEditor.css` + `detectedAddons.css` — NEW
- `src/features/catalog/catalog.css` + `productDetail.css` — расширены
- `src/features/settings/ApiKeysPage.jsx` + `apiKeys.css` — NEW
- `src/features/settings/SettingsPage.jsx` — link на /settings/api-keys
- `src/features/layout/DashboardLayout.jsx` — sub-links Categories + Detected add-ons (с count badge)
- `src/features/layout/layout.css` — .sidebar-sub + .sidebar-badge
- `src/App.jsx` — новые routes (/catalog/categories, /catalog/detected-addons, /settings/api-keys)
- `src/shared/api/apiClient.js` — +patch + del методы

### project_v4/backend
- `internal/adapters/postgres/postgres_catalog.go` — ListProducts/GetProduct/VectorSearch на двухпутевой JOIN; SELECT расширен variant + listing колонками; mergeProductWithMaster под display_name/original_name override и variant fields
- `internal/adapters/postgres/catalog_migrations.go` — +migrationCatalogProductsM4Columns (idempotent ALTER + CREATE master_variants stub) для standalone deploys
- `internal/domain/product_entity.go` — Product +9 fields
- `internal/tools/tool_visual_assembly.go` — ProductToMap эмитит новые ключи

### curator/ (full new)
- `backend/go.mod` + `go.sum`
- `backend/cmd/server/main.go` + `cmd/seed-curator/main.go`
- `backend/internal/{adapters/postgres.go, domain/types.go, handlers/handlers.go, session/middleware.go}`
- `backend/Dockerfile`
- `frontend/{package.json, vite.config.js, index.html, nginx.conf, Dockerfile}`
- `frontend/src/{main.jsx, App.jsx, api.js, app.css}`
- `frontend/src/pages/{LoginPage, CandidatesPage, JunkPage, AuditPage}.jsx`

### scripts
- `start_curator.sh` + `stop_curator.sh` — NEW

### docs
- `docs/api/v1/openapi.yaml` — NEW (M10)
- 6 файлов в `docs/Updates/` — по логу на каждый milestone

## Verification

| Codebase | Result |
|---|---|
| `project_admin/backend` | `go build && go vet && go test` — clean (units + usecases tests pass) |
| `project_admin/frontend` | `npm run build` — built in 3.61s, 152 KB CSS, 1.92 MB JS |
| `project_v4/backend` | `go build && go vet && go test` — clean (engine_v4, tools, usecases tests pass) |
| `curator/backend` | `go build && go vet` — clean |
| `curator/frontend` | Не build'ился локально (npm install не запускался) — собирается в Dockerfile при деплое |

### Smoke (после деплоя на Railway)

1. **V4 chat heybabes (prod)** — 5-10 запросов через виджет; проверить что recall и качество не упали относительно alpha-0.4.
2. **Admin /catalog** — таблица heybabes продуктов рендерится через COALESCE.
3. **Admin /catalog/categories** — пустое дерево, можно создать руками; продукт можно прикрепить к нескольким категориям.
4. **Admin /catalog/detected-addons** — empty state.
5. **Admin /settings/api-keys** — Generate → видно plainKey один раз → Copy → Revoke работает.
6. **Public API** — `curl -H 'X-API-Key: kp_xxx' /api/v1/products?limit=5` → 200; неверный ключ → 401.
7. **Curator** — отдельный Railway service на 8082; `seed-curator -email vlad@... -password 12chars` → login через UI → Candidates пустые (без harvester'а) → manual SQL insert candidate → Promote → ALTER TABLE отработал → audit_log получил запись `actor_kind=curator`.
8. **Audit на admin** — Edit price → ProductDetailPage History показывает diff чип; SQL подтверждает запись.

## What remains

Этой сессией закрыты M6/M8/M9/M10/M11/M12 от плана. Что **не** входит:

1. **M4d** — harvester orchestrator (статус: 🔴 deferred to focused session). Что нужно сделать описано в логе `main-admin-catalog-m4abc-discovery-tested_2026-04-26_13-13.md` секция "Plan correction". Потребуется сидячая сессия с пользователем у руля.
2. **M7** — heybabes 967 backfill (статус: 🔴 deferred). Названия на русском и кривые — нужен focused review.
3. **Polish из known gaps** каждого milestone — собран в индивидуальных логах сессий. Самые заметные:
   - M6: `?view=master` query parameter в admin handler (план упоминал, не сделано — для curator-debug использовать одновременную выдачу master+listing JSON shape).
   - M8: explicit `UpdateTenantCategory(id, ...)` метод на port (PATCH сейчас полагается на upsert).
   - M9: listing preview (image+name) на junk record — сейчас только `listing.id.slice(0,8)`.
   - M10: bulk POST `/api/v1/products` + DELETE через X-API-Key — 501 заглушки, нужны harvester и SoftDelete метод.
   - M10: cursor-based pagination на public API (план упоминал `(updated_at, id)`-cursor; сейчас offset).
   - M10: API v1 endpoints не пишут audit (M12 не покрыл их — потому что нужен `actor_kind=api`).
   - M11: match-reviews и master-cleanup pages — заглушки (нужны таблицы которых пока нет).
   - M11: CategoryCandidate promote (UI показывает list, но действовать не умеет).
   - M12: PATCH tenant_category, mapping changes, listing M:N category links — audit не пишет (нужны pre-read для diff'а).
   - V4: добавить integration test против реальной heybabes Neon БД для smoke-test M6 changes без ручной проверки.

Эти gaps **не блокируют** деплой и пользовательское тестирование того, что отшиплено. Они собраны в один список, чтобы M4-polish-сессия могла их подобрать вместе с harvester-orchestrator'ом.

## Next steps (operational)

1. Push to origin (`git push`).
2. Railway redeploy admin-backend (нужен — миграция api_keys.key_prefix + новые routes); admin-frontend (новые страницы); v4-engine-production (catalog read-path меняется, но heybabes должен продолжить работать через legacy fallback).
3. Создать новый Railway service для curator-backend (env: `DATABASE_URL=<Neon>`, `CURATOR_BIND=:8082`, `CURATOR_COOKIE_SECURE=true`) и для curator-frontend (через nginx Dockerfile).
4. После curator deploy: запустить `seed-curator` для создания первого user'а.
5. Smoke-test heybabes V4 chat в prod (5-10 запросов).
6. Smoke-test admin /catalog в prod (heybabes продукты видны).
7. Когда пользователь будет готов — сесть на M4d polish (harvester + cut-over legacy + frontend progress UI на ShopifyConnectPage).
