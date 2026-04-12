# feature/engine-v4 — KeepstarCanvas Phase 3 (`<tenant_design_context>` in Agent2 prompt)

- **Branch**: `feature/engine-v4`
- **Date (UTC)**: 2026-04-12 23:57 UTC / 2026-04-13 02:57 MSK
- **Parent commit**: `de81801` (feat(v4): KeepstarCanvas Phase 2 — tenant preset loader)
- **Commit sha**: `5b25962`

## Context

Phase 2 (`de81801`) wired tenant-scoped published presets into the V4 chat
runtime — if Agent2 happens to pick a preset name that a tenant has published,
the tenant's version is served. But Agent2 was still blind to the tenant's
design system: the tool schema listed only the 12 global presets via an
`enum`, and the system prompt said nothing about what the tenant has
published. Tenants with brand-new preset names (e.g. `banana_card`) were
unreachable because Agent2 never saw the option.

Phase 3 closes the loop:

1. **`<tenant_design_context>` block** — appended to Agent2's system prompt
   after the existing `<fields>` block. Lists the tenant's published presets
   (with binds and description), components (name + description), and design
   tokens (grouped by category, with theme axes). Compact format, ~400–500
   tokens for a typical tenant library.

2. **Version-aware cache** — the loader now tracks
   `admin.tenant_design_context_version` (the monotonic counter Phase 1
   already bumps on every publish/delete/token edit). The full system prompt
   is memoized per `(tenantSlug, version)`. A publish in admin invalidates
   the prompt on the very next Agent2 turn — no more 60s TTL lag for the
   prompt. The individual preset `Resolve()` cache is also invalidated on
   version change.

3. **Dropped the `preset` enum** — the tool schema's `preset` field is now a
   free-form string with a rich description listing global fallbacks inline
   and pointing Agent2 at `<tenant_design_context>` for tenant-scoped names.
   The tool-array becomes fully static byte-for-byte across every tenant, so
   Anthropic's `[tools+system]` cache-read rate stays maximal. Runtime
   resolution is unchanged — `resolvePresetForTenant` already accepts any
   string and returns "unknown preset: X" for misses.

## Approach

### 1. New ports (engine-agnostic)

Two new port files in `ports/`:

- `CanvasComponentPort` + `CanvasComponentView` — lists published
  components. `OpsJSON` carried as `json.RawMessage` so Phase 7 (component
  render-time resolution) needs zero port rework.
- `CanvasDesignContextPort` + `CanvasDesignTokenView` — `GetVersion(ctx,
  tenantIDOrSlug)` returns the monotonic counter (0 for missing row),
  `ListDesignTokens` returns every token row ordered by category/name/theme.

### 2. New Postgres adapters

Two new adapter files mirroring `CanvasPresetAdapter`:

- `CanvasComponentAdapter` — `DISTINCT ON (c.id)` + `JOIN
  tenant_component_versions` + `JOIN catalog.tenants` + `WHERE v.status =
  'published' ORDER BY c.id, v.version DESC`. Reuses `normalizeOpsRaw`.
- `CanvasDesignContextAdapter` — `GetVersion` treats `pgx.ErrNoRows` as
  `(0, nil)`. `ListDesignTokens` uses `COALESCE` for nullable theme
  columns. Both slug/UUID tolerant.

### 3. Loader extension (version-aware caching)

`TenantPresetLoader` gains:

- `componentPort`, `designContextPort` fields (both optional).
- `versionTTL = 5s` coalescing window on version counter reads.
- `DesignContextSnapshot` struct — read-only payload with presets,
  components, tokens, version, and pre-computed `OverridesGlobal` set.
- `LoadDesignContext(ctx, tenantSlug)` — version check coalesced within 5s,
  snapshot memoized per `(tenant, version)`, rebuilds on version change.
  On version change, also invalidates per-preset `Resolve` cache entries.
- `NewTenantPresetLoaderWithPorts(preset, component, designContext)` — new
  constructor. `NewTenantPresetLoader(preset)` is a thin wrapper for Phase 2
  callers.
- Refactored `ListForTenant` to share an unlocked helper
  `listForTenantLocked` with `LoadDesignContext`.

### 4. Agent2 usecase — version-aware prompt cache

