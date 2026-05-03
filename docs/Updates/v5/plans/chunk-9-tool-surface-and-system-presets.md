# Chunk 9 — Tool surface unblock + system presets seeded

> Closes P0-A items 1, 2, 3 + P0-B item 6 from `docs/v5-engine-plan.md`.
> Backend only. Pre-req for chunk 10 (frontend renderer).

## Context

Two related gaps surfaced when re-reading V4 against current V5:

1. **V5 `visual_assembly` is more constrained than V4.** The tool schema
   in `tool_visual_assembly.go:60` declares `"required": ["preset"]`, so the
   LLM cannot call without naming a preset. V4 supports three call shapes:
   - preset only (cheap path)
   - preset + ops on top (overrides)
   - **freestyle ops, no preset** ("BUILDING FROM SCRATCH" — V4 prompt
     lines 148-205)
   - **multi-widget composing** for landings / presentations
     ("COMPOSING" — V4 prompt 207-228)
   - **modify-mode ops on existing tree** (no preset, only ops)
   V5 today only does paths 1+2; the other three are blocked at the schema
   level.

2. **V5 has 2 presets in DB; the prompt advertises 12.** Chunk-5
   micropresets seeded `product_card` + `product_card_list_row`.
   `agent2_prompt.go:51-65` lists 12 names. When LLM picks `product_detail`
   or `empty_not_found`, the tool errors with «preset not found». No
   visible product impact today because we don't run live except in tests
   that pre-seed — but the moment chunk 10 ships a frontend, this becomes
   blocking.

These two are tightly coupled: testing multi-widget composition needs a
literal-text preset (`text_explainer`) for hero blocks; testing modify-path
needs a working preset to modify; freestyle build needs nothing extra but
shares the test-bench. So one chunk.

## Approach

### Part A — tool schema + applier

**`internal/tools/tool_visual_assembly.go`:**

- Drop `preset` from schema `required`.
- Accept `parent: ""`, `parent: "root"`, and `parent: "formation"` as
  aliases for «document root» when the LLM emits a top-level insert. V4
  uses `"formation"`; V5 engine treats empty parentID as root
  (`op_insert.go:41`). We just normalise the three strings to "" before
  calling `engine.NewInsertCommand`.
- New branch in `Execute`:
  - if `preset` present → load preset + components → Materialise (current path)
  - if `preset` absent + `ops` present → start from `engine.NewDocument()`,
    skip preset loading, skip Materialise, skip ResolveAndInline UNLESS
    ops introduce ref nodes (then resolve at the end)
  - if neither preset nor ops → return `IsError: true` («pass preset, ops,
    or both»)
- Update `Description` field on tool: «Build or modify the scene-graph.
  Three call shapes: (1) preset name + optional ops on top — fast; (2)
  ops only, no preset — for freestyle and modify; (3) multi-widget
  compose via top-level frame inserts. ...»
- Update slog at tool entry: log which path was taken (`mode=preset`,
  `mode=freestyle`, `mode=modify`).

**`internal/engine/apply_ops.go`:**

- Normalize `parent` aliases ("root", "formation") → "" in
  `buildCommand`, before `resolveRef`. Single one-liner. Add unit test
  `TestApplyOpsParentAliases`.

### Part B — Agent2 prompt

**`internal/prompts/agent2_prompt.go`:**

Add three new sections, mirroring V4 structure but with V5 syntax:

- **«BUILDING FROM SCRATCH»** — when no preset matches, walk the LLM
  through `insert frame → insert children → insert atoms with
  fieldBinding`. Mirror V4 lines 148-205 (Examples 1-3: product card grid,
  single product detail, compact rows) using V5 `frame` / `text` / `image`
  / `ref` vocabulary.
