# Chunk 5.5 — V5 Hygiene

## Context

Five chunks of V5 work (engine port → state/delta → binding → first preset → micropresets) have shipped a working architecture validated by integration tests against live Neon. Before chunk 6 (LLM in the loop) lands, an audit of the codebase + docs surfaced four real problems and one missing measurement:

1. **Latent bug**: `ComponentResolver.expandRef` recursive expansion leaves `reusable:true` on inner inlined subtrees — only the top-level `ResolveAndInline` strips it. As soon as a component references another component, the inner instance is silently skipped by `BindData` and all its leaf nodes go un-bound. No seed exercises this today, so the test suite is green.
2. **Wrong wire format**: V5 image-fill writes `{type:"image", image:"<url>"}`, but v9 TS source defines the field as `url`, not `image`. Confirmed in `/Users/starknight/Keepstar_project/Keepstar_one_v9/packages/domain/src/value-objects/fill.ts:35-42`. Will break the frontend renderer port the moment we get there.
3. **Missing format/wrapper layer**: V4 atoms carry `format` (currency/stars/percent/...) and `wrapper` (badge/tag/price/...) as properties — backend stores them, ops change them, frontend applies the formatting on render. V5's leaf nodes (TextNode/ImageNode) currently have neither. Without them, a TextNode bound to a numeric field carries a raw float in `content`, which the frontend can't render correctly. V4-aligned approach: store on the node, format on the frontend, no new backend formatter.
4. **Doc drift**: `v5-known-gaps.md` Preset-storage row only lists `v5_presets`/`v5_preset_versions` — chunk 5 added `v5_components`/`v5_component_versions` and the row should reflect both. Chunk 6 in the roadmap is overloaded (Anthropic + HTTP + tracer + prompt-builder + tx fixes + token measurement) — needs a split.
5. **Token cost never measured**: The V5 plan declared "same or better than V4" as a hard constraint, but five chunks shipped without a single number. We have everything we need to do a paper measurement now (V4 prompts + tool def exist; V5 prompt + tool def can be sketched). One concrete number before chunk 6 starts.

Outcome: a clean punch-list that closes the four problems and produces the first token measurement, leaving chunk 6 with a smaller, more focused scope.

## Step-by-step plan

### Step 1: Fix nested-ref `reusable` strip

**File**: `project_v5/backend/internal/engine/component_resolver.go`

Move the `delete(node, "reusable")` call from `ResolveAndInline` into `expandRef`, applied to `clone` immediately after `cloneNode(source)` (line 86). This guarantees every successful expansion produces a clone without the marker, including recursively-resolved nested refs at lines 113-117.

**File**: `project_v5/backend/internal/engine/resolve_inline.go`

Remove the now-redundant `delete(resolved.Root, attrReusable)` at line 55 (with a one-line comment pointing at the new location in expandRef).

**File**: `project_v5/backend/internal/engine/resolve_inline_test.go`

Add a regression test `TestResolveAndInlineStripsReusableFromNestedRefs`: build a component A whose definition contains a RefNode pointing at component B; materialise both into a doc; consume A from a preset; resolve; assert that the inlined A subtree (top level) has no `reusable`, AND the nested B subtree inside it also has no `reusable`. Bind data; assert leaves under the nested B are bound.

The existing `TestResolveAndInlineStripsReusableFromInstances` should still pass unchanged.

### Step 2: Fix v9 image-fill wire format

**File**: `project_v5/backend/internal/engine/binding.go`

Change `attrFillImageURL = "image"` (line 33) to `attrFillImageURL = "url"`. The v9 schema is `{type: "image", url: "...", mode: "stretch"|"fill"|"fit"}` — confirmed in `/Users/starknight/Keepstar_project/Keepstar_one_v9/packages/domain/src/value-objects/fill.ts:35-42`. Update the doc comment block at lines 153-158 to reflect the confirmed shape.

**File**: `project_v5/backend/internal/engine/binding_test.go`

Update the existing image-fill assertions to expect key `"url"` instead of `"image"`. Search for `"image":` in fill maps in tests; replace.

**File**: `project_v5/backend/internal/adapters/postgres/engine_pipeline_integration_test.go`

Same: line 276 uses `first["image"]` — change to `first["url"]`. The `first["type"] == FillTypeImage` check stays as-is (the discriminator key is `type`, not the URL key).

### Step 3: Add format + wrapper to leaf nodes

**Architecture (V4-aligned)**:
- `format` (string) and `wrapper` (string) are plain properties on TextNode / ImageNode like `fieldBinding`. LLM sets/changes them via ops.
- BindData stores the **raw** bound value in `content` (TextNode) — no string conversion. Numeric, string, array — all written as-is.
- Frontend renderer reads `content` + `format` and produces the visible string. Same model as V4 (`prompt_compose_widgets.go` + V4 frontend `AtomV2Renderer`).
- Wrapper is a stylistic hint (badge/tag/price/...) that the frontend uses to pick a CSS class. Backend stores it; doesn't render anything from it.

