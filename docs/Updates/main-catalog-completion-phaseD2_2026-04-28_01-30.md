# Catalog Completion — Phase D2 (deterministic applier, GenerateReport) shipped

- **Branch:** `main`
- **Date (UTC):** 2026-04-28 01:30
- **Parent commit:** `a97befb` (Phase D1, extended discovery agent)
- **Active plan:** `~/.claude/plans/synchronous-twirling-hoare.md`

## Context

After Phase D1 the discovery agent produces a full `MappingArtifact` (field/category/brand mapping + junk rules + match strategy config). D2 adds the deterministic applier: walks every `catalog.products` listing for a tenant, applies the artifact rules, and emits a `merge_report` of per-listing proposals. Read-only against master_products / master_variants — actually applying approved proposals (D3) is a separate transaction-heavy step intentionally deferred.

This commit is **report generation only**. Curator can review the proposals; no master writes happen here.

## What landed

### `merge_reports` table (catalog_migrations.go)

One row per applier run. Per-tenant FK with cascade delete. Status enum: `pending`/`reviewed`/`applied`/`partial`/`reverted`/`superseded`. `Save` supersedes any prior pending/reviewed report for the same tenant in a single transaction so the curator UX never sees two active reports at once.

Counters denormalized on the row (auto_link / new_master / needs_review / skip / already_linked) so list views render without re-walking the proposals JSONB.

### Domain types (`merge_report.go`)

- `MergeReport` — header + counters + Proposals JSONB
- `MergeProposal` — per-listing decision, polymorphic on `Action`
- `ProposedMaster` / `ProposedVariant` — shape the applier would create on `new_master`
- `FieldDecision` — per-field plan (inherit / propagate_to_master / keep_master / override_listing / skip)
- `MatchEvidence` — strategy + score + confidence + reasoning, drives sort by confidence in the UI

### `MergeApplyUseCase.GenerateReport`

Algorithm — applied in this order, cheapest checks first:

1. **Already linked** — if `listing.master_variant_id` or `listing.master_product_id` is set, emit `already_linked` and return. This was the bug from the first iteration: the applier ignored existing merges and proposed `new_master` over already-merged listings, which would have doubled master_products on apply. The check is now the very first thing in `buildProposal`.
2. **Brand mapping** — vendor → `link_existing` / `create_new` / `skip` (skip wins outright).
3. **Junk rules** — vendor blacklist / axis name patterns / require-identifier.
4. **Vertical resolution** — BrandMapping → MasterTemplates category hints → fallback `unknown`.
5. **Match cascade** — runs the existing M4b cascade with `MatchStrategyConfig` thresholds. Embedding step disabled per-vertical when the artifact says so.
6. **Score → action** — auto-link above threshold, needs_review in the band, new_master if cascade returned no candidate.

Pagination through `catalog.products` in 500-row pages so big tenants don't blow heap.

### CLI: `cmd/run-merge-apply`

Mirrors the pattern of `run-discovery` / `sync-tenant-now`. Loads the tenant integration, decrypts the access token (unused here but kept for symmetry), wires the adapters, calls `GenerateReport`, prints summary + optionally per-proposal detail with `-print`. Used to iterate on applier behavior outside HTTP path.

## Files changed

| File | Action |
|---|---|
| `internal/domain/merge_report.go` | NEW (~140 lines) |
| `internal/ports/merge_reports_port.go` | NEW (~50 lines) |
| `internal/adapters/postgres/merge_reports_adapter.go` | NEW (~190 lines) |
| `internal/adapters/postgres/catalog_migrations.go` | EDIT (+30 lines: merge_reports table + already_linked column) |
| `internal/usecases/merge_apply.go` | NEW (~430 lines) |
| `cmd/run-merge-apply/main.go` | NEW (~140 lines) |
| `.gitignore` | EDIT (+ run-merge-apply binary) |

`go build && go vet && go test ./internal/...` — all clean.

## What happened during the session (the iteration that shaped this)

The first generation of the applier was missing the "already linked" early return. When tested on the heybabes/dev-store tenant — which contains 962 legacy heybabes listings already linked to master_product_id — the run produced **998+ "create new master" proposals** including the 962 already-merged Russian-named listings. Caught at the print phase before any apply step ran. Apply step is unimplemented anyway, so no master writes happened — but if D3 had been ready and someone bulk-approved, master_products would have been doubled.

Lesson re-confirmed for the plan: any applier touching master needs an explicit "what's the source of truth here?" check at the very top, before all the rule resolution. Added as Rule 0 in `buildProposal` with a long comment naming the failure mode.

## Known gaps / D3 next

1. **Apply step** — the actual transactional writer that takes approved proposals and creates master_products / master_variants / sets listing.master_variant_id. Per-proposal transaction with pre-state snapshot in `RollbackData` for revert. **This is the destructive piece** — design carefully and write tests.
2. **Curator endpoints** — `POST /merge/run`, `GET /merge/reports/:tenant_id`, `POST /merge/reports/:id/apply`. Backend only; UI is D4.
3. **Tier-2 transforms in extractTier2** — currently extracts raw_attributes values 1:1 without applying `transform: ml_from_string` etc. The transforms are spec'd in FieldMappingTarget; the applier should call them. Defer to D3 when tested against real data flow.
4. **EmbeddingDisabledFor enforcement** — the applier creates a local cascade-without-embedder when the artifact says to skip embedding for the vertical. Works, but allocates a new struct per listing — fine for thousands of listings, watch if we hit hundreds of thousands.
5. **dev-store smoke test** — deferred; user wanted to wrap the session. Re-running on a clean tenant would verify the "already linked" gate and the new_master path produces sane proposed_master shapes. First task next session.
6. **The bad merge_report from the first run** is in `catalog.merge_reports` with status either `pending` (if no second save happened) or `superseded`. Cleanup at start of D3 — single DELETE on the row.
