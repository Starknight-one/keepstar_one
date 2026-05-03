# V5 Engine Plan

> **Owner**: Vlad. **Doc author**: Claude (Opus 4.7).
> **Branch**: `v5`. **Started**: 2026-05-02. **Last update**: 2026-05-03.
> **Budget**: started as ~4 days. Chunks 1-8 closed in ~6-8 working hours
> on day 1 (2026-05-02). Today (day 2, 2026-05-03) — close everything left
> in the list below, test, decide on prod swap.

---

## Snapshot — where we are now

**Chunks 1-11 closed.** Backend (chunks 1-9, 11) + frontend (chunks 10, 11) cover
the full P0-A (tool surface), P0-B (render path), and P0-C (interaction loop)
blocks. V5 can now:

- accept all three V4-compatible tool call shapes (preset / freestyle /
  multi-widget compose / ops-only modify), with parent-name aliasing
- back-stop tenants who haven't authored anything via 7 in-process
  system presets (DB miss → registry fallback)
- compute a compact `tree_map` and inject it into Agent2 in modify-mode
- render any scene-graph the backend emits via a Vite + React 19 +
  Shadow DOM bundle at `project_v5/frontend/`
- close the interaction loop end-to-end: closed action vocab (9 kinds)
  with `like`/`cart_add` auto-injected on entity-bound subtrees,
  POST /actions for backend kinds, POST /navigation/{expand,back}
  with one-level prefetch payload on every pipeline response,
  frontend dispatcher + clickable cards + back button

End-to-end exercised by a live HTTP smoke test against Neon + Haiku
(5 pipeline turns: search → system-fallback detail → multi-widget
compose → ops-only modify → fresh-session preset rebuild — plus
POST /actions/like, /navigation/expand, /navigation/back).

**What's still NOT shippable for prod**:

1. **Visual quality of cards is bad.** 4 render-quality gaps from the
   first manual test on 2026-05-03 still open (no grid layout, tenant
   field name mismatch heroImage/priceFormatted vs images/price, no
   size attr on cards, Agent2 modify-bias). See chunk 12 in
   `docs/v5-known-gaps.md`.
2. **Not deployed.** Everything still runs locally (V5 backend on :8084
   in dev to avoid clashing with V4 on :8082). See P1 items.
3. **No measured comparison vs V4.** No real-prompt smoke at scale,
   no token/latency parity numbers from production region. See P1.

Everything else (search quality parity, layout constraints, observability
UI, tenant canvas, internal hardening) is real but secondary — addressed
in P1 / P2 / Deferred sections.

---

## Status — chunks 1-10 (closed in `v5` branch)

