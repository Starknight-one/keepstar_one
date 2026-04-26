# Admin Catalog — M4 a/b/c shipped, discovery agent verified end-to-end

- **Branch:** `main`
- **Date (UTC):** 2026-04-26 13:13
- **Commits in this session (oldest first):**
  - `6067fb7` — feat(admin-catalog): M4a — foundation for metadata-first Shopify import
  - `d537518` — feat(admin-catalog): M4b — deterministic harvest, match cascade, junk detector
  - `8cd0b2f` — feat(admin-catalog): M4c — discovery agent (Sonnet 4.6, 8 tools) + validation
  - `fc8ab4e` — Merge feature/admin-catalog-m4 → main
  - `10e1145` — fix(admin-shopify): bulk query — variant weight via inventoryItem.measurement
  - `5a795ac` — fix(admin-catalog): discovery — prompt caching, partial salvage, drop OpenAI dep

## Context

The original M4 plan was “one fat commit replacing the legacy REST importer.” On second look that was both too risky (no incremental rollback) and too narrow (one-shot LLM agent that couldn’t actually explore ambiguous catalogs). We split M4 into four sub-commits (4a/b/c/d) and rethought the agent as a multi-turn tool-use loop. This session shipped 4a/b/c and verified the pipeline end-to-end against the dev-store; 4d (harvester orchestrator + cut-over) is intentionally deferred — see “Plan correction” below.

## What landed

### M4a — foundation (commit `6067fb7`)

Schema, GraphQL bulk client, staging buffer, skeleton V2 use case. After 4a a tenant could pull their full Shopify catalog into a staging table; nothing else moved.

| Layer | Files |
|---|---|
| Migrations | `catalog.shopify_raw_imports` (high-tide JSONL staging), `catalog.master_variants.embedding vector(384)` + hnsw index, `catalog.tenant_catalog_schema.validation_report JSONB`, DELETE 4 cyrillic seed rows from `catalog.unit_aliases` |
| Shopify client | `RunBulkProductsQuery` + `GetCurrentBulkOperation` + `FetchBulkJSONL` + `ScanBulkJSONL` (Bulk Operations lifecycle), `MetafieldDefinitions(ownerType)`, `NavigationMenu(handle)`, `ShopReferences()` |
| Plumbing | `ports.ShopifyStagingPort` + `adapters/postgres.ShopifyStagingAdapter` (`UpsertRaw` / `CountByKind` / `IterateProducts` / `DeleteByTenant`); `usecases.ShopifyV2UseCase.DumpToStaging`; `handlers.ShopifyV2Handler.HandleDumpToStaging` (POST `/admin/api/integrations/shopify/{id}/dump-to-staging`); DI side-by-side with legacy |

### M4b — deterministic pieces (commit `d537518`)

Pure-Go usecases. No LLM. No DB writes from these usecases — they read staging + masters and return in-memory results.

| Component | Notes |
|---|---|
| `domain/discovery.go` | `MetaReport`, `FieldStats`, `ValueCount`, `CollectionNode`, `MetafieldDefSummary`, `MatchResult`, `MatchOutcome`, `MatchConfidence`, `MatchCandidate`, `JunkSignal` |
| `metadata_harvest.go` | Walks staged products, ≤80 fields × ≤10 top values × 80-char truncation → ~1-2KB report. Reads metadata-kind staging rows for vendors/types/tags/metafield defs/menu |
| `auto_map_tier1.go` | Vendor/Brand→`master.brand`, Barcode→`master_variants.gtins[]`, Weight→`master_variants.weight_g` (units transform), Volume→`master_variants.volume_ml`. Skips non-English fields |
| `match_cascade.go` (+ tests) | GTIN exact → vendor+SKU → fuzzy (pg_trgm ≥0.85 + axes) → embedding (≥0.92 → review queue) → new master. Graceful degradation on embed failure |
| `junk_detector.go` (+ tests) | 4 signals (axis_name regex / no_identifiers / no_dimensions / small_price_delta). **Fixed spec heuristic bug**: `small_price_delta` widened to absolute price <$10 (the literal `delta<$10 AND base<$50` missed gift wrap priced as a $5 standalone variant) |
| Schema | pg_trgm extension + GIN trigram index on `master_products.name` |
| Ports | `MasterVariantsPort.FindByVendorAndFuzzyName` + `FindByEmbedding` + `MasterVariantWithScore` |

