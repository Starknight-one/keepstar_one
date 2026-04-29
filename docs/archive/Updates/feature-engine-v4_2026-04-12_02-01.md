# feature/engine-v4 — KeepstarCanvas Phase 2 (tenant preset loader in V4)

- **Branch**: `feature/engine-v4`
- **Date (UTC)**: 2026-04-11 23:01 UTC / 2026-04-12 02:01 MSK
- **Parent commit**: `4ed581c` (feat(admin): KeepstarCanvas Phase 1 — preset CRUD backend)
- **Commit sha**: `f86c5c7`

## Context

KeepstarCanvas Phase 1 landed the admin-side CRUD for tenant-scoped presets
(`4ed581c`). Phase 2 picks up the other half: make the V4 chat backend
actually serve those presets to Agent2 at render time. The survivable plan is
`docs/New features/KEEPSTAR_CANVAS_PLAN.md` (§ Phase 2).

End-goal for this phase: when a tenant publishes their own `product_card`
via the admin canvas API, the very next Agent2 turn on that tenant's widget
should render the tenant's version instead of the global Go-registry preset.
Tenants without custom presets continue to see the global fallbacks (`product_card`, `product_detail`, `empty_not_found`, ...) exactly as before — zero
behavioural change for the 99% of tenants that never publish anything.

## Approach

All work lives in `project_v4/backend/`. Admin backend is untouched — it's
already the producer side; Phase 2 is pure consumer wiring.

### 1. Ports stays engine-agnostic

`ports.CanvasPresetView` carries ops as `json.RawMessage` so the port layer
doesn't import `engine_v4`. The `TenantPresetLoader` inside `engine_v4` is the
single place where JSON bytes become typed `[]Op`. Keeps the layering clean
and mirrors how `FieldDefinitionPort` avoids pulling engine types into ports.

Two methods on the port:

- `GetPublishedPreset(ctx, tenantIDOrSlug, name)` — hot path for Agent2 turns.
  Returns `(nil, false, nil)` on missing row; error only on actual DB failures.
- `ListPublishedPresets(ctx, tenantIDOrSlug)` — shipped now so Phase 3's
  `<tenant_design_context>` prompt block has the data it needs without
  re-plumbing.

### 2. Postgres adapter accepts slug OR UUID

`CanvasPresetAdapter` joins `admin.tenant_presets + admin.tenant_preset_versions + catalog.tenants` with `WHERE (p.tenant_id::text = $1 OR t.slug = $1)` so callers can pass whichever they have, identical to how `FieldDefinitionAdapter`
already handles it. In practice, V4 request flow carries a slug
(`req.TenantSlug`), so the slug branch is the hot path.

Latest-published-only semantics:

- `GetPublishedPreset` uses `WHERE v.status = 'published' ORDER BY v.version DESC LIMIT 1` — exactly one result, the newest published revision.
- `ListPublishedPresets` uses `DISTINCT ON (p.id) ... ORDER BY p.id, v.version DESC` — one row per preset, each its newest published version.

Draft rows and archived rows are invisible to this adapter by design; old
chat sessions stay reproducible because published versions are immutable.

### 3. TenantPresetLoader = TTL cache + global fallback

`engine_v4/tenant_preset_loader.go` is the Phase 2 linchpin.

- Parses ops JSON once at cache-write time (`viewToPreset`) so Agent2 hot
  paths hit only a map lookup.
- Caches both positive and negative results for 60s (plan-specified TTL).
  Without the negative cache every miss for a vanilla tenant would re-query
  Postgres on every Agent2 turn.
- Two entry points: `Resolve(ctx, tenantSlug, name)` for the top-level
  preset param and `ResolverFor(ctx, tenantSlug)` which returns a closure
  consumable by `ExpandInlinePresets`.
- `ListForTenant` is wired and cached but currently unused — Phase 3 will
  call it to build the `<tenant_design_context>` block.
- Nil-tolerant at every layer (nil loader, nil port, empty tenant). Matches
  the boot-without-DB pattern used elsewhere in V4.

### 4. Resolver threaded through the engine pipeline