| Chunk | Commits | What it shipped |
|---|---|---|
| 1 — engine port | `2093bec`, `5a0a89e` | v9 → Go: scene-graph + ops + components + variables (`internal/engine/`) |
| 2 — state + delta | `0746a07`, `5c494a9`, `599b4b7` | Sectional state (`current` / `view` / `viewStack` / `actions` / `conversationHistory` / `step`) + append-only delta-stream + reconstruct + rollback |
| 3 — binding | `524d6c3`, `4c1d580` | Per-instance scene-graph binding, slot ↔ field vocabulary, `ProductToMap` port, "грабля #1" closed (`__bound` only set when value resolved) |
| 4 — first preset | `08117b1`, `1989f21` | First `product_card` preset end-to-end + replicate fan-out + image binding |
| 5 — micropresets | `cb15111`, `245c11d` | v9 RefNode components, two presets sharing two reusable subtrees |
| 5.5 — hygiene | `fa39276`, `9a1ba6f`, `361554f`, `c112fa4` | Nested-ref `reusable` strip, image-fill `url` alignment, `format`/`wrapper` props, first cache-aware token measurement (V5 system+tools = 3001 tokens; gate raised to ≥4500 for chunk 6b) |
| 6a — anthropic | `9d0169f`, `06002d7`, `a701e40`, `8121c85` | LLMPort + Anthropic adapter (`ChatWithToolsCached`) + `count_tokens` against real API |
| 6b — agent2 | `76e6f33`, `e9d5f6e`, `3e59403`, `9f56f22`, `8539ed9` | Tool registry + `visual_assembly` tool + Agent2 prompt-builder with `<fields>` block + per-tenant prompt cache + first end-to-end Agent2 turn (prompt clears 4500-token gate) |
| 6c — http | `d0505cb`, `d223ca7`, `95018c5`, `92a0424` | HTTP server, handlers (`pipeline` / `navigation` / `session`), config, DI, migrations on boot, ops applier (JSON ops → engine.Command), live integration test against Neon + Haiku |
| 6d — tx + tracer | `5da40a2`, `c2ec2c5`, `b00a995`, `ceaf999` | `zoneWriteWithDelta` wrapped in `pgx.Tx`, `AddDelta` retry on 23505, SpanCollector + `domain.SpanFromContext` re-added on PG adapters and use cases |
| 7 — agent1 | `bae9fde`, `5871ffe`, `a176daa` | Agent1 tools (`catalog_search` keyword-only / `state_filter` / `history_lookup`) + Agent1 prompt + catalog digest + pipeline orchestrator (Agent1 → Agent2) + microcontext |
| 8 — trace upgrade | `58571d2`, `0b0376f`, `aa40504` | `Span.id` / `parent_id` / `status` / `attrs`; LLM spans carry tokens + cost; postgres spans carry rows + tenant; `request_id` flows through ctx |
| 9 — tool surface + presets + tree_map | `aad071a` | `visual_assembly` `preset` optional; freestyle / multi-widget compose / modify shapes; parent aliases (root/formation/""); 7 system presets via in-process registry (DB miss → fallback); BUILDING + COMPOSING sections in Agent2 prompt; `tree_map` computation + `<formation_tree>` injection in modify-mode; pair-aware history trim fix; live 4-turn HTTP smoke green |
| 10 — frontend renderer | `e9589b7` | New `project_v5/frontend/` (Vite + React 19 + Shadow DOM, IIFE 206KB / 64KB gz). SceneGraphRenderer → Frame/Group/Text/Image/Ref. format.js (currency/stars/percent/etc) + wrapper.js (badge/tag/button/etc; buttons render `<button>` with no-op onClick logging). 13/13 vitest jsdom smoke. |
| 11 — actions + nav + prefetch | `2e3df77` | Closed P0-C interaction loop. `domain.UserActionKind` (9-kind closed vocab), `engine.InjectDefaultActions` pass after BindData (auto-injects like + cart_add on entity-bound subtrees), POST `/api/v1/actions` (backend handles like/unlike/cart_add/cart_remove), POST `/api/v1/navigation/{expand,back}` with snapshot stack restore, hardcoded `presets.SystemAdjacency` map, `usecases.PrefetchBuilder` ships 1-level prefetch payload (`{adjacentTemplate, entities}`) on every pipeline response. Frontend `actionDispatch.js` + `fillTemplate.js` + `RenderContext` + clickable replicate clones (Frame.jsx) + back button. Agent2 tool-filter fix (only visual_assembly, mirroring Agent1 prefix-filter). 32/32 vitest, 5-turn live HTTP smoke + actions + nav green. |

Tree clean. Last commit `2e3df77`. Live HTTP smoke (chunk-11 five-turn
+ actions + nav run): ~47 s total, all assertions pass.

**Local dev**: V5 backend on `:8084` (V4 holds `:8082`), V5 frontend on
`:5173`. Backend reads `project_v5/.env` (PORT + DB + keys mirrored from
V4). See `docs/Updates/v5/v5_2026-05-03_15-08_chunk-10.md` for full
manual-check recipe.

---

## What's left — 25 items, prioritised for "ship today"

Status legend: ❌ not started · 🟡 partial · ⏸ deferred (not today) ·
🚨 ships V5 worse than V4 if not closed.

---

### P0-A — Tool surface regressions vs V4 🚨 — CLOSED in chunk 9

Closed by chunk 9 (`docs/Updates/v5/v5_2026-05-03_14-59_chunk-9.md`).
Live HTTP test exercises all three modes end-to-end against Neon + Haiku.

