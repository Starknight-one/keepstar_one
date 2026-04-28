# Catalog Completion — Phase D3 (applier+revert+endpoints) + Phase 4 (curator UI MVP)

- **Branch:** `main`
- **Date (UTC):** 2026-04-29
- **Parent commit:** `c6288df` (Phase D2)
- **Latest commit:** `6f31393` (Phase D3 backend) + UI commit pending after this log

## Context

Заkriрытие двух фаз каталога в одной сессии: **D3** — destructive applier + revert + curator HTTP endpoints (под backend); **Phase 4** — curator UI MVP под D3 backend (MergeReportPage + Reports tab). До этой сессии каталог был "виден но не управляем" — D2 умел писать read-only отчёт `merge_reports`, но ни одной фичи "одобрить и применить" не существовало ни в backend, ни в UI.

Цель: полноценный flow «прогон агента → ревью proposals → approve subset с edit-and-approve → apply → видим master changes / при ошибке revert». Backend и UI.

## What landed

### Phase 1 — smoke test D2 (20 мин)

- DELETE bad `merge_report` (id=1, 999 листингов от первого прогона до already_linked fix). Predicates `id=1 AND total_listings=999 AND status='pending'` как safeguard.
- `cmd/run-merge-apply -shop keepstar-neaqpan1.myshopify.com` на dev-store: `999 total / 979 already_linked / 16 new_master / 4 skip / 0 auto_link`. Already_linked gate работает, heybabes не попал в new_master proposals.

### Phase 2 — D3 backend (commit `6f31393`)

**Domain / ports:**
- `domain/audit.go` — `AuditActionMergeRevert`.
- `ports/audit_port.go` — `LogCurator(actor_kind='curator')` метод; реализация в `audit_adapter.go`.
- `ports/merge_apply_tx_port.go` — новый порт с 4 методами (`ApplyNewMaster` / `ApplyLinkExisting` / `ApplyVariantOfExisting` / `RestoreListingLink`). Каждый метод инкапсулирует одну транзакцию, не выводит `pgx.Tx` в use case → hexagonal изоляция сохранена.

**Adapter:**
- `adapters/postgres/merge_apply_tx_adapter.go` — реализация порта. `ApplyNewMaster` использует ON CONFLICT (sku) DO UPDATE с `tier2 = tier2 || EXCLUDED.tier2` для idempotency. SKU генерируется как UUID при отсутствии (master_products.sku NOT NULL).

**Use case:**
- `usecases/merge_apply_d3.go` — `ApplyProposals(ctx, ApplyProposalsRequest)` + `Revert(ctx, reportID, actorID)`.
- `ApplyProposalsRequest.Edits map[string]ProposalEdit` — курратор может переопределить ProposedMaster / FieldDecisions / TargetIDs перед apply (edit-and-approve).
- `RollbackData` snapshot pre-state ДО write (listing_master_*_id_before + created_master_*_id для new_master). Revert восстанавливает FK; created masters intentionally orphaned.
- `WithApplyTx(tx)` / `WithAudit(audit)` builders — оба опциональны; nil-safe для тестов.

**Endpoints (admin-backend):**
- `POST /admin/api/internal/curator/tenants/:id/merge/run`
- `GET  /admin/api/internal/curator/tenants/:id/merge-reports`
- `GET  /admin/api/internal/curator/merge-reports/:id`
- `POST /admin/api/internal/curator/merge-reports/:id/apply`
- `POST /admin/api/internal/curator/merge-reports/:id/revert`

Защищены `InternalKeyMiddleware` через `X-Internal-Key` header. Empty `CURATOR_INTERNAL_KEY` → 503 (defense-in-depth — забытый env не открывает endpoints молча).

**Curator backend proxy:**
- `curator/backend/internal/handlers/handler_merge.go` — `MergeProxy` форвардит `/curator/merge/*` запросы в admin-backend с X-Internal-Key + X-Curator-User (из session middleware).