- `fieldsPromptCache sync.Map` replaced with `promptCache sync.Map` keyed by
  tenantSlug, value `*promptCacheEntry{version, prompt}`.
- `designContextLoader *engine_v4.TenantPresetLoader` field added.
- `NewAgent2ExecuteUseCase` gains a 6th parameter `designContextLoader`.
- `buildSystemPromptWithFields` renamed to `buildSystemPrompt`:
  - Fetches `LoadDesignContext` up front, compares cached version.
  - On miss or drift, rebuilds: base + `<fields>` + `<tenant_design_context>`.
  - Byte-stable output for Anthropic prompt caching.
- New `formatDesignContextBlock(snap)` — renders presets (with binds,
  description, overrides flag, replicate flag), components (name +
  description), tokens (grouped by category with theme axis). Truncation
  caps: 50 presets / 25 components / 80 tokens with `<more count="N"/>`
  footer.
- New `compactPresetBinds(p)` — walks `p.Build()` collecting
  `props["fieldName"]` in insertion order, agnostic to `Op.Type`
  normalization.
- Helper functions: `sortPresetsByName`, `sortComponentsByName`,
  `groupTokensByCategory`, `sortedCategoryKeys`, `escapeXMLInline`.

### 5. Tool schema — drop the enum

`Definition()` in `tool_visual_assembly.go`: removed `"enum":
engine_v4.PresetNames()` from the `preset` field. Replaced with a rich
description listing the 12 global fallback names inline and pointing at
`<tenant_design_context>`. Schema is now fully static — no `engine_v4`
package call from `Definition()`.

### 6. main.go wiring

- Constructs `NewCanvasComponentAdapter(dbClient)` and
  `NewCanvasDesignContextAdapter(dbClient)` in the `dbClient != nil` block.
- Calls `NewTenantPresetLoaderWithPorts(preset, component, designContext)`.
- Passes `tenantPresetLoader` as the new 6th arg to
  `NewAgent2ExecuteUseCase`.

## Files changed

| File | Kind | Notes |
|---|---|---|
| `project_v4/backend/internal/ports/canvas_component_port.go` | new | `CanvasComponentPort` + `CanvasComponentView` |
| `project_v4/backend/internal/ports/canvas_design_context_port.go` | new | `CanvasDesignContextPort` + `CanvasDesignTokenView` |
| `project_v4/backend/internal/adapters/postgres/canvas_component_adapter.go` | new | ~80 LOC; slug/UUID tolerant; latest-published-only |
| `project_v4/backend/internal/adapters/postgres/canvas_design_context_adapter.go` | new | ~90 LOC; GetVersion (ErrNoRows→0), ListDesignTokens |
| `project_v4/backend/internal/engine_v4/tenant_preset_loader.go` | modified | +`DesignContextSnapshot`, +`LoadDesignContext`, +`NewTenantPresetLoaderWithPorts`, +`listForTenantLocked` helper, version-aware invalidation |
| `project_v4/backend/internal/engine_v4/tenant_preset_loader_test.go` | new | 5 test functions: CachesByVersion, VersionFetchedOncePerTTL, NilTolerance, OverridesGlobal, Phase2Compat |
| `project_v4/backend/internal/usecases/agent2_execute.go` | modified | `promptCacheEntry`, `designContextLoader` field, `buildSystemPrompt` (renamed), `formatDesignContextBlock` + helpers |
| `project_v4/backend/internal/usecases/agent2_execute_test.go` | modified | +7 test functions: BasicShape, EmptyInput, Truncation, CompactPresetBinds ordering/nil, TokensWithThemeAxis |
| `project_v4/backend/internal/tools/tool_visual_assembly.go` | modified | `preset` field: enum dropped, rich description added |
| `project_v4/backend/cmd/server/main.go` | modified | +component/design-context adapters, `NewTenantPresetLoaderWithPorts`, +loader arg to Agent2 constructor |

## Verification

**Compile + vet**: `cd project_v4/backend && go build ./... && go vet ./...`
— both clean.

**Admin sanity**: `cd project_admin/backend && go build ./... && go vet ./...`
— clean. Admin wasn't touched.

