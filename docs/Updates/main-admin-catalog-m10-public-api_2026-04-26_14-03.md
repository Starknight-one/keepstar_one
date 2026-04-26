# Admin Catalog — M10 Public REST API + tenant API key management

- **Branch:** `main`
- **Date (UTC):** 2026-04-26 14:03
- **Parent:** `6aba054` (M9 detected add-ons)

## Context

`catalog.api_keys` создан в M1 (`id, tenant_id, key_hash, label, last_used_at, revoked_at, created_at`), но никаких domain types / port / adapter / handlers не было. Цель M10 — закрыть всё снизу вверх и подключить публичный REST API v1 для тенантов (read/update своего каталога через X-API-Key).

## Approach

### Token format

Plain text: `kp_<base64url(32 random bytes)>` ≈ 52 chars. Префикс `kp_` помогает grep'нуть утечки в логах.

### Lookup performance (key_prefix index)

Стандартный lookup был бы full-scan + bcrypt на каждой строке. Решение: дополнительная колонка `key_prefix TEXT` (первые 12 символов plain) + partial index `WHERE revoked_at IS NULL`. Verify path:
1. Header → берём первые 12 chars → `SELECT id, tenant_id, key_hash WHERE key_prefix = $1 AND revoked_at IS NULL`
2. По каждому match (обычно 1) — `bcrypt.CompareHashAndPassword`

При collision'ах prefix всё равно bcrypt дёрнется только пару раз. Cost 10 (~50ms) — приемлемо.

Migration `ALTER TABLE catalog.api_keys ADD COLUMN IF NOT EXISTS key_prefix TEXT` + index добавлены в `catalog_migrations.go::migrations`. Idempotent.

### Adapter: `bcrypt.GenerateFromPassword(plain, cost=10)` хранится в `key_hash`

`Create` возвращает `(domain.APIKey, plainText, error)` — caller обязан показать `plainText` пользователю один раз.

`Verify` возвращает `(tenantID, apiKeyID, error)`. Ошибка типа `domain.ErrAPIKeyInvalid` для всех miss'ов.

`TouchLastUsed` вызывается best-effort через goroutine из middleware'а — не блокирует request даже если БД медленная.

### Middleware

`APIKeyMiddleware` — отдельный from JWT `AuthMiddleware`. Reads `X-API-Key`, вызывает `Verify`, кладёт `tenantID + apiKeyID` в context (тот же ключ `ctxTenantID` что и JWT — handlers не знают какой auth method был). Дополнительный `ctxAPIKeyID` для audit (M12).

### Routes

Admin-protected (JWT) — CRUD api_keys:
- `GET /admin/api/api-keys` → список (без plainText, только metadata)
- `POST /admin/api/api-keys {label}` → создание, возвращает `plainKey` один раз
- `DELETE /admin/api/api-keys/{id}` → revoke (UPDATE SET revoked_at)

Public (X-API-Key):
- `GET /api/v1/products?limit=50&offset=0&search=&categoryId=` → page
- `GET /api/v1/products/{id}` → single
- `PATCH /api/v1/products/{id}` → tenant-only fields (displayName, description, price, stock, rating, rawAttributes) — переиспользует тот же `productsUC.Update`
- `DELETE /api/v1/products/{id}` → 501 (нет SoftDelete метода в ports.AdminCatalogPort, отложено в M4 polish)
- `POST /api/v1/products` → 501 (bulk push требует harvester pipeline, M4d)
- `GET/POST /api/v1/categories` + `PATCH/DELETE /api/v1/categories/{id}` — переиспользует `categories_v2_adapter`

API v1 routes сидят на отдельном sub-mux, обернутом `APIKeyMiddleware`. `AuthMiddleware` не применяется. Таким образом тот же admin сервер обслуживает оба пути без конфликта.

### Frontend