Minimum-diff strategy to preserve the legacy zero-resolver path (used by
tests, testbench, and any other caller of `ApplyOps`):

- New thin wrapper `ApplyOpsWithResolver(formation, ops, resolver)`; old
  `ApplyOps(formation, ops)` now just calls it with a nil resolver.
- Same pattern for `ExpandInlinePresetsWithResolver` vs
  `ExpandInlinePresets`.
- `ExecuteInput` gains a `PresetResolver` field. `engine.Execute` passes it
  to `ApplyOpsWithResolver`.
- Inside `ExpandInlinePresetsWithResolver`, a new `resolvePreset(resolver, name)`
  helper consults the resolver first and falls back to the package-level
  `GetPreset`. Zero callers of the public `ExpandInlinePresets` / `ApplyOps`
  had to change.

### 5. VisualAssemblyTool wiring

- `NewVisualAssemblyTool` now takes an optional `*engine_v4.TenantPresetLoader`.
- `Execute` sets `engineInput.PresetResolver = t.tenantPresets.ResolverFor(ctx, toolCtx.TenantSlug)` once per turn — both the top-level preset param and
  any inline per-widget `props.preset` references honour per-tenant overrides.
- Top-level preset lookup goes through a new `resolvePresetForTenant` helper
  that preserves the "unknown preset" error message when both tenant and
  global miss.

### 6. main.go + tool registry wiring

- `main.go` constructs `postgres.NewCanvasPresetAdapter(dbClient)`, wraps it
  with `engine_v4.NewTenantPresetLoader(...)`, and passes the loader into
  `tools.NewRegistry(...)`. Nil when `dbClient == nil`.
- `tools.NewRegistry` gained a `tenantPresets *engine_v4.TenantPresetLoader`
  parameter and forwards it into `NewVisualAssemblyTool`. Admin + testbench
  callers unaffected because only `main.go` calls `NewRegistry`.

## Files changed

| File | Kind | Notes |
|---|---|---|
| `project_v4/backend/internal/ports/canvas_preset_port.go` | new | `CanvasPresetPort` + `CanvasPresetView` (raw ops bytes) |
| `project_v4/backend/internal/adapters/postgres/canvas_preset_adapter.go` | new | ~150 LOC; slug/UUID tolerant; latest-published-only semantics |
| `project_v4/backend/internal/engine_v4/tenant_preset_loader.go` | new | ~210 LOC; TTL cache, pos/neg caching, nil-tolerant |
| `project_v4/backend/internal/engine_v4/types.go` | modified | `ExecuteInput.PresetResolver` + `PresetResolver` func type |
| `project_v4/backend/internal/engine_v4/presets.go` | modified | `ExpandInlinePresetsWithResolver` + `resolvePreset` helper; old `ExpandInlinePresets` is a thin wrapper |
| `project_v4/backend/internal/engine_v4/ops.go` | modified | `ApplyOpsWithResolver` variant; old `ApplyOps` is a thin wrapper |
| `project_v4/backend/internal/engine_v4/engine.go` | modified | `engine.Execute` routes to `ApplyOpsWithResolver` with the input resolver |
| `project_v4/backend/internal/tools/tool_visual_assembly.go` | modified | constructor takes loader; `Execute` threads `ResolverFor` into engine input; new `resolvePresetForTenant` helper |
| `project_v4/backend/internal/tools/tool_registry.go` | modified | `NewRegistry` gains tenantPresets arg |
| `project_v4/backend/cmd/server/main.go` | modified | construct adapter + loader + pass into registry |

## Verification

**Compile + vet**: `cd project_v4/backend && go build ./... && go vet ./...`
— both clean.

**Unit tests**: `go test ./internal/engine_v4/... ./internal/tools/... ./internal/usecases/...`
— all pass. The existing `tool_visual_assembly_test.go` exercises the
zero-resolver path; no regression. `engine_v4` tests exercise `ApplyOps` and
`ExpandInlinePresets` via their legacy signatures; no regression there either.

**Admin sanity**: `cd project_admin/backend && go build ./... && go vet ./...`
— clean. Admin wasn't touched, but this confirms nothing upstream of the
shared schema got tangled.

