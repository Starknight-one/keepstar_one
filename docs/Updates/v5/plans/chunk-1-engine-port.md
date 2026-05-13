# Chunk 1 plan — port v9 packages/domain to Go

> Implementation plan for the first V5 chunk. Generated in plan mode and approved
> before execution. Session log: `../v5_2026-05-02_15-26.md`. Outcome:
> commit `2093bec`.

## Context

V5 takes v9's TypeScript scene-graph engine as foundation and ports it to Go.
V4 has a structural ceiling (3-level Formation/Widget/Atom, no arbitrary
nesting, no container-atoms) that no applier rewrite lifts — on a real prompt
like "draw a product landing", V4 vs v9 output is dramatically different. V4
stays in `project_v4/` as the production engine until V5 is ready to swap in.

This plan covers the **first chunk only** — porting v9's `packages/domain` to
Go and getting an ops applier that mutates a hand-crafted Document. State,
delta-stream, binding layer, presets, pipeline integration are subsequent
chunks with their own plans.

**Goal of this chunk**: a Go package `project_v5/backend/internal/engine` where
we can build a Document programmatically, apply Insert/Update/Override/Move/
Delete commands via `CommandHistory`, expand RefNodes with descendants
overrides, and resolve `$variable` references — covered by tests. No HTTP, no
DB, no LLM yet.

**Scope of v9 source**: `/Users/starknight/Keepstar_project/Keepstar_one_v9/packages/domain/src/` — 1319 lines TS, 24 files. Expected Go output: ~2000–2500 lines.

---

## Step 0 — Pre-work

### 0.1 Commit current uncommitted state

Before starting chunk 1, commit pending CLAUDE.md cleanup + the high-level
plan doc as a single commit on branch `v5`:

```
docs(v5): start V5 branch — high-level work plan + CLAUDE.md cleanup

- docs/v5-engine-plan.md: high-level plan for V5 (v9 as foundation, port V4 strengths)
- CLAUDE.md: remove obsolete purple-primary; add chat overlay layout (360px column right)
- docs/v9-integration-spec.md: deleted (superseded by v5-engine-plan)
```

### 0.2 Set up session-log folder

Create `docs/Updates/v5/` and a README explaining log naming/format. Naming:
`v5_<YYYY-MM-DD>_<HH-MM>.md`. Format mirrors existing
`docs/Updates/feature-engine-v4_*.md` files (header with branch/date/commit/
parent, then context/approach/files-changed/verification/known-gaps).

---

## Step 1 — Scaffold `project_v5/backend/`

Mirror V4's hexagonal layout. Empty packages now, fill in subsequent chunks.

```
project_v5/
└── backend/
    ├── go.mod                         (module: keepstar_v5, go 1.24)
    ├── README.md                      (purpose + status)
    ├── cmd/
    │   └── server/
    │       └── main.go                (stub — prints "v5 server", does nothing yet)
    └── internal/
        ├── domain/                    (empty — populated in chunk 2)
        ├── engine/                    (this chunk — v9 port lives here)
        ├── ports/                     (empty — populated in chunk 3)
        ├── adapters/                  (empty — populated in chunk 4)
        ├── usecases/                  (empty)
        ├── handlers/                  (empty)
        ├── tools/                     (empty)
        └── prompts/                   (empty)
```

`go.mod` minimal — only deps needed for engine: nothing external (stdlib only).
Anthropic/pgx/uuid come later.

---

## Step 2 — Port entities + value-objects

Target files in `project_v5/backend/internal/engine/`:

