# Catalog Pipeline — Gaps Tracker

> Live tracker. Updated 2026-05-22 alpha-0.9.4: Group A + B1 landed.

## Where we are

- Tag: `alpha-0.9.4`
- **Group A (Sephora seed blockers) closed** — Bug 1 (cosmetics nil-array crash + structural helper), Bug 2 (BulkMarkApplied + LOWER(sku) index), per-field approve router, idempotency lock-in test.
- **B1 closed** — discovery now patches existing artifact on `mapping_miss` / `unknown_vertical` triggers instead of rebuilding from scratch.
- Discovery agent correctly built the Sephora map in 0.9.3. With A1-A4 + B1 landed, the next Sephora seed run should land 8494 rows end-to-end with `master_cosmetics` populated and curator approvals actually mutating columns.

## Frame

This pipeline automates a seasoned PIM content manager who:
1. Receives data from a new client (CSV / Shopify / JSONL)
2. Maps client fields to master schema
3. Cleans and transforms (units, prices, categories)
4. Matches to existing masters → creates new → enriches existing
5. Categorizes into a stable taxonomy
6. **Handles ongoing updates** — daily re-syncs with new columns, new
   SKUs, removed SKUs
7. Detects junk and quality issues; routes for review
8. Maintains master lifecycle long after initial ingest

Gaps below are read against this frame.

---

## Group A — Sephora seed blockers — **DONE in alpha-0.9.4**

Goal: Sephora-shape import lands end-to-end in under 60 seconds with
master_cosmetics populated and curator approval that actually mutates
columns.

### A1. Fix Bug 1 + extract structural array-nil helper — DONE

`BulkUpsertCosmetics` marshals each row as JSON, then SQL unpacks array
columns via `jsonb_array_elements_text`. Nil slices marshal to `null`
which crashes the SQL. Fix: coerce nil array fields to `[]string{}`
before marshalling.

**Structural part:** extract a `coerceNilArrays(row, arrayFields)`
helper into one place. **Every** `Bulk*` writer routes through it. Any
future `BulkUpsertElectronics` / `BulkUpsertFashion` inherits the fix —
this class of bug cannot reappear.

Files: `internal/adapters/postgres/catalog_v2_writer_adapter.go`.

### A2. Fix Bug 2 — apply throughput — DONE

- `BulkMarkApplied(itemIDs []string)` via UNNEST text[] + UPDATE …
  WHERE id = ANY. Replace the per-row loop in `apply_v2.go`.
- EXPLAIN the `BulkMatchOrCreateMaster` stage A query against current
  `master_products` (≥1k rows). If it scans, add partial indexes:
  `(LOWER(sku))`, `(gtin) WHERE gtin IS NOT NULL`,
  `(normalized_match_key) WHERE normalized_match_key IS NOT NULL`.

Acceptance: 8494-row apply in < 60 sec.

Files: `internal/adapters/postgres/inbox_adapter.go`,
`internal/usecases/apply_v2.go`,
`internal/adapters/postgres/catalog_v2_writer_adapter.go`.

### A3. Approve actually mutates master columns — DONE

Curator's "Approve" currently flips `approval_status='approved'` and
`master_pending_changes.status='approved'`. It does **not** write the
proposed `pending_value` into the target column.

Fix: per-field router reads `tenant_catalog_schema.field_targets` to
dispatch each change to one of three surfaces:

- `master_products.<scalar_column>` — universal fields (name, brand, image, sku, gtin)
- `master_cosmetics.<scalar_column>` — typed cosmetics fields (volume_ml, key_ingredients[], skin_type[])
- `master_products.tier3 ->> <json_key>` — everything else (current haircare lives here, future electronics too)

**Vertical-agnostic** — the router doesn't know "cosmetics" or
"electronics"; it knows "this field lands in scalar column X" or "this
field lands in JSONB key Y". New verticals plug in via artifact, not
code.

Bulk approve runs in one transaction per batch.