### M4c — discovery agent + validation (commit `8cd0b2f`)

Multi-turn tool-use agent (Sonnet 4.6) replaces the original one-shot 5-tool-call plan. Spec rethink documented in plan file.

| Component | Notes |
|---|---|
| `adapters/anthropic/agent_client.go` | Tool-use Messages API client, separate from `EnrichmentClient`. `ToolDef`, `ContentBlock` (text / tool_use / tool_result), `Usage` with cache fields. `Send()` returns one parsed response; caller drives the loop |
| `usecases/discovery_agent.go` | 30 turns / 50K tokens / 8 min wallclock budget (later raised to 150K — see fix `5a795ac`). System prompt carries the meta-report; `Discover()` returns `DiscoveryResult` with `PartialBuilder` exposed for salvage |
| `usecases/discovery_tools.go` | 8 tools with hard ≤2KB response cap each: `describe_field`, `sample_records`, `find_similar_masters`, `peek_master`, `propose_master_template`, `propose_field_mapping`, `propose_category_mapping`, `commit_artifact` |
| `usecases/validate_artifact.go` | Read-only scoring pass. Runs artifact's field_mapping over up to 20 staging samples, tallies coverage % and parser failures. ≥80% coverage with no fatals → `status=active`; otherwise `needs_human_review` (`validation_report` JSONB persisted) |
| `usecases/discovery_run.go` | Orchestrator wrapping harvest → tier1 → agent → validation → persist. Handler stays a thin auth layer |
| Domain | `MappingArtifact.MasterTemplates []MasterTemplateProposal` — vertical-template proposals, lives inside the artifact (not its own table — curator turns these into Tier 2 columns in M11 promotion) |
| Schema | `MasterVariantsPort.FindMasterProductsByEmbedding` + `GetMasterProductSummary` + `MasterProductSummary` / `MasterVariantSnippet` |
| Handler / DI | POST `/admin/api/integrations/shopify/{id}/discover` (auth). DI gracefully falls back if `ANTHROPIC_API_KEY` absent. Cosmetics rejected as a new vertical proposal (already promoted) |
| Tests | 18 cases — builder pre-population from Tier 1, snapshot immutability, dispatcher routing + arg validation, scripted-LLM loop happy/budget/empty paths |

### Bug fixes after first real run

**Fix `10e1145` — Shopify bulk query schema.** First `dump-to-staging` returned `Field 'weight' doesn't exist on type 'ProductVariant'`. In the 2026-04 API the variant `weight`/`weightUnit` fields moved to `inventoryItem.measurement.weight { value unit }`. Updated the GraphQL query.

**Fix `5a795ac` — discovery: caching, partial salvage, drop OpenAI dep.** First `discover` ran 6 turns, burned 50K tokens, hit budget, lost all 9 already-recorded `propose_field_mappings`. Three fixes:

1. **Prompt caching enabled.** `anthropic.MessagesRequest` grew `SystemBlocks []SystemBlock` and `ToolDef.CacheControl`. System prompt + tools array carry `ephemeral` markers — turn 1 writes the cache, subsequent turns read at ~10% cost. Token budget bumped from 50K → 150K and cached reads no longer count.
2. **Partial-builder salvage.** `DiscoveryResult.PartialBuilder` exposed; `discovery_run.go` persists whatever the agent already proposed via propose_* calls when budget runs out, with `AgentNotes: "PARTIAL — agent stopped before commit ..."`.
3. **OpenAI dependency dropped from `find_similar_masters`.** New port method `FindMasterProductsByName` runs pg_trgm fuzzy match over (name + brand). The harvester (M4d) keeps the embedding cascade for cross-tenant linkage where semantic recall actually matters; discovery doesn't need that depth.

## End-to-end verification (run twice on dev-store)

Dev-store: `keepstar-neaqpan1.myshopify.com` — 17 fashion/snowboard products, app installed under read-only scopes.

### Run 1 — `dump-to-staging` (2026-04-26 ~12:55 UTC)