**Unit tests**: `go test ./internal/engine_v4/... ./internal/usecases/... ./internal/tools/...`
— all pass. 5 new loader tests + 7 new usecase tests + all existing tests green.

**Manual end-to-end** (to be run by user — needs live DB with Phase 1 schema):

```bash
# 1. Start everything
scripts/start_all.sh

# 2. As tenant A, create + publish a custom preset with a brand-new name
TOKEN_A=$(curl -s -X POST http://localhost:8081/admin/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"owner@heybabescosmetics.com","password":"..."}' | jq -r .token)

curl -s -X POST -H "Authorization: Bearer $TOKEN_A" \
  -H 'Content-Type: application/json' \
  -d '{
    "name":"banana_card",
    "category":"product",
    "entityType":"product",
    "description":"Yellow promo card for flash sales",
    "defaultReplicate":true,
    "ops":[
      {"op":"insert","ref":"w","parent":"formation","props":{"type":"widget"}},
      {"op":"insert","ref":"root","parent":"$w","props":{"type":"column"}},
      {"op":"insert","parent":"$root","props":{"type":"image","fieldName":"images","slot":"hero"}},
      {"op":"insert","parent":"$root","props":{"type":"text","fieldName":"name","slot":"title"}},
      {"op":"insert","parent":"$root","props":{"type":"number","fieldName":"price","format":"currency","slot":"price"}}
    ]
  }' \
  http://localhost:8081/admin/api/canvas/presets | jq .

# 3. Publish it
PID=<from step 2>
curl -s -X POST -H "Authorization: Bearer $TOKEN_A" \
  "http://localhost:8081/admin/api/canvas/presets/$PID/publish" | jq .

# 4. Upsert design tokens
curl -s -X POST -H "Authorization: Bearer $TOKEN_A" -H 'Content-Type: application/json' \
  -d '{"category":"color","name":"accent","value":"#FFD600"}' \
  http://localhost:8081/admin/api/canvas/tokens
curl -s -X POST -H "Authorization: Bearer $TOKEN_A" -H 'Content-Type: application/json' \
  -d '{"category":"radius","name":"card","value":"12px"}' \
  http://localhost:8081/admin/api/canvas/tokens

# 5. Open chat widget for tenant A → send "show me products"
#    (No 60s wait — version counter invalidates immediately on next turn.)

# 6. Inspect /debug/traces/<session>:
#    - Agent2.systemPrompt has BOTH <fields entity="product"> AND <tenant_design_context version="N">
#    - <tenant_design_context> lists banana_card with binds=images, name, price
#    - <tokens> section has color.accent + radius.card
#    - Agent2.toolInput has preset="banana_card"
#    - Resulting formation reflects the tenant's ops

# 7. Tenant isolation: tenant B sees no <tenant_design_context>

# 8. Cache warmth: 2 more turns without publishing — cacheReadInputTokens > 0

# 9. Publish bump: delete a token → version increments → next turn rebuilds

# 10. Fallback: boot without DATABASE_URL → no <fields>, no <tenant_design_context>
```

## Known gaps / caveats

- **Component render-time resolution not yet.** Phase 3 only exposes
  component names/descriptions via the context block. The `$ref` expansion
  for components at `ApplyOps` time is Phase 7. `OpsJSON` is carried on
  `CanvasComponentView` now so Phase 7 needs no port rework.
- **NOTIFY-based invalidation not yet.** The 5s `versionTTL` coalescing
  window is good enough for now. Postgres `LISTEN/NOTIFY` would cut max
  staleness to ~0s but is out of scope.
- **LRU / bounded cache size not yet.** `sync.Map` of tenant caches grows
  unbounded. Same gap as Phase 2; expected <100 tenants, no concern.
- **Service field definitions in Agent2 prompt.** `buildSystemPrompt` still
  only reads `domain.EntityTypeProduct` for the `<fields>` block. Deferred.
- **Seed script.** No helper to copy global preset ops into a tenant draft.
  Manual curl is the only path today.
- **Shared-DB coupling is load-bearing.** V4 chat backend now reads from
  `admin.tenant_presets`, `admin.tenant_components`,
  `admin.tenant_design_context_version`, and `admin.tenant_design_tokens`
  directly. Same cluster constraint as Phase 2 — not new, just expanded.
