# Chunk 5 — micropresets: reusable v9 RefNode components + second preset

**Branch**: v5
**Plan author**: Claude (Opus 4.7)
**Reference**: `docs/v5-engine-plan.md` (Order of attack, item 5 — "Microprésets discipline"), chunks 1-4 in `docs/Updates/v5/`

---

## Context

Chunk 4 shipped one preset (`product_card`) inline — five Frames with the atoms hard-wired into the JSON. That's not what makes V5 different from V4. The architectural claim of V5 is that **v9 RefNode + descendants overrides** lets a tenant define a reusable component once and consume it from many presets, with per-instance variation through overrides. Until we exercise that, we have not validated the v9-foundation choice.

Chunk 5 is the cheapest way to falsify the claim: build 2 reusable components, refactor `product_card` to use them via Refs, port a structurally different second preset (`product_card_list_row`) that **reuses the same components** with different layout, and verify the full pipeline still binds correctly under replicate × component-expansion together.

If this works end-to-end, the v9 foundation is real. If it doesn't, we know now (cheap to course-correct) rather than at chunk 6+ when the LLM is in the loop.

User-confirmed direction: future canvas microservice will write to V5's tables, so the schema designed here is the FINAL shape. Apply the same discipline to `v5_components` as we did to `v5_presets`.

---

## Approach

### 1. Component storage — mirror preset schema

`v5_components` + `v5_component_versions` in V5's own Neon schema. Same column shape, same draft/published/archived workflow, same `(component_id, version)` UNIQUE.

```sql
CREATE TABLE IF NOT EXISTS v5_components (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          UUID NOT NULL REFERENCES catalog.tenants(id) ON DELETE CASCADE,
    name               VARCHAR(200) NOT NULL,
    category           VARCHAR(100) NOT NULL DEFAULT 'atom',
    description        TEXT NOT NULL DEFAULT '',
    latest_version_id  UUID,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(tenant_id, name)
);
CREATE TABLE IF NOT EXISTS v5_component_versions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    component_id    UUID NOT NULL REFERENCES v5_components(id) ON DELETE CASCADE,
    version         INTEGER NOT NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'draft'
                    CHECK (status IN ('draft','published','archived')),
    doc_json        JSONB NOT NULL,
    author_user_id  UUID,
    published_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(component_id, version)
);
```

`doc_json` for a component is a tiny `engine.Document` with exactly **one top-level child** — the component's root Frame/Text/etc. Multi-root components are not on the table for chunk 5; they'd just need a downstream loop change.

### 2. Domain + Port + Adapter

Direct copy of the chunk-4 preset shape, renamed:
- `internal/domain/component.go` — `Component` struct + `ComponentStatus` enum (mirror of `Preset` exactly; `DocumentJSON json.RawMessage` to avoid the same domain → engine cycle).
- `internal/ports/component_port.go` — `ComponentPort.GetPublishedComponent(tenant, name)` + `ListPublishedComponents(tenant)`.
- `internal/adapters/postgres/component_migrations.go` — DDL.
- `internal/adapters/postgres/postgres_component.go` — adapter mirroring `postgres_preset.go`. Reuses `resolveTenantID` (already lives in `postgres_preset.go`; move to a shared helper file `tenant_resolve.go` so both adapters can call it).
- `internal/adapters/postgres/postgres_component_integration_test.go` — Get / NotFound / List against live Neon, mirroring chunk-4 preset tests.

### 3. Pipeline helper — `Materialise`

New `internal/engine/materialise.go`:

```go
func Materialise(preset *Document, components []*Document) *Document
```

Appends each component's `doc.Children[0]` to a deep-clone of the preset's Children, with `reusable: true` stamped on the appended root. Result: one merged Document where preset RefNodes can resolve to component bodies via `FindNodeByID`.

Why a clone of the preset: the loaded preset is potentially shared (cache, tests reusing fixtures); we don't want to mutate the cached input. Cheap deep-clone via `cloneNode` already exists.

