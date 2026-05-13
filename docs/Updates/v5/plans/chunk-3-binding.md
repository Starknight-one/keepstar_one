# Chunk 3 — port V4 binding layer to V5 scene-graph

> Chunks 1-2 done (`5a0a89e`, `0746a07`). Detailed plans pinned in
> `docs/Updates/v5/plans/`. Living gaps registry in `docs/v5-known-gaps.md`.

## Context

V5 has the engine (chunk 1) and state+delta-stream (chunk 2). What's missing
is the bridge from "I have N products from search" to "atoms in the
scene-graph carry the right values". That's the binding layer — V4's moat,
because it lets the LLM emit slot-level intents (`fieldBinding:"price"`)
instead of spelling out values, which is the difference between cheap
tool-calls and expensive ones.

This chunk ports V4's binding mechanism to V5 with three substantive
adaptations that came out of the deep dive (research notes captured below):

1. **`fieldBinding` on scene-graph nodes**, not `fieldName` on Formation
   atoms. Same idea, different host structure.
2. **ProductToMap precedence reversed**: Typed > Tier2 > Extra (V4 had
   Typed > Extra > Tier2 — curator's Tier2 lost to raw vendor Extra). This
   matches the merge-apply DB-side rule which already says "curator wins"
   (`merge_apply_tx_adapter.go:102`).
3. **Honest `__bound` flag** — V4 sets it `true` blindly after replicate
   even when the field didn't resolve, which produces empty cards (Грабля
   #1 in `docs/v5-known-gaps.md`). V5 marks bound only when a value
   actually landed.

**LLM-text inserts** stay as-is from v9: a TextNode without `fieldBinding`
just carries `content` directly. The LLM writes the string into `content`
through ops; binding skips such nodes. No new field needed for "static
literal vs bound" — presence of `fieldBinding` is the discriminator.

## User decisions (confirmed before planning)

- **Q1 — Precedence**: Typed > Tier2 > Extra. Curator > raw import.
- **Q2 — Catalog ownership**: V5 reads from the shared `catalog.*` schema
  (owned by `project_admin`), no V5-side migrations for catalog.
  `field_definitions`, `master_products`, `master_variants`, `products` —
  same tables as V4.
- **Q3 — TextNode static value**: yes, use v9 TextNode `content` for
  LLM-static text. Verify on tests.

## What we port (V4 sources)

| V4 file | LOC | What we extract |
|---|---|---|
| `domain/atom_entity.go` | ~140 | AtomType, AtomSubtype, AtomFormat, AtomSlot, AtomDisplay enums (FieldDefinition references them) |
| `domain/tenant_entity.go` | ~20 | Tenant struct (FieldDefinition adapter joins to tenants by slug) |
| `ports/catalog_port.go` lines 9-28 | ~30 | ProductFilter (used by ListProducts) |
| `ports/catalog_port.go` lines 43-100 | ~60 | CatalogPort interface — port subset (ListProducts, GetProduct, ListServices, GetService, GetTenantBySlug) |
| `ports/field_definition_port.go` | 41 | FieldDefinitionPort + FieldDefinition struct |
| `adapters/postgres/postgres_catalog.go` lines for read methods | ~400 | Postgres CatalogAdapter — ListProducts + helpers (mergeProductWithMaster, scanProduct) |
| `adapters/postgres/field_definition_adapter.go` | 251 | Postgres FieldDefinitionAdapter — verbatim |
| `tools/tool_visual_assembly.go` lines 357-422 | 65 | ProductToMap (with reversed precedence) + ServiceToMap |
| `engine_v4/binding.go` | 62 | BindData logic (rewritten for scene-graph) |

Total port surface: ~1.0K LOC, mostly verbatim except BindData (rewrite for
scene-graph node walk) and ProductToMap (precedence change).

## Scope

**In:**
1. Port AtomType/Subtype/Slot/Format/Display enums + Tenant struct + ProductFilter to V5 domain
2. Port subset of CatalogPort (read-only: ListProducts, GetProduct,
   ListServices, GetService, GetTenantBySlug) + Postgres adapter
3. Port FieldDefinitionPort + Postgres adapter (verbatim)
4. ProductToMap + ServiceToMap with new precedence (Typed > Tier2 > Extra)
5. BindData adapted to scene-graph: walks `Document.Children` recursively,
   finds nodes with `fieldBinding` attribute, fills the appropriate target
   attribute (TextNode → `content`, image-style nodes via fills → `fills[0].image`)
6. Per-instance scope: a replicated node carries `dataIndex: N`; binding
   uses `data[N]` for that subtree. (Replication itself stays in chunk 4 —
   chunk 3 produces and consumes the marker.)
7. Honest `__bound` marker — set `true` only when value resolved
8. Smoke + per-component tests

**Out (later chunks):**
- Replication of nodes via RefNode (chunk 4 — first preset end-to-end)
- `<fields>` block in prompt + meta.Fields-from-field_definitions fix (chunk 4)
- Conditional tree_map serializer for prompt (chunk 4)
- LLM/HTTP wiring (chunk 6)
- Catalog write side (lives in admin, not V5)

## Steps

### Step 1 — Domain additions
Files in `project_v5/backend/internal/domain/`:
- `atom_role.go` — AtomType, AtomSubtype, AtomFormat, AtomDisplay, AtomSlot
  string-typed enums + constants. Verbatim port from V4.
- `tenant.go` — Tenant struct (id, slug, name, created_at, updated_at).
- Tests: round-trip + constant equality

Why we still need atom-* enums: `FieldDefinition` exposes `AtomType` /
`AtomSubtype` / `DefaultSlot` so it can hint Agent2 what kind of node to
build for each field. Even on scene-graph these stay as semantic
descriptors — they map to v9 node types at preset-build time, not at
binding time. Don't conflate them with engine.NodeType (which is the actual
v9 node-type vocabulary — frame/text/rectangle/image/...).

