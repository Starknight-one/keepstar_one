# Catalog — live smoke on dev-store + 5 bugs surfaced + 1 hot-fix

- **Branch:** `main`
- **Date (UTC):** 2026-04-29 02:30
- **Parent commit:** `42587f6`
- **Latest commit:** `1db672d` (revert hot-fix)

## Context

Phase A (deploy-readiness) совмещён с Phase A.3 (live smoke). До этой сессии каталог был "написан и собран", но никто не прошёл цепочку end-to-end на проде. Прогон выявил 5 багов разной серьёзности; 1 из них (revert SQL cast) пофикшен горячим коммитом, остальные 4 в backlog.

## Что сделано

### A.0 — "Wipe & disconnect" в Admin UI (commit `c32df2b`)

`Disconnect` чистил только OAuth-row, оставляя 999 listings + raw_imports + reports. Reinstall сразу видел "already_linked" мусор, недопустимо. Добавлен `WipeAndDisconnect`: один tx удаляет catalog.products (только source=integration.kind), shopify_raw_imports, merge_reports, tenant_categories, tenant_catalog_schema + сам integration row. Master_products preserved (orphan-safe). Endpoint POST `/admin/api/integrations/{id}/wipe`. UI: вторая красная ссылка с двойным confirm.

### A.1 — Curator на Railway (`https://curator-production-46d7.up.railway.app`)

Empty service Curator в проекте `selfless-tranquility`, region `us-east4-eqdc4a` (как Chat/Admin). Source: `Starknight-one/keepstar_one`, branch main, root directory `/curator`, builder Dockerfile. Один контейнер обслуживает SPA + API на одном порту (Dockerfile mirroring project_admin). Доменное имя сгенерировано через `railway domain`.

### A.2 — `CURATOR_INTERNAL_KEY` в обоих сервисах

Сгенерирован openssl rand -hex 32, прописан в Curator (через `railway add -v`) и Admin (через `railway variable set`). Verified: `curl https://admin/.../merge-reports/2 -H "X-Internal-Key: ..."` → 200 + полный отчёт. Цепочка curator-frontend → curator-backend (proxy) → admin-backend (internal key) — рабочая.

### Discovery trigger в curator UI (commit `ab78d60`)

До коммита курратор запускал discovery только из CLI. Добавлен POST `/admin/api/internal/curator/tenants/{id}/discover` + curator-backend proxy + кнопка "Run discovery agent (~$0.40)" на Schema tab.

### A.3 — Live smoke на dev-store (через прод UI)

Цепочка пройдена:
1. Wipe dev-store через Admin → SQL: 20 listings / 24 raw imports / 1 report / 5 categories / 1 schema удалены, 979 heybabes products + master_products preserved ✓
2. Reinstall Shopify app → tenant_integration created, harvester_lite заполнил 20 products полными raw_attributes (variants 20, metafields 9, collections 16), tenant_categories 4 real + 1 showcase, master_attribute_candidates 17 новых по 3 vertical ✓
3. Run discovery agent (~$0.40, 432k tokens, 20 turns, 145s): MappingArtifact с 24 field_mapping, 18 brand_mapping, 10 category_mapping, 2 master_templates (furniture+footwear), 3 junk vendors, match_strategy_config ✓
4. Run merge agent: report #3 — 999 listings, 16 new_master, 4 skip (junk), 979 already_linked ✓
5. Apply 1 proposal (Lavender Hand Cream → master_product `78a661f1...` + master_variant `19287814...`, listing залинкован, audit_log actor_kind='curator' action='merge') ✓
6. Revert — **первый раз сломан** (см. ниже), пофикшен hot-commit'ом

### Hot-fix `1db672d` — revert SQL cast + честный final status