| Go file | TS source | Lines (TS → Go est.) | Notes |
|---|---|---|---|
| `node_types.go` | `entities/nodes.ts` | 154 → 280 | 13 node types as discriminator constants over `Node = map[string]any`. Predicates `HasChildren()`, `HasGraphics()`, `IsRef()`. Helpers `Children()`, `SetChildren()` that normalize across `[]Node` and `[]any` shapes. |
| `document.go` | `entities/document.ts` | 31 → 60 | Document struct with typed top-level (Version/Themes/Imports/Variables/Children). VariableDef as a tagged union (`Type` + `Value any`). `NewDocument()` constructor (version "2.10"). |
| `value_color.go` | `value-objects/color.ts` | 29 → 60 | `IsVariable(v any) bool`, `ParseColor(hex string) (ParsedColor, error)`. Type alias `Color = string`. Helper unwrappers `AsBool`, `AsNumber`, `AsString` for `*OrVariable` unions. |
| `value_fill.go` | `value-objects/fill.ts` | 69 → 120 | `NormalizeFills(any) []any`. String constants for fill type discriminators (color/gradient/image/mesh_gradient), gradient subtypes, image modes, BlendMode (19 values). |
| `value_stroke.go` | `value-objects/stroke.ts` | 20 → 5 | Placeholder (untyped through Node map). |
| `value_effect.go` | `value-objects/effect.ts` | 34 → 70 | `NormalizeEffects(any) []any`. Constants for effect types (blur/background_blur/shadow), shadow subtypes (inner/outer). |
| `value_layout.go` | `value-objects/layout.ts` | 31 → 60 | `NormalizePadding(any) [4]float64`. Constants for LayoutMode, JustifyContent, AlignItems. |
| `value_text_style.go` | `value-objects/text-style.ts` | 19 → 35 | `TextGrowth` constants (auto/fixed-width/fixed-width-height). |
| `value_position.go` | `value-objects/position.ts` | 27 → 50 | `ParseSizing(any) ParsedSizing`. Modes: fixed/fill/fit. |

**Gotchas in Go**:
- TS uses optional fields with `?`; Go uses pointers OR `omitempty` JSON.
  Decision: pointers for primitives that have meaningful zero (e.g.
  `Opacity *float64`); plain types where zero=absent is fine.
- TS `string | number` unions: `any` (interface{}) with helper resolvers.
- TS `BooleanOrVariable`/`NumberOrVariable`/`StringOrVariable`: helpers
  `AsBool`, `AsNumber`, `AsString`.
- `metadata: { type: string; [key: string]: unknown }`: stays as
  `map[string]any` inside the Node bag; no special unmarshal needed.

---

## Step 3 — Port operations + Command pattern