**Tests:**
- `usecases/merge_apply_test.go` — 5 unit-тестов с in-memory `fakeCatalogPort` / `fakeReportsPort` / `fakeApplyTxPort`. Кейсы: NewMaster_CreatesAndLinks / LinkExisting_UpdatesListingOnly / LinkExisting_WithEdits_PropagatesFieldDecisions / AlreadyLinked_NoOp / Revert_RestoresListingLinks_KeepsCreatedMasters. Все pass.

### Phase 3 — design сессия в Pencil (артефакт `pencil-new.pen`)

Дизайн curator UI разобран с пользователем. Финальные решения:

- **Accent: sky #0EA5E9** (single attention-grabber, не путается со status). Юзер отверг violet.
- **Status palette:** good #10B981, review #E5A05C, blocked #EF4444. Цвет несёт смысл, не декорацию.
- **Anatomy carousel** для Candidates: header (что+откуда+confidence dot) → body (samples + AGENT SUGGESTS блок с эффектом) → footer (primary action в цвете-смысле).
- **Дерево catalog с inheritance**: атрибуты привязаны к узлам; common attributes на vertical/parent наследуются вниз; sub-attributes на ветках имеют кнопку "promote up" — поднимает атрибут в parent (юзер кейс: diameter из gvozdi/dreli/shurupoverty в parent tools).
- **AGENT SUGGESTS — не новый агент**, а composition: D1 discovery artifact + детерминистические эвристики (regex/string-similarity) + опциональный Haiku-on-demand для "explain this candidate".
- **Lazy loading везде** — список candidates / tree paginated by node, samples on hover.

Дизайн-сессия закрыта по запросу юзера ("этой правки хватит, в процессе использования станет понятно что и как допиливать").

### Phase 4 — curator UI MVP

**New files:**
- `curator/frontend/src/styles/tokens.css` — design tokens (sky accent, semantic status colors, spacing/type/radius scales).
- `curator/frontend/src/styles/merge.css` — стили для MergeReportPage (action chips, confidence dots, status badges, expandable proposal cards, per-field decision tables).
- `curator/frontend/src/pages/MergeReportPage.jsx` — главная новая страница:
  - Header: tenant breadcrumb + title + status badge + counters (auto-link / new master / needs review / skip / already linked) с цветами.
  - Toolbar: filter by action / confidence + search + "Select all high-confidence" + "Apply selected (N)" / "Revert report" (на applied state).
  - Per-proposal expandable row: action chip (`NEW MASTER` / `LINK` / `VARIANT` / `REVIEW` / `SKIP` / `LINKED`) + listing name + target summary + confidence dot + status pill (applied/failed). Click row → expand.
  - Proposal detail (expanded): evidence line + ProposedMaster form для new_master (editable name/brand/vertical/description/tier2) + Link target для link_existing + Per-field decisions table с editable action dropdown per row (inherit / propagate_to_master / keep_master / override_listing / skip) + collapsible Rollback snapshot.

