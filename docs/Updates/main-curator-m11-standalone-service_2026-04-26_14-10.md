# Curator standalone service (M11)

- **Branch:** `main`
- **Date (UTC):** 2026-04-26 14:10
- **Parent:** `7779fa9` (M10 public API)

## Context

Дизайн caталога (`§1.7, §6.8`) требует standalone curator-сервис — отдельный backend (порт 8082) и frontend (порт 5175 в dev) с собственной auth-схемой `curator.users + curator.sessions`. Curator имеет прямой service-account доступ к `catalog.*` и не пользуется тенантским middleware. M11 закрывает каркас.

## Approach

### Repo layout

```
curator/
  backend/
    go.mod                   (separate Go module: keepstar-curator)
    Dockerfile
    cmd/server/main.go
    cmd/seed-curator/main.go
    internal/
      adapters/postgres.go   (single-file DB layer: users + sessions + catalog reads)
      domain/types.go        (small subset mirroring admin types)
      handlers/handlers.go   (auth + candidates + junk + audit endpoints)
      session/middleware.go  (cookie or Bearer token)
  frontend/
    package.json (vite + react 19 + react-router 7)
    vite.config.js (port 5175, proxy /curator → :8082)
    Dockerfile + nginx.conf
    src/
      main.jsx, App.jsx, api.js, app.css
      pages/{LoginPage,CandidatesPage,JunkPage,AuditPage}.jsx
scripts/
  start_curator.sh, stop_curator.sh
```

Curator — отдельный Go-модуль (`keepstar-curator`), он не импортирует `project_admin/internal/*`. Это умышленно — сервисы независимы при деплое и могут переезжать в разные контейнеры/Neon-БД без сложного рефакторинга. Цена — небольшое дублирование domain types и SQL adapter.

### Auth: opaque sessions, не JWT

Login принимает email + password, верифицирует bcrypt, генерирует случайный 32-byte token (`cs_<base64url>`). Запись в `curator.sessions(token_hash, user_id, expires_at)` где `token_hash = sha256(plain)` — **не bcrypt**, потому что resolve должен находить запись по PRIMARY KEY (bcrypt не deterministic).

7-дневный TTL. Cookie `curator_session` (HttpOnly, SameSite=Lax, Secure если `CURATOR_COOKIE_SECURE=true`). Bearer header работает параллельно для curl/CLI.

### Promote: транзакционный ALTER TABLE с paranoid validation

`PromoteAttribute(candidateID, key, vertical, columnType)`:

1. Whitelist verticals → таблицы (`cosmetics → master_cosmetics`, `laptops → master_laptops`)
2. Whitelist column types (`text/text_array/integer/numeric/boolean → TEXT/TEXT[]/INTEGER/NUMERIC/BOOLEAN`)
3. Strict ident validation на `key`: `^[a-z][a-z0-9_]{0,59}$` — потому что мы стампим candidate.key напрямую в DDL
4. BEGIN
   - `ALTER TABLE catalog.{table} ADD COLUMN IF NOT EXISTS {key} {pgType}`
   - `UPDATE master_attribute_candidates SET status='promoted', promoted_to_column='{table}.{key}'`
   - `UPDATE tenant_catalog_schema SET status='stale' WHERE mapping_artifact->'field_mapping' @> '{"target":"candidate:{key}"}'::jsonb`
5. COMMIT
6. Audit (best-effort, вне tx — rollback не убьёт запись)

Без harvester'а пункты 4.2 и 4.3 будут no-op (нет данных), это OK.

### Endpoints (port 8082)

| Method | Path | Auth | Purpose |
|---|---|---|---|
| POST | `/curator/auth/login` | — | email+password → cookie + token |
| POST | `/curator/auth/logout` | — | drop session |
| GET  | `/curator/me` | session | current user info |
| GET  | `/curator/candidates/attributes?status=` | session | pending attribute candidates |
| POST | `/curator/candidates/attributes/{id}/promote` | session | ALTER TABLE + bookkeeping |
| POST | `/curator/candidates/attributes/{id}/dismiss` | session | mark dismissed |
| GET  | `/curator/candidates/categories?status=` | session | category candidates |
| GET  | `/curator/junk?status=` | session | junk candidates across all tenants |
| POST | `/curator/junk/{id}/classify` | session | confirmed_addon \| false_positive |
| GET  | `/curator/audit?limit=` | session | recent audit_log entries |
| GET  | `/health` | — | liveness probe |

### Frontend