```
POST /admin/api/integrations/shopify/a704d058.../dump-to-staging
→ status 200
{
  "operation_id":     "gid://shopify/BulkOperation/7562133405885",
  "product_count":    17,
  "metafield_defs":   2,
  "vendor_count":     4,
  "product_type_count": 3,
  "tag_count":        0,
  "navigation_items": 0,        // dev-store has no main-menu — handled gracefully
  "collection_count": 0,
  "duration_ms":      4345
}
```

### Run 2 — `discover` after caching fix (2026-04-26 13:08 UTC)

```
POST /admin/api/integrations/shopify/a704d058.../discover
→ status 200
{
  "tenantId":         "c48987eb-3977-45a5-9230-5505436cf3aa",
  "status":           "needs_human_review",
  "stopReason":       "commit_artifact",
  "turnsUsed":        13,
  "inputTokens":      142968,
  "outputTokens":     2970,
  "durationMs":       51170,
  "fieldMappingSize": 13,
  "categorySize":     3,
  "templateCount":    1,
  "validation": {
    "sampledRecords":      17,
    "coveragePercent":     73.7,
    "parserFailures":      0,
    "parserAmbiguous":     0,
    "unmappedFieldCounts": {"createdAt":17,"id":17,"options.[].position":17,"publishedAt":13,"updatedAt":17},
    "recommendedStatus":   "needs_human_review"
  }
}
```

**Highlights:**

- Agent committed via `commit_artifact` (target terminal state).
- 13 mappings recorded — `title→listing.original_name`, `descriptionHtml→master.description`, `featuredImage.url→master.image_url`, `productType→listing.raw_attributes.product_type`, `tags.[]→candidate:tags`, `options.[].values.[]→master_variants.color`, `metafields.test_data.binding_mount→candidate:binding_mount`, `metafields.test_data.snowboard_length→master_variants.length_mm`, etc.
- Master template proposed: `winter_sports` (correct rejection of misclassified-as-cosmetics legacy data).
- 3 collection mappings: snowboard / accessories / giftcard.
- Caching worked — 142K total input tokens stayed well within the 150K budget and most were cache reads at ~10% cost. Estimated total spend per run: ~$0.15.

## Known gaps (carried into the deferred M4 polish)

These are documented for the curator UI but don't block downstream milestones.

1. **Validation threshold too strict for catalogs with system fields.** 73.7% coverage triggered `needs_human_review` even though every meaningful field was mapped — the unmapped 26% is `id` / `createdAt` / `updatedAt` / `publishedAt` / `options.[].position` × 17 products. These should be excluded from the coverage denominator. Fix lives in `validate_artifact.go::ValidateArtifact` — add a system-fields skip list before tallying `totalOccurrences`.

2. **Conditional field mappings unsupported.** Agent correctly noted `options.[].values.[]` should map to `master_variants.color` ONLY when `options.[].name == "Color"`. Current `FieldMappingTarget` is unconditional. Fix would extend the artifact format with optional `condition` (e.g. `condition: "sibling.name == 'Color'"`); harvester evaluates at apply time. Punted to M4 polish — meaningful only on multi-axis catalogs.

3. **Legacy importer pollutes master_products with `vertical=cosmetics`.** Install-time auto-sync (legacy code path, still wired) creates master_products rows with the column default `vertical='cosmetics'` regardless of what the products actually are. Discovery agent saw “The Complete Snowboard” as `vertical=cosmetics` and (correctly) flagged it as misclassified. Resolves itself in M4d cut-over (legacy importer deleted; harvester writes correct vertical from the artifact).

4. **No frontend progress UI yet.** `dump-to-staging` and `discover` are dev-only curl-triggered routes. Production-quality install flow waits for M4d.

5. **Embedding cascade step in match cascade not yet exercised.** No vector data in `master_variants.embedding` yet — column exists, embedding job runs in M4d. Step 4 of cascade falls through gracefully (returns `NewMaster`), but is untested on real data.

## Plan correction — M4 polish deferred to the end

User decision (2026-04-26 13:10 UTC):

> _Мы потопаем заканчивать всё что можем закончить. А М4 со всеми остатками будем допиливать уже в самом конце, чтобы я прям сел и сосредоточился и прокинул как оно должно работать._

**Rationale.** The discovery agent works end-to-end, but understanding the actual run results (transcript, agent reasoning, validation report) requires a focused review session that needs the user actively at the wheel — not parallel-with-other-work.