Constraints / behaviour:
- Component with empty Document.Children → skipped silently (caller error, not engine error).
- Multi-child components → only `Children[0]` is appended in chunk 5; tracked in `v5-known-gaps.md`.
- Duplicate ID across components → no dedup, last appended wins (refs would resolve to one of them via `FindNodeByID` returning first match — a known but acceptable footgun for chunk 5).

### 4. Pipeline helper — `ResolveAndInline`

New `internal/engine/resolve_inline.go`:

```go
func ResolveAndInline(doc *Document) ResolveStats
```

Walks `doc.Children` and every nested frame/group's children; for each `NodeTypeRef` child, calls `cr.Expand(refNode, doc)` and **replaces** the ref child with the resolved tree in-place. Non-ref children pass through. Returns count of refs resolved + count failed (+ list of failed ref ids, for diagnostics).

Why a separate function (and not in `ComponentResolver.ExpandAll`): chunk 1's `ExpandAll` returns a map and does not mutate the doc — that's the v9-compatible read-only surface. Chunk 5 introduces the in-place mutation as a deliberate pipeline step that the binding layer depends on. Keeping `ExpandAll` untouched preserves chunk-1 tests.

The ResolveStats return value lets the e2e test assert how many refs were resolved (chunk-5 expectation: 3 refs per cloned card × 3 clones = 9 successful expansions for the product_card test).

### 5. BindData — skip `reusable: true` subtrees

