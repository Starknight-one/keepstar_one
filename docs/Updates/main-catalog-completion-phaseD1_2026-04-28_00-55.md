# Catalog Completion — Phase D1 (extended discovery agent) shipped

- **Branch:** `main`
- **Date (UTC):** 2026-04-28 00:55
- **Parent commit:** `68ffd25` (Phase A2 cleanup CLI)
- **Active plan:** `~/.claude/plans/synchronous-twirling-hoare.md` — Phase C session converged on a single-agent / declarative-schema / deterministic-applier design.

## Context

After the Phase C design conversation we agreed on a single-agent model: one Sonnet call per tenant produces a **declarative `MappingArtifact`** that fully describes how the tenant's catalog should map into our schema AND how each listing should merge with master. A deterministic Go applier (Phase D2, next commit) walks the listings without any further LLM calls and emits a `merge_report` of per-listing proposals.

The existing M4c `DiscoveryAgent` already produces ~70% of that artifact (FieldMapping, CategoryMapping, MasterTemplates, MatchStrategy as a string array, AgentNotes). What was missing for the merge half:

- **BrandMapping** — what to do with each tenant vendor (link / create-new / skip)
- **JunkRules** — vendor blacklist + axis-name patterns + require-identifier flag
- **MatchStrategyConfig** — structured form of MatchStrategy with score thresholds
- **Master overview** in the agent's system prompt — so it doesn't have to drill via `find_similar_masters` to find out which brands and verticals are already on master

Phase D1 closes those gaps in the agent. The applier itself (D2) lands next.

## What landed

### Domain extension — `MappingArtifact` (additive, fully backwards-compat)

`internal/domain/mapping_artifact.go` adds three optional fields:

- `BrandMapping map[string]BrandMappingTarget` — vendor → action with optional master_brand and reason
- `JunkRules *JunkRules` — vendor_blacklist, axis_name_patterns, require_identifier (default true)
- `MatchStrategyConfig *MatchStrategyConfig` — order, auto_link/needs_review/skip thresholds, embedding_disabled_for verticals

All three are pointers / maps with `omitempty`, so existing artifacts on disk (no merge fields) continue to deserialize cleanly. Older code paths reading legacy `MatchStrategy []string` still work — the structured config supersedes only when present.

### `MasterOverview` port + adapter

New types in `ports.MasterVariantsPort`:

- `GetMasterOverview(ctx) (*MasterOverview, error)` — three SQL queries in `master_variants_adapter.go` (verticals + top-10 brands per vertical / master_categories per vertical / master_field_definitions registry)

Compact output (~500-1000 tokens for a tenant like heybabes):

```
verticals_present: cosmetics(979)
cosmetics:
  top_brands: COSRX, MEDI-PEEL, The Saem, Some By Mi, ...
  master_categories: face-care, hair-care, ...
registered_tier2_fields: (none yet)
```

Fed into the discovery agent's system prompt at run start. Prompt cache picks it up because the system block already has `CacheControl: ephemeral`. Adding the overview costs ~$0.02 once per run (cache write), then ~$0.0002 per subsequent turn (cache reads at 10%).

### Three new tools on the discovery agent

- **`propose_brand_mapping(vendor, action, master_brand?, reason?)`** — agent builds BrandMapping incrementally. Vendor key normalized to lowercase+trimmed by the builder.
- **`propose_junk_rule(rule_type, value)`** — three rule_types: `vendor_blacklist`, `axis_name_pattern`, `require_identifier` (the third takes "true"|"false"). Builder lazily inits JunkRules.
- **`set_match_strategy(order, auto_link_threshold, needs_review_threshold, skip_below, embedding_disabled_for)`** — single call, replaces the structured config. Validates threshold ordering (skip < needs_review < auto_link).

Tool descriptions in the system prompt explain when each fires + the expected default behaviour if the agent skips them.

### `ArtifactBuilder` extension

New setters: `SetBrandMapping`, `AddJunkVendor`, `AddJunkAxisPattern`, `SetRequireIdentifier`, `SetMatchStrategyConfig`. Concurrency-safe via the existing mutex. `Build()` snapshots the new fields with defensive slice copies so post-Build mutations don't bleed into the persisted artifact.

### `run-discovery` CLI

`cmd/run-discovery/main.go` — fires the agent on a tenant outside the HTTP path. Same pattern as `cmd/sync-tenant-now`: decrypt token via `secretbox`, build adapters, call `DiscoveryUseCase.Run`, write the full `DiscoveryRunResult` (status + transcript + token counts) to `discovery-<shop>-<ts>.json`. Used to iterate on the agent prompt without spinning up the admin server.