Vite-app с 4 страницами + login. Минималистичный dark UI, своя цветовая схема (purple accent #7c3aed). Pages:
- `LoginPage` — email + password
- `CandidatesPage` — pending attribute candidates с inline promote-form (column type select) + dismiss
- `JunkPage` — pending junk с двумя кнопками per row (mark add-on / mark real)
- `AuditPage` — последние 100 записей audit_log

В dev `vite.config.js` проксирует `/curator/*` на `:8082` чтобы cookie оставался same-origin.

### Bootstrap

`go run ./cmd/seed-curator -email vlad@keepstar.tech -password "min12chars"` — создаёт первого user'а. Idempotent (UPDATE password_hash on conflict). UI signup'а нет — только CLI.

### Deploy

- `curator/backend/Dockerfile` — multi-stage build, статические бинарники + Alpine
- `curator/frontend/Dockerfile` — Vite build + nginx с SPA fallback
- `scripts/start_curator.sh` / `scripts/stop_curator.sh` — для локального dev
- Railway: 2 новых сервиса (`Curator-backend`, `Curator-frontend`) когда пользователь готов поднять. Env vars:
  - `CURATOR_DATABASE_URL` (or fallback `DATABASE_URL`) — та же Neon
  - `CURATOR_BIND` (default `:8082`)
  - `CURATOR_COOKIE_SECURE=true` для HTTPS
  - `CURATOR_CORS_ORIGIN=https://curator.keepstar.tech` если frontend на отдельном домене

## Files changed

| Scope | File | Change |
|---|---|---|
| backend module | `curator/backend/go.mod`, `go.sum` | NEW |
| backend | `curator/backend/cmd/server/main.go` | NEW |
| backend | `curator/backend/cmd/seed-curator/main.go` | NEW |
| backend | `curator/backend/internal/domain/types.go` | NEW |
| backend | `curator/backend/internal/adapters/postgres.go` | NEW (auth + 7 catalog read methods + PromoteAttribute) |
| backend | `curator/backend/internal/handlers/handlers.go` | NEW |
| backend | `curator/backend/internal/session/middleware.go` | NEW |
| backend | `curator/backend/Dockerfile` | NEW |
| frontend | `curator/frontend/package.json` + `vite.config.js` + `index.html` + `nginx.conf` + `Dockerfile` | NEW |
| frontend | `curator/frontend/src/{main.jsx,App.jsx,api.js,app.css}` | NEW |
| frontend | `curator/frontend/src/pages/{LoginPage,CandidatesPage,JunkPage,AuditPage}.jsx` | NEW |
| scripts | `scripts/{start,stop}_curator.sh` | NEW |

## Verification

- `cd curator/backend && go build ./... && go vet ./...` — clean
- `cd curator/backend && go run ./cmd/seed-curator -email test@local -password testpassword12` — создаёт user'а в Neon
- `scripts/start_curator.sh` (если frontend node_modules уже установлен) — backend на 8082, frontend на 5175
- POST `/curator/auth/login` `{email,password}` → 200 + Set-Cookie
- GET `/curator/candidates/attributes` → `{"candidates": []}` (пусто без harvester'а)
- Manual SQL: `INSERT INTO catalog.master_attribute_candidates (key, vertical, sample_values, proposed_type) VALUES ('scent', 'cosmetics', '["floral","woody"]', 'text')` → страница показывает запись → нажать Promote → SQL `\d+ catalog.master_cosmetics` → колонка `scent TEXT` существует, candidate `status=promoted, promoted_to_column='master_cosmetics.scent'`. Audit_log получает запись `actor_kind=curator, action=promote`.

## Known gaps

1. **Match-reviews / master-cleanup pages в плане M11** — оставлены как заглушки. Match-reviews требует отдельную таблицу `match_review_queue`, которой нет в M1 — её надо ввести с harvester'ом (M4d). Master-cleanup для дубликатов master_products — отдельная сложная задача (UI для merge), её отложу до production-traffic'а.
2. **CategoryCandidate promote** — endpoint показа есть (`/curator/candidates/categories`), но promote не реализован (нет ALTER TABLE — promote категории это INSERT в `master_categories`). UI показывает list, но не действует. Сделается в M4 polish с реальными данными.
3. **`columnType=text_array`** в promote генерирует `TEXT[]`, но spec предполагает что dataset для backfill этого типа — JSON array из listing.raw_attributes. Без harvester'а нечего backfill'ить — пропущен. Промоушен `text_array` создаст пустую колонку.
4. **Одинокий backend deploy без frontend** — deploy curator-backend без UI работает только через curl. Frontend nginx Dockerfile есть — но Railway service для него надо настроить отдельно (вне scope этого commit'а).
5. **bcrypt для login + sha256 для sessions** — login (~50ms на bcrypt cost 10) умышленно медленный для anti-brute. Sessions deterministic sha256 для O(1) lookup на каждом запросе.
6. **Email→user** lookup идёт по plaintext email с lowercase. Email enumeration через timing attack теоретически возможен. На MVP с одним curator'ом не критично.

## Next

M12 — audit log wiring в admin endpoints (productsHandler updates → LogHuman, api_keys CRUD → LogHuman, junk classify → LogHuman, categories CRUD → LogHuman). Adapter+port в admin уже готовы — wire'ить осталось.
