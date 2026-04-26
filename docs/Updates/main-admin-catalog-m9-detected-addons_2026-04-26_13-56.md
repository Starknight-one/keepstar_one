# Admin Catalog — M9 Detected add-ons (junk triage UI)

- **Branch:** `main`
- **Date (UTC):** 2026-04-26 13:56
- **Parent:** `7887d02` (M8 categories)

## Context

`tenant_variant_candidates_junk` создан в M1. `candidates_adapter.go` реализован в M2 (`InsertJunkCandidate`, `ListJunkCandidates`, `ClassifyJunkCandidate`, `CountJunkPending`). Не было wiring в DI, handler'а и UI. M9 закрывает.

Очередь будет реально заполняться когда M4d harvester запустится — junk_detector.go (готов в M4b) сейчас вызывается только из тестов. До M4d страница рендерит empty state и sidebar-бэйдж не показывается.

## Approach

Backend `handler_junk.go`:
- `GET /admin/api/junk?status=pending` — список через `ListJunkCandidates`
- `GET /admin/api/junk/count` — pending count для sidebar бэйджа
- `POST /admin/api/junk/{id}/classify` `{classification}` — `ClassifyJunkCandidate`. Whitelist на `confirmed_addon | false_positive`. ClassifiedBy = userID из JWT.

DI: `candidatesAdapter := postgres.NewCandidatesAdapter(dbClient, log)` + `junkHandler := handlers.NewJunkHandler(candidatesAdapter, log)`. Routes под protected. `handler_junk.go` extractJunkID parse `/{id}/classify`.

Frontend `DetectedAddonsPage.jsx`:
- 3 tabs: Pending / Confirmed add-ons / False positives
- Empty-state с подсказкой что harvester заполнит
- На pending row две кнопки "Mark as add-on" / "Mark as real"; на остальных — meta (classifier + date)
- detectedReason JSONB рендерится chip'ами с человекочитаемыми лейблами (axis_name_pattern, no_identifiers, ...)

Sidebar бэйдж:
- DashboardLayout.jsx опрашивает `/junk/count` каждые 60s. Тихо игнорирует ошибки авторизации (на странице sign-in бэйдж не виден).
- При count > 0 — оранжевый бэйдж рядом с пунктом "Detected add-ons".

Routing:
- `/catalog/detected-addons` → `DetectedAddonsPage`
- Sidebar sub-link под "Catalog → Categories"

## Files changed

| Scope | File | Change |
|---|---|---|
| backend handler | `internal/handlers/handler_junk.go` | NEW |
| backend DI | `cmd/server/main.go` | +candidatesAdapter, +junkHandler, +routes |
| frontend page | `src/features/catalog/DetectedAddonsPage.jsx` | NEW |
| frontend css | `src/features/catalog/detectedAddons.css` | NEW |
| frontend route | `src/App.jsx` | +/catalog/detected-addons |
| frontend layout | `src/features/layout/DashboardLayout.jsx` | +junkCount poll, +sidebar sub-link с бэйджем |
| frontend css | `src/features/layout/layout.css` | +.sidebar-badge |

## Verification

- `cd project_admin/backend && go build/vet ./...` — clean
- `cd project_admin/frontend && npm run build` — clean (`built in 6.01s`)
- Smoke (после деплоя):
  - Без harvester'а: `/catalog/detected-addons` показывает empty state, бэйджа нет
  - Manual SQL `INSERT INTO catalog.tenant_variant_candidates_junk (tenant_id, listing_id, detected_reason) VALUES (...)` → страница его рендерит → нажать "Mark as add-on" → SQL `SELECT classification FROM ... WHERE id=...` → `confirmed_addon`. Sidebar count → 0.

## Known gaps

1. **Listing preview (image+name)** не реализован в карточке. Сейчас только `listing.id.slice(0,8)`. Чтобы показать продукт, нужно либо JOIN с products в `ListJunkCandidates`, либо отдельный fetch по listingID. Полного UI коммита спека требует превью; это TODO для M4 polish, когда будет реальный пользовательский трафик.
2. **Batch classify** в плане упомянут как placeholder (`/junk/batch-classify`) с TODO-комментарием на LLM. Endpoint не реализован — единственный сценарий пока ручной per-record. Это норм для пустой очереди.
3. **Audit log integration** — classify пишет в `tenant_variant_candidates_junk` напрямую через адаптер; вызова `auditPort.LogHuman` ещё нет. Это сделается в M12 (audit wiring), когда `auditAdapter` войдёт в DI.

## Next

M10 — public API + api_keys management.