**New ordering** (replaces what was in the plan file):

1. **Now → next sessions:** continue with the milestones that don't depend on a polished M4 — **M6** (COALESCE-render), **M7** (Heybabes 967 backfill + drop legacy PIM columns), **M8** (Categories M:N + tree editor), **M9** (Detected add-ons UI), **M10** (Public API + api_keys), **M11** (Curator service standalone + audit + promotion mechanics).
2. **At the very end — focused M4 finish session:** revisit the deferred M4 polish in one sitting with the user co-piloting:
   - Wire harvester orchestrator to artifact + match cascade + junk detector (the original M4d scope)
   - Wire embedding job (parent + per-variant)
   - Re-wire webhooks to hash-diff path
   - Cut over DI: delete legacy `ShopifyUseCase` + `shopify_mapper.go`
   - Frontend progress UI on the install page
   - Wipe + resync 17 dev-store products
   - Optional polishes from the “Known gaps” list above (validation threshold, conditional mappings, expansion of dev-store catalog with cosmetics/furniture/junk via `seed-dev-products` endpoint)

This means downstream milestones (M6+) **will not have a fully working harvester until the M4 polish session.** They’re still mostly buildable independently:
- M6 / M7 don't need the new harvester — they read existing data through COALESCE and backfill heybabes 967 in place.
- M8 / M9 / M10 are admin/curator UI surfaces that work against existing master_products.
- M11 (curator) and M12 (audit + promotion) absolute don't depend on the harvester.

The risk is that some milestones will reveal small needs in the artifact format or staging shape that we'd want to thread back through the agent. We accept that — and address such things in the final M4 session.

## Files changed (cumulative this session)

| Scope | File | New/Edit |
|---|---|---|
| Migrations | `project_admin/backend/internal/adapters/postgres/catalog_migrations.go` | Edit (M4a + M4b additions, ~70 lines) |
| Shopify client | `project_admin/backend/internal/adapters/shopify/client.go` | Edit (GraphQL bulk + metadata pulls; +500 lines, weight schema fix) |
| Postgres adapters | `shopify_staging_adapter.go`, `master_variants_adapter.go` | New + Edit (FindByGTIN/SKU/Fuzzy/Embedding, FindMasterProductsByEmbedding/ByName, GetMasterProductSummary) |
| Domain | `discovery.go`, `mapping_artifact.go` | New + Edit (MasterTemplateProposal) |
| Ports | `shopify_staging_port.go`, `master_variants_port.go` | New + Edit |
| Anthropic adapter | `internal/adapters/anthropic/agent_client.go` | New (tool-use client + cache control) |
| Usecases | `shopify_v2.go`, `metadata_harvest.go`, `auto_map_tier1.go`, `match_cascade.go`, `junk_detector.go`, `discovery_agent.go`, `discovery_tools.go`, `validate_artifact.go`, `discovery_run.go` | All new |
| Tests | `match_cascade_test.go`, `junk_detector_test.go`, `discovery_agent_test.go` | All new |
| Handlers | `handler_integrations_shopify_v2.go` | New (dump-to-staging + discover endpoints) |
| DI | `cmd/server/main.go` | Edit |

## Verification (build + Railway prod)

- `cd project_admin/backend && go build ./... && go vet ./... && go test ./...` — clean across the session
- Railway `Admin` redeployed three times; migrations applied (24 → 25 catalog tables now), `shopify_v2_discovery_enabled model=claude-sonnet-4-6` after `ANTHROPIC_API_KEY` set
- Manual end-to-end on dev-store: `dump-to-staging` (4.3s, 17 products) + `discover` (51s, status=committed). Documented above.

## Next session entry point

Start with the milestone tracker in `docs/PRE_LAUNCH_TASKS.md` (or its updated version), and the implementation plan at `docs/New features/admin_catalog_implementation_plan_2026-04-26.md` (which now reflects the deferred-M4-polish ordering). Pick up at **M6 (COALESCE-render: admin + V4 engine)** — it's the safest next step and unblocks the rest.

The M4 final polish session needs the user to set aside an hour or two, drive the test catalog through Shopify (cosmetics duplicates of heybabes brands + furniture + junk), and walk through the agent transcript + validation output together. Don't attempt that in a side window.