`/settings/api-keys` (новая страница, route добавлен в `App.jsx`):
- Таблица ключей: label / prefix / created / last used / status / actions
- "Generate key" — модалка с label, на success показывает `plainKey` в желтой плашке "Copy this now"
- Revoke per row (confirm dialog)
- При empty state — компактный empty block

В SettingsPage.jsx добавлена ссылка "API access →" наверху чтобы пользователи нашли страницу.

## Files changed

| Scope | File | Change |
|---|---|---|
| migrations | `catalog_migrations.go` | +ALTER TABLE api_keys ADD COLUMN key_prefix + indexed |
| domain | `domain/api_key.go` | NEW |
| domain | `domain/errors.go` | +ErrAPIKeyInvalid |
| ports | `ports/api_keys_port.go` | NEW |
| adapter | `adapters/postgres/api_keys_adapter.go` | NEW (bcrypt cost 10, prefix index lookup) |
| middleware | `handlers/middleware_apikey.go` | NEW |
| handler admin | `handlers/handler_api_keys.go` | NEW (Create/List/Revoke) |
| handler public | `handlers/handler_api_v1_products.go` | NEW (list/get/patch + 501 stubs) |
| handler public | `handlers/handler_api_v1_categories.go` | NEW |
| DI | `cmd/server/main.go` | +adapter + handlers + routes (admin + sub-mux for /api/v1) |
| frontend | `src/features/settings/ApiKeysPage.jsx` | NEW |
| frontend | `src/features/settings/apiKeys.css` | NEW |
| frontend | `src/features/settings/SettingsPage.jsx` | +link to /settings/api-keys |
| frontend | `src/App.jsx` | +/settings/api-keys route |
| docs | `docs/api/v1/openapi.yaml` | NEW (3.0.3 minimal) |

## Verification

- `cd project_admin/backend && go build/vet/test ./...` — clean
- `cd project_admin/frontend && npm run build` — `built in 3.57s`, no errors
- Smoke (после деплоя):
  - `curl -X POST -H "Authorization: Bearer $JWT" /admin/api/api-keys -d '{"label":"prod"}'` → 200 `{plainKey: "kp_..."}`
  - `curl -H "X-API-Key: kp_..." /api/v1/products?limit=5` → 200 + до 5 продуктов
  - `curl -H "X-API-Key: invalid" /api/v1/products` → 401
  - `curl -X PATCH -H "X-API-Key: kp_..." /api/v1/products/{id} -d '{"price": 1500}'` → 200; SQL подтверждает `price=1500`
  - `curl -H "X-API-Key: kp_..." /api/v1/categories` → 200 + список
  - В админке `/settings/api-keys` → table показывает ключ; revoke работает; ключ становится `revoked` и `/api/v1/...` с ним возвращает 401

## Known gaps

1. **Bulk push (`POST /api/v1/products`)** — 501 returned. Не реализован, потому что нужен полный harvester pipeline (match cascade, candidates, junk detection). Это M4d territory. Сегодня внешний клиент не может пушить новые товары — только править существующие через PATCH.
2. **DELETE на listing** — 501. `ports.AdminCatalogPort` не имеет публичного `SoftDelete(id)` метода (только `SoftDeleteProductBySource(tenantID, sourceSystem, sourceID)`). Добавим в M4 polish — пока внешние клиенты могут только PATCH.
3. **Cursor-based pagination** упоминалась в плане как `(updated_at, id)`-cursor. Я оставил offset-based — уровень MVP. Cursor добавим когда тенанты упрутся в 200-record limit и захотят стабильный sync.
4. **Rate limiting** — план упоминал "per tariff", но billing tariff'ов сейчас нет. Без RL легитимный тенант может задудосить себя — для MVP это OK.
5. **TouchLastUsed best-effort** — асинхронно через goroutine, без обработки ошибок. На high-load возможна потеря timestamps, но это metadata, не критично.
6. **Audit log** — Create/Revoke/PATCH/DELETE через api_key пока **не пишутся** в `audit_log`. Будет в M12.

## Next

M11 — curator standalone service (отдельный Go + Vite app + Railway deploy).