This means **chunk 5.5 ships no formatter code** — we only declare the property vocabulary, document it, and seed a couple of presets with realistic values so chunk 6 has examples to test against.

**Files**:
- `project_v5/backend/internal/engine/binding.go` — extend the doc block at the top to mention `format` / `wrapper` as untouched-by-binding pass-through properties (frontend reads them).
- `project_v5/backend/internal/domain/atom_role.go` — add `Format` and `Wrapper` string-typed enum sets (port the V4 vocabulary verbatim from `project_v4/backend/internal/domain/atom_entity.go:64-72` and `project_v4/backend/internal/tools/tool_visual_assembly.go:99,112`).
  - Format set: `currency`, `stars`, `stars-compact`, `stars-text`, `percent`, `number`, `date`, `text`
  - Wrapper set: `none`, `badge`, `tag`, `pill`, `avatar`, `tooltip`, `alert`, `link`, `progress`, `button`
- `project_v5/backend/internal/engine/presets/seed/component_price_rating.json` — add `"format": "currency"` to `pr-price` and `"format": "stars-compact"` to `pr-rating`.
- `project_v5/backend/internal/engine/presets/seed/component_brand_badge.json` — add `"wrapper": "badge"` to `brand-badge-root` (already exists in seed line 8 — verify, keep, document).
- `project_v5/backend/internal/engine/presets/seed/product_card.json` and `product_card_list_row.json` — add `"format": "text"` to title nodes for explicitness (optional, helps Agent2 examples).
- New test file `project_v5/backend/internal/engine/format_wrapper_test.go` — assert that BindData does NOT touch `format` / `wrapper` properties; assert seed values round-trip through Materialise → Bind unchanged.

No formatter code, no value mutation. The renderer port (later chunk) will own the actual `4.5 → "★ 4.5"` step.

### Step 4: Update `docs/v5-known-gaps.md`

**Edits to the markdown table**:
- Strike (close) the **"Image-fill body shape"** row, with closing commit ref placeholder.
- Strike (close) the **bug-for-bug "Грабля #2"** entry for nested-ref reusable — actually a new entry to add and immediately close, since it wasn't in the registry. Format consistent with the existing strikethrough rows.
- Update **"Preset storage location"** row to mention `v5_components` / `v5_component_versions` alongside `v5_presets` / `v5_preset_versions`.
- Add a new **"Format / wrapper rendering"** row clarifying that backend stores them as pass-through properties; frontend renderer (forthcoming) is the actual consumer. Closes-in-chunk: frontend renderer port.

**Edits to chunk 6 in `docs/v5-engine-plan.md`** (lines 122-135 "Order of attack"):
- Split current chunk-6/7/8 into:
  - **6a — first token measurement + Anthropic adapter shell** (just enough to call the API and count tokens)
  - **6b — Agent2 prompt-builder with `<fields>` block + first real LLM turn**
  - **6c — HTTP server + handlers + DI of state/preset/component migrations**
  - **6d — tx fix for `zoneWriteWithDelta`, retry/advisory-lock for `AddDelta`, span tracing port**
- Renumber subsequent items (run-binding cache, pipeline integration) to land after 6d.

### Step 5: Token measurement — first numbers

**Goal**: produce one comparison number for a representative scenario ("show 3 products"), runnable via `go test`.

**Approach**:

