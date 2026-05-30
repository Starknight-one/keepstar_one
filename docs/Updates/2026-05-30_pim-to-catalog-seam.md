# 2026-05-30 — Projection seam `cmd/pim-to-catalog` (PIM-backed demo tenant)

Full session handoff: `../../../SESSION_HANDOFF_2026-05-30.md`.

## Context
Build the demo on the NEW decomposed services: project canonical PIM data + per-tenant
commercial facts (a new Price-Stock service) into the admin `catalog.*` read-model that
the deployed v5 already reads — so the chat widget renders furniture from the new stack.

## Added
- **`project_admin/backend/cmd/pim-to-catalog/main.go`** — the projection seam (a JOB, not a
  service; owns no data). Reads PIM canonical (its Neon) + Price-Stock offers (HTTP) and
  writes the read-model via the existing `CatalogV2WriterAdapter`:
  `ensure tenant → upsert master_categories → BulkMatchOrCreateMaster → category junction →
  BulkMergeTier2 (specs) → BulkUpsertListing (price/stock from Price-Stock) → supplementary
  UPDATE (rating + images) → RebuildSearchProjection (384-dim embeddings)`.
  Join key PIM↔Price-Stock = source product id (Amazon ASIN via `external_id_map`).

## Result (verified)
- Tenant `pim-furniture-demo` (id c7fddf9c-4409-43ff-ac9c-4be3444fd2dc) in the live admin Neon
  (flat-moon-68826275): **1209 listings + 1209 projection rows**, cards hydrate fully
  (name + price + image + category + rating + vector). Verified via v5's SearchProjection SQL.
- One follow-up applied: empty listing-name → set `catalog.products.name = master name`
  (and the seam now sets `CustomTitle`) so cards always show a title.

## Not changed
v5 engine code untouched this session. A1/A2 prompt fixes are scoped but NOT applied (see handoff).

## Boundaries
PIM owns canonical; Price-Stock owns price/stock; this seam owns nothing (CQRS read-model
builder). Promote later to an event-driven indexer (PIM outbox consumer); never a god-orchestrator.
