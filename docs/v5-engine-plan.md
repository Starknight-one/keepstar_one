# V5 Engine Plan

> **Owner**: Vlad. **Doc author**: Claude (Opus 4.7).
> **Branch**: `v5`. **Started**: 2026-05-02.
> **Budget**: ~4 working days.

---

## Direction (locked)

V4 has a structural ceiling that no applier rewrite will lift (3-level Formation/Widget/Atom, no arbitrary nesting, no container-atoms, no adaptive recomposition). On a "draw a product landing" prompt the gap between V4 and v9 output is dramatic.

**Decision**: take **v9 as the engine foundation** (ops, scene-graph, components, variables, batch_design DSL), **port the V4 strengths into it** (binding, state with delta-stream, transition graph, actions, prefetch). Strip v9's editor UI from the chat runtime — chat needs the engine, not a canvas.

V4 in `project_v4/` stays as the production engine until V5 is ready to swap in. No path-deep ops on V4 (rejected — patches the ceiling, doesn't lift it).

---

## Two streams

### Stream A — Engine upgrade (priority)

The new chat-runtime engine, lives next to current backend. v9 ops/scene-graph + V4 capabilities ported on top.

### Stream B — Canvas as admin microservice (secondary)

Full-page v9 canvas embedded in admin. Tenants draw presets there (the role v9 was originally designed for). Canvas exports preset JSON in the engine's format. Stream B can ship after Stream A or in parallel — it does not block chat.

---

## Stream A — what to port from V4

### 1. Binding (the moat)

V4 binding is **per-instance + semantic**. LLM emits `slot:"price"` on an atom; engine matches slot ↔ field on the data record; engine fills the value. LLM never sees data indices, never picks fields. This is what keeps tool-call payloads small and quality high.

v9's variables are global design tokens — a different mechanism. They CAN be the substrate, but the V5 binding layer must add three things on top:

- **Per-instance scope** — when a preset is replicated for `data[0..n]`, each instance gets its own variable set (use v9 RefNode + descendants overrides as the scoping mechanism)
- **Semantic slot↔field matching** — a vocabulary of slot names (price/title/badge/hero/...) and a resolver that picks the field on the data object (V4 has `slot/subtype/format/wrapper` on each atom — port the vocabulary)
- **Multi-source data flattening** — port `ProductToMap`: `product` + `master_products.tier2` (JSONB) + `Extra` (per-listing overrides) merged into one map with precedence rules

**Prompt-side**: keep the V4 `<fields>` block (tenant exposes available fields to Agent2) and the **conditional `tree_map`** (Agent2 sees bound atoms differently from open atoms — this prevents emitting ops for already-bound atoms, the key token-saver).

Reference: `project_v4/backend/internal/tools/tool_visual_assembly.go` (ProductToMap), `project_v4/backend/internal/prompts/prompt_compose_widgets.go` (FIELD BINDING section), `docs/archive/New features/METADATA_DRIVEN_BINDING_2026-04-09.md`.

### 2. State + delta-stream (foundational for everything, not just Agent1)

**Sectional state** — `current` (data + meta + template), `view` (mode + focused), `viewStack` (snapshots for back-nav), `actions` (likes + cart), `conversationHistory`, `step`. Each section updates independently. See `project_v4/backend/internal/domain/state_entity.go`.

**Delta-stream** — append-only log of every change, with rich metadata: `source` (user/llm/system), `actor_id`, `trigger`, `delta_type` (add/remove/update/push/pop/rollback), `path`, `action`, `result_meta`. Used for:

- **Rollback** to any previous step (`state_rollback.go`)
- **State reconstruction** by delta replay (`state_reconstruct.go`)
- **Multi-actor coordination** — user clicks, LLM tool calls, and system events all write deltas with their actor identity
- **Pipeline tracing** — DeltaTrace feeds the debug UI
- **View navigation** — push/pop snapshots are deltas tied to step numbers

**Port as-is** — the model is sound, no need to redesign. Just rewire it to the v9 scene-graph.

### 3. Transition graph + prefetch

V4's `adjacentTemplates` + `fillFormation()` lets the widget drill-down to detail with no backend round-trip. The graph encodes "from this state, the user can go here" — like a sitemap. We know what buttons exist on a preset, we know what state each button leads to, so we precompute the adjacent presets and ship them with the response.

**Port the concept** — adjacency map per preset, prefetch payload in the pipeline response. The `Action` button vocabulary (LIKE/UNLIKE/CART_ADD/SEARCH/FILTER/SORT/LAYOUT/CLARIFY/ROLLBACK) already exists in `state_entity.go` — reuse it.

### 4. Microprésets

A detail-card preset is split into ~5-7 reusable groups (hero / info / actions / similar / specs / reviews / ...). Each group is its own v9 component (RefNode-based). LLM picks a top-level preset, then operates on one group at a time — single tool call with 5-10 parameters instead of 25.

**This is mostly preset-design discipline, not engine code**. v9's RefNode + descendants override is the primitive — already exists. Work = build the preset library with the right granularity.

### 5. Actions

Buttons on widgets emit actions (LIKE, CART_ADD, etc) that flow as deltas with `Source: SourceUser, ActorID: "user_click"`. Auto-injection of default actions on entity-bound widgets — same pattern as V4's `DefaultWidgetActions` in `project_v4/backend/internal/engine_v4/default_ops.go`.

### 6. Run-binding cache in state

v9's batch_design DSL has run-scoped bindings (`foo=I(...)` then `U(foo+"/x", ...)`) that **die between batches**. If LLM emits multiple tool calls in one turn, refs from batch 1 are gone in batch 2.

**Fix**: maintain a `binding_id → node_id` map in state, persisted across tool calls within a turn. Same role as V4's `tree_map` (compact context for next Agent2 turn) — just rename and adapt.

### 7. Token efficiency (hard constraint)

**Same or better than V4.** This is the constraint that disqualifies architectures, not a nice-to-have. Mechanisms:

- One tool call per turn for preset-based asks (the majority case)
- Microprésets cap parameter count at ~10 per call
- Conditional `tree_map` in prompt — bound atoms collapsed
- Prompt caching on system + tools + history (already in V4, port over)

If at any point a candidate change would push average tokens up — stop and rethink.

### 8. Rendering: Yoga-WASM, where to run it

v9 uses Yoga (Facebook's flexbox layout engine) compiled to WASM. Two options:

- **Browser-side layout**: chat frontend runs Yoga-WASM, receives raw scene-graph JSON, computes layout client-side. Simpler backend, frontend pipeline change.
- **Backend-side layout**: Go service embeds Yoga-WASM (via wasmer-go or similar), responds with laid-out positions. Heavier backend, frontend stays dumb.

**Default**: browser-side. Simpler. Revisit if frontend perf suffers.

---

## Stream B — Canvas in admin

Full-page v9 canvas embedded in `project_admin/`. Tenants edit preset components there. Export = write preset JSON to a server endpoint that stores it in the catalog. No bridge needed for runtime — chat reads presets from storage like everything else.

**Status**: blocked on Stream A's preset format being stable. Start after Stream A's binding port is done.

---

## What we drop

- V4 Formation/Widget/Atom hardcoded schema (replaced by v9 scene-graph)
- V4's 5-op vocabulary (replaced by v9's 7-op DSL)
- V4 hardcoded preset builders in `engine_v4/presets_*.go` (replaced by tenant-editable v9 components)
- Path-deep ops as a feature (not needed — v9 ops are already path-deep)

---

## Order of attack

State + binding **first** — they are the foundation. Graph and actions depend on state shape; if state is rebuilt later, graph/actions get rewritten too.

1. **Scaffold V5 backend** — skeleton Go service + v9 ops engine wrapped as a Go package or service
2. **Port state + delta-stream** — sectional state + AppendDelta + reconstruct + rollback. Adapt to scene-graph instead of Formation. Plumb through pipeline_execute.
3. **Port binding layer** — slot vocabulary, ProductToMap, conditional tree_map in prompt, per-instance scope via RefNode descendants. Tests on a single product card preset.
4. **Build first preset** end-to-end (product card, 5-7 groups). Verify tool-call payload size.
5. **Microprésets discipline** — port 3-5 of V4's existing presets into v9 component form. Verify token efficiency on real queries.
6. **Transition graph + prefetch** — port adjacency map, plumb prefetch payload.
7. **Actions** — auto-inject defaults, wire button → delta flow.
8. **Run-binding cache** — implement the `binding_id → node_id` persistence across batches in a turn.
9. **Pipeline integration** — wire into existing `POST /api/v1/pipeline` endpoint, keep API contract.
10. **Smoke test on real prompts** — measure tokens, latency, output quality vs V4.

---

## Hard constraints

- **API contract** — `POST /api/v1/pipeline`, `/navigation/expand`, `/navigation/back`, `/session/init` stay the same. Frontend should not need to change for engine swap.
- **Latency** — same or better than V4 (currently ~X ms p50 — measure first).
- **Token cost** — same or better than V4.
- **Backwards compatibility** — V4 stays runnable in `project_v4/` until V5 fully replaces. No breaking changes during build-out.

---

## Open questions (resolve when we hit them)

- **Pencil-style generative bindings** (the script-node mechanism that draws charts) — useful but generative and slow. Our binding stays deterministic + flag-driven for speed. Revisit later if charts become a need.
- **Existing chat frontend rendering** — current `FormationRenderer` is React-DOM and assumes V4 schema. New engine emits scene-graph. Frontend needs a new renderer; size of that work TBD when we touch it.
- **State schema versioning** — V4 sessions in DB use V4 state shape. V5 sessions are different. Need a migration strategy or fresh sessions only? Defer until we know schema is stable.

---

## What we explicitly accept as risk

- v9's RefNode + descendants as the binding-scope mechanism is unproven at scale (V4's per-instance binding is hand-rolled). If RefNode fights us, fall back to a custom scope wrapper.
- 4-day budget assumes Vlad-hours ≈ 4× Claude estimates. If a sub-task blows up, the order in "Order of attack" is also the priority order — items lower on the list slip first.