**Manual end-to-end** (to be run by user — needs live DB with Phase 1 schema):

```bash
# 1. Start everything
scripts/start_all.sh

# 2. As tenant A, create + publish a custom product_card
TOKEN_A=$(curl -s -X POST http://localhost:8081/admin/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"owner@heybabescosmetics.com","password":"..."}' | jq -r .token)

# Minimal "tenant override" preset — a single widget with one text atom.
# Real tenants will copy the global product_card ops and tweak; this
# minimal shape just proves the loader picks up the override.
curl -s -X POST -H "Authorization: Bearer $TOKEN_A" \
  -H 'Content-Type: application/json' \
  -d '{
    "name":"product_card",
    "category":"product",
    "entityType":"product",
    "description":"tenant override test",
    "defaultReplicate":true,
    "ops":[
      {"op":"I","ref":"w","parent":"formation","props":{"type":"widget"}},
      {"op":"I","ref":"root","parent":"$w","props":{"type":"column"}},
      {"op":"I","parent":"$root","props":{"type":"text","fieldName":"name","textStyle":{"fontSize":"xl","fontWeight":"bold"}}}
    ]
  }' \
  http://localhost:8081/admin/api/canvas/presets | jq .

# 3. Capture the new preset ID and publish it
PID=<from step 2>
curl -s -X POST -H "Authorization: Bearer $TOKEN_A" \
  "http://localhost:8081/admin/api/canvas/presets/$PID/publish" | jq .

# 4. Open the chat widget for tenant A → send "покажи товары"
#    Wait 60+ seconds if you have an older Agent2 turn cached; fresh
#    sessions see the tenant version on turn 1.
# 5. Inspect /debug/traces/<session>:
#    - Agent2 toolInput shows preset="product_card"
#    - Resulting formation reflects the single-text-atom override
#      (not the full global product_card with image/price/rating/brand)

# 6. Tenant isolation check: log in as tenant B, send "покажи товары",
#    confirm they STILL see the global product_card (no bleed).

# 7. Fallback check: as tenant A, send "what's pricing?" — if agent1 finds
#    nothing it triggers empty_not_found (system preset). Confirm the
#    frontend renders the global empty state (no custom override exists).
```

## Known gaps / caveats

- **No invalidation via NOTIFY yet.** Cache is strict 60s TTL, so a publish
  in admin becomes visible within 60s. Phase 3 will tie invalidation to
  the `admin.tenant_design_context_version` counter that Phase 1 already
  bumps on every publish. Until then, do not try to demo "publish → edit →
  re-publish → refresh" in < 60s and expect the second revision instantly.
- **`ListForTenant` is plumbed but unused in the request path.** Phase 3
  will call it from `agent2_execute.buildSystemPromptWithFields` to build
  the `<tenant_design_context>` block. Keeping it wired now costs nothing
  and removes a refactor later.
- **No seed script.** The admin curl flow above is the only way to populate
  a tenant override today. The plan defers a seed helper ("copy global
  product_card into a tenant draft") until we want it for automated tests.
- **Tool schema enum still lists only global presets.**
  `PresetNames()` in `engine_v4.presets` is tapped directly by the tool
  definition, so tenants can only override names that already exist in the
  global registry. Brand-new tenant-only names (e.g. `banana_card`) are not
  reachable via `preset="banana_card"` — Agent2 won't see the option. This
  gap is intentional for Phase 2: overrides of existing names is the 80%
  value. Phase 3 widens the enum (or swaps it for a per-tenant dynamic list)
  alongside the design-context prompt block.
- **LRU / max cache size: not yet.** `sync.Map` with TTL entries grows
  unbounded if a tenant has thousands of preset names. Real tenants won't
  hit this (expected <30 presets/tenant), but worth a note if we ever see
  a process RSS spike.
- **Shared-DB coupling is load-bearing.** V4 chat backend now reads from
  `admin.tenant_presets` directly. The two services must stay on the same
  Postgres cluster or the loader fails closed to global fallback. Already
  true for the metadata binding port, so this isn't new — just re-stated.
- **Legacy V1/V2 backend (`project/backend/`) is untouched.** V4 is the
  only consumer of the canvas preset tables.