| Go file | TS source | Notes |
|---|---|---|
| `command.go` | `operations/command.ts` | `Command` interface (`Execute(*Document) error`, `Undo(*Document) error`). `CommandHistory` with `Execute`/`Undo`/`Redo`/`CanUndo`/`CanRedo`/`Clear`. Errors surface; commands NOT pushed to stack on error. Redo cleared on new execute (matches v9). |
| `op_insert.go` | `operations/insert.ts` | `InsertCommand` with shallow copy of input Node (does NOT mutate caller's map — matches v9 spread). Auto-generates ID via `GenerateID()` if missing. parentID == "" means document root. Errors on missing parent or non-container parent. |
| `op_update.go` | `operations/update.ts` | `UpdateCommand` snapshots only the keys being updated, plus a `hadProps` map tracking whether each key existed before. Undo restores existing or deletes new keys. |
| `op_override.go` | `operations/override.ts` | `SetOverrideCommand` (descendant overrides on RefNode), `SetRootOverrideCommand` (RefNode root-level overrides). Both snapshot existing override map for undo. Empty descendants record cleaned up on undo. |
| `op_move.go` | `operations/move.ts` | `MoveCommand` with explicit `useIndex bool` (Go has no optional args). `findParent` returns parent + index. Splice from old, insert at new index (clamped). Undo reverses (falls back to doc root if old parent missing — defensive, drift from v9 but safer). |
| `op_delete.go` | `operations/delete.ts` | `DeleteCommand` removes node from parent.children, snapshots node + parent ID + index for undo. |

**Critical decision: how to model the dynamic property bag for Update/Override?**

TS does `Object.assign(node, newProps)` because everything is a JS object. In
Go we need a strategy:

- **Option A**: `map[string]any` representation throughout. `UpdateCommand`
  operates as `node[key] = value`. Trivial. Weak typing in Go, every node
  access is a map lookup, no compile-time guarantees.
- **Option B**: Typed structs + reflection in operations. Strong typing for
  users, ops code is reflection-heavy and slower.
- **Option C**: Hybrid — typed structs but with `Properties map[string]any`
  extra-bag inside each node. Operations only mutate the Properties bag.

**Decision: Option A for chunk 1.** v9's TS treats nodes as duck-typed objects.
The fastest faithful port uses `map[string]any` throughout `engine/`. We add
typed wrappers (`AsFrame(node) *FrameNode`) at the layer above (chunk 2 —
domain). The engine operates on raw maps, like the TS source operates on raw
objects. If it bites later, refactoring to Option C is mechanical.

---

## Step 4 — Port services

| Go file | TS source | Notes |
|---|---|---|
| `id_generator.go` | `services/id-generator.ts` | `GenerateID() string` — 5 chars random from `[A-Za-z0-9]`. `math/rand` (auto-seeded since Go 1.20). No collision check. |
| `scene_graph.go` | `services/scene-graph.ts` | `FindNodeByID`, `FindParent`, `WalkNodes`, `FindReusableNodes`, `FindNodesByType`, `FindRefsToNode`, `FindSlots`. Critical: do NOT recurse into RefNode children — match v9 semantics (RefNode is opaque until expanded). Helpers `childrenOf`, `SetParentChildren`, `ParentID`. |
| `variable_resolver.go` | `services/variable-resolver.ts` | `ResolveContext` (Document + ActiveThemes). `ResolveColor`/`Number`/`Boolean`/`String`. Single-level chained variable resolution. Theme matching: ALL keys in ThemedValue.theme must match ActiveThemes; fallback to first untyped or first entry. Magenta `#FF00FFFF` for unresolved color, 0/true/"" for other types. Coerces JSON-shape themed values from `[]any` into `[]ThemedValue` via `toThemedValues`. |
| `component_resolver.go` | `services/component-resolver.ts` | `Expand(refNode, doc) ResolvedRef`. Max depth 10. Deep clone source via `cloneNode`/`cloneAny` helpers. Apply root overrides (skip `type/ref/descendants/id`). Apply descendant overrides (skip `children` — handled by slot pass). Process slots: replace `frame.children` with `descendants[frameID].children`. Recursively expand nested RefNodes. |

**Deep clone helper**: `cloneAny(v any) any` handles maps, slices, primitives.
`cloneNode(n Node) Node` recurses into `children`. Used by component-resolver
+ tests.

---

## Step 5 — Tests

Colocated `_test.go` files matching V4 convention. No shared fixture directory.

Test files & coverage:

- **`node_types_test.go`** — predicate behavior; Children normalization across `[]Node` and `[]any`; SetChildren append + nil-deletes.
- **`value_color_test.go`** — IsVariable; ParseColor for 3/6/8-digit and invalid lengths/chars; AsBool/AsNumber/AsString unwrappers.
- **`value_position_test.go`** — ParseSizing for fixed/fill/fit with optional fallback; NormalizePadding for all union shapes.
- **`document_test.go`** — NewDocument basics; full JSON Marshal → Unmarshal round-trip with frames, refs, descendants, themes, variables; ComponentResolver works on post-roundtrip data.
- **`id_generator_test.go`** — length, charset, distinct values across 100 generations.
- **`value_normalize_test.go`** — NormalizeFills/NormalizeEffects edge cases.
- **`op_insert_test.go`** — insert at root; into frame; auto-id; missing parent error; non-container parent error; undo.
- **`op_update_test.go`** — snapshot semantics; missing-key delete on undo; missing-node error.
- **`op_override_test.go`** — SetOverride writes + merges; undo restores or removes; cleanup of empty descendants. SetOverride on non-ref → error. SetRootOverride writes + undoes root-level keys.
- **`op_move_test.go`** — across parents; to root; index clamping; undo reverses parent swap.
- **`op_delete_test.go`** — from frame; from root; undo restores at original index.
- **`command_history_test.go`** — execute/undo/redo cycle; redo cleared on new execute; CanUndo/CanRedo signals; Clear empties both stacks.
- **`scene_graph_test.go`** — FindNodeByID nested; FindParent returns Document for root children; FindReusableNodes; FindNodesByType; FindRefsToNode; FindSlots with `[]any` and `[]string` and `slot:false`.
- **`variable_resolver_test.go`** — passthrough; simple `$var`; themed; fallback (magenta/0); single-level chained.
- **`component_resolver_test.go`** — simple ref; root overrides; descendant overrides; slot injection (`descendants[frameID].children`); missing source → error.
- **`smoke_test.go`** — end-to-end: build reusable Button → RefNode with override → Expand → Update → Undo.

**Run**: `cd project_v5/backend && go test ./internal/engine/...`

---

## Critical files to create

```
project_v5/backend/
├── go.mod
├── README.md
├── cmd/server/main.go
└── internal/engine/
    ├── node_types.go            value_color.go         op_insert.go
    ├── document.go              value_fill.go          op_update.go
    ├── id_generator.go          value_stroke.go        op_override.go
    ├── scene_graph.go           value_effect.go        op_move.go
    ├── variable_resolver.go     value_layout.go        op_delete.go
    ├── component_resolver.go    value_text_style.go    command.go
    ├── value_position.go
    └── *_test.go (matching files above + smoke_test.go)
```

Plus:

```
docs/Updates/v5/
├── README.md             (log format conventions)
└── plans/
    └── chunk-1-engine-port.md   (this file)
```

---

## References

**v9 source** (read-only, for reference) — paths under
`/Users/starknight/Keepstar_project/Keepstar_one_v9/packages/domain/src/`:
- `entities/nodes.ts`, `entities/document.ts`
- `value-objects/{color,fill,stroke,effect,layout,position,text-style}.ts`
- `operations/{insert,update,override,move,delete,command}.ts`
- `services/{id-generator,scene-graph,variable-resolver,component-resolver}.ts`

**V4 conventions to match** — paths under `project_v4/backend/`:
- Hexagonal layout: `internal/{domain,ports,adapters,usecases,handlers,tools}` mirrored in V5
- `_test.go` colocated, no shared fixtures
- `go.mod` style: minimal deps, go 1.24

**Memory notes from prior sessions to honor**:
- Dev docs in English (CLAUDE.md, READMEs) — chat with Vlad in Russian
- No purple anywhere
- Time estimates: don't inflate; engine port is hours-of-Vlad-work, not days

---

## Verification

End of this chunk, the following must pass:

1. **Build**: `cd project_v5/backend && go build ./...` → exit 0
2. **Tests**: `cd project_v5/backend && go test ./internal/engine/...` → all pass
3. **Smoke** (`smoke_test.go`):
   - Build a Document with a button component (Frame containing Rect + Text) marked `reusable: true`
   - Insert a RefNode pointing to the button, with `descendants` overriding the Text content
   - Run ComponentResolver.Expand → assert expanded tree contains overridden text
   - Apply Update on the resolved tree's Text → assert content changed
   - Undo → assert content reverted

If any of the above fails, do not proceed to chunk 2.

---

## Out of scope for this chunk

These are subsequent chunks, each with their own plan:

- **Chunk 2**: Port V4 state + delta-stream into V5 (sectional state, append-only deltas, replay/rollback). Adapter for Postgres (cherry-pick from V4).
- **Chunk 3**: Binding layer — slot↔field vocabulary, port `ProductToMap` from V4, `<fields>` block in prompt, conditional `tree_map`. RefNode + descendants + variable-resolver as the substrate.
- **Chunk 4**: First preset (product card) end-to-end as a v9 component (reusable Frame). Verify token efficiency target.
- **Chunk 5**: Microprésets — discipline of breaking presets into 5–7 component groups. Port 3–5 V4 presets.
- **Chunk 6**: Pipeline integration — handlers + Agent1/Agent2 usecases. Cherry-pick LLM/Postgres adapters from V4.
- **Chunk 7**: Transition graph + actions + run-binding cache.
- **Chunk 8**: Smoke + perf + token-cost measurement vs V4 baseline.

Each subsequent chunk only starts after the previous chunk's verification passes.

---

## Risks / what could blow this chunk up

- **Map-based representation (Option A) bites in tests** — if `map[string]any`
  makes node access painful, fall back to Option C (typed structs + Properties
  bag). Mid-chunk refactor adds ~2 hours. **Outcome**: did not bite.
- **JSON round-trip mismatch with v9** — v9's TS marshals are subtly different
  (e.g. omitting falsy values). For chunk 1, JSON round-trip just needs to be
  self-consistent. **Outcome**: round-trip test added in `document_test.go`.
- **VariableDef tagged union JSON** — Go doesn't have native discriminated
  unions. **Outcome**: not blocking; `Value any` + helper coercion works.
- **Component-resolver deep clone** — easy to write a buggy clone. Hand-rolled
  `cloneAny` helper with explicit handling of map/slice/primitive — verified
  in tests. **Outcome**: clean.

If any of these escalate beyond ~1 hour, stop and consult before pushing through.