New file `project_v5/backend/internal/engine/tokens/measurement_test.go` (build tag `tokens`, so it's opt-in):

1. **V4 baseline**:
   - Read V4 system prompt verbatim from `project_v4/backend/internal/prompts/prompt_compose_widgets.go:Agent2ToolSystemPrompt`.
   - Read V4 tool def verbatim from `project_v4/backend/internal/tools/tool_visual_assembly.go:Definition()`.
   - Synthesize a `<fields>` block of typical size (~1KB) and a `tree_map` of the 3-products case (~1.5KB). Use the format described by the Explore findings.
   - Concatenate into a synthetic Anthropic-format request body.
   - Count tokens via Anthropic SDK's `count_tokens` endpoint. Requires `ANTHROPIC_API_KEY` env var; test skips with `t.Skip` if absent.

2. **V5 paper sketch**:
   - Hand-write a v5 system prompt outline of comparable length, focused on v9 ops vocabulary (insert/update/override/move/delete + ref expansion mental model). Include the V5 `<fields>` block format (same as V4).
   - Hand-write a v5 visual_assembly tool def with V5's parameter set (preset + ops + replicate count, NO layout/columns/size — those live as preset properties).
   - Sketch a v5 tree_map for the 3-products scenario in scene-graph terms (top-level reusable defs + 3 instance roots + the dataIndex inheritance markers).
   - Count tokens the same way.

3. **Output**: t.Logf prints input-token counts for each side + delta. Goal is a number, not a guarantee. Re-runnable as v5 evolves.

**File added**:
- `project_v5/backend/internal/engine/tokens/measurement_test.go` — the test
- `project_v5/backend/internal/engine/tokens/sketches/v5_system_prompt.txt` — the v5 prompt sketch (inputs to the test, version-controlled so we can iterate)
- `project_v5/backend/internal/engine/tokens/sketches/v5_tool_def.json` — v5 tool def sketch
- `project_v5/backend/internal/engine/tokens/sketches/v5_tree_map_3products.json` — sample tree_map

**Anthropic count_tokens API**: SDK has `client.Beta.Messages.CountTokens(...)`. If the project's anthropic-go module isn't yet imported (chunk 6 plans to add it), use a thin wrapper that POSTs to `/v1/messages/count_tokens` directly via net/http to avoid pulling the SDK in 5.5.

### Step 6: Commit + session log + push

1. **Commit messages** (one per logical step, conventional style):
   - `fix(v5): strip reusable from inner inlined subtrees in expandRef (chunk 5.5)`
   - `fix(v5): align image-fill wire key with v9 (image → url)`
   - `feat(v5): seed format/wrapper properties on leaf nodes (chunk 5.5)`
   - `docs(v5): chunk 5.5 known-gaps + chunk-6 split`
   - `chore(v5): first token measurement scaffolding (V4 vs V5 paper sketch)`
   - `docs(v5): chunk 5.5 session log`

2. **Session log** at `docs/Updates/v5/v5_<YYYY-MM-DD>_<HH-MM>.md` (UTC of final commit) with required sections (Context / Approach / Files changed / Verification / Known gaps).

3. **Frozen plan copy** at `docs/Updates/v5/plans/chunk-5_5-hygiene.md` (copy of this file).

4. **Push the v5 branch**: `git push -u origin v5` (or `git push origin v5` if already tracking). Confirm push succeeded by checking remote tip matches local.

## Verification

```sh
cd project_v5/backend

# Static checks
go build ./...
go build -tags=integration ./...
go vet ./... && go vet -tags=integration ./...

# Unit tests
go test ./...                                  # green expected; new tests must pass
go test ./internal/engine/... -v               # new resolve_inline + format_wrapper tests
go test ./internal/engine/presets/... -v       # seed round-trips green after format/wrapper edits

# Integration tests against live Neon
TEST_DATABASE_URL=$DATABASE_URL go test -tags=integration ./...
# Existing 14/14 must stay green; the engine_pipeline_integration_test.go assertion
# on `first["url"]` (was `first["image"]`) must pass with v9-aligned key.

# Token measurement (opt-in, requires ANTHROPIC_API_KEY)
ANTHROPIC_API_KEY=... go test -tags=tokens -v ./internal/engine/tokens/...
# Logs token counts; doesn't assert pass/fail, just produces numbers.
```

After local green: commit each logical group, write session log, copy plan into docs/Updates/v5/plans/, push the v5 branch to remote.

## Critical files modified

| File | Why |
|---|---|
| `project_v5/backend/internal/engine/component_resolver.go` | Move `reusable` strip into `expandRef` |
| `project_v5/backend/internal/engine/resolve_inline.go` | Remove now-redundant strip |
| `project_v5/backend/internal/engine/resolve_inline_test.go` | Nested-ref regression test |
| `project_v5/backend/internal/engine/binding.go` | Image-fill key `image` → `url`; doc format/wrapper pass-through |
| `project_v5/backend/internal/engine/binding_test.go` | Update fill-key assertions |
| `project_v5/backend/internal/adapters/postgres/engine_pipeline_integration_test.go` | Update fill-key assertion |
| `project_v5/backend/internal/domain/atom_role.go` | Add Format + Wrapper enum vocab |
| `project_v5/backend/internal/engine/presets/seed/*.json` | Seed format/wrapper realistic values |
| `project_v5/backend/internal/engine/format_wrapper_test.go` (new) | BindData doesn't touch format/wrapper |
| `project_v5/backend/internal/engine/tokens/measurement_test.go` (new) | Token measurement |
| `project_v5/backend/internal/engine/tokens/sketches/*` (new) | V5 prompt + tool def + tree_map sketches |
| `docs/v5-known-gaps.md` | Close 2 rows, add 1, update preset storage row |
| `docs/v5-engine-plan.md` | Split chunk 6 into 6a/6b/6c/6d |
| `docs/Updates/v5/v5_<UTC>.md` (new) | Session log |
| `docs/Updates/v5/plans/chunk-5_5-hygiene.md` (new) | Frozen plan copy |

## Time budget

Realistic estimate: one focused session (Vlad-time ≈ 1-2 hours of clock-time review + commit). Each step is small; the most uncertain is the token measurement (Anthropic API access + sketch quality), which is also the easiest to defer if it blows up — leave a stub measurement test that documents what's missing.

## What this chunk explicitly does NOT do

- No frontend renderer work — `format`/`wrapper` consumption lives there but is its own chunk.
- No Anthropic adapter, no HTTP server, no Agent2 prompt-builder — chunk 6a-6d.
- No transaction or retry fix for `zoneWriteWithDelta` / `AddDelta` — chunk 6d.
- No multi-root component support, no ID-namespacing for resolved subtrees — known gaps stay open, no consumer yet.