### Step 2 — Port FieldDefinitionPort
Files:
- `internal/ports/field_definition_port.go` — verbatim (V5 imports)
- `internal/adapters/postgres/postgres_field_definitions.go` — verbatim
  port; reads `catalog.field_definitions` joined to `catalog.tenants`
- Add to integration test (`postgres_state_integration_test.go` is the
  template): one test that lists field definitions for a real tenant slug
  (heybabes-cosmetics) and one that samples values

### Step 3 — Port CatalogPort (read subset)
Files:
- `internal/ports/catalog_port.go` — interface with ProductFilter +
  VectorFilter structs + 5 read methods (ListProducts, GetProduct,
  ListServices, GetService, GetTenantBySlug). VectorSearch lives later
  when we wire embeddings (chunk 6).
- `internal/adapters/postgres/postgres_catalog.go` — port the SELECT/scan
  logic for ListProducts (with master_products LEFT JOIN, tier2 unmarshal)
  and GetProduct. Skip the write-side methods entirely.
- Skip Stock methods (binding doesn't need stock; chunk 5 if needed).
- Integration test: list products for heybabes tenant, assert at least
  one row has Tier2 populated.

### Step 4 — ProductToMap + ServiceToMap
File: `internal/engine/binding_to_map.go`
- Function `ProductToMap(p domain.Product) map[string]any` — port from
  V4 with precedence flipped: typed-fields first (winning), then Tier2
  (curator), then Extra (raw vendor). Each layer only fills missing keys.
- Function `ServiceToMap(s domain.Service) map[string]any` — analogous.
- Tests:
  - heybabes-shape product: name/price/brand/etc. land in correct keys
  - test-electronics: Extra fields (cpu, manufacturer) land in map
  - precedence: when same key in both Tier2 and Extra, Tier2 value wins
  - precedence: when typed and Tier2 conflict, typed wins (typed always
    wins — it's the canonical column)
  - empty Product → empty map (no zero values pollute)

### Step 5 — BindData on scene-graph
File: `internal/engine/binding.go`
- `BindData(doc *Document, data []map[string]any) BindResult` — walks
  the entire document tree, including expanded RefNode children.
- For each node:
  - If node has `fieldBinding` (string) attribute:
    - Resolve which `data[i]` to use:
      - If node has `dataIndex` (int) attribute → `data[dataIndex]`
      - Else (no replication context) → `data[0]`
    - Lookup `data[i][fieldBinding]`
    - If found and non-nil, write to target attribute:
      - TextNode → `content`
      - For other node types defer to a small `bindTarget(nodeType)`
        helper that knows the right attribute (image → `fills[0].image`?
        — keep it simple: only TextNode and a generic `value` fallback in
        chunk 3; image-binding properly in chunk 4 when first preset
        actually needs it)
    - Mark `__bound: true` on the node only if the write succeeded
  - Recurse into children
- `BindResult` returns `{Bound, Skipped, Missing []string}` for
  diagnostics — useful in tests and for the fix to "Грабля #1".
- Walk uses chunk-1 `Children`/`SetChildren` helpers so it copes with
  both `[]Node` and `[]any` (post-JSON-unmarshal) shapes.

### Step 6 — Tests
- `binding_test.go`:
  - product card with 3 bindable text nodes (name, price, brand) +
    1 literal text node (no fieldBinding) + 1 unbound text node
    (fieldBinding present but field missing in data)
    → bound 3, missing 1, literal untouched
  - text_explainer-shaped Document: one TextNode with `content` set
    directly, no fieldBinding → BindData leaves it untouched
  - replicate scenario via dataIndex: 3 sibling text nodes each with
    `dataIndex` 0/1/2 + same `fieldBinding:"name"` → each gets the
    matching product's name
  - precedence-via-data: Tier2 vs Extra collision (using ProductToMap
    output, not raw data)

### Step 7 — Plan copy + session log
- Copy this plan to `docs/Updates/v5/plans/chunk-3-binding.md`
- After commit: session log `docs/Updates/v5/v5_<date>_<time>.md`
- Update `docs/v5-known-gaps.md`: mark Грабля #1 (`__bound` honest) as
  **closed in chunk 3 with commit SHA**

## Files to create

```
project_v5/backend/internal/
├── domain/
│   ├── atom_role.go            # ~120 LOC enums (AtomType/Subtype/Slot/Format/Display)
│   ├── tenant.go               # ~20 LOC
│   └── *_test.go               # round-trip
├── ports/
│   ├── catalog_port.go         # ~80 LOC (read subset + ProductFilter)
│   └── field_definition_port.go # ~45 LOC verbatim
├── adapters/postgres/
│   ├── postgres_catalog.go     # ~400 LOC read methods only
│   ├── postgres_field_definitions.go # ~260 LOC verbatim
│   └── *_integration_test.go   # add to existing integration suite
└── engine/
    ├── binding_to_map.go       # ~140 LOC ProductToMap + ServiceToMap
    ├── binding.go              # ~120 LOC BindData on scene-graph
    └── binding_test.go         # 4-5 scenarios
```

Plus `docs/Updates/v5/plans/chunk-3-binding.md` (frozen copy) and a session
log on commit. Update to `docs/v5-known-gaps.md` to close Грабля #1.

## Verification

```sh
cd project_v5/backend
go build ./...                                     # OK
go test ./internal/...                             # all unit tests pass
TEST_DATABASE_URL="$DATABASE_URL" go test -tags=integration ./internal/adapters/postgres/...
                                                   # Postgres reads against live Neon
```

Acceptance criteria:
- ProductToMap precedence is Typed > Tier2 > Extra, asserted in tests
- BindData fills text nodes from data, leaves literal nodes alone, marks
  `__bound` only when a value actually landed
- FieldDefinitionPort returns rows for heybabes tenant against live DB
- One end-to-end smoke test: load heybabes product via CatalogPort →
  ProductToMap → BindData over a hand-built scene-graph → atoms have
  expected values

## Risks

- **`dataIndex` attribute name collision with v9** — chunk 1's
  ComponentResolver uses `descendants[id]` for overrides but doesn't put
  anything called `dataIndex` on nodes. Should be safe; verify by
  grepping the engine package.
- **Image-binding deferred** — chunk 3 only binds text. Image fills
  (`fills[0].image = url`) needs the v9 image-fill shape. Defer until
  chunk 4 when first preset has a real image atom.
- **Catalog adapter scope creep** — V4 CatalogPort is huge (~15 methods).
  Resist porting all of it. Read subset only: 5 methods. Vector + writes
  later as needed.
- **Tenant table dependency** — FieldDefinition adapter JOINs to
  `catalog.tenants`. We don't seed tenants in V5 tests; integration test
  uses an existing heybabes tenant from prod data. If that tenant gets
  removed, test breaks.

## Time estimate (Vlad-hours)

~2-3 hours total. Step breakdown:
- domain enums + tenant: 15 min
- field_definition port + adapter: 25 min
- catalog port + adapter (read subset): 40 min (most of the porting)
- ProductToMap + tests: 20 min
- BindData + tests: 45 min (most of the thinking)
- integration tests + plan + log: 15 min

## Resume protocol after /compact

1. Read this file (`~/.claude/plans/spicy-dreaming-waffle.md`)
2. Read `docs/v5-engine-plan.md` (high-level), `docs/v5-known-gaps.md`
   (decisions log)
3. Read `docs/Updates/v5/plans/chunk-2-state-delta.md` for chunk-2
   conventions (file layout, test patterns, integration test wiring)
4. Skim `project_v5/backend/internal/engine/document.go` and `scene_graph.go`
   to remember the Node shape and walk helpers
5. Read V4 sources listed in "What we port"
6. Continue from whatever step is incomplete per Step 1-7
