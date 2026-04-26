# Admin Catalog — M12 Audit log integration

- **Branch:** `main`
- **Date (UTC):** 2026-04-26 14:15
- **Parent:** `0c70871` (M11 curator standalone)

## Context

`audit_adapter.go`, `AuditPort`, `AuditEntry`, `FieldChange` уже готовы (M2), таблица `catalog.audit_log` создана (M1). Нужно было:
1. Wire `auditAdapter` в admin DI.
2. Внедрить `LogHuman` calls в endpoints, которые меняют состояние (productsHandler.HandleUpdate, junkHandler.HandleClassify, apiKeysHandler.HandleCreate/Revoke, categoriesHandler.HandleCreateTenant/HandleDeleteTenant).
3. Добавить `GET /admin/api/audit?entity_kind=&entity_id=` для frontend.
4. Добавить History секцию на ProductDetailPage.

Curator service (M11) уже пишет audit при `PromoteAttribute` через свой adapter — теперь admin тоже пишет, и оба попадают в одну таблицу `catalog.audit_log` → curator-frontend `/audit` показывает unified ленту.

## Approach

### Optional audit DI

Все handler'ы получили `audit ports.AuditPort` как опциональное поле (nil = no-op). Это позволяет:
- Сохранять старую сигнатуру тестов с `nil`-audit
- Postpone wiring если audit-таблица недоступна (например при локальной разработке без неё)

В production DI `auditAdapter` всегда non-nil — пробрасываем его в каждый handler.

### Field-level diff для product update

`ProductsHandler.HandleUpdate`:
1. Snapshot `before` через `Get` (best-effort — если падает, audit получает one-sided info с только новыми значениями)
2. `Update` как раньше
3. `diffProductUpdate(before, update)` сравнивает only the touched fields (DisplayName, Price, Stock, Rating, Description) и пишет map[string]FieldChange{"price": {Old: 1290, New: 1500}}
4. Если diff пустой — audit запись не пишется (no-op edits не загрязняют ленту)

Помежуточная race window между Get и Update: возможна, но для single-tenant single-user практически не наблюдается. Допускаем.

### Junk classify

Простой diff: `{"classification": {Old: "pending", New: "confirmed_addon"}}`. Action `classify` (новая константа `domain.AuditActionClassify`).

### API key create/revoke

Create: `{"label": {New: "production"}}` — Old отсутствует (это create).
Revoke: `nil` field changes (action=delete достаточно).
EntityKind = `domain.EntityKindAPIKey` (новая константа).

### Categories

Create — `{"name": {New}, "kind": {New}}`. Delete — nil.
PATCH тенант-категории не отслеживается (сложно diff'ить через текущий upsert flow — TODO для M4 polish).

### Read endpoint

`GET /admin/api/audit?entity_kind=listing&entity_id={id}&limit=50&offset=0` — обязательные `entity_kind + entity_id`. Защищает от случайного скачивания всего лога.

### Frontend History

Секция вставлена на `ProductDetailPage` между "Categories" и "Master fields". Загружается параллельно с продуктом. Empty state когда записей нет. Каждая запись:
- `{date} · {actor_kind} · {action}` 
- Chip-строка с per-field diff'ом (`price: 1290 → 1500`)

## Files changed

| Scope | File | Change |
|---|---|---|
| domain | `domain/audit.go` | +EntityKindAPIKey, +AuditActionClassify |
| handler | `handlers/handler_audit.go` | NEW (GET /admin/api/audit) |
| handler | `handlers/handler_products.go` | +audit field, snapshot+diff в HandleUpdate, helper diffProductUpdate |
| handler | `handlers/handler_junk.go` | +audit field, LogHuman после classify |
| handler | `handlers/handler_api_keys.go` | +audit field, LogHuman на create + revoke |
| handler | `handlers/handler_categories.go` | +audit field, LogHuman на create + delete |
| DI | `cmd/server/main.go` | +auditAdapter, +auditHandler, audit прокинут во все handlers, route /admin/api/audit |
| frontend | `src/features/catalog/ProductDetailPage.jsx` | +history fetch + History секция |
| frontend | `src/features/catalog/productDetail.css` | +.pd-history стили |

## Verification

- `cd project_admin/backend && go build/vet/test ./...` — clean
- `cd project_admin/frontend && npm run build` — clean
- Smoke (после деплоя):
  - Edit price в ProductDetailPage → Save → SQL `SELECT * FROM catalog.audit_log WHERE entity_kind='listing' ORDER BY id DESC LIMIT 1` → запись с `field_changes={"price":{"old":1290,"new":1500}}`
  - Перегрузить страницу → секция History показывает chip "price: 1290 → 1500"
  - Generate API key → `SELECT * FROM audit_log WHERE entity_kind='api_key'` → запись `action=create, field_changes={"label":{"new":"prod"}}`
  - Revoke key → запись `action=delete, field_changes=null`
  - Classify junk record → запись `entity_kind=candidate, action=classify, field_changes={"classification":{"old":"pending","new":"confirmed_addon"}}`
  - Curator UI `/audit` → показывает все эти записи (плюс curator's promote actions из M11)

## Known gaps

1. **PATCH tenant_category** не пишет audit — текущий upsert helper не отдаёт before-state. Чтобы зафиксировать diff, нужен явный `UpdateTenantCategory(id, ...)` метод на port. Отложим до M4 polish.
2. **Mapping changes** (POST `/admin/api/categories/mapping`) audit не пишет — это N:1 присваивание, фиксировать `{master_category_id: {old, new}}` требует pre-read. Отложено.
3. **Listing M:N category links** на `PUT /admin/api/products/{id}/categories` — `LinkListingToCategories` атомарно перезаписывает, и audit мог бы зафиксировать diff `{category_ids: {old: [...], new: [...]}}`, но снова — нужен pre-read. Откложено.
4. **Public API v1 PATCH** через X-API-Key пишет audit как `actor_kind=user` (через `UserID(ctx)` который пуст). Нужна отдельная ветка `actor_kind=api, actor_id=apiKeyID`. Сейчас audit на api-routes не пишется (М10 endpoints не получили audit field — отложено в M4 polish).
5. **Race condition в snapshot+update** для product update — окно между Get и Update. На single-user low-write workload не проблема; решается явным RETURNING или pgx batch tx, отложено.
6. **Pagination на /admin/api/audit** — limit + offset поддерживается, но без index'а на `(entity_kind, entity_id, created_at DESC)` запросы будут медленны при большом логе. Index уже создан в M1 миграциях, проверять не надо.

## Сessions complete

Этим милстоном завершается план M6 → M8 → M9 → M10 → M11 → M12. Все изменения, которые **не зависят от harvester'а (M4d)** и от **heybabes backfill (M7)** — закрыты. Сегмент готов к деплою и пользовательскому тестированию.

Что осталось до полного релиза каталог-модуля:
- **M4d** (financial polish сессия) — harvester orchestrator, embedding job, hash-diff webhooks, cut-over legacy importer, frontend progress UI на ShopifyConnectPage. Ожидает focused-сессии с пользователем.
- **M7** (heybabes 967 backfill) — пересмотр кривых русских названий, потом пишется однократный скрипт.
- **Polish из known gaps по каждому milestone** — собрано в логах сессий.