## Verification — live run on dev-store

```
DATABASE_URL=… ADMIN_ENCRYPTION_KEY=… ANTHROPIC_API_KEY=… \
go run ./cmd/run-discovery -shop keepstar-neaqpan1.myshopify.com
```

Result:

```
status:           needs_human_review   ← validation found 56% coverage; agent did commit
stop_reason:      commit_artifact
turns:            20
input_tokens:     431,960
output_tokens:    8,627
duration_ms:      145,241
field_mappings:   24
category_mappings:6
master_templates: 3   (furniture, lighting, footwear)
```

Tool histogram from the saved transcript confirms the new tools fired cleanly:

| Tool | Calls |
|---|---|
| propose_brand_mapping | **18** (every distinct vendor) |
| propose_field_mapping | 24 |
| find_similar_masters | 9 |
| propose_category_mapping | 6 |
| sample_records | 5 |
| propose_junk_rule | **4** |
| describe_field | 4 |
| propose_master_template | 3 (furniture / lighting / footwear) |
| set_match_strategy | **1** |
| commit_artifact | 1 |

Anthropic's reported cost for this run was ~$0.40 (40¢) — system prompt + tools cached after turn 1, only output tokens + new conversation deltas billed at full rate.

## Files changed

| File | Action |
|---|---|
| `internal/domain/mapping_artifact.go` | EDIT (+50 lines: BrandMappingTarget, JunkRules, MatchStrategyConfig) |
| `internal/ports/master_variants_port.go` | EDIT (+30 lines: MasterOverview, MasterVerticalSummary, MasterFieldDefSummary) |
| `internal/adapters/postgres/master_variants_adapter.go` | EDIT (+90 lines: GetMasterOverview impl) |
| `internal/usecases/discovery_agent.go` | EDIT (+120 lines: builder setters, master overview wire, prompt extensions) |
| `internal/usecases/discovery_tools.go` | EDIT (+150 lines: 3 new tool defs + dispatch + handlers) |
| `internal/usecases/match_cascade_test.go` | EDIT (test fake stubs out GetMasterOverview) |
| `cmd/run-discovery/main.go` | NEW (~140 lines, CLI for ad-hoc agent runs) |

`go build && go vet && go test ./internal/...` — all clean.

## Cost analysis (re-run for the user's question)

Sonnet 4.6 pricing:
- Input $3 / 1M, Output $15 / 1M, Cache write ×1.25, Cache read ×0.10

Per-tenant cost is **independent of catalog size** — it's bound by the number of distinct vendors / collections / metafield keys, not by the total number of products. Empirically on dev-store with 20 products: 56 tool calls, 20 turns, ~$0.40. A real-world tenant with 5k-50k products and ~30 vendors / ~10 collections / ~30 metafield keys would do roughly the same number of turns and cost $0.40-$1.00 per discovery run.

This is the design's main win over per-listing LLM merging: linear-in-product-count cost (e.g. $50 for 10k products) collapses to constant-per-tenant cost.

## Known gaps / Phase D2 next

1. **Deterministic applier** — the entire merge half is set up in the artifact, but nothing yet *consumes* it. Phase D2: write `internal/usecases/merge_apply.go` that walks `catalog.products` for the tenant, applies BrandMapping → vertical resolution → field_mapping → match cascade with MatchStrategyConfig → JunkRules, and emits a `merge_report` of per-listing proposals. Curator approves the report (per-row or bulk by confidence), final transactional writer hits master/listings.
2. **`merge_reports` table** — schema lives in the plan; needs a migration in Phase D2.
3. **Curator UI for schema review** — Phase D3. Right now the artifact lives in `catalog.tenant_catalog_schema.mapping_artifact` JSONB; curator can edit raw JSON via an SQL client, but a structured form (categories tab, brands tab, junk-rules tab) is the next polish step.
4. **Validation update** — `ValidateArtifact` currently scores coverage on FieldMapping only. It should now also check BrandMapping covers all vendors in the meta-report and JunkRules account for skipped collections. Same code path, additive checks.
5. **Default thresholds when agent skips `set_match_strategy`** — the applier (D2) needs to fall back to defaults (auto=0.90 / needs_review=0.50 / skip=0.30) when MatchStrategyConfig is nil. The struct field is already a pointer; D2 implements the fallback.