- **«COMPOSING — multi-widget responses»** — explicit instruction to
  insert MULTIPLE top-level frames in one call when the user asks for a
  «landing», «presentation», «hero + grid + cta». Per-widget replicate
  goes inside the frame's props (e.g. `{type:"frame", replicate:true,
  layout:..., children:[...]}`). Mirror V4 lines 207-228, but adapt to
  scene-graph (no `widget` node type — just frames).
- Update **DECISION RULES** at the bottom: add «4. Three call shapes» rule.

Token budget: V5 prompt currently ≈ 5300 tokens (chunk 6b). Adding
~700-900 tokens for these sections keeps us above 4500. Re-run
`internal/engine/tokens/measurement_test.go` (build tag `tokens`) to
verify post-edit.

### Part C — system presets

Architecture decision: **in-process system preset registry** with DB
fallback semantics, NOT per-tenant DB seeding.

**Why**: per-tenant seeding raises «which tenant?» ambiguity and forces
re-seeding on every new tenant. System presets are by definition not
tenant-specific. Embedding them as JSON + serving via a registry that
the PostgresAdapter checks BEFORE / AFTER DB lookup is cleaner. Tenant
overrides (when canvas microservice ships) take precedence — DB hit
wins, registry is fallback.

**New JSON files in `internal/engine/presets/seed/`** (7 system presets;
nav presets `liked_grid` / `cart_grid` / `catalog_category_card` defer
to P0-C interaction work):

- `product_card_compact.json` — small card for dense grids (5+); image
  smaller, no rating ref; replicate:true
- `product_card_horizontal.json` — image left, info right; replicate:true
- `product_detail.json` — full detail with hero / title / price-rating
  ref / brand / description / tags; replicate:false
- `product_detail_horizontal.json` — same with image-left; replicate:false
- `text_explainer.json` — literal title + body atoms (no fieldBindings);
  replicate:false; consumers pass `ops` to set the actual text via
  `update target=headline content="..."`
- `empty_not_found.json` — literal headline + subtext for empty state
- `error_generic.json` — literal headline + subtext for error state

Each new preset reuses the two existing components (price-rating-root,
brand-badge-root) where it makes sense — keeps cache prefix stable + tests
existing component-resolution path.

**`internal/engine/presets/presets.go`:**

- Add `//go:embed` declarations for each new file.
- Add `SystemPresetsByName map[string][]byte` — name → embedded JSON,
  populated in `init()`.

**New `internal/engine/presets/system_registry.go`:**

- `SystemPresetRegistry` struct. Methods:
  - `Get(name string) (*engine.Document, bool)` — parse from embedded JSON
    on first call, cache parsed Document in `sync.Map`.
  - `List() []string` — sorted preset names.
- Default-replicate metadata is encoded inside each preset doc (top-level
  `default_replicate` field on the doc envelope, NOT the engine schema —
  we read it via a side struct on PresetPort responses).

**`internal/adapters/postgres/postgres_preset.go`:**

- `GetPublishedPreset` — when DB returns `ErrPresetNotFound`, fall back
  to `SystemPresetRegistry.Get(name)`. Wrap into `*domain.Preset` with
  `IsSystem: true` flag.
- `ListPublishedPresets` — union DB results with system registry, dedup
  by name (DB wins).

**`internal/domain/preset.go`** (or wherever Preset lives):

- Add `IsSystem bool` to `domain.Preset`. Used by tracing + future canvas
  override warnings.

### Part D — tests

**Unit:**

- `engine/apply_ops_test.go` — `TestApplyOpsParentAliases`: ops with
  `parent: "root"`, `"formation"`, `""` all insert at root.
- `engine/presets/system_registry_test.go` — Get / List / cache hit
  behaviour for system registry; one test per new JSON file asserts it
  parses to a valid Document with at least one root child.
- `tools/tool_visual_assembly_test.go` — three new cases:
  - freestyle (no preset, ops only) builds a card
  - multi-widget (3 top-level frames in one call)
  - error when both preset and ops are absent
- `prompts/agent2_prompt_test.go` (extend) — assert new sections are
  present in `Agent2SystemPrompt`.

**Integration (live, build tag `live`):**

Extend the existing `handler_pipeline_live_test.go::TestHTTPLiveSmoke` to
add three more turns AFTER the existing one:

- Turn 2: Russian prompt «покажи детальную карточку первого товара» —
  expect Agent2 to pick `product_detail`, no «preset not found» error.
- Turn 3: «сделай мне презентацию: заголовок «новинки», 3 карточки,
  потом большая кнопка «смотреть все»» — expect Agent2 to do a
  multi-widget compose (assert resulting Document.Children has ≥3 root
  frames).
- Turn 4: ops-only follow-up «сделай заголовок красным» — expect Agent2
  to send ops without preset, modification lands.

**Token measurement:**

- Re-run `go test -tags=tokens ./internal/engine/tokens/...`. Assert V5
  system+tools prefix still ≥ 4500.

### Part E — main.go wiring

Single line: instantiate the system registry once at boot and inject
into `NewPresetAdapter`. No new migration (registry is in-process).

### Part F — slog observability

Per the «логи чтобы трейсы смотреть можно было» request:

- Tool entry log line: `slog.Info("visual_assembly", "mode", path,
  "preset", presetName, "ops", len(ops), "session", sessionID)`
- System registry hit log: `slog.Debug("preset.system_fallback", "name",
  name)` — distinguishes DB-served vs system-served presets at runtime.
