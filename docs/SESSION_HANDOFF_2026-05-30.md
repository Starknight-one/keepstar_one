# Session Handoff — 2026-05-30 — PIM-backed demo data plane

> Written while context was rich; this is the source of truth to resume from.
> Goal of the arc: **get the chat widget working again, rendering products from the
> NEW decomposed services** (PIM + Connector + Price-Stock → v5). This session built &
> PROVED the data plane end-to-end on one vertical (furniture).

## TL;DR — where we are
A demo tenant **`pim-furniture-demo`** now lives in the LIVE admin catalog DB with
**1209 fully-hydrated furniture cards** (name + price + image + category + rating +
384-dim embedding), sourced from the **new clean PIM (canonical)** + a **new
Price-Stock service (price/stock)** via a new **projection seam**. The deployed v5 reads
that DB, so it can already serve this tenant.
**Remaining to "open widget → type query → see result":** point the deployed v5/widget
at slug `pim-furniture-demo`, and apply two small prompt fixes (A1/A2). That's it.

## Architecture (the clean boundaries — decided & built this session)
| Concern | Owns | Form | Status |
|---|---|---|---|
| **PIM** | canonical: name, brand, category, specs, description (+ image_url attr) | service (own Neon) | ✅ live, clean |
| **Price-Stock** | price, stock (+ denormalized rating) | **NEW service** (own Neon) | ✅ built, tested |
| **Seam / Indexer** (`pim-to-catalog`) | reads PIM + Price-Stock → builds v5 read-model | **JOB** in project_admin (owns nothing) | ✅ built, proven |
| **read-model** = admin `catalog.*` (+ projection/embeddings) | what v5 queries (CQRS cache) | tables | ✅ has the demo tenant |
| **v5** | generative engine; reads read-model | deployed service | ✅ live |
| **CMS** | per-tenant presentation overrides | later | ⏸ deferred |

Services link via **events (PIM outbox) + a gateway**, NOT a central orchestrator. The
seam is a one-shot JOB now; promote later to an event-driven indexer (consume PIM outbox).

## Built this session
- **Connector** (`Keepstar_Connector`): deterministic engine gained **array-index paths**
  (`categories.1`, `images.0.large`) + a **`join`** transform (`internal/core/mapping/engine.go`,
  `transform.go`). New **`amazon.FurnitureMapping` / `FurnitureAttributeDefs`** — ~15 curated
  furniture specs + category-from-array + joined description + image_url
  (`internal/sources/amazon/amazon.go`). New **`cmd/seed-pim`** concurrent loader. Tests green.
- **NEW service `Keepstar_PriceStock/`** (Go, hexagonal like PIM/connector, own Neon):
  `Offer{tenantId, productRef, priceCents, currency, stock, rating}`; `POST/GET /offers`;
  `cmd/seed-offers`. Tests green. Owns price/stock ONLY.
- **Seam** (`Keepstar_one_ultra/project_admin/backend/cmd/pim-to-catalog/main.go`): reads PIM
  (its Neon) + Price-Stock (HTTP) → writes admin `catalog.*` via the proven
  `CatalogV2WriterAdapter` (`BulkMatchOrCreateMaster` / `BulkMergeTier2` / `BulkUpsertListing` +
  a supplementary rating/images UPDATE) → `RebuildSearchProjection` (384-dim OpenAI embeddings).

## Live infra (project IDs only — get connstrings via Neon dashboard / `mcp__neon__get_connection_string`; passwords NOT stored here)
- **PIM Neon**: `snowy-lab-31162996` — ~1280 furniture products.
- **Price-Stock Neon**: `crimson-credit-83527435` — 3000 offers for `pim-furniture-demo`.
- **Admin (live) Neon**: `flat-moon-68826275` — the `catalog.*` read-model the deployed v5 reads.
  Tenant `pim-furniture-demo` = id `c7fddf9c-4409-43ff-ac9c-4be3444fd2dc`, 1209 listings + projection rows.
- **Local helpers this session** (gone when shells close — restart to re-run the seam):
  `/tmp/keepstar_pim` (PIM HTTP :8090), `/tmp/pricestock` (:8095). The seam calls Price-Stock at :8095.