**1. Drop `preset` from `required` — allow freestyle build.** ✅🚨
Right now `tool_visual_assembly.go:60` declares `"required": ["preset"]`
so the LLM physically cannot call the tool without naming a preset. V4 lets
the LLM build a tree from primitives via `ops` only ("BUILDING FROM SCRATCH"
section, V4 prompt 148-205). Fix: make `preset` optional in JSON schema;
when absent, run ops on an empty `engine.Document`. Add a "BUILDING FROM
SCRATCH" section to `agent2_prompt.go` mirroring V4 lines 148-205 with
scene-graph syntax.

**2. Multi-widget composing in one tool call.** ✅🚨
V4 supports inserting MULTIPLE widget templates in one rebuild call (V4
prompt 207-228) — the engine groups them into sections (hero literal +
replicated gallery + literal CTA). V5 tool today only accepts ONE preset
name; the scene-graph supports any tree but the API does not expose this.
Fix: extend the schema so `ops` can carry top-level `frame` / `widget`
inserts; when present, do not require `preset`. Add a "COMPOSING" section
to the prompt.

**3. Verify modify path actually works end-to-end.** ✅🚨
V5 prompt says "if you see a tree_map, send ops only, no preset" (lines
186-203). But the tool requires preset (item 1) so this path is broken
*by the schema*. After items 1+2 close, write a live test that does:
  - turn 1: `preset: "product_card", replicate: 3`
  - turn 2: `ops: [update target=card-meta color=red]` *with no preset*
and assert the modification lands on the existing tree without rebuilding.
Mirror V4's mode-rebuild-vs-mode-modify split if needed (V4 enforces it via
explicit `mode` parameter; V5 can do it implicitly by "preset present →
fresh build, ops-only → modify").

---

### P0-B — Render path: user sees nothing without these

**4. Frontend renderer for scene-graph.** ✅
The chat widget (`project/frontend/`) today renders V4 Formation via
`FormationRenderer` → `WidgetRenderer` → `AtomV2Renderer`. V5 emits a v9
scene-graph (Frame / Text / Image / Ref nodes, arbitrary nesting). Need a
new renderer that walks the scene-graph and produces React DOM. Plan doc
default is browser-side Yoga-WASM for layout; sub-decision: use Yoga or
fall back to flexbox-via-CSS for the MVP and add Yoga later. Flexbox-CSS is
the faster path for today.

**5. Format + wrapper rendering on the frontend.** ✅
Backend stores `format` (currency/stars/percent/...) and `wrapper`
(badge/tag/button/...) as pass-through strings on leaf nodes. The actual
"4.5 → ★ 4.5" + "wrap in badge" step is the renderer's job (see
`v5-known-gaps.md` row 42). Pairs with item 4.

