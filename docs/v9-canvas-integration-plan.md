# v9 Canvas → ultra admin — integration plan (ADR)

> Date: 2026-05-30. Goal: bring the v9 canvas into ultra admin as the
> preset / data-binding / adjacency authoring control-plane for the v5 engine
> (replace the weak tldraw KeepstarCanvas; enable tenant self-serve presets +
> the A→B drill map that v5 lacks). Method: workflow `wf_9636071c-9d0`
> (18 agents: investigate v9/admin/v5/binding-UX → design → plan → 3× adversarial verify).

## TL;DR — engine-first; the canvas is NOT on the pilot critical path

- **v5 has NO write side.** `preset_port.go:16-25` / `component_port.go:14-17`
  are read-only ("stays read-only until the v9-canvas microservice lands").
  A canvas with nowhere to Save is useless → **the write endpoint is the gate.**
- **That gate is also the cheap win:** the `doc_json` JSONB column already exists
  (`preset_migrations.go:41`, already read at render time), and admin's TX pattern
  (`canvas_adapter.go:155-210`) is the template — v5's version is *simpler*
  (store validated `doc_json`, no ops-replay). **~4-6h.**
- **v9 ↔ v5 format is ~100% compatible** (both `version 2.10`; v5 is a faithful
  Go port of v9's domain — same nodes/ops/variables/refs). Export ≈
  `JSON.stringify(document)`. Only real translations: v5's `image` node type
  (`node_types.go:27-31`) and the strict `id=="actions"` frame
  (`inject_actions.go:109-118`).
- **The full 1:1 canvas embed (Phases 4-7) is a multi-week track** (WebGL2-only
  renderer, yoga-WASM, a whole React app to vendor from a separate repo). All 3
  verifiers: **defer it post-pilot.** Verdict = `feasible-with-caveats`.
- **Cheapest path for the pilot (all 3 verifiers endorse):** build the write
  endpoint, then **author presets by hand (curl/Postman) OR run the v9 canvas
  STANDALONE (it already works) and POST its JSON export to the v5 endpoint.**
  Zero embedding, zero WebGL/yoga risk. The embedded editor waits for proven demand.

## Order (the owner's explicit question), settled

**ENGINE-FIRST.** Do the three Go/SQL phases (~10-13h total), which unblock a
demoable, tenant-customisable pilot with **no canvas at all**:

| Phase | What | Effort | Deps |
|---|---|---|---|
| **1. v5 write-side preset/component endpoints** | Add `CreatePreset/SaveDraftVersion(doc_json)/PublishPreset/ForkPreset` to PresetPort/ComponentPort + handlers + routes; TX mirrors `canvas_adapter.go:155-210` but writes `doc_json` not `ops_json`. Unblocks owner-authored presets via curl. | 4-6h | none — **start here** |
| **2. Tenant-editable adjacency map** | `ALTER` add nullable `adjacency_json` (or metadata col) on `v5_presets`; `handler_navigation.go:107-112` reads it instead of the hardcoded map (code already says "hardcoded today; future v5_presets.metadata"). Closes the real v5 gap. | 2-3h | none (parallel to 1) |
| **3. Golden round-trip test** | Feed a representative v9 Document through Materialise→ExpandReplicates→ResolveAndInline→BindData→InjectDefaultActions; assert image-node→`fills[0]`, literal `actions` frame survives + gets like/cart, `ref` expands. Locks the v9→v5 contract before any canvas code. | 3-4h | pairs after 1 |

**Do NOT go canvas-first.** Building WebGL2 + zustand + Inspector before a save
target exists = a beautiful editor that throws its work away, while debugging
yoga-WASM/WebGL2 compat as the pilot waits on a half-day SQL change.

## Post-pilot canvas track (only when a tenant needs in-admin self-serve)

Winner approach = **extract-v9-packages** (avg 5.5 vs embed-as-is 4.25): vendor
`@keepstar/{domain,renderer,layout}` (cross-repo copy from
`/Users/starknight/Keepstar_project/Keepstar_one_v9`; they're `private:true`,
**not npm-installable** — alias in Vite) into a native `/canvas-v2` admin route.

| Phase | What | Effort | Deps |
|---|---|---|---|
| 4. Vendor v9 packages + Vite alias + WebGL2 smoke | copy domain/renderer/layout (dist/ prebuilt, ESM), add earcut + yoga-layout | 3-5h | P1 |
| 5. Native `/canvas-v2` route composing v9 editor (Canvas/Inspector/LayerTree/Toolbar/store); swap v9 `ApiClient` StoragePort → POST to P1 endpoint via admin→v5 proxy (tenant JWT) | 1.5-2.5d | P4,P1 (validated by P3) |
| 6. **BindingSection** in Inspector (`Inspector.tsx:47-55` seam): fieldBinding picker from tenant field catalog, dataIndex, replicate toggle, format, wrapper, image-bind toggle → `updateNode` (undoable, persists to doc_json) | 1-1.5d | P5 |
| 7. Variables/themes panel (reuse v9 `VariableEditor.tsx`) → tokens live in `Document.variables`; then tear out admin tldraw + `DROP CASCADE admin.tenant_*` | 1d | P5,P6 |

### The 4 bridges (for when you build the embed)
1. **Format export**: ≈ `JSON.stringify(document)`; stamp `type:'image'` on image-bind nodes; preserve literal `actions` frame id; components keep a stable root id.
2. **Databinding UX**: BindingSection — per-prop "bind" toggle → field picker (Plasmic/Figma pattern); node-level "repeat over collection" → `replicate` with item-relative scope; typed slots/format gate which fields are offered.
3. **Registration seam**: canvas → P1 write endpoint → `v5_presets`/`v5_preset_versions` (`doc_json`). Do NOT route through admin's `ops_json` CanvasPort (flagged for deletion). Proxy admin→v5 (one origin; v5 stays internal).
4. **Adjacency editor**: panel writes the A→B drill map (P2 column); runtime reads at expand-time. Keep it data, not code.

## Minimal scope (do NOT rebuild Figma)
Goal = single-author scene-graph editor that lays out a card/detail preset, tags
atoms with bindings, sets tokens, Saves a render-clean `doc_json`. The v9 pieces
already deliver this; only NEW UI = BindingSection + (optional) adjacency panel.
**Explicitly skip:** realtime/multiplayer, comments/branching/merge, plugin
system/marketplace/DAM, responsive-breakpoint UI, vector/pen tools, gradient
editor beyond v9's, animation/prototyping, a separate token-management screen,
image crop/filters/masks. **For the first pilot the canvas may not exist in the
designer's hands at all** — owner hand-authors via the P1 endpoint.

## Risks (top)
- **Dependency inversion**: canvas before P1 = nowhere to save (P1 verified absent today).
- **Contract drift (highest-quality)**: miss `type:'image'` stamping or rename the `actions` frame → card renders with no image/no actions and **nothing errors**. P3 golden test exists to fail loud.
- **WebGL2 hard requirement, no fallback** (`renderer/.../context.ts`) — capability check needed (post-pilot concern).
- **Vendoring drift**: v9 is a SEPARATE repo; copies fork — record the source commit.
- **Adjacency on header vs versioned doc_json**: header column isn't version-rolled-back; decide explicitly.
- **Scope creep into "make the canvas nice"** is the #1 schedule risk — hold the minimal-scope line.

## Bottom line
Engine work = **hours, gating, reusable**. Canvas embed = **weeks, dependent,
post-pilot**. Ship Phases 1-3 (~10-13h Go/SQL) → pilot with hand-authored or
v9-standalone-authored presets. Build the embedded `/canvas-v2` only when a
paying tenant needs in-admin self-serve.

Source: workflow `wf_9636071c-9d0` (full result in session task `wshec10th`).