- **Deployed** (have all keys/env): v5 `v5-engine-production.up.railway.app`, admin `admin-production-4ae4...`.
  Railway MCP token expired → needs `railway login` for Railway ops.

## Verified
- **Card hydration** (tenant pim-furniture-demo, search "bed"): GAOPAN Velvet Sectional Sofa
  $517.65 / Boyd Sleep bed frame $33.69 / Evelots storage $37.99 — each with name, USD price,
  Amazon-CDN image, brand, category, and a non-null 384-dim vector.
- **PIM internal layout is CLEAN**: every product has exactly 1 default variant + 1 sku
  (0 products without a variant); all ~15 attributes at PRODUCT level; variants/skus carry no
  attrs; every furniture product categorized. Correct single-variant shape for Amazon parent-level data.

## OPEN PROBLEMS (raised by Vlad — solve next)
1. **WRITE is absurdly slow vs READ.** Loading 3k took ~35 min and only ~1280 landed.
   **Root cause (diagnosed):** the connector's PIM HTTP client times out on big `/ingest` batches —
   each call carried ~250 records and PIM processes each record in its OWN transaction → ~3 min/call
   (PIM logs show `duration: 2m59.99s` pegged at the client deadline) → client gives up + retries
   while PIM keeps grinding server-side → redundant work + lost records (seed-pim reported `total=0`
   yet ~1280 landed). **Reading is one indexed query; writing is N per-record network transactions.**
   FIXES to evaluate: (a) much smaller PIM `/ingest` batches (~20–50 rec) so each finishes well under
   the timeout; (b) raise the connector PIM-client timeout; (c) PIM-side: batch many records per
   transaction instead of per-record; (d) bump Neon compute during bulk loads. **This is THE blocker
   to loading full catalogs.**
2. **Neon compute almost always ON (cost).** Likely causes: PIM's **outbox relay polls every 2s**
   (`RELAY_INTERVAL=2s`) → pins the PIM compute awake; plus the long-running local `/tmp` services hold
   pooled connections. Stop the local services + raise/disable the relay interval (or `suspend_timeout`)
   to let computes sleep. Investigate per-project `suspend_timeout_seconds` (PIM was 0 = never suspend).
3. **Only ~1280/3000 furniture in PIM** (a consequence of #1). 1209 projected — plenty for demo;
   scale after fixing #1, then re-run the seam.

## RESUME PLAN — get the widget rendering
1. **See it:** point the DEPLOYED v5 widget at tenant slug **`pim-furniture-demo`** (data is already in
   flat-moon). Find the widget embed/URL + how it takes the tenant — recipe is in the workflow output
   `…/5cefa42c-…/tasks/wnvsxlcw8.output` (agent `scope:widget`). Run a query ("sofas under $300", "beds")
   → confirm generative cards render. A plain render should work without redeploy.
2. **A1/A2 prompt fixes** (low-risk, exact edit text in `wnvsxlcw8.output`, agents `scope:a1`/`scope:a2`):
   A1 = route greeting → `text_explainer` (not `empty_not_found`) in `project_v5/.../prompts/agent2_prompt.go`;
   A2 = force `catalog_search` on category change in `agent1_execute.go` / `agent1_prompt.go`. Then redeploy v5.
3. **Later:** fix write-slowness (#1) → scale to full 3000 → re-run `/tmp/pim-to-catalog` → promote seam to
   event-driven indexer (PIM outbox consumer). Other verticals (Sephora/Electronics) need their own rich maps.

## How to re-run the seam (after restarting local PIM:8090 + Price-Stock:8095)
`/tmp/pim-to-catalog -admin-db <flat-moon connstr> -pim-db <snowy-lab connstr> -pricestock http://localhost:8095 -pim-source furniture-demo -tenant pim-furniture-demo -vertical furniture`
(needs `OPENAI_API_KEY` in a loaded `.env` for embeddings — it was present this session.)

## Memory & context
- `~/.claude/projects/-Users-starknight-Keepstar-project/memory/project-demo-path.md` (+ MEMORY.md index)
  holds the locked plan, IDs, and status — survives context compaction.
