# Design System — Phase 1 (runtime theme keystone): committed + deployed

**Date:** 2026-06-22
**Branch:** `feat/design-system-phase1-theme` → FF-merged to `main`
**Commit:** `e736765` (code), docs commit follows
**Plan:** `DESIGN_SYSTEM_PLAN.md` (Phase 1)

## Context

Phase 1 of the design system (per-tenant runtime theme — the "keystone")
was *built and self-verified on 2026-06-16* but had been left **uncommitted
and undeployed** on `main`, sitting as a working-tree diff awaiting the
owner's explicit "ok" to deploy. Owner gave the go-ahead. This session
committed and deployed it.

## Approach

Followed the plan's resume steps (`DESIGN_SYSTEM_PLAN.md` §"Точка возобновления")
and branch-discipline:

1. Confirmed the boot migration is safe (no repeat of the migration #23
   1600-column outage): `theme_migrations.go` is `CREATE TABLE IF NOT EXISTS
   v5_themes` — a standalone v5-owned table, does NOT touch `master_products`,
   is unrelated to the `catalog.schema_version` gate, registered in the boot
   migration slice (`main.go:57 {"theme", pgClient.RunThemeMigrations}`),
   fail-loud.
2. Verified green locally BEFORE committing (see Verification).
3. Scoped the commit to `project_v5` only. Per the plan, the
   `keepstar-admin/translate.go` variables/themes passthrough is **part of
   Phase 2** and was deliberately left uncommitted.
4. Branch → commit `e736765` → `git merge --ff-only` to `main` → `git push
   origin main` → Railway auto-deploy via GitHub webhook.

## Files changed (commit e736765 — 22 files, +1576/-17)

**Backend (`project_v5/backend`):** new `internal/domain/theme.go` (+`_test`),
`internal/adapters/postgres/{theme_migrations.go, postgres_theme.go,
postgres_theme_integration_test.go}`, `internal/ports/theme_port.go`,
`internal/handlers/{handler_theme.go, handler_theme_test.go}`; modified
`cmd/server/main.go` (migration + themePort wiring), `handlers/routes.go`
(GET/PUT `/api/v1/internal/theme`), `handler_navigation.go` (attach theme on
nav expand/back/filter — adversarial fix), `handler_pipeline.go`,
`handler_pipeline_stream.go`, `handler_presets.go` (+tests).

**Frontend (`project_v5/frontend`):** new `src/theme-style.js`
(buildThemeCss/applyTheme/tokensFromDoc) + `tests/theme-style.test.jsx`;
modified `src/{widget.jsx, WidgetApp.jsx, widget-preview.jsx, widget.css}`.

## Verification

- Backend: `go build ./...` clean, `go vet ./...` clean, `go test ./...` —
  all packages `ok`.
- Frontend: `vitest run` — **102/102** passed (11 files); `vite build` —
  `dist/widget.js` 237.33 kB / 74.29 kB gz.
- **Deploy on prod confirmed** (Railway MCP token was expired at the time, so
  verified out-of-band against `v5-engine-production.up.railway.app`):
  - net-new `GET /api/v1/internal/theme` → **403** (route exists, missing key)
    where it returned 404 before the deploy;
  - a bogus path under the same `/api/v1/internal/` prefix → **404**
    (proves the router matches routes; the key-middleware does not blanket-403
    the prefix), so the 403 on `theme` means the route is registered in the
    running binary = new code is live;
  - service responds without 502 ⇒ boot succeeded ⇒ the fail-loud
    `v5_themes` boot migration applied (else it would crash-loop).

## Known gaps / not done

- **Behavioral live-smoke NOT run:** the keystone proof (`PUT theme` with
  `{"colors":{"blue":"#FF0000"}}` → widget blue accents turn red) requires the
  `X-Internal-Key` (`V5_INTERNAL_KEY`), which was not available this session.
  Deploy-existence and boot-migration are proven; end-to-end override behavior
  on prod is not yet independently demonstrated.
- **Railway MCP token expired** (`Unauthorized. Please run railway login
  again`) — `list_projects` works but all project-scoped calls (services,
  deployments, logs) fail. Re-login needed for direct build/deploy-log
  visibility. Deploy was confirmed without it (above).
- `keepstar-admin/translate.go` variables/themes passthrough remains
  uncommitted — intentional, it lands with Phase 2.
- Phase 1 debt carried forward: no dedicated nav-theme regression test;
  `translate.go` passthrough is shape-blind (add form validation in Phase 2);
  byte-identical drift guard between `widget.css:16-46` and
  `DefaultThemeTokens()`.

## Next

Phases 2–5 are unstarted with grounded file:line sub-specs in
`DESIGN_SYSTEM_PLAN.md`. Immediate next = the live keystone smoke (needs the
internal key), then Phase 2 (tokens flow from canvas → tenant theme).
