# Catalog Group D — Implementation Spec

> **Authoritative implementation spec** for the Group D catalog cleanup.
> Tracker-level status lives in `docs/CATALOG_GAPS.md`; **this document is the
> detailed contract we implement and verify against.**
> Visual target schema (ER): `docs/project_map/catalog/multi-tenant-example.html`
> Language: **English only** (project rule — no Russian in code, data, or docs).
> Created 2026-05-25.

---

## ⏱ Execution status (2026-05-25)

> **▶ LIVE EXECUTION PLAN for the remaining work: `docs/Updates/main_2026-05-25_18-59.md`.**
> Read that first — it has the locked decisions, code-verified corrections (e.g. the `sync`
> import must be REMOVED, not kept), and exact file:line edits per item. The table below is
> the high-level status; that doc is what you execute from.

Session logs: `docs/Updates/main_2026-05-25_18-15.md` (items 1+2) →
`docs/Updates/main_2026-05-25_18-59.md` (remaining-work plan) →
`docs/Updates/main_2026-05-25_20-18.md` (Task #7 + D3 categories shipped). Owner authorized
breaking V5 search + running destructive migrations against Neon (= the dev stand).

**Decisions locked 2026-05-25:** **D1b (variant identity) DEFERRED** to a separate milestone
(apply creates no variants today; all listings have `master_variant_id=NULL`); the Phase-1
final gate below is downscoped (1b.*, INT.4 OUT). **D3** reroutes admin + rebuild-embeddings to
V2 and drops V1; V5's V1-category reads are left for the V5-milestone (V5 already broken).
**Cross-tenant dedup (Decision #6) already works in code** — E2E only proves it.

| Item | Status | Notes |
|---|---|---|
| **1. Drop dead services** | ✅ DONE + run on Neon | services/master_services dropped (were 0 rows). Removed adapter/port/import.go/csv_mapping/domain service.go/test mocks. |
| **2. tier2 canonical (drop master_cosmetics + 17 legacy cols)** | ✅ DONE + run on Neon + verified | apply→tier2, curator `tier2.<key>` routing, enrichment/embedding repointed. Migration: seed 18 cosmetics field-defs + backfill master_cosmetics (1000 rows)→tier2 + backfill 17 legacy cols (~960 rows)→tier2/tier3 + drops. **reconcile=0 (zero data loss)** verified on Neon. build+vet+catalog/apply tests green. |
| **Task #7 — dead-code cleanup** | ✅ DONE (Alpha 0.9.7) | Removed `UpsertCosmetics`/`BulkUpsertCosmetics`/`bulkUpsertCosmeticsBatch`/`probeCosmeticsSchema` + `sync` import + probe struct fields + 4 now-dead pgx helpers (`nullableStr`/`nullableInt`/`stringSliceArg`/`coerceStringSlice`); port `UpsertCosmetics`/`BulkUpsertCosmetics`/`BulkCosmeticsItem`/`ErrCosmeticsSchemaNotReady` + `errors` import; `master_variants` `UpsertMasterCosmetics`/`GetMasterCosmetics` + port + `domain.MasterCosmetics`; fakeWriter mocks. KEPT `ports.MasterCosmeticsUpsert`. build + usecases tests green (only pre-existing Shopify 052–055 red). |
| **3. D6 controlled vocabularies** | ⬜ NOT STARTED | dim-tables + aliases (mirror `catalog.unit_aliases` / `internal/units/aliases.go`) + normalizer in apply (resolved ids → tier2) + brand→brand_id dedup + curator vocab queue. |
| **4. Categories: drop V1 (D3)** | ✅ DONE + run on Neon + verified (Alpha 0.9.7) | Full migration to target model: V1 `catalog.categories` → `master_categories` + `master_product_categories` junction (reused V1 UUIDs → hierarchy + links preserved). Rerouted ALL admin readers (filter recursive-CTE, 7 display LATERALs, `GetCategories`, `GetOrCreateCategory`, `GetCategoryBySlug`, `UpsertMasterProduct`, `UpdateMasterProductPIM`) + `cmd/seed` + `rebuild-embeddings` to the junction. Dropped `category_id` column + `catalog.categories` + stale V1 indexes. **Neon: 30 categories migrated, 987 product links, 0 data loss, V1 gone.** Category `vertical` defaulted to `'unknown'` (curator refines later). V5 category reads left for the V5-milestone (already broken). Bumped migration ctx 30s→120s (Neon cold-start headroom). |
| **5. Stock consolidation** | ⬜ NOT STARTED | `catalog.stock` canonical; curator reads stale `products.stock_quantity`; fix curator + drop denorm col. |
| **6. E2E verification on 4 seeds (+ tenant E)** | ⬜ NOT STARTED | ingest A/B/C/D via real pipeline; verify tier2 populates for all verticals; tenant E (overlap seed) for cross-tenant dedup — see §8.2. |

**Resume tip:** Neon already migrated for items 1+2 (master_cosmetics/services/17-cols gone; tier2
populated). Migrations in `catalog_migrations.go` are idempotent — a fresh server start re-applies
them safely. **Execute from `docs/Updates/main_2026-05-25_18-59.md`** in order:
Task #7 (dead code, 0 risk) → D3 (drop V1 categories) → D4-stock (curator → catalog.stock) →
D6-data (controlled vocabularies) → E2E (4 seeds + tenant E). No code changed yet this round.

---

## 0. How to use this document

- Sections 1–4 = the *what* and *why* (architecture + constraints). Read once.
- Sections 5–7 = the *work* (per-item changes, execution order). The build checklist.
- **Section 9 = Definition of Done.** This is the contract. A work item is not
  "done" until its DoD checks pass with the exact verification listed. No check
  is satisfied by "looks right" — every check has a command, SQL query, or test.
- Section 8 = test data. **Read 8.2 first — there is a blocking seed prerequisite.**
- Section 10 = decisions still open. Resolve before the relevant work item.

---

## 1. Problem & goal

The catalog schema accumulated debt: duplicated columns (17 cosmetics columns on
`master_products` that also live in `master_cosmetics`), a half-deprecated
`master_variants` table, six category tables without clear roles, dead service
tables, and free-text fields where controlled vocabularies belong. Every new
feature grows `if`-branches ("write here or there?"). This is the root cause of
slow movement.

**Goal:** one clean, consistent PIM foundation — correct identity model, controlled
vocabularies, no dead weight — ready for hyper-PIM scale (10M+ masters) and clean
enough that subsequent features (B3–B5) ship fast instead of fighting the schema.

**Group D is a prerequisite for B3–B5.** B3 (junk validators) and B5 (master
conflict resolution) are designed against the listing/identity structure; doing
them before the cleanup means doing them twice.

---

## 2. Target architecture

Full ER view with sample rows: `docs/project_map/catalog/multi-tenant-example.html`.
Summary below.

### 2.0 Core principle — VERTICAL-AGNOSTIC (read this first)

The catalog has **NO per-vertical typed tables.** There is no `master_cosmetics`,
no `master_electronics`, no `master_laptops` as the design target. **Cosmetics is not
special.** Every vertical — cosmetics, electronics, furniture, apparel — works through
the exact same mechanisms:

- **`vertical`** (Tier-1 column) — what kind of product.
- **`tier2 jsonb`** — ALL vertical-specific typed attributes; valid key set per vertical
  declared in `master_field_definitions`; per-key functional indexes added at promotion
  time (matches a typed table's filter speed).
- **`master_variants`** — variant axes; the *kinds* of axes depend on the vertical
  (cosmetics: volume/scent; apparel: size/color) but the table is universal and first-class.
- **dim-vocab tables** (D6) — controlled values for tier2 attributes, shared across verticals.

`master_cosmetics` is a **legacy typed table** from an earlier, wrong direction. The code's
own migration comment (`catalog_migrations.go`) already labels the typed tables legacy with
"migration into tier2 deferred." **Group D finishes that migration:** tier2 becomes the
canonical home for vertical attributes in Phase 1; `master_cosmetics` is demoted to a
V5-read shim and physically **dropped in Phase 2**.

Rule for any future catalog work: if you find yourself writing vertical-specific table
logic, stop — it goes in tier2. Never use cosmetics as "the model"; design and test with
electronics/furniture as equal first-class citizens.

### 2.1 Three layers

| Layer | Tables | Scope | Access pattern |
|---|---|---|---|
| **L1 — Identity (Master)** | `master_products` (incl. `tier2`/`tier3` jsonb), `master_variants`, `master_categories`, `master_field_definitions`, dim-vocab tables. *(`master_cosmetics` = legacy shim, drops in Phase 2)* | Shared across all tenants | Slow churn, rich data, ingest-heavy |
| **L2 — Search projection** *(Phase 2)* | `tenant_search_projection` | Per-tenant slice | Read-heavy, latency-sensitive |
| **L3 — Listing (Offering + CMS)** | `catalog.products`, `catalog.stock`, `tenant_categories`, `category_mapping` | Per-tenant | Commerce writes + presentation reads |

### 2.2 Three-tier knowledge model (NOT duplication — a hierarchy)

| Tier | Holds | Storage | Example (L'Oreal Mascara) |
|---|---|---|---|
| **Tier 1** universal facts | name, brand, sku, **vertical** | scalar columns | `brand="L'Oreal"`, `vertical="cosmetics"` |
| **Tier 2** vertical-typed attributes | per-vertical typed keys | `tier2 jsonb` + schema in `master_field_definitions` | `{"product_form":"mascara"}` |
| **Tier 3** search soup | reviews, transcripts, packaging, anything | `tier3 jsonb` | `{"reviews":1247, "claims":[...]}` |

`vertical` is one Tier-1 column that *determines which keys are valid in Tier 2*.
It is not the same thing as Tier 2 and it is not duplicated.

### 2.3 Identity model — Position B (family + variants)

- `master_products` = **family** (one entity, e.g. "L'Oreal Mascara Volume Million").
- `master_variants` = **variant axes** (volume / color / scent / size). First-class,
  mandatory: every master has ≥1 variant.
- Non-variant products get a **default variant** (one row, axes null,
  `variant_kind='default'`), so a listing *always* references a concrete variant.
- `catalog.products` (listing) always points to a specific `master_variant_id`.

This serves all three query shapes a buyer can use:
- "L'Oreal" → brand-level (filter `brand_id`)
- "L'Oreal mascara" → family-level (master)
- "L'Oreal mascara 7ml" → variant-level (master + variant axis filter)

### 2.4 Multi-tenancy model

- One shared master; many tenant listings point to it.
- Search hits the same masters; **visualization differs per tenant** (display_name,
  media, badges via `tenant_overrides`) and per **variant subset** (a tenant may
  carry only the 7ml variant, not 12ml).
- Tenant isolation in search comes from the listing layer
  (`catalog.products WHERE tenant_id`) and, in Phase 2, the per-tenant projection.

---

## 3. Scope

### 3.1 Phase 1 — PIM core + cleanup (implement now, ~21–28h; **+8–12h if variant grouping is pulled in — Decision #5**)

Everything about data, identity, and master quality. **Search and V5 are not touched.**
Expand-contract: build clean structures *alongside* the old ones; V5 keeps reading
the old ones and does not break.

| Item | What | Est. |
|---|---|---|
| **D1a** | DROP 17 legacy cosmetics columns from `master_products`; make `tier2` the canonical home for vertical attributes; demote `master_cosmetics` to a V5-read shim | 5–7h |
| **D1b** | `master_variant_id` NOT NULL + synthetic default-variants | 6–8h |
| **D3** | DROP `catalog.categories` (V1); confirm V2 category tables | 2–3h |
| **D4** | DROP dead service tables; resolve stock duplication | 3–4h |
| **D6-data** | dim-tables + aliases + ingest normalizer + brand dedup + ingredient link (writes new `*_ids` columns *alongside* old `TEXT[]`) | 6–8h |

### 3.2 Phase 2 — search wiring (deferred to V5 milestone, ~7–11h)

The only parts that touch the V5 read path. Build when the owner moves to V5.

| Item | What | Est. |
|---|---|---|
| **D2** | `tenant_search_projection` table + populate + repoint V5 search to it | 5–7h |
| **D6-search** | switch V5 vector filters to `*_ids UUID[]`; DROP old `TEXT[]` columns | 2–4h |
| **D5** | decide `vertical` vs `tier2` unification | trivial after D1/D6 |

### 3.3 Out of scope

- **Search behaviour / relevance** — separate, hard problem; owner owns timing.
- **Russian-language search** — not supported, not a goal. English-only data and search.
- **New agent capabilities** — no new "agent fills/edits PIM" features. Existing
  discovery agent / B2 drift / apply are preserved, not extended.
- **Group E (PIM as separate microservice)** — shelved, trigger-based (no trigger met).
- **B3–B5** — come after Group D.

---

## 4. Constraints (non-negotiable)

1. **Regression safety.** After every step, the existing `inbox → apply → master/listing`
   flow works exactly as before. Existing tests stay green. Existing flows (Shopify
   install, discovery agent, B2 drift) keep working.
2. **Don't touch V5 / search in Phase 1.** Use expand-contract. New columns/tables are
   additive; old readers keep their old columns until Phase 2 flips them.
3. **English only.** All canonical vocab values, aliases, display labels, code,
   and docs are English. Aliases normalize English variants/typos/casing/abbreviations
   — never translations.
4. **No silent data loss.** Every inbox value either maps to a master/listing field or
   lands in a review queue (`master_attribute_candidates`). Nothing dropped silently.
5. **Idempotency.** Re-running the same seed/import produces no duplicates (no dup
   variants, no dup masters, no dup vocab rows).

---

## 5. Work items — Phase 1 (detailed)

Severity: **S** = small/local · **M** = method rewrite · **L** = significant refactor.
File paths relative to repo root.

### D1a — DROP 17 legacy columns + make `tier2` canonical (vertical-agnostic)

This item finishes the deferred "typed-table → tier2" migration the code's own comment
already calls for. Two moves:

**(i) DROP 17 legacy cosmetics columns from `master_products`:** `skin_type, concern,
key_ingredients, target_area, free_from, benefits, marketing_claim, how_to_use, volume,
product_form, texture, routine_step, routine_time, application_method, inci_text,
short_name, original_name, product_line, enrichment_version`.

**(ii) Establish `tier2` as the canonical home for vertical attributes** (cosmetics included,
per §2.0 vertical-agnostic). `apply_v2` writes vertical attributes into `tier2` (validated
against `master_field_definitions`). **`master_cosmetics` is NOT the target** — it is demoted
to a legacy V5-read shim: keep it populated (apply keeps writing it, unchanged) ONLY because
V5 search still reads it; it is no longer the source of truth and is physically dropped in
Phase 2 (D6-search). No new per-vertical typed tables, ever — electronics/furniture/etc. all
use the same `tier2` path with no typed table.

| File | Method / section | Change | Sev |
|---|---|---|---|
| `project_admin/backend/internal/adapters/postgres/catalog_adapter.go` | `UpdateMasterProductPIM` | Remove the 17 column assignments | M |
| ` ` | `GetUnenrichedMasterProducts` | Remove `enrichment_version = 0` filter | M |
| ` ` | `GetMasterProductsWithoutEmbedding` | Remove SELECT of legacy cols (read from `tier2`) | S |
| `project_admin/backend/internal/usecases/apply_v2.go` | `marshalMasterProductUpsert` (~L934) | Write vertical attributes into `tier2` (canonical); keep existing `master_cosmetics` write unchanged as V5 shim | M |
| `project_admin/backend/internal/domain/product.go` | `MasterProduct` struct | Remove 17 fields | M |
| `project_admin/backend/cmd/rebuild-embeddings/main.go` | main | Remove `enrichment_version >= 2` | S |

Procedure: grep each of the 17 names first → reroute any readers to `tier2` (NOT to
`master_cosmetics`) → ensure `apply_v2` writes those attributes into `tier2` → then drop the
columns in a migration. `master_cosmetics` writes stay as-is (V5 shim); it is not extended.

### D1b — `master_variant_id` NOT NULL + synthetic default-variants

Make `master_variants` first-class. Every listing references a variant. Non-variant
products get one synthetic default variant.

| File | Method / section | Change | Sev |
|---|---|---|---|
| `project_admin/backend/internal/adapters/postgres/catalog_v2_writer_adapter.go` | `UpsertListing` (~L507) | For null variant, create/link a default variant before insert | **L** |
| ` ` | `BulkUpsertListing` (~L988) | Same, batched (synthesize default ids pre-insert) | **L** |
| `project_admin/backend/internal/adapters/postgres/master_variants_adapter.go` | new `CreateDefaultVariant` | Insert one variant (`variant_kind='default'`, axes null) when a master has none | M |
| `project_admin/backend/internal/adapters/postgres/catalog_adapter.go` | bind/create product path | Bind `master_variant_id` on insert | M |
| `project_admin/backend/internal/domain/product.go` | `Product` | `MasterVariantID` → non-nullable | S |
| migration | — | `ALTER ... SET NOT NULL` after backfill | S |

**Highest-risk item.** Default-variant creation must be idempotent: re-apply must not
create a second default variant for the same master.

### D3 — Categories cleanup

`catalog.categories` (V1) is legacy; the V2 set (`master_categories`,
`master_product_categories`, `tenant_categories`, `category_mapping`,
`tenant_listing_categories`) is alive (verified). Target: drop V1 only.

| File | Method / section | Change | Sev |
|---|---|---|---|
| `project_admin/backend/internal/adapters/postgres/catalog_adapter.go` | `GetOrCreateCategoryBySlug` + 3 `SELECT ... FROM catalog.categories` | Remove / reroute to `master_categories` | M/S |
| `project_admin/backend/cmd/seed/main.go` | category seed | Remove legacy seed | S |
| `categories_v2_adapter.go` | all | Verify no fallback to V1 | S |

### D4 — DROP dead code

| File | Method / section | Change | Sev |
|---|---|---|---|
| `project_admin/backend/internal/adapters/postgres/catalog_adapter.go` | `CreateOrUpsertService`, `GetServices`, `UpdateServiceEmbedding` + 3 unused `LEFT JOIN catalog.master_services` | DELETE methods + joins, then DROP `master_services`/`services` | M/S |
| ` ` | `BulkUpdateStock` | Write both `catalog.stock` and `products.stock_quantity` (transitional) | S |

- **`merge_reports` is ALIVE** (admin handlers + `integrations_wipe.go`) — **do not touch.**
- Stock: `catalog.stock` is canonical (hot path, future KeyDB/Tarantool); `products.stock_quantity`
  stays as denorm for compatibility; deprecate later.

### D6-data — controlled vocabularies (expand phase)

Build dim-tables and aliases; the ingest normalizer resolves raw strings to canonical IDs.
**Vertical-agnostic (per §2.0):** resolved IDs land in `tier2` (e.g. `tier2.skin_type_ids`,
`tier2.color_ids`) — the same path for every vertical, NO per-vertical columns. `brand`
resolves to `brand_id` on `master_products`. The legacy `master_cosmetics` keeps its raw
`TEXT[]` columns unchanged (V5-read shim) so search is untouched in Phase 1; Phase 2 switches
V5 to read `tier2` ids and drops `master_cosmetics`.

dim-table pattern (mirrors the existing `catalog.unit_aliases` prototype):

```sql
catalog.dim_<attr> (
  id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  canonical TEXT UNIQUE,        -- "oily"
  vertical  TEXT,               -- "cosmetics"
  display   TEXT,               -- "Oily skin" (English label)
  source    TEXT CHECK (source IN ('seed','curator','promoted'))
);
catalog.dim_<attr>_aliases (
  raw_token TEXT PRIMARY KEY,   -- "oily-prone", "oily skin", "combination-oily", "OILY"
  <attr>_id UUID REFERENCES catalog.dim_<attr>(id),
  source    TEXT
);
```

**Which attributes are controlled is driven by `master_field_definitions`** (a `controlled`
flag / `dim_table` ref per key) — NOT a hardcoded list. The initial seed happens to cover
`brand` (universal), the cosmetics set (`skin_type, concern, target_area, free_from, benefits`),
and general physical attrs (`color, material, size`); any other vertical's keys (e.g. electronics
`connector_type`, furniture `frame_material`) join the same mechanism the moment they're flagged.
**Ingredient link is vertical-conditional** — only verticals that have ingredients (cosmetics,
food, supplements) link to `catalog.ingredients` via `product_ingredients`; electronics/furniture
have no ingredients and skip it. Don't assume every product has ingredients.

| File | Method / section | Change | Sev |
|---|---|---|---|
| `project_admin/backend/internal/usecases/apply_v2.go` | transform stage | Resolve raw values → canonical ids via aliases; write ids into `tier2`; miss → `master_attribute_candidates` | **L** |
| new | `vocab_normalizer.go` + `DimAliasBatch` | Batch alias lookup to avoid N+1 in apply | M |
| `project_admin/backend/internal/adapters/postgres/catalog_adapter.go` | brand write | Resolve `brand` → `brand_id` on `master_products` (dedup) | M |
| `project_admin/backend/internal/adapters/postgres/master_variants_adapter.go` | `UpsertMasterVariant` (~L34) | Resolve color/material/size to dim ids (stored on variant or in tier2 axes) | M |
| `project_admin/backend/internal/adapters/postgres/catalog_v2_writer_adapter.go` | `UpsertCosmetics` (~L246), `BulkUpsertCosmetics` (~L777) | NO new typed-column work — leave `master_cosmetics` `TEXT[]` writes as-is (V5 shim). Canonical ids live in `tier2`, written by apply | S |
| `curator/backend` | new "Vocab promotion queue" tab | Surface `master_attribute_candidates` for curator | M |

Brand dedup works in Phase 1 (brand is not a V5 search filter). Switching V5 filters to
`tier2` ids and dropping `master_cosmetics` + old `TEXT[]` is Phase 2 (D6-search).

---

## 6. Work items — Phase 2 (deferred to V5 milestone)

### D2 — tenant_search_projection + V5 read-path
New `catalog.tenant_search_projection` (PK `(tenant_id, master_variant_id)`) denormalizing
master + variant + tenant_overrides, with its own HNSW + GIN. Populate via explicit
`Rebuild(tenant_id, master_variant_id)` from apply and listing writes (not a jsonb trigger).
Repoint `project_v5/backend/internal/adapters/postgres/postgres_catalog_vector.go` to query
the projection with `WHERE tenant_id = $1` instead of `master_products JOIN catalog.products`.
Rationale: per-tenant slice keeps HNSW latency flat as the master grows to 10M+.

### D6-search — switch V5 filters to UUID[] + drop old TEXT[]
`postgres_catalog_vector.go` filters (skin_type/concern/target_area/free_from) switch from
`ANY(mc.<attr>)` on `TEXT[]` to `*_ids UUID[]`; then DROP the old `TEXT[]` columns (contract phase).

### D5 — vertical → tier2 unification
After D1/D6, decide: keep `vertical` as a hot fast-path column, or fold it into
`master_field_definitions` as a promoted attribute with a denormalized cache. Low effort once
the rest is clean.

---

## 7. Execution order (Phase 1)

```
1. D4   (DROP dead code)          ← smallest blast radius, clears noise first
2. D3   (categories V1 cleanup)   ← independent, low risk
3. D1a  (DROP 17 legacy columns)  ← grep readers → reroute → drop
4. D1b  (variant identity)        ← highest risk; do on a clean post-D1a schema
5. D6-data (vocabularies)         ← depends on D1a (tier2 established as canonical home)
```

Rationale: clear dead weight first (D4, D3) so greps during D1a/D6 are clean; D1a before
D1b (variant work on a clean schema); D6-data last (depends on D1a).

---

## 8. Test data / seeds

### 8.1 Current seeds (verified 2026-05-25)

| Tenant | Vertical | Rows | Identity | Variants in source? | Images |
|---|---|---|---|---|---|
| A | Cosmetics (Sephora) | 8494 | `product_id` (**no GTIN**) | Yes (`variation_type/value`, `child_count`) | **No** |
| B | Electronics (Amazon) | 3000 | `parent_asin` | Yes (color / config / capacity) | Yes |
| C | Mixed (Office/Sports/Toys/Pet/Tools) | 3000 | `parent_asin` | Yes (size / pack / color) | Yes |
| D | Furniture (Amazon) | 3000 | `parent_asin` | Yes (color / size / material) | Yes |

**All four verticals have variants — do NOT treat B/C/D as flat.** Vertical attributes for
B/C/D land in `tier2` (no typed table; never `master_cosmetics`). Two facts that break
cosmetics-centric assumptions and MUST be honored:
- **Tenant A has no image URLs and no GTIN** (only `product_id`). No apply step or validator
  may *require* an image or a GTIN — otherwise cosmetics ingest breaks. Cross-tenant matching
  for cosmetics must work without GTIN (name+brand normalization).
- **Variant-grouping and matching keys differ per source** (Sephora `child_count`/`variation_type`
  vs Amazon `parent_asin`). The grouping logic must be source-aware, not Sephora-shaped.

### 8.2 Overlap seed — BLOCKING PREREQUISITE for multi-tenancy DoD

**Finding (verified against the data, not the doc):** the 4 seeds are **fully disjoint**.
- Product overlap by `parent_asin`: B∩C = 0, B∩D = 0, C∩D = 0.
- Brand overlap exists (B∩C = 39 brands, C∩D = 33, B∩D = 11, A∩B = 1, A∩C = 1) but these are
  the *same brand selling different products* in different categories — not the same product.

**This contradicts the owner's intent** (he asked for overlapping seeds, with only one tenant
fully disjoint as a control). As built, there is **no data to test cross-tenant product dedup**
("same real product sold by two tenants → one shared master").

**Required before DoD-INT.3 can pass — DECIDED 2026-05-25: build tenant E.**
Tenant E = a 20–50 product subset of an existing tenant (default: subset of tenant A), re-fed
under a new tenant_id. Purpose: assert those products **match existing masters** instead of
creating duplicates (cross-tenant dedup). The other 4 seeds stay disjoint as the isolation
control. (Owner's original intent was overlap across most seeds with one disjoint control; the
built seeds came out fully disjoint — tenant E is the minimal fix to unblock the dedup DoD; a
broader seed-overlap rework is only needed if wider multi-tenant scenarios are required.)

Brand-level overlap that already exists **is** usable for D6 brand-dedup testing (e.g. "Dell"
appears in B and C) — see DoD-6.3.

---

## 9. Definition of Done

**Rule:** an item is done only when every check below passes via its stated verification.
No check is satisfied by inspection alone.

Conventions:
- `psql` = `/opt/homebrew/Cellar/libpq/18.1_1/bin/psql "$DATABASE_URL"` (`DATABASE_URL` from `project_v4/.env`).
- "tests green" = `go test ./...` exits 0 in `project_admin/backend`, `project_v5/backend`, `curator/backend`.
- "grep" = `grep -rn "<term>" --include=*.go` across the three backends (excluding `_test.go` unless noted).

### 9.1 Per-item DoD — Phase 1

#### D1a — DROP 17 legacy columns
| ID | Assertion | Verification | Pass |
|---|---|---|---|
| 1a.1 | No production code references any of the 17 column names on `master_products` | grep each of the 17 names | 0 production hits (test refs reviewed individually) |
| 1a.2 | The 17 columns no longer exist on `master_products` | `psql -c "\d catalog.master_products"` | none of the 17 listed |
| 1a.3 | Builds and tests pass | `go build ./...` + `go test ./...` ×3 | exit 0 |
| 1a.4 | Vertical attributes now in `tier2` (canonical), no data lost | re-seed tenant A; `SELECT count(*) FROM catalog.master_products WHERE tier2 ? 'skin_type' OR tier2 ? 'skin_type_ids'` | ≥ pre-migration cosmetic-attr count |
| 1a.5 | No new per-vertical typed table introduced | grep for `master_electronics`, `master_furniture`, `master_apparel`, new `CREATE TABLE catalog.master_<vertical>` | 0 hits — only legacy `master_cosmetics` shim remains |

#### D1b — variant identity
| ID | Assertion | Verification | Pass |
|---|---|---|---|
| 1b.1 | Every listing has a variant | `SELECT count(*) FROM catalog.products WHERE master_variant_id IS NULL` | 0 |
| 1b.2 | Constraint enforced | `psql -c "\d catalog.products"` | `master_variant_id ... not null` |
| 1b.3 | Idempotent re-apply | apply tenant B twice; compare `SELECT count(*) FROM catalog.master_variants` before/after 2nd run | identical |
| 1b.4 | Non-variant → exactly 1 default variant | pick a known single-SKU product (tenant D); count its variants | exactly 1, `variant_kind='default'` |
| 1b.5 | Variant product → master + N variants *(CONTINGENT on Decision #5 — variant grouping in/out)* | pick a Sephora product with `child_count=N` **AND** an Amazon product (B/C/D) with `parent_asin` siblings; count variants for each | matches N for both verticals (source-aware grouping) |
| 1b.6 | V5 read regression | run existing V5 catalog list test | green (LEFT JOIN still returns rows) |

#### D3 — categories cleanup
| ID | Assertion | Verification | Pass |
|---|---|---|---|
| 3.1 | V1 table dropped | `psql -c "\dt catalog.categories"` | does not exist |
| 3.2 | No code references | grep `catalog.categories` (exclude `master_`, `tenant_`, `master_product_categories`) | 0 hits |
| 3.3 | Category browse still works | run admin category endpoint / existing test | same results as before |
| 3.4 | Tests pass | `go test ./...` ×3 | exit 0 |

#### D4 — DROP dead code
| ID | Assertion | Verification | Pass |
|---|---|---|---|
| 4.1 | Service tables dropped | `psql -c "\dt catalog.master_services catalog.services"` | neither exists |
| 4.2 | No service code refs | grep `master_services`, `CreateOrUpsertService`, `GetServices` | 0 hits |
| 4.3 | Stock single-source | update stock via `BulkUpdateStock`; read `catalog.stock` and `products.stock_quantity` | both consistent; V5 reads correct qty |
| 4.4 | `merge_reports` untouched | run admin merge-report endpoint / existing test | works (unchanged) |
| 4.5 | Tests pass | `go test ./...` ×3 | exit 0 |

#### D6-data — controlled vocabularies (expand)
| ID | Assertion | Verification | Pass |
|---|---|---|---|
| 6.1 | dim-tables exist + seeded | `psql -c "\dt catalog.dim_*"` + row counts | all 9 attrs present, rows > 0 |
| 6.2 | No silent value loss | seed tenant A; reconcile distinct input attr values vs (resolved ids + `master_attribute_candidates` rows) | every input value accounted for |
| 6.3 | Brand dedup | confirm a brand present in 2 seeds (e.g. "Dell" in B & C, or casing variants) maps to one `brand_id` | single `brand_id` for the variants |
| 6.4 | Canonical ids in tier2, legacy shim kept | `SELECT count(*) FROM catalog.master_products WHERE tier2 ? 'skin_type_ids'` > 0 AND `master_cosmetics.skin_type TEXT[]` still populated (V5 shim) | both true |
| 6.7 | Vocab path is vertical-agnostic | confirm a non-cosmetics controlled attr (e.g. furniture `material`, electronics-adjacent) also resolves to a `dim_*` id in `tier2` | resolved, same path, no typed table |
| 6.5 | Ingredient link | pick a product; verify `product_ingredients` junction rows exist linking it to `catalog.ingredients` | rows exist |
| 6.6 | V5 search regression | run existing V5 search test (still reads `TEXT[]`) | same results as before |

### 9.2 Integration & multi-tenancy DoD

Run all 4 seeds (+ overlap seed once §10 resolved) through full `ingest → apply`.

| ID | Assertion | Verification | Pass |
|---|---|---|---|
| INT.1 | All seeds ingest+apply without error | run the 4 seeds; check logs + `inbox_items` counts | A=8494, B/C/D=3000 inbox rows; masters created; 0 errors |
| INT.2 | Tenant/vertical isolation (no false merges) | `SELECT` masters that have listings across incompatible verticals (cosmetics ↔ electronics ↔ furniture) | 0 false cross-vertical masters |
| INT.3 | Cross-tenant dedup *(needs overlap seed)* | ingest overlap seed E (subset of A); compare master count delta | masters grow only by E's non-overlapping count; overlapping products bind to existing masters |
| INT.4 | Variant + non-variant correct across verticals | spot-check one variant master (A) and one default-variant master (D) | both correct per 1b.4 / 1b.5 |
| INT.5 | No data loss end-to-end | reconcile: every `inbox_items` row is either applied (→ master+listing) or flagged | sum(applied)+sum(flagged) = total inbox rows |
| INT.6 | Non-cosmetics attributes land in `tier2` (not lost, no typed table) | pick a tenant B/D master; inspect `tier2` | typed attrs present in `tier2`; zero `master_<vertical>` tables exist |
| INT.7 | Cross-tenant matching works WITHOUT GTIN | tenant E (subset of A) re-fed with **different SKUs**; verify dedup via name+brand | E's products bind to A's masters, not duplicated (see Decision #6) |

### 9.3 Regression DoD

| ID | Assertion | Verification | Pass |
|---|---|---|---|
| REG.1 | All existing tests green | `go test ./...` ×3 | exit 0 |
| REG.2 | Existing ingest→apply unchanged in behaviour | run pre-existing apply scenario tests | green |
| REG.3 | Discovery agent + B2 drift still work | run their existing tests/flows | green |
| REG.4 | V5 chat unaffected | seed a tenant, ask a known query in V5 | returns results as before (search code untouched) |

### 9.4 Final gate — Group D Phase 1 is 100% done when:

> All 4 seeds **plus the overlap seed** complete full `ingest → apply` on the new schema with:
> 1. **Zero data loss** (INT.5 reconciliation balances), and
> 2. **Zero false cross-vertical merges** (INT.2 = 0), and
> 3. **Correct variant / default-variant creation** (1b.3–1b.5, INT.4), and
> 4. **Cross-tenant product dedup proven** (INT.3 with overlap seed), and
> 5. **Brand dedup working** (6.3), and
> 6. **All 17 legacy columns gone, no readers** (1a.1–1a.2), and
> 7. **Dead tables gone, `merge_reports` intact** (4.1–4.4), and
> 8. **All existing tests green + V5 chat unaffected** (REG.1–REG.4).

If any one of these fails, Phase 1 is not done. Fail loud — do not mark "done" with a
skipped check.

---

## 10. Open decisions (resolve before the relevant item)

| # | Decision | Status | Resolution |
|---|---|---|---|
| 1 | **Overlap seed strategy** | **RESOLVED 2026-05-25** | Build tenant E (20–50 product subset of A, re-fed under new tenant_id). Other 4 seeds stay disjoint as control. See §8.2. |
| 2 | **D2 fill mechanism** (Phase 2) | open (Phase 2) | Explicit `Rebuild()` calls from apply + listing writes (not jsonb triggers — easier to debug) |
| 3 | **Non-cosmetics variant axes** | **RESOLVED 2026-05-25** | Per §2.0 vertical-agnostic: default variant + axes in `tier2`; no per-vertical typed axes table. Promote to a typed key only if a vertical proves it needs structured filtering. |
| 4 | **Doc language** (English-only rule) | **RESOLVED 2026-05-25** | Owner: docs MAY be Russian; everything else (code, DB identifiers, data values) English. This spec is English (allowed); `CATALOG_GAPS.md` stays Russian as an internal tracker. No forced translation. |
| **5** | **🔴 Variant grouping IN or OUT of Group D.** §2.3 Position B (7ml + 12ml = ONE master with 2 variants) REQUIRES variant *grouping* (recognizing which input rows are siblings of one family). But D1b as scoped only does *default-variants* (1:1) + NOT NULL; true grouping (1 product → N SKU) is **deferred in `CATALOG_GAPS.md` Group C** (8–12h, "cascades to curator/widget/Shopify ingest"). **Without grouping you get Position A wearing a variants costume** — and DoD 1b.5 is unsatisfiable. | **OPEN — BLOCKS D1b scope & estimate** | Recommend grouping IN (it's the whole point of the identity model, and it shares the matching machinery with Decision #6 — doing them apart = touching the same code twice). Cost: Phase 1 → ~30–40h. Alternative: Phase 1 = scaffolding only, downscope §2.3/1b.5, grouping as named follow-up. |
| **6** | **🔴 Cross-tenant matching cascade unspecified.** The whole shared-master / multi-tenancy model (INT.3/INT.7) depends on "given an incoming row, which existing master does it match?" — sku / gtin / `normalized_match_key`. Sephora has **no GTIN**, so cross-cosmetics-tenant dedup leans on fuzzy name+brand (this is B5 conflict-resolution territory, deferred). Spec never specifies the cascade; tenant E re-fed with same IDs would be a trivial exact-match test. | **OPEN — BLOCKS INT.3/INT.7 + tenant E design** | Specify the matching cascade in D1b; build tenant E with **different/mangled SKUs** so dedup must use name+brand/gtin (real path). Decide how much of B5's conflict logic is needed for dedup vs deferred. |

---

## 11. Risks

- **D1b default-variant idempotency** — the single biggest risk. A non-idempotent default-variant
  creation duplicates variants on every re-apply / daily sync. Lock with DoD 1b.3 before moving on.
- **D6 N+1 on alias lookup** — per-row alias resolution in apply would tank throughput on the 8494-row
  Sephora seed. `DimAliasBatch` (batch lookup) is mandatory, not optional.
- **Expand-contract discipline** — if a Phase-1 change accidentally drops/renames a `TEXT[]` column that
  V5 still reads, search breaks. Keep old columns until Phase 2 explicitly contracts them.
- **Seed prerequisite** — multi-tenancy DoD cannot be fully satisfied until the overlap seed exists (§8.2).
```
