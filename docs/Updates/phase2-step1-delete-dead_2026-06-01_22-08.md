# Phase 2 · Step 1 — Delete genuinely-dead legacy ingest (DEFER scope)

**Branch:** `phase2/step1-delete-dead-legacy`
**Date:** 2026-06-01 22-08
**Status:** done + verified; committed to the branch, NOT merged to main, NOT pushed/deployed.

## Context
Phase 2 decomposition (see `/PHASE2_DECOMPOSE_ULTRA.md`). Step 0 (pre-flight) proved against
prod flat-moon (`flat-moon-68826275`/`neondb`, 34 tenants): the dead-set tables are empty,
`dailySyncEnabled` is off on all 34 tenants (cron inert), and the old Shopify→apply_v2 ingest
path is dormant (inbox last activity 2026-05-15, 17 days idle). Owner decision: **defer** the
LLM ingest chain + Shopify — they are welded together
(`ShopifyV2 → ShopifyIngester → UpdateOrchestrator → apply_v2/discovery_v2`) — to **Step 4**
(Admin extraction, when Connector Wave 4 can take over Shopify onboarding). Step 1 removes
ONLY the genuinely-dead, fully-isolated code.

## Approach
Mapped the chain with a 5-agent workflow, then a defer-mode re-verification (greps) split
truly-isolated dead code from code still wired into kept surfaces:
- `candidates` writer methods have no callers, BUT `CandidatesPort` is still consumed by
  `handler_junk` (curator junk-audit) → KEPT, defers with the chain.
- `merge_reports` adapter/port/domain have NO Go importers (migration + `integrations_wipe`
  touch only the table via raw SQL) → safe to delete the Go code.

## Files changed (−1635 lines, 9 files)
**Deleted:** `cmd/crawler/main.go`, `cmd/cleanup-tenant-stale/main.go`,
`internal/adapters/postgres/merge_reports_adapter.go`, `internal/domain/merge_report.go`,
`internal/ports/merge_reports_port.go`, `internal/usecases/cron_daily_source_pull.go`.
**Modified:** `cmd/server/main.go` (removed inert 24h-cron block + the now-unused
`catalogV2Orchestrator` forward-decl/assignment), `internal/adapters/postgres/catalog_adapter.go`
(removed `UpsertListingFromSource` — no callers), `internal/ports/catalog_port.go` (removed its
interface decl).

## Verification
- `go build ./...` ✅ · `go vet ./...` ✅
- `go test ./...` — all test files compile; only `TestScenario_052-055` fail, and **proven
  pre-existing** by stashing this change and running them on a clean tree (Shopify consent-gap
  red-tests, unrelated).
- Pure dead-code removal — behavior unchanged. **Pre-merge gate (before merging to main):**
  boot admin locally + run the v5 furniture smoke to confirm the widget still renders.

## Deliberately kept / deferred
- `candidates` (adapter/port/domain/tables) → `handler_junk` dependency; defers to Step 4.
- LLM ingest chain (`discovery_v2`/`apply_v2`/`orchestrator`/`ingest_shopify`/`ingest_csv`/
  `classify_vertical`/`match_key`/`mapping_artifact`/B2 `schema_drift`/`inbox`) → Step 4.
- `curator` untouched — its "Sync now" → admin `/catalog/v2/sync-now` still works.

## Known gaps / follow-ups (→ Step 4)
- **`merge_reports` EMPTY table NOT dropped.** Go code removed, but `catalog_migrations.go`
  still CREATEs it (~598-638) and `integrations_wipe.go` still DELETEs from it (works on the
  empty table). Drop table + remove migration + prune wipe at Step 4, bundled with adding
  `catalog.schema_version` + `CREATE catalog.field_definitions`.
- **`catalog.field_definitions` confirmed MISSING in prod** (only `master_field_definitions`
  exists) — v5 reads it and fail-opens to data-derived `<fields>`. Owner/CREATE it at Step 4.
- `shopify_raw_imports`: table exists in prod (0 rows) with no CREATE migration; left as-is
  (tied to deferred Shopify).