Live smoke выявил два бага в `RestoreListingLink`:
1. `NULLIF($1,'')` возвращает text, master_*_id это uuid → pgx 42804 SQLSTATE на каждом revert. Добавлены явные `::uuid` cast'ы на всех listing FK writes (Apply* и Restore*).
2. `Revert` помечал report `status='reverted'` независимо от того, сколько proposals реально откатилось. Сейчас финальный статус отражает реальность: `reverted` (все ОК), `partial` (часть восстановлена, часть упала), `applied` (все Restore упали — не лжём про cleanup). Возвращается ошибка если есть failures.

## Files changed

| File | Action |
|---|---|
| `project_admin/backend/internal/usecases/integrations_wipe.go` | NEW |
| `project_admin/backend/internal/handlers/handler_integrations.go` | EDIT — `HandleWipe` |
| `project_admin/backend/internal/handlers/handler_curator_merge.go` | EDIT — `HandleDiscover` |
| `project_admin/backend/internal/adapters/postgres/merge_apply_tx_adapter.go` | EDIT — `::uuid` casts |
| `project_admin/backend/internal/usecases/merge_apply_d3.go` | EDIT — Revert final status logic |
| `project_admin/backend/cmd/server/main.go` | EDIT — wiring |
| `project_admin/frontend/src/features/integrations/IntegrationsPage.jsx` | EDIT — Wipe button |
| `curator/Dockerfile` | NEW |
| `curator/backend/cmd/server/main.go` | EDIT — PORT env, /discover route |
| `curator/backend/internal/handlers/handler_merge.go` | EDIT — `HandleDiscover` |
| `curator/frontend/src/api.js` | EDIT — `mergeApi.discover` |
| `curator/frontend/src/pages/TenantDetailPage.jsx` | EDIT — discovery button on Schema tab |

## Known gaps (in backlog)

| # | Severity | Issue |
|---|---|---|
| 14 | medium | products в Admin catalog UI показываются пустыми после reinstall — read-path не разворачивает raw_attributes; БД полная, листинги работают, проблема только в admin рендере |
| 15 | medium | proposed_master.vertical='unknown' для всех new_master — `resolveVertical` fallthrough; brand_mapping action=create_new не несёт vertical hint. Fix: расширить BrandMappingTarget + prompt |
| 16 | high | proposed_master.tier2 пустой — discovery agent prompt описывает M3 typed-column schema (`master_cosmetics.X`), а D-серия использует `master_products.tier2 JSONB`. Нужно либо обновить prompt на новые targets, либо добавить translation в `extractTier2` |
| 17 | low | gtins не извлекаются у тестовых dev-store продуктов — возможно особенность seed-данных без UPC |

## Что фактически работает после этой сессии

- ✓ Wipe → reinstall → harvester_lite (20 products, raw_attrs full, candidates flow живой)
- ✓ Discovery agent ($0.40 per tenant, не зависит от каталога) → MappingArtifact с правильной структурой
- ✓ Merge agent (deterministic, ~20 секунд) → корректные proposals по всем actions (already_linked / new_master / skip / link_existing)
- ✓ Apply: master_products + master_variants создаются, listing залинкован, audit_log пишет
- ✓ Revert (после hot-fix `1db672d`): listing FK сбрасывается, master orphan'ится, audit_log пишет merge_revert

## Что НЕ протестировано в этой сессии

- Apply на subset > 1 proposal с edit-and-approve (per-field decision dropdown)
- Re-running merge agent после apply (report supersession + already_linked detection)
- V4 chat на dev-store с залинкованными master_products
- Webhook update от Shopify после initial sync
- Concurrent apply (две вкладки)

## Plan-mode log note

Сессия началась через plan mode (`async-tumbling-wall.md`). Этот документ закрывает обязательство про update log.

## Reality check

Каталог shipped в смысле "минимальный happy-path работает". Это **не** "мы готовы подключать клиентов". 4 бэклог-бага и невыполненные тесты выше — список перед реальным тенантом. Юзер сам отметил: "работает основная часть, с грехом пополам и через пень колоду одновременно. Но это малая часть, а нам нужно кучу тестов проделать по хорошему".