**6. Default presets seeded in DB.** ✅
V5 prompt names 12 presets (`agent2_prompt.go:51-65`) but the `v5_presets`
table currently has only what chunk-5 seeded (2 micropresets). When the LLM
asks for `product_detail` or `empty_not_found` the tool errors with "preset
not found". Need to seed at least: `product_card`, `product_card_compact`,
`product_card_horizontal`, `product_card_list_row`, `product_detail`,
`product_detail_horizontal`, `text_explainer`, `empty_not_found`,
`error_generic`. Two paths:
  - (a) hardcode them as Go-built scene-graph documents and insert via
    migration (fast, matches V4's `presets_*.go` model);
  - (b) author them in v9 canvas and export JSON (correct long-term but
    needs Stream B which is deferred).
For today: path (a). Mark them as "system-published" so the future canvas
microservice can override per-tenant.

---

### P0-C — Interaction loop: user can't do anything without these

**7. Auto-inject default actions on entity widgets.** ✅ (chunk 11)
`engine.InjectDefaultActions` walks Document, finds replicate clones +
single-entity detail subtrees, locates an empty `actions` frame and
appends like (♥) + cart_add (+) buttons bound to the resolved entity.
Idempotent — a populated `actions` frame is left alone (LLM-explicit
actions win). System seeds carry the empty `actions` frame as a hook.

**8. `POST /api/v1/actions` endpoint.** ✅ (chunk 11)
Wired in `handler_action.go`. Closed vocab in `domain.UserActionKind`
(9 kinds). Like/unlike/cart_add/cart_remove mutate
`state.Actions` via the existing `UpdateActions` zone-write (delta with
`Source: SourceUser, ActorID: "user"`). Other 5 kinds reject with 400
("client-handled"). `?sync=true` skips body for V4-style fire-and-forget.

**9. Drill-down to detail without round-trip (transition graph + prefetch).** ✅ (chunk 11)
`presets.SystemAdjacency` is a hardcoded Go map (`product_card*` →
`product_detail*`); lifts to `v5_presets.metadata` JSONB when the
canvas microservice ships. `usecases.PrefetchBuilder` runs after
Agent2, materialises the drill-target preset against ONE entity (no
replicate, no BindData — frontend binds on click), and ships
`{adjacentTemplate: {product: doc}, entities: {product: [...]}}` on
the pipeline response. Frontend `fillTemplate` + `actionDispatch`
fill template on click for instant drill.

**10. Back navigation.** ✅ (chunk 11)
`POST /api/v1/navigation/back` pops the view stack and restores
`view + template` from `ViewSnapshot.{Mode, Focused, Template,
PresetInUse}`. Snapshot stores the rendered template directly so
restore is a single zone-write — no Agent2 re-render.

**11. `POST /api/v1/navigation/expand` (drill-down handler).** ✅ (chunk 11)
Wired in `handler_navigation.go`. Pushes a snapshot, materialises the
drill-target preset (Materialise → ResolveAndInline → BindData →
InjectDefaultActions), updates view + template zones, returns the
rendered Document. Used as the fallback when prefetch (item 9) is
absent or the entity isn't in the prefetch entities list.

**12. Session endpoints (`POST /session/init`, `GET /session/{id}`).** 🟡
Chunk 6c shipped the routes but I didn't verify they're fully implemented
end-to-end against the V5 state shape. Verify + close any gaps.

**13. Pipeline endpoint contract — V4 swap or `/v5/` prefix.** ❌
The frontend today hits `POST /api/v1/pipeline` on V4 backend (port 8082).
Two options:
  - (a) add `/v5/` route prefix on V5 backend, point a flagged-on frontend
    build at it, run V4 + V5 in parallel during transition;
  - (b) swap the V4 endpoint to V5 in one go.
Plan doc says "API contract stays the same, frontend should not need to
change". For today: option (a) — easier rollback. The frontend renderer
(item 4) will be flag-gated on the same flag.

---

### P1 — Production readiness

**14. Railway deploy.** ❌
V5 has only run on `httptest.NewServer` from macOS hitting Neon. Need a
real Railway service: separate from V4's `v4-engine-production`, own DB
URL, env vars, `Procfile` / Dockerfile.

**15. Health check endpoints (`/healthz`, `/readyz`).** ❌
Required by Railway to know "is the service alive, ready for traffic".

**16. Smoke test V4 vs V5 on real prompts.** ❌
20-30 representative prompts (search / drill-down / modify / compose /
landing / empty / error). Run through both engines, compare:
  - did the engine emit something coherent?
  - tokens (input / output / cache_read) per turn;
  - cost per turn;
  - latency p50/p95;
  - output quality (subjective Vlad call).

**17. Latency baseline from production region.** ❌
Re-run the live HTTP test against Railway-deployed V5 (not localhost).
Capture turn 1 cold cache + turn 2 warm cache numbers separately.

---

### P1 — Search quality parity with V4

**18. Vector search / EmbeddingPort.** ⏸→❌
V4's `catalog_search` is hybrid: keyword SQL + pgvector cosine + RRF merge.
V5 today is keyword-only (`tool_catalog_search.go`). Quality cost: V5 loses
on semantic-only queries ("for dry skin" without category match). Port
OpenAI EmbeddingPort + pgvector index + RRF merge. Tool schema already
accepts `vector_query` parameter (kept for V4-prompt byte-stability) — just
needs the executor branch.

**19. Services entity in Agent1.** ❌
V4 supports both products and services in catalog_search. V5 only products.
StateData carries `Services []Service` for forward-compat but the executor
branch is absent (~50 lines: `ListServices` + `VectorSearchServices` on
catalog adapter + merge with products in tool result).

---

### P2 — Visual quality / polish

**20. Constraints engine (W8 / C1 / C3).** ❌
V4 normalises LLM output: trim badges over 12 chars, strip images on tiny
size, equalise heights across cards in a group. Without this V5 ships LLM
output raw. Port the per-atom (W8) and cross-widget (C1/C3) passes after
`BindData` in `engine.go`. Pair with item 21.

**21. `GroupID` stamping on replicate clones.** ❌
V4 `expand.go` assigns `rg-{counter}` shared by all clones from the same
template — used to scope cross-widget constraints (C1/C3). V5
`ExpandReplicates` does not stamp this because constraints (item 20) were
deferred. Add together with item 20.

---

### P2 — Observability

**22. `/debug/traces` waterfall UI.** ❌
Spans are rich (chunk 8) but no rendering. Build a static HTML page that
reads `/api/v1/traces?session=X` and draws a waterfall + click-to-expand
attrs. V4 has the equivalent under `handlers/handler_debug.go` — port the
HTML+CSS+JS but read the new span shape.

---

### P2 — Internal hardening

**23. Real `state_reconstruct` replay for Push / Pop / Rollback / Remove.** 🟡
`state_reconstruct.go:99-115` has stub branches for these delta types —
they only update `state.Step`, no actual replay. Means rollback "to step N"
works only when delta types are add/update + template replay. Bug-for-bug
ported from V4. Fix means walking the viewStack snapshots and replaying.

**24. Run-binding cache (`binding_id → node_id`).** ❌
v9's batch_design has run-scoped bindings (`foo=I(...)`, then
`U(foo+"/x", ...)`) that die between batches. If LLM emits multiple tool
calls in one turn, refs from batch 1 are gone in batch 2. Fix: persist a
`binding_id → node_id` map in state across tool calls within a turn. Same
role as V4's `tree_map` — already half there in the engine; needs the
state-side persistence and the cross-turn ctx wiring.