`internal/engine/binding.go` `bindNodeRecursive` adds: if `n["reusable"] == true`, return without descending. Reasoning: after `Materialise`, the component definitions live at the top of `doc.Children`. We do not want `BindData` to bind their `fieldBinding` atoms (they're templates, not instances). They're already inlined where consumed via Refs.

Single-line check at recursion entry; behaviour for non-reusable nodes is unchanged. Chunk-3 + chunk-4 binding tests stay green.

### 6. Pipeline order

```
PresetAdapter.GetPublishedPreset       → preset.Document
ComponentAdapter.ListPublishedComponents → []Component
       ↓
Materialise(preset, components)         → merged *Document
       ↓
(ApplyOps — none yet; chunk 6+)
       ↓
ExpandReplicates(doc, count)            → replicate fan-out
       ↓
ResolveAndInline(doc)                   → refs replaced with their bodies
       ↓
BindData(doc, data)                     → atoms filled, reusable subtrees skipped
```

`ExpandReplicates` runs **before** `ResolveAndInline` for two reasons:
- Each cloned RefNode keeps its `ref` pointer + receives `dataIndex` via inheritance from the cloned ancestor at bind time
- After ResolveAndInline, refs are gone — replicating expanded subtrees would require deep-cloning a much larger structure

The plan-agent for chunk 4 already validated this order; chunk 5 just uses it.

### 7. Three seed components + refactored / new presets

**`price_rating` component** (multi-atom):
- Frame "price-rating-root" (row, gap=md)
  - Number atom id=`pr-price`, `fieldBinding: priceFormatted`
  - Number atom id=`pr-rating`, `fieldBinding: rating`

**`brand_badge` component** (single-atom + wrapper):
- Text atom id=`brand-badge-root`, `fieldBinding: brand`, `wrapper: badge`

**`product_card` preset (refactored)** — replace inline price/rating/brand atoms with refs to the components above:
```
card frame (replicate:true)
├── hero frame → image atom (fieldBinding:heroImage)
├── info frame
│   ├── text "title" (fieldBinding:name, xl/bold)
│   ├── ref id="card-meta", ref="price-rating-root"   ← was inline
│   └── ref id="card-brand", ref="brand-badge-root"   ← was inline
├── actions frame (empty)
└── specs frame (empty)
```

**`product_card_list_row` preset (new)** — chosen via Explore agent as best second preset:
- Row layout (vs column)
- Adds `description` atom (text, lineClamp:2 — V4 styling concept; we just store the field, frontend renderer handles overflow later)
- Reuses both components — same fieldBindings, different layout context
```
row-card frame (replicate:true, row layout, gap=md)
├── hero frame → image atom (fieldBinding:heroImage)
└── info frame (column, gap=xs)
    ├── text "row-title" (fieldBinding:name, md)
    ├── text "row-desc" (fieldBinding:description, sm)
    ├── ref id="row-meta", ref="price-rating-root"
    └── ref id="row-brand", ref="brand-badge-root"
```

Both preset JSONs go in `internal/engine/presets/seed/` and get embedded via go:embed in `presets.go`.

### 8. Tests

Unit:
- `materialise_test.go` — empty components → preset clone returned unchanged-shape; one component appended; reusable:true stamped; preset original NOT mutated; multi-child component takes only Children[0]
- `resolve_inline_test.go` — ref pointing at top-level reusable resolved; non-existent ref left in place + counted in stats; nested refs (a ref inside a resolved tree) resolved recursively; reusable component definition is NOT removed from doc.Children (covered by binding skip)
- `binding_test.go` — reusable subtree fully skipped (its fieldBinding atom does NOT bind to data[0]); non-reusable subtree under a reusable parent is NOT reachable (single-rule "skip subtree" semantics). Add 2-3 cases.
- `presets_test.go` — round-trip guards on the two new components + the new list_row preset (mirrors chunk-4 product_card guard)

Integration (live Neon):
- `postgres_component_integration_test.go` — Get / NotFound / List, mirroring preset adapter tests
- `engine_pipeline_integration_test.go` — extend to seed both components + both presets, then run the full pipeline TWICE (once per preset) and assert:
  - product_card: 3 cloned cards × 2 refs each → 6 refs resolved; per-clone heroImage / name / price / rating / brand bind to the matching product
  - product_card_list_row: same products, different shape — 3 cloned rows × 2 refs each → 6 refs resolved; description atom binds to its product's description field (skipped per-product if the catalog row has no description)
  - Reusable components (top-level) are present in doc.Children but their atoms are NOT in `BindResult.Bound` (skip semantics)
  - Same `dataIndex` flows through replicate × ref expansion (closes the chunk-4-known-gap implicit risk)

### 9. Files

| File | Status |
|---|---|
| `project_v5/backend/internal/domain/component.go` | added |
| `project_v5/backend/internal/domain/component_test.go` | added |
| `project_v5/backend/internal/domain/errors.go` | modified — `+ErrComponentNotFound` |
| `project_v5/backend/internal/ports/component_port.go` | added |
| `project_v5/backend/internal/adapters/postgres/component_migrations.go` | added |
| `project_v5/backend/internal/adapters/postgres/postgres_component.go` | added |
| `project_v5/backend/internal/adapters/postgres/postgres_component_integration_test.go` | added |
| `project_v5/backend/internal/adapters/postgres/tenant_resolve.go` | added — extracted `resolveTenantID` + `isUUID` helpers shared by preset & component adapters |
| `project_v5/backend/internal/adapters/postgres/postgres_preset.go` | modified — call shared `resolveTenantID` from new file |
| `project_v5/backend/internal/adapters/postgres/postgres_state_integration_test.go` | modified — `setupClient` runs `RunComponentMigrations` too |
| `project_v5/backend/internal/adapters/postgres/engine_pipeline_integration_test.go` | modified — extend e2e to cover both presets + components |
| `project_v5/backend/internal/engine/materialise.go` | added |
| `project_v5/backend/internal/engine/materialise_test.go` | added |
| `project_v5/backend/internal/engine/resolve_inline.go` | added |
| `project_v5/backend/internal/engine/resolve_inline_test.go` | added |
| `project_v5/backend/internal/engine/binding.go` | modified — skip subtree if `reusable:true` |
| `project_v5/backend/internal/engine/binding_test.go` | modified — +2-3 cases for reusable skip |
| `project_v5/backend/internal/engine/presets/presets.go` | modified — embed new seed files |
| `project_v5/backend/internal/engine/presets/presets_test.go` | modified — round-trip guards on new seeds |
| `project_v5/backend/internal/engine/presets/seed/product_card.json` | modified — refactored to use refs |
| `project_v5/backend/internal/engine/presets/seed/product_card_list_row.json` | added |
| `project_v5/backend/internal/engine/presets/seed/component_price_rating.json` | added |
| `project_v5/backend/internal/engine/presets/seed/component_brand_badge.json` | added |
| `docs/Updates/v5/plans/chunk-5-micropresets.md` | added — frozen plan |
| `docs/v5-known-gaps.md` | modified — register chunk-5 deferrals |

Total estimate: ~1.6K LOC over 25 files (most are tiny seeds + tests).

---

## Critical files & helpers to reuse

- `engine/component_resolver.go:33-122` — `Expand`, `expandRef`, `applyDescendantOverrides`, `processSlots`. All chunk-1 work; chunk 5 wraps it.
- `engine/component_resolver.go:204 cloneNode` — used by Materialise to clone the preset before mutation.
- `engine/replicate.go reIDSubtree` — pattern to study; ResolveAndInline does not need it (resolved subtrees inherit IDs from the cloned source nodes; if a downstream test wants unique ids per ref expansion, that's a chunk-6 concern).
- `engine/binding.go:64 bindNodeRecursive` — extension point for the reusable-skip rule.
- `adapters/postgres/postgres_preset.go` — template for `postgres_component.go` (~80% verbatim).
- `adapters/postgres/postgres_state_integration_test.go::setupClient` — single migration entry point; chunk 5 just appends one more migration call.

## Known gaps to register in `docs/v5-known-gaps.md`

| Gap | Reason | Closes |
|---|---|---|
| Multi-root components — `Materialise` only appends `Document.Children[0]` from each component | One-root is the natural shape; multi-root would need either a separate `materialise mode` or component packs. Not needed today. | Re-evaluate when canvas microservice ships its first multi-root component |
| ID collisions across components — Materialise has no dedup; if two components define a node with the same id, `FindNodeByID` returns whichever is appended first. | Chunk 5 has 2 components with deterministically-distinct IDs; the failure mode is real but not exercised. | Canvas-microservice chunk — enforce uniqueness at write time, or namespace component IDs at read time |
| `ResolveAndInline` mints no fresh IDs on resolved subtrees — multiple refs to the same component yield trees with the same descendant IDs | Each tree lives under a distinct ref's id (line 98 of `expandRef` enforces it on the root), so `FindNodeByID` won't collide at the top level. Inner-id collisions only matter for path-deep ops which V5 does not support yet. | Constraints / path-deep ops chunk (TBD) |
| `BindData` skip rule is "subtree under `reusable:true` is fully skipped" — even legitimate inner refs inside a reusable wouldn't bind | Reusables are templates; binding inside them is meaningless until they're inlined. | No re-evaluation needed unless we change the materialisation model |

## Verification

```sh
cd project_v5/backend
go build ./...                                    # OK
go build -tags=integration ./...                  # OK
go vet ./... && go vet -tags=integration ./...    # OK

go test ./...                                     # all unit tests pass
go test ./internal/engine/... -v                  # ≥30 engine tests green (chunks 1+3+4 + new materialise/resolve/binding-skip)

# Live Neon:
TEST_DATABASE_URL=$DATABASE_URL go test -tags=integration ./...
# Expected ≥14 integration tests green:
#   chunk-3/4: catalog (1) + field defs (1) + state (2 + 3 scanDeltas)
#   chunk-4: preset adapter (3) + e2e (1)
#   chunk-5: component adapter (3) + extended e2e (replaces chunk-4 e2e with two-preset version)
```

What to watch in next chunks:
- Chunk 6 (LLM in the loop) — if `<fields>` block in the prompt references atom IDs *inside* a refactored component, the LLM might emit ops targeting IDs that are inside reusable subtrees rather than expanded instances. Need to think about whether the prompt should expose component IDs at all, or only the consuming preset's RefNode IDs.
- Token efficiency claim: chunk 5 doesn't measure tokens (no LLM yet). Chunk 6 is where v5 vs v4 numbers happen for real. The architecture-validation here is structural, not behavioural.