Files: `curator/backend/internal/adapters/pending.go`,
`curator/backend/internal/handlers/handler_pending.go`,
admin's catalog_v2_writer for column-write helpers.

### A4. Idempotency on identical re-import — **already implemented; DONE — regression test added in 0.9.4**

Discovery from exploration: `inbox_adapter.go:166-170` already had the right behavior — `ON CONFLICT DO UPDATE` preserves `applied_at` when `payload_hash` matches and clears it on hash diff. `ListUnapplied` filters by `applied_at IS NULL`, so identical re-imports correctly skip apply.

A4 in 0.9.4: regression test in `regression_inbox_idempotency_test.go` locks in the implicit contract (`(a)` identical hash preserves applied_at; `(b)` hash diff resets it).

---

## Group B — Master lifecycle (~4.5 h)

Goal: pipeline survives ongoing client updates without silent data loss.
This is the back side of the conveyor — what happens AFTER the master
table is populated and the same client keeps sending updates.

### B1. Discovery patches existing artifact — DONE in 0.9.4

Today, `discovery_v2.go:170` starts `draft := newDiscoveryDraft()` on
every trigger including `mapping_miss`. The agent rebuilds the artifact
from scratch every time, guided only by the prompt instruction
"focus on the offending field". The existing artifact is overwritten on
save.

Fix: `mapping_miss` / `unknown_vertical` / `manual` triggers load the
existing artifact into draft (`newDiscoveryDraftFromArtifact(art)`).
`first_install` stays fresh start. Add an audit log entry on each save
recording which fields changed.

Unblocks B2 (drift detection's "Apply suggestion" button needs
patch-discovery to work cheaply).

Files: `internal/usecases/discovery_v2.go`,
`internal/usecases/discovery_v2_draft.go`.

### B2. Schema drift detection — 90 min

After every apply, compare `set(inbox_items.raw keys)` vs
`set(artifact.field_map[*].source_key)`. Unmapped keys → batched LLM
call (one per apply run, not per row):

> "Here is the current artifact. Here are new keys with sample
> values. For each: (a) typo of an existing key, (b) an existing
> attribute under a different name, (c) genuinely new."

Findings → `tenant_schema_drift_findings` table. Admin tab "Schema
drift" lists findings with "Apply suggestion" button → triggers
patch-discovery (B1).

Files: new table, new `internal/usecases/schema_drift.go`,
`project_admin/frontend/src/features/catalog/SchemaDriftTab.jsx`.

### B3. Junk routing — Layer 1 (deterministic validators) — 60 min

Per-field-kind validators run during apply. Bad rows are NOT rejected —
they get flagged. Mark `inbox_items.data_issues jsonb[]` and
`master_products.has_issues bool`.

Validators:
- price: `is_null OR <= 0` → `missing_commercial`
- image_url: `NOT (http|https) OR pattern_known_broken` → `broken_image`
- name: matches `/^(test|demo|tmp|sample|delete me)/i` → `suspicious_name`
- gtin: not 8/12/13/14 digits OR fails checksum → `bad_gtin`
- identity: name AND sku both empty → reject (already implemented)

Admin frontend gets a "Products with issues" tab with per-issue filter
and counts. Tenant can self-fix from their own panel.

Layer 2 (attribute-value aliases like "salt" vs "sodium chloride") is
in Group C — wait for real cases in curator.

Files: `internal/usecases/apply_v2_validators.go` (new),
`internal/adapters/postgres/catalog_migrations.go` (add columns),
admin frontend new tab.

### B4. In-batch duplicate scream — 40 min

If a CSV contains the same SKU twice, both rows currently merge silently
into one master. Make it visible:

- Save `apply_run_summary` per run: `{total, new_masters, bound,
  dup_in_batch, dup_skus_sample}` into new `tenant_apply_runs` table
- Admin → tenant view → "Sync history" tab: list of recent runs, yellow
  badge on rows where `dup_in_batch > 0`, click expands the sample
- Curator card badge "merged from N input rows" when a master sourced
  from 2+ inbox rows in the same run