- Both flow through the existing logging middleware so they're correlated
  with `request_id`.

## Files changed (planned)

| File | Status | Notes |
|---|---|---|
| `internal/tools/tool_visual_assembly.go` | modify | drop required preset; freestyle branch; mode log |
| `internal/tools/tool_visual_assembly_test.go` | modify | freestyle / compose / both-absent error tests |
| `internal/engine/apply_ops.go` | modify | normalise parent aliases |
| `internal/engine/apply_ops_test.go` | modify | `TestApplyOpsParentAliases` |
| `internal/prompts/agent2_prompt.go` | modify | + BUILDING FROM SCRATCH / COMPOSING / updated DECISION RULES |
| `internal/prompts/agent2_prompt_test.go` | modify | section presence assertions |
| `internal/engine/presets/seed/product_card_compact.json` | add | new system preset |
| `internal/engine/presets/seed/product_card_horizontal.json` | add | new system preset |
| `internal/engine/presets/seed/product_detail.json` | add | new system preset |
| `internal/engine/presets/seed/product_detail_horizontal.json` | add | new system preset |
| `internal/engine/presets/seed/text_explainer.json` | add | new system preset |
| `internal/engine/presets/seed/empty_not_found.json` | add | new system preset |
| `internal/engine/presets/seed/error_generic.json` | add | new system preset |
| `internal/engine/presets/presets.go` | modify | + embed + SystemPresetsByName |
| `internal/engine/presets/system_registry.go` | add | SystemPresetRegistry |
| `internal/engine/presets/system_registry_test.go` | add | unit + per-preset round-trip |
| `internal/adapters/postgres/postgres_preset.go` | modify | DB miss → registry fallback |
| `internal/domain/preset.go` | modify | + IsSystem flag |
| `cmd/server/main.go` | modify | inject registry into preset adapter |
| `internal/handlers/handler_pipeline_live_test.go` | modify | + 3 new turns (detail / compose / modify) |
| `docs/v5-engine-plan.md` | modify | mark P0-A 1-3 + P0-B 6 done |
| `docs/v5-known-gaps.md` | modify | close «only 2 presets» row |
| `docs/Updates/v5/plans/chunk-9-tool-surface-and-system-presets.md` | add | this plan |
| `docs/Updates/v5/v5_2026-05-03_<HH-MM>.md` | add | session log |

## Verification

```sh
cd project_v5/backend

# Static + unit (every build tag)
go build ./... && go build -tags=integration ./... && \
  go build -tags="integration live" ./... && go build -tags=tokens ./...
go vet ./... && go vet -tags=integration ./... && \
  go vet -tags="integration live" ./...
go test -count=1 ./...

# Token gate
go test -count=1 -tags=tokens ./internal/engine/tokens/... -v

# Live HTTP smoke (4 turns: original + detail + compose + modify)
ANTHROPIC_API_KEY=$KEY TEST_DATABASE_URL=$DB \
  go test -tags="integration live" -v -count=1 \
  ./internal/handlers/... -run TestHTTPLiveSmoke
```

Acceptance criteria:
- All existing tests still pass.
- New unit tests pass.
- Token measurement: V5 system+tools ≥ 4500.
- Live test turn 2 (`product_detail` request): Agent2 calls visual_assembly
  with `preset:"product_detail"`; tool resolves via system registry; no
  error; resulting Document has hero + title + price block + brand at
  minimum.
- Live test turn 3 (compose): Document.Children ≥ 3 root frames after
  Agent2 turn; at least one is replicate:true (the gallery), at least one
  literal (the hero).
- Live test turn 4 (modify): Agent2 emits ops-only call (no preset key in
  the tool input); modification lands on the existing tree.

## Known gaps after this chunk

- **Per-tenant preset overrides via canvas microservice** — Stream B; not
  this chunk. System registry is the bridge.
- **Nav presets** (`catalog_category_card`, `liked_grid`, `cart_grid`) —
  defer to P0-C interaction chunk where they're meaningful.
- **DefaultReplicate metadata** — encoded in JSON envelope today; when
  canvas ships, it should be a header column on `v5_presets`. Add
  migration when needed.
- **Modify-mode tree_map injection** — V5 prompt assumes Agent2 sees a
  tree_map for ops-only calls, but the orchestrator doesn't compute one
  yet. For chunk 9 we test that the tool *accepts* ops-only calls; full
  tree_map compaction is its own item (still on the 25-list as a hidden
  P2 — verify whether it's wired or not in chunk 10 prep).