**25. Conversation history trim at 20 messages.** ❌
V4 trims to 20 messages before sending to LLM (cost protection). V5
prompt-builder doesn't. Add a trim pass in `agent2_execute.go` and
`agent1_execute.go` before the LLM call.

---

### Deferred — not today

**D1. Stream B canvas microservice.** ⏸
The full v9 canvas as a separate admin microservice that writes to
`v5_presets` / `v5_components`. Plan calls this Stream B; it can ship
after Stream A. Today P0-B item 6 hardcodes the system presets — that's
the bridge.

**D2. Multi-root v5 components.** ⏸
`Materialise` only appends `Document.Children[0]` from each component. One
root is the natural shape for canvas-authored components. Re-evaluate
when canvas microservice ships its first multi-root case.

**D3. ID collision guards / fresh ID minting in `ResolveAndInline`.** ⏸
Multiple refs to the same component yield trees that share descendant IDs
(only the resolved root is unique). Inner-ID collisions only matter for
path-deep ops on internal nodes — not exercised today. Re-evaluate when
constraints / path-deep ops chunk lands.

**D4. Catalog digest persistence + background refresh.** ⏸
V5 builds digest on demand and caches in-process forever. V4 stores it in
`tenants.catalog_digest` and refreshes via background job. Add when digest
staleness becomes a real product issue (not today).

**D5. Span migration for remaining PG methods.** ⏸
`GetTenantBySlug`, `GetProduct`, preset/component reads still use the
legacy `Start` API. Migrate piecemeal when their traces become useful.

**D6. Migration plan for live V4 sessions.** ⏸
What to do with users mid-conversation in V4 when we swap. Today's swap
path (P0-C item 13) uses a flag, so existing V4 sessions keep flowing
through V4 until they expire. No active-migration logic needed.

---

## Order of attack — today (2026-05-03)

**Done today (chunks 9 + 10)**:

1. ✅ **P0-A items 1-3** (tool surface): preset optional, multi-widget
   compose, modify-mode end-to-end. Closed in chunk 9 (`aad071a`).
2. ✅ **P0-B item 6** (seed system presets): 7 system presets via
   in-process registry, DB-miss fallback. Closed in chunk 9.