Files: new table, summary capture in `apply_v2.go`,
admin frontend "Sync history" tab, curator card field.

---

## Group C — Deferred

| Item | Why deferred | Trigger to revisit |
|---|---|---|
| Brand normalization + duplicate detection (pg_trgm + curator queue) | Tenants are few; brand overlap rare. Curator can merge manually. Standalone ~90 min, not on critical path. | When a 2nd cosmetics tenant arrives with overlapping brands and manual merges hurt |
| Removed-SKU tracking (Shopify Bulk async + listing soft-delete) | Standalone 3-4 h piece. Per-tenant — master untouched, only `catalog.products` for that tenant gets `deleted_at`. | When production daily-sync turns on for real tenants |
| Junk routing — Layer 2 (attribute-value aliases, e.g. "salt"/"sodium chloride") | Wait for real cases | When alias gaps surface in curator pending review |
| Variant grouping (1 product → N SKUs) | Structural change. Reshapes `master_products` from 1:1 to 1:N. Cascades to curator UI, V5 widget, Shopify ingest. 8-12 h piece. | Standalone milestone after Group A+B ships |
| Description enrichment (LLM rewrite of product copy) | Master stays clean; owner curates manually | Not planned |
| Confidence score per row / needs_review prioritization | Owner does manual QC; no need for auto-priority | Not planned |
| Re-discovery against accumulated master state on schedule | B1 + B2 cover the reactive path; periodic refresh is later | When tenants accumulate >3 months of master history |

Minimal cheap step for variant grouping: at Shopify ingest time, persist
`source_parent_id` + `source_variant_id` into `inbox_items.raw`. Costs
nothing now; later the structural rework has the data without
re-ingest.

---

## Structural notes

### New verticals (electronics, fashion, …) work without code changes

Current schema:
- `master_products` — universal scalar columns (name, brand, sku, image, vertical, ...)
- `master_cosmetics` — typed cosmetics-specific scalar columns + arrays
- `master_products.tier3 jsonb` — tail of source-specific or vertical-specific fields

**haircare already lives entirely in tier3** — no typed table — and
works fine. Same applies to a new electronics tenant: universal fields
in `master_products`, tail in `tier3`. No `master_electronics` table
needed unless the vertical proves popular AND V5 needs fast structured
filters (e.g. "RAM ≥ 16 GB").

### The cosmetics nil bug is a CLASS

A1's structural fix (`coerceNilArrays` helper) means any future
`BulkUpsertElectronics` cannot reintroduce the same bug. Worth doing
even though only one vertical has a typed table today.

### Discovery has no "see previous artifact" tool

`discovery_v2.go` has no tool that lets the agent read the existing
committed artifact. On mapping_miss the agent gets a one-line trigger
context and rebuilds the artifact from scratch using builder tools.
B1 fixes this by loading the existing artifact into the draft on
non-first_install triggers.

---

## Out of scope here

- V5 chat engine gaps — see `docs/v5-known-gaps.md`
- Auth / billing / admin SPA — see `docs/PRE_LAUNCH_TASKS.md`
- Pre-existing test failures (`Scenario_052-055` Shopify consent flow)
- The "PIM as separate service" architectural question — under review
  separately

---

## Source sessions

- `docs/Updates/main_2026-05-21_03-08.md` — Alpha 0.9.1 bundled milestone (pending approval + bidirectional discovery + cron), first Sephora seed
- `docs/Updates/main_2026-05-22_00-14.md` — Alpha 0.9.2 + 0.9.3 (builder pattern + bulk apply), Bug 1 + Bug 2 discovered, agent built map correctly
- Conversation 2026-05-22 — PIM-manager-frame audit, owner verdict per item, group split

---

## Estimate summary

| Group | Items | Hours | Outcome |
|---|---|---|---|
| A | A1–A4 | ~3.5 | Sephora seed lands end-to-end |
| B | B1–B4 | ~4.5 | Lifecycle survives ongoing client updates |
| C | deferred | — | Revisit per trigger |

Total critical work: ~8 hours. Two long sessions or three short ones.
