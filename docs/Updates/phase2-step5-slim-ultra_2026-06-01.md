# Phase 2 · Step 5 — Slim ultra to the V5 engine core

**Branch:** `phase2/step5-slim-ultra` (FF-merged to `main`)
**Date:** 2026-06-01
**Status:** done + verified.

## Context
Final step of Phase 2 (`/PHASE2_DECOMPOSE_ULTRA.md`). All four services now run from their
own repos (Steps 2–4: `keepstar-landing`, `keepstar-curator`, `keepstar-admin` — all deployed
and owner-verified). This step removes their now-redundant copies from the `ultra` monorepo,
leaving `ultra` = the V5 engine core.

## Changes (~564 files removed)
- **DELETED from ultra** (now live in their own repos): `project_admin/`, `curator/`, `Keepstar_one_landing/`.
- **DELETED stale:** `demo/` (superseded by `project_v5/frontend` + `scripts/v5-smoke.sh`), `tests/` (pre-v5 Python E2E, no CI).
- **ADW slimmed:** deleted `sdlc.go` + `go.mod`/`go.sum` + `specs/` (dead pre-v5 Go module). KEPT `ADW/dev-inspector` (used by `scripts/start.sh:14`) + `adw.yaml`(`.template`).
- **scripts:** `start_all.sh` (dropped the `start_admin.sh` call) + `stop_all.sh` (dropped admin/curator port kills) → V5-only. Deleted `scripts/start_admin.sh`.
- **Docs:** `CLAUDE.md` + `README.md` groomed to V5-only — stripped admin/curator/catalog routing + dev-essentials rows; fixed the stale `project_v4/.env` pointer → `project_v5/.env`.

## Verification
- `project_v5/backend`: `go build ./...` green; `go test -run TestEnginePurity` → `internal/engine` PASS.
- V5 is a hermetic module (zero imports of the deleted dirs — only comment references), so removal doesn't touch its build.
- `v4-engine` Railway service already deleted by owner (during Step 2).

## NOT done — deferred post-carve backlog (now lives in the extracted repos)
- Remove the deferred old-PIM ingest **chain** (`discovery_v2`/`apply_v2`/orchestrator/`ingest_shopify`/`ingest_csv`/classify/`mapping_artifact`/B2 `schema_drift`/`inbox` + `candidates`/`handler_junk`) and resolve the **Shopify-OAuth fork** (full cut / rewire to seam / keep) — it is welded to Shopify.
- Migrations: add `catalog.schema_version` (+ v5/curator startup gate); add `CREATE catalog.field_definitions` (confirmed MISSING in prod) or retire it; drop the empty `merge_reports` table + remove its migration + prune `integrations_wipe`.
- Drop KeepstarCanvas (tldraw) from the admin SPA.
- Seam (`pim-to-catalog`) automation (Railway scheduled job) + circuit-breaker on the Price-Stock `/offers` call.
- Strip Curator's PIM-overlap (keep owner cockpit, drop approvals/sync-now).

These are iteration items in `keepstar-admin` / `keepstar-curator`, not blockers of the carve.