3. ✅ **P0-B items 4-5** (frontend renderer + format/wrapper): new
   `project_v5/frontend/` with scene-graph renderer + format/wrapper
   passes. Closed in chunk 10 (`e9589b7`).
4. ✅ **Bonus**: tree_map computation + injection (was hidden gap in
   chunk 7-8 — Agent2 prompt advertised tree_map but runtime never
   built one). Closed in chunk 9 alongside P0-A item 3.

**Next session (after Vlad's design discussion)**:

5. **P0-C items 7-8** (default actions + actions endpoint): widgets get
   buttons that work.
6. **P0-C items 9-12** (prefetch + nav handlers + session): drill-down +
   back work, sessions persist.
7. **P0-C item 13** (V5 route prefix + frontend flag): switch the
   frontend to V5 behind a flag.
8. **P1 items 14-15** (Railway deploy + healthz): V5 in real prod region.
9. **P1 items 16-17** (smoke test + baseline): real numbers vs V4.
10. **P2 items as time permits** — search vector / constraints / debug
    UI / hardening.

P0-C item 13 (frontend swap behind flag) is the gate for "decide on prod
swap" — but visual verification is now possible, so the call doesn't
have to wait for full P0-C completion. We can decide on V5's quality
based on local manual testing once chunk 10's UI renders things the
user is happy with.

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

## Order of attack — historical (chunks 1-8)

> This was the original chunk-1-through-11 plan. Items 1-6 closed on day 1
> (2026-05-02). Items 7-11 were reabsorbed into the 25-item list above
> (P0-C transition graph + actions, P2 run-binding, P0-C pipeline swap, P1
> smoke). Kept here for traceability.

State + binding **first** — they are the foundation. Graph and actions depend on state shape; if state is rebuilt later, graph/actions get rewritten too.

1. **Scaffold V5 backend** — skeleton Go service + v9 ops engine wrapped as a Go package or service ✅ (chunk 1)
2. **Port state + delta-stream** — sectional state + AppendDelta + reconstruct + rollback. Adapt to scene-graph instead of Formation. ✅ (chunk 2)
3. **Port binding layer** — fieldBinding vocabulary, ProductToMap, per-instance scope via dataIndex inheritance. ✅ (chunk 3)
4. **First preset end-to-end** — product card, 5-7 groups, replicate fan-out, image binding. ✅ (chunk 4)
5. **Microprésets via v9 RefNode components** — two presets sharing two components, validate reuse + replicate × resolve combo. ✅ (chunk 5)
5.5. **Hygiene** — nested-ref reusable strip, image-fill `url` alignment, format/wrapper pass-through, first token measurement, chunk-6 split. ✅ (chunk 5.5)
6. **LLM in the loop** — split into:
   - **6a** — Anthropic adapter shell + `count_tokens` integration; first real token measurement against the V4 baseline using actual API.
   - **6b** — Agent2 prompt-builder with `<fields>` block; first end-to-end Agent2 turn against V5 (no HTTP yet — invoked from a test entry point). **HARD GATE**: V5 system + tools prefix must clear ≥ 4500 tokens with `cache_control: ephemeral` applied, otherwise prompt caching is unstable and the per-turn win evaporates. Re-run `internal/engine/tokens/measurement_test.go` after each prompt-builder iteration.
   - **6c** — HTTP server + handlers (`pipeline`, `navigation`, `session`) + DI of state / preset / component migrations from `cmd/server/main.go`.
   - **6d** — `zoneWriteWithDelta` transaction fix + `AddDelta` retry/advisory-lock; span tracing port + re-add `domain.SpanFromContext` to PG adapters; LLMMessage cache_control hint plumbing.
7. **Transition graph + prefetch** — port adjacency map, plumb prefetch payload.
8. **Actions** — auto-inject defaults, wire button → delta flow.
9. **Run-binding cache** — implement the `binding_id → node_id` persistence across batches in a turn.
10. **Pipeline integration** — wire into existing `POST /api/v1/pipeline` endpoint, keep API contract.
11. **Smoke test on real prompts** — measure tokens, latency, output quality vs V4 (close the loop on the constraint declared in §7 above).

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