**Edit-and-approve UX:** локальный state `edits[proposalId]` сливается с saved proposal в `mergeProposalAndEdit`, передаётся в `apply` request как `Edits map[proposalId]ProposalEdit`. Backend применяет overrides поверх saved proposal (write `WithEdits` test pass'ит).

**Edited files:**
- `curator/frontend/src/api.js` — добавлен `mergeApi.{run,list,get,apply,revert}`.
- `curator/frontend/src/main.jsx` — import tokens.css и merge.css.
- `curator/frontend/src/App.jsx` — route `/tenants/:tenantId/merge-reports/:reportId`.
- `curator/frontend/src/pages/TenantDetailPage.jsx` — Reports tab активен: кнопка "Run merge agent" (POST /merge/run → redirect на page) + таблица prior reports со счётчиками + Open ссылка.

**Verification:**
- Backend: `go build ./... && go vet ./... && go test ./internal/...` clean в обоих модулях (admin + curator).
- Frontend: `npm run build` clean (267 kB JS / 16 kB CSS gzipped to 82+3.7 kB).

## Files changed

| Scope | File | Action |
|---|---|---|
| domain | `internal/domain/audit.go` | EDIT — `AuditActionMergeRevert` |
| ports | `internal/ports/audit_port.go` | EDIT — `LogCurator` |
| ports | `internal/ports/merge_apply_tx_port.go` | NEW (~50 lines) |
| adapter | `internal/adapters/postgres/audit_adapter.go` | EDIT — `LogCurator` impl |
| adapter | `internal/adapters/postgres/merge_apply_tx_adapter.go` | NEW (~240 lines) |
| usecase | `internal/usecases/merge_apply.go` | EDIT — `WithApplyTx`/`WithAudit` builders + UUID stamp on proposal |
| usecase | `internal/usecases/merge_apply_d3.go` | NEW (~330 lines) |
| usecase | `internal/usecases/merge_apply_test.go` | NEW (~310 lines, 5 tests) |
| handler | `internal/handlers/handler_curator_merge.go` | NEW (~200 lines) |
| handler | `internal/handlers/middleware_internal_key.go` | NEW (~30 lines) |
| server | `cmd/server/main.go` | EDIT — DI for masterVariants/mappingArtifact/mergeReports/mergeApplyTx + handler routes |
| curator be | `internal/handlers/handler_merge.go` | NEW (~140 lines proxy) |
| curator be | `cmd/server/main.go` | EDIT — proxy wiring + routes |
| curator fe | `src/styles/tokens.css` | NEW |
| curator fe | `src/styles/merge.css` | NEW |
| curator fe | `src/pages/MergeReportPage.jsx` | NEW (~360 lines) |
| curator fe | `src/api.js` | EDIT — `mergeApi` |
| curator fe | `src/main.jsx` | EDIT — css imports |
| curator fe | `src/App.jsx` | EDIT — route |
| curator fe | `src/pages/TenantDetailPage.jsx` | EDIT — active Reports tab |

## Verification — local

1. `cd project_admin/backend && go test ./internal/...` — pass.
2. `cd curator/backend && go build ./...` — clean.
3. `cd curator/frontend && npm run build` — clean.
4. Прод-БД: bad merge_report id=1 удалён; fresh report id=2 (status=pending, total=999, new_master=16, already_linked=979) лежит в `catalog.merge_reports`.

**Не сделано (E1 на проде):** живой smoke прогон через UI на dev-store (Run agent → review → approve subset → verify SQL master_products / catalog.products.master_*_id → revert → verify FK reset). Делается вручную после деплоя — отдельная сессия.

## Known gaps / next

1. **E1 e2e на dev-store** — реальный прогон через UI на свежем тенанте после reinstall. Включая edit-and-approve хотя бы на одном proposal и revert.
2. **`CURATOR_INTERNAL_KEY` не в env** — нужно сгенерить и добавить в `.env` admin + curator перед smoke. Сейчас 503 при попытке использовать proxy.
3. **Сохранить `pencil-new.pen` в репо** — артефакт дизайн-сессии не сохранён в `design/curator.pen`. Юзер сделает вручную из Pencil app, либо потом скопируем.
4. **Tier-2 transforms** в `extractTier2` всё ещё 1:1 — `FieldMappingTarget.transform` (ml_from_string и т.д.) не применяются при apply. Низкий приоритет — данные пишутся, just unparsed.
5. **Lazy loading** в MergeReportPage не реализован — proposals JSONB загружается целиком (на 999 листингов получается ~200KB JSON). Виртуализация списка proposals + range-query для proposals — отдельная задача когда у тенанта станет 10k+ листингов.
6. **Candidates redesign** (дерево с inheritance) — спроектирован в Pencil, но **не имплементирован** в коде. CandidatesPage/JunkPage остались в старом виде. Юзер согласился что это можно итерировать после реального использования.
7. **MergeReportPage UX полировка** — alert() на success/failure (вместо toast), нет skeleton loaders, no virtualization. Достаточно для MVP, не для прода с 50+ тенантов.
8. **Phase B refactor** (shared `pkg/catalog`, переписать `PromoteAttribute` через `master_field_definitions` вместо ALTER TABLE) — отложено явно, не блокирует D3/Phase 4.

## Plan-mode rule note

Сессия началась через plan mode (`/Users/starknight/.claude/plans/async-tumbling-wall.md`). Этот документ — обязательный update log по правилу из `CLAUDE.md`.
