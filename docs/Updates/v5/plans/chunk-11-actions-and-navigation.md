# V5 — Chunk 11: Actions + auto-injection + nav loop + 1-level prefetch

## Context

Chunks 9 + 10 closed the P0-A (tool surface) + P0-B (render path) gaps:
the engine accepts all three V4-equivalent call shapes, 7 system
presets back-stop the prompt's catalog via an in-process registry, the
new frontend at `project_v5/frontend/` renders any scene-graph the
backend emits. End-to-end was verified via 4-turn live HTTP smoke
against Neon + Haiku.

What blocks prod swap from here is the **interaction loop** (P0-C
items 7-11 from `docs/v5-engine-plan.md`): buttons render but onClick
is a `console.log` no-op, no actions endpoint, no drill-down, no back,
no prefetch. Today the user can SEE V5 output but can't INTERACT with
it — V5 is a write-once read-only widget.

User landed two key product/architecture decisions in this planning
session:

- **Closed action vocabulary** (~7-8 kinds, all with known runtime
  handlers — no LLM in the click loop).
- **Navigation graph as preset metadata**, not a separate adjacency
  table. When canvas microservice ships, tenant draws "card click →
  detail preset X" inside the same UI where they edit visuals. Today
  the map is hardcoded in Go.

And the framing that justifies why both belong in the **shop layer**
not the **chat layer**:

> «по сути мы же просто делаем магазин, который решейпится по запросу
> пользователя в чат, но логика магазина то остаётся»

Card → detail is a SHOP fact regardless of how the chat reshapes the
visual. Actions live where the shop's behavior lives, the LLM only
chooses the visual shape. This separation is locked in: action handlers
+ adjacency map live in backend as stationary knowledge, LLM cannot
break or pick from them; chat reshape and shop navigation are two
**orthogonal layers**.

## Locked-in decisions

- **Closed action vocab (9 kinds)**: `like` / `unlike` / `cart_add` /
  `cart_remove` / `drill_detail` / `back` / `open_category` /
  `external_link` / `search`. Future kinds = explicit enum extension +
  paired backend handler + frontend dispatcher; not a tenant-editable
  surface.
- **Hybrid injection (option C)**: backend auto-injects defaults
  (`like` + `cart_add`) on every entity-bound replicate clone via an
  engine pass after `BindData`; LLM may also emit explicit `action`
  props on its own atoms (e.g. `external_link` on a custom landing
  CTA).
- **Navigation graph**: today a hardcoded Go map in
  `engine/presets/adjacency.go` (only `product_card*` →
  `product_detail*`); future `v5_presets.metadata` JSONB read by the
  same lookup. One level of depth shipped now; second level is
  same mechanism + one more entity type when categories appear.
- **Prefetch shape mirrors V4 exactly**: one template per entity type
  (not N pre-built docs) + raw entity list shipped alongside; frontend
  binds template with chosen item on click. Cheap payload, instant
  drill, scales by adding entity types. Reference:
  `project_v4/.../pipeline_execute.go:427` `buildAdjacentTemplates`.
- **Action props on the atom node, not on a parent widget**. V4 used
  `widget.Actions []ActionDef` because it had a widget concept. V5
  doesn't — action lives directly on the button atom as
  `{type:"text", wrapper:"button", action:{kind:..., entity:{type, id}}}`.
- **Card-body click also drills detail**. `Frame.jsx` adds onClick on
  any frame carrying `__templateOrigin` (replicate clone root) →
  emits `drill_detail` with the dataIndex's entity. Inner buttons
  use `e.stopPropagation()`.

## Approach

### Backend

**Part A — domain.Action** (new file
`internal/domain/action.go`, ~40 lines):
- `ActionKind` enum (the 9 kinds above).
- `Action` struct: `Kind ActionKind, Entity *EntityRef, Params map[string]any`.
- Reuses existing `EntityRef` from `internal/domain/state.go` (already
  has `Type, ID`).
- Reuses existing `StateActions` (already has `LikedIds []string` +
  `CartItems []CartItem`, fully wired in `postgres_state.go:54-86,
  613-670` per the explore agent — no schema changes needed).

**Part B — engine.InjectDefaultActions** (new file
`internal/engine/inject_actions.go`, ~80 lines + test):
- Walk `Document`, find each replicate clone (carries
  `__templateOrigin` from `replicate.go::fanOut`).
- For each clone, find a child frame whose id contains `actions` (the
  system seeds already declare `{type:"frame", "id":"actions",
  "children":[]}` on every product card — see
  `internal/engine/presets/seed/product_card.json:46-50` and
  `product_card_compact.json` etc. — this is a deliberate hook).
- Resolve the entity for this clone using `dataIndex` + `data` (already
  passed to BindData; need to thread through).
- Append `{type:"text", content:"♥", wrapper:"button", action:{kind:"like",
  entity:{type:"product", id:<resolved>}}}` and the cart_add equivalent.
- Idempotent: skip if `actions` frame already has children (LLM may
  have emitted custom actions; respect them).
- Hook called from `tools/tool_visual_assembly.go::Execute` after
  `BindData` when (preset present AND replicate > 0) OR (freestyle with
  resolved entities). Skip on modify-only or when no entity data.

**Part C — POST /api/v1/actions** (new file
`internal/handlers/handler_action.go`, ~100 lines + test):
- Request: `{sessionId, kind, entity:{type, id}, params?, sync?}`.
- Switch on kind:
  - `like` / `unlike` → mutate `state.Actions.LikedIds` (toggle for
    `like`, remove for `unlike`).
  - `cart_add` / `cart_remove` → mutate `state.Actions.CartItems`
    (qty=1 default, qty=params.quantity if given).
  - other kinds → `400` with «handled by client» (drill_detail / back
    / search / external_link don't hit this endpoint — they're
    frontend-only or routed elsewhere).
- Persist via existing
  `state.UpdateActions(sessionID, actions, deltaInfo)` — already wired.
- Response: `{success: true, actions: state.Actions}`. `?sync=true`
  query param skips body for fire-and-forget V4 pattern (see
  `project_v4/.../handler_action.go:51` for behaviour).
- Wire route in `internal/handlers/routes.go`.

**Part D — POST /api/v1/navigation/{expand,back}** (new file
`internal/handlers/handler_navigation.go`, ~150 lines + test):
- Reference V4 impls at
  `project_v4/.../handler_navigation.go:29-183` and
  `project_v4/.../navigation_expand.go:92-98` /
  `navigation_back.go:66, 123`.
- **expand**: request `{sessionId, entityType, entityId}`. Push current
  `ViewSnapshot{Mode, Focused, Refs:[entityId], Step, Template,
  CreatedAt}` to ViewStack. Look up adjacency map for the source
  preset's drill target; Materialise that preset; bind one entity;
  store in state.current.template; update state.view.focused. Return
  `{success, document, viewMode, focused, stackSize, canGoBack}`.
- **back**: request `{sessionId}`. Pop ViewSnapshot. Restore
  `state.current.template = snap.Template`; restore state.view from
  snap. Return same shape as expand. If stack empty → 400 «cannot go
  back».
- Reuses existing `state.PushView` / `state.PopView` /
  `state.GetViewStack` (already wired in `postgres_state.go`).
- Wire two routes in `routes.go`.

**Part E — adjacency map + prefetch** (`internal/engine/presets/adjacency.go`
new ~30 lines + edits to `pipeline_execute.go`):
- New file with hardcoded map:
  ```go
  var SystemAdjacency = map[string]string{
      "product_card":              "product_detail",
      "product_card_compact":      "product_detail",
      "product_card_horizontal":   "product_detail_horizontal",
      "product_card_list_row":     "product_detail",
  }
  ```
- New struct `PrefetchPayload{AdjacentTemplate map[string]map[string]any,
  Entities map[string][]map[string]any}` exposed in `pipelineResponse`.
- In `usecases/pipeline_execute.go` after Agent2 returns, build
  prefetch:
  - Read `state.Current.Meta.PresetInUse` (added in Part F below).
  - Look up `SystemAdjacency[presetInUse]` → drill target name.
  - Materialise drill-target preset via `tools.NewVisualAssemblyTool`-
    style pipeline (or extract a helper) — bind ONE dummy entity
    (first product) — produce template Document.
  - Output `prefetch.adjacentTemplate["product"] = template` +
    `prefetch.entities["product"] = ProductsToMaps(state.products)`.
- If no products / no adjacency match → omit prefetch entirely
  (LLM-built freestyle view → no auto drill).

**Part F — track preset_in_use in state** (small edit to existing
files):
- `internal/domain/state.go` — add `PresetInUse string` to `StateMeta`.
- `internal/tools/tool_visual_assembly.go` — when preset path runs, set
  `state.Current.Meta.PresetInUse = presetName` before UpdateTemplate.
  When freestyle/modify, set to "".
- The lookup in Part E uses this value; "" means «no auto-drill
  available».

**Part G — pipeline response shape** (small edit to
`internal/handlers/handler_pipeline.go`):
- Add `Prefetch *PrefetchPayload `json:"prefetch,omitempty"``.
- Wire from `usecases.PipelineExecuteResponse`.

### Frontend

**Part H — api/actions.js** (new file
`project_v5/frontend/src/api/actions.js`, ~40 lines):
- `postAction({baseUrl, tenantSlug, sessionId, kind, entity, params, sync})`
  → POST /actions.
- `expandView({baseUrl, tenantSlug, sessionId, entityType, entityId})`
  → POST /navigation/expand.
- `goBack({baseUrl, tenantSlug, sessionId})`
  → POST /navigation/back.

**Part I — actionDispatch.js** (new file
`project_v5/frontend/src/renderer/actionDispatch.js`, ~80 lines):
- `dispatchAction(action, ctx)` where `ctx` carries
  `{apiBaseUrl, tenantSlug, sessionId, prefetch, entities, onUpdateDocument}`.
- Switch on `action.kind`:
  - `like` / `unlike` / `cart_add` / `cart_remove` → call `postAction`,
    on success update local actions cache (so liked-state can be
    surfaced visually in a future chunk).
  - `drill_detail` → if `ctx.prefetch.adjacentTemplate[entity.type]`
    exists AND `ctx.entities[entity.type]` contains the entity →
    `fillTemplate(template, entity)` → `ctx.onUpdateDocument(filled)`.
    Fire-and-forget POST to `/navigation/expand` for backend sync.
    Otherwise full POST and use returned document.
  - `back` → POST /navigation/back, set returned document.
  - `external_link` → `window.open(action.params.url, '_blank')`.
  - `search` → fill chat input with `action.params.query` (let user
    review + send).
  - `open_category` → POST /pipeline with preformatted query
    `{query: "show category " + action.params.categoryId}` (treats as a
    new turn; LLM may pick a category-grid preset).

**Part J — fillTemplate.js** (new file
`project_v5/frontend/src/renderer/fillTemplate.js` + test, ~60 lines):
- Port of V4 `fillFormation.js` for the scene-graph shape.
- Walk template `Document.children` recursively; for each leaf with
  `fieldBinding`, set `content = entity[fieldBinding]`. Image atoms:
  set `fills[0].url = entity[fieldBinding][0]` if array, else
  `entity[fieldBinding]`.
- Skip nodes with `__resolvedFrom` walk-into (component subtrees
  already had their bindings resolved at template-build time).
- Returns a fresh deep-cloned Document so prefetch template is not
  mutated.

**Part K — wrapper.js wire button onClick**
(`project_v5/frontend/src/renderer/wrapper.js`, replace existing no-op):
- For `wrapper === 'button'`, if `node.action` present →
  `dispatchAction(node.action, ctx)`.
- For `wrapper === 'link'`, similar.
- Click ctx flows down via React Context (new `RenderContext` provider
  set up in WidgetApp; renderer reads via `useContext`).

**Part L — Frame.jsx — entity-bound card click** (small edit to
`project_v5/frontend/src/renderer/nodes/Frame.jsx`):
- If `node.__templateOrigin` is set (replicate clone root) AND the
  context indicates `prefetch.adjacentTemplate[currentEntityType]`
  exists → wrap the frame in a clickable div that emits
  `drill_detail` with the entity for this clone's `dataIndex`.
- Inner buttons already stopPropagation by default since they're
  `<button>` elements + their onClick prevents bubbling.

**Part M — WidgetApp.jsx — prefetch refs + context**:
- New `useRef` for `prefetch.adjacentTemplate` and `entities` (V4
  pattern — `project/frontend/src/WidgetApp.jsx:20`).
- On pipeline response: store `resp.prefetch?.adjacentTemplate` +
  `resp.prefetch?.entities`.
- Provide `RenderContext.Provider` with
  `{apiBaseUrl, tenantSlug, sessionId, prefetch: {...}, entities: {...},
  onUpdateDocument: setDocument}`.

**Part N — Inject default actions into LLM-emitted nodes vs system
seeds**: the engine's `InjectDefaultActions` (Part B) handles BOTH
preset-driven cards (existing `actions` frame) and any freestyle
LLM-built card that has a frame literally named `actions` (or no
`actions` frame at all → engine creates one inside the clone root).
LLM-emitted explicit `action: {...}` props are respected as-is — no
duplication.

### Tests

**Backend unit:**
- `engine/inject_actions_test.go` — entity widget gets like + cart_add;
  idempotency on re-run; no inject when freestyle without entity data.
- `handlers/handler_action_test.go` (mock state) — like toggles,
  cart_add/remove mutate, sync=true skips body.
- `handlers/handler_navigation_test.go` (mock state) — expand pushes
  + sets focused; back pops + restores; back on empty stack errors.
- `usecases/pipeline_execute_test.go` (extend) — prefetch populated
  when preset_in_use ∈ adjacency; absent otherwise.

**Frontend unit (vitest):**
- `tests/fillTemplate.test.js` — bind template with one entity;
  arrays/strings/numbers; skip resolved-component subtrees.
- `tests/actionDispatch.test.js` — each kind dispatches correctly
  (mock fetch).
- `tests/wrapper-button-click.test.jsx` — click → dispatchAction
  called with right action.
- `tests/widget-app-drill.test.jsx` — drill_detail uses prefetch
  template; back calls API.

**Live HTTP smoke** (extend `handler_pipeline_live_test.go`):
- After turn 1 (product_card grid), POST /actions/like → 200, response
  shows entity in LikedIds.
- POST /navigation/expand → 200, returns Document focused on entity,
  ViewStack push observed.
- POST /navigation/back → 200, restores prior Document.

### Files changed (planned)

Backend:
| File | Status | Notes |
|---|---|---|
| `internal/domain/action.go` | added | ActionKind enum + Action struct |
| `internal/domain/state.go` | modified | + StateMeta.PresetInUse |
| `internal/engine/inject_actions.go` | added | engine pass + helper |
| `internal/engine/inject_actions_test.go` | added | unit tests |
| `internal/engine/presets/adjacency.go` | added | hardcoded preset → drill target map |
| `internal/handlers/handler_action.go` | added | POST /actions |
| `internal/handlers/handler_action_test.go` | added | mock state |
| `internal/handlers/handler_navigation.go` | added | POST /navigation/{expand,back} |
| `internal/handlers/handler_navigation_test.go` | added | mock state |
| `internal/handlers/handler_pipeline.go` | modified | + Prefetch field in response |
| `internal/handlers/routes.go` | modified | wire 3 routes |
| `internal/handlers/handler_pipeline_live_test.go` | modified | + 3 live HTTP turns |
| `internal/tools/tool_visual_assembly.go` | modified | set Meta.PresetInUse, call InjectDefaultActions |
| `internal/usecases/pipeline_execute.go` | modified | build prefetch payload |

Frontend (`project_v5/frontend/`):
| File | Status | Notes |
|---|---|---|
| `src/api/actions.js` | added | POST /actions, /navigation/{expand,back} |
| `src/renderer/fillTemplate.js` | added | port of V4 fillFormation |
| `src/renderer/actionDispatch.js` | added | switch on action.kind |
| `src/renderer/RenderContext.js` | added | React Context for ctx flow |
| `src/renderer/wrapper.js` | modified | button onClick wired |
| `src/renderer/nodes/Frame.jsx` | modified | entity-bound card click |
| `src/WidgetApp.jsx` | modified | prefetch refs + RenderContext.Provider |
| `tests/fillTemplate.test.js` | added | unit |
| `tests/actionDispatch.test.js` | added | unit |
| `tests/wrapper-button-click.test.jsx` | added | unit |
| `tests/widget-app-drill.test.jsx` | added | integration |

Docs:
| File | Status | Notes |
|---|---|---|
| `docs/v5-engine-plan.md` | modified | mark P0-C 7-11 done; add post-chunk-11 roadmap section |
| `docs/v5-known-gaps.md` | modified | add the «shop-layer / chat-layer» principle as locked-in design note (so it's not forgotten in future chunks); close any rows superseded by this chunk |
| `docs/Updates/v5/plans/chunk-11-actions-and-navigation.md` | added | frozen plan from this file |
| `docs/Updates/v5/v5_2026-05-03_<HHMM>_chunk-11.md` | added | session log |
| `docs/Updates/v5/README.md` | modified | + chunk 11 entry in index |
| `CLAUDE.md` | modified | mention chunk 11 in V5 status block |

## Verification

```sh
cd project_v5/backend
go build ./... && go build -tags=integration ./... && \
  go build -tags="integration live" ./... && go build -tags=tokens ./...
go vet ./... && go vet -tags=integration ./... && \
  go vet -tags="integration live" ./...
go test -count=1 ./...

# Live HTTP smoke (one-shot — costs ~$0.05)
ANTHROPIC_API_KEY=$KEY TEST_DATABASE_URL=$DB \
  go test -tags="integration live" -v -count=1 -timeout 8m \
  ./internal/handlers/... -run TestHTTPLiveSmoke

cd ../frontend
npm test                         # vitest jsdom smoke

# Manual browser check (cleared dev session):
# Term 1: cd project_v5/backend && export $(grep -v '^#' ../.env | xargs) && go run ./cmd/server  → :8084
# Term 2: cd project_v5/frontend && npm run dev → :5173
# In browser:
#   1. «покажи 3 крема» → grid renders (still 4 layout gaps from yesterday — expected)
#   2. Click a card body → instant drill to detail (no waiting bar)
#   3. Click ❤ in detail → console + network: POST /actions/like, response 200
#   4. Click + button → POST /actions cart_add, response 200
#   5. Click "back" (will need a back button in the chat shell — NOT in scope this chunk; verify via direct POST /navigation/back from devtools)
```

Acceptance:
- All four button kinds (like, cart_add, drill_detail, back) work
  end-to-end.
- Drill_detail is instant (no roundtrip latency) when prefetch was
  shipped; falls back to /navigation/expand otherwise.
- Backend log shows `POST /actions` and `POST /navigation/expand` /
  `back` rows with non-zero latency and `status=200`.
- Live HTTP smoke green at all build tags.

## Known gaps after chunk 11

- **Visual quality of cards still bad** — 4 render-quality gaps from
  the manual test on 2026-05-03 still open (no grid layout, tenant
  field name mismatch, no size, modify-bias). Chunk 12.
- **Cross-tenant chat inspection in Curator** still needed (the
  curator-UI gap added today). Chunk 13.
- **Liked / in-cart visual state** — buttons fire actions but don't
  change visually after click (no «filled heart», no cart count
  badge). Tracked as a P2 polish item; needs a backend post-action
  re-render OR a small frontend-only state cache. Defer.
- **Back button UI** — chunk 11 wires the `back` action plumbing but
  the chat shell doesn't have a back button yet. Stepper / breadcrumb
  is its own visual element, defer to chunk 12 (render polish).
- **`open_category` and `search` action handlers** are wired in
  `actionDispatch.js` but no preset emits them yet — they'll get
  exercised when category presets land.
- **Adjacency in DB** — `SystemAdjacency` is hardcoded Go map. Lifts
  to `v5_presets.metadata` when canvas microservice ships (Stream B,
  not today).

## Post-chunk-11 roadmap — what remains for V5 prod readiness

The 25-item list in `docs/v5-engine-plan.md` tracks everything; this
section is the **forward-looking subset** that fresh-me needs after
context compact.

### Immediate next chunks (priority)

1. **Chunk 12 — Render-quality polish**. Four overlapping gaps from
   the first manual test (logged in `docs/v5-known-gaps.md` section
   «Render-quality gaps surfaced by first manual test (2026-05-03)»):
   (a) grid-layout mechanism (no `formation.layout=grid + columns=N`
   equivalent in V5); (b) tenant field name mismatch (system seeds
   bind `heroImage`/`priceFormatted`, real catalog has `images`/`price`);
   (c) no `size` on cards; (d) Agent2 modify-bias (returning explicit
   `mode: rebuild|modify` parameter is the recommended fix). These
   four ride together — fix in one cohesive chunk touching seed JSONs
   + tool schema (mode + layout + columns) + prompt (rebuild bias) +
   renderer (kw-grid class + size clamps).

2. **Chunk 13 — Cross-tenant chat / trace inspection in Curator**
   (logged in the same gaps file under «Cross-tenant chat / trace
   inspection in Curator (no UI yet)»). Three layers:
   (a) DB persistence — new `v5_chat_session_traces` table per turn
   with spans + tokens + cost (today only deltas are persisted);
   (b) Curator backend endpoints — `GET /api/chats?tenant=&active=&q=`
   + per-session timeline + per-turn span fetch; (c) Curator frontend
   — `pages/ChatsPage.jsx` + `ChatDetailPage.jsx` + sidebar entry
   between «Tenants» and «Master». Same span schema as the deferred
   V5 `/debug/traces` waterfall UI (chunk-12 bucket inside V5), so
   persistence becomes the foundation for both surfaces.

3. **Chunk 14 — Railway deploy + healthz + baseline latency**. V5 has
   only run on `httptest.NewServer` from macOS. Real Railway service,
   own DB env vars, `/healthz` + `/readyz`, capture turn 1 cold cache
   + turn 2 warm cache numbers from production region.

4. **Chunk 15 — Smoke comparison V4 vs V5**. 20-30 representative
   prompts (search / drill / modify / compose / landing / empty),
   measure tokens, cost, latency p50/p95, subjective quality. Output
   feeds the prod-swap decision.

5. **Chunk 16 — V5 route prefix swap + frontend flag** (P0-C item 13).
   Add `/v5/` prefix on V5, point flagged frontend build at it, run
   V4 + V5 in parallel during transition. The actual V5 = production
   prod-swap moment.

### P1 — search quality parity

- **Vector search / EmbeddingPort** (port OpenAI embeddings + pgvector
  + RRF merge in `catalog_search`). Tool schema already accepts
  `vector_query` for V4-prompt byte-stability; just needs executor
  branch.
- **Services entity in Agent1** (~50 lines: `entity_type=service`
  branch in catalog_search/state_filter, `ListServices` /
  `VectorSearchServices` on catalog adapter, merge logic).

### P2 — polish + observability + hardening

- Constraints engine (W8 / C1 / C3) + GroupID stamping on replicate
  clones (the proper port; chunk-9 only added `__templateOrigin` for
  tree_map — the constraint-engine GroupID is a separate richer
  contract).
- `/debug/traces` waterfall UI (data is there since chunk 8; rendering
  not built; will share span schema with Curator chats UI from
  chunk 13).
- Real `state_reconstruct` push/pop/rollback replay (today stubs from
  V4).
- Run-binding cache (`binding_id → node_id` persisted across tool
  calls in one turn; needed for batch_design DSL semantics).
- Conversation history trim at 20 messages (V4 cost protection).
- tree_map cost optimisation — skip injection on preset-only calls
  (Vlad flagged 2026-05-03 — current impl injects every turn that has
  a non-empty template).

### Deferred (timing TBD)

- Stream B canvas microservice (lets tenants edit presets +
  components + adjacency in a v9 canvas). Until it ships, system
  presets back-stop everything.
- Multi-root v5 components.
- ID collision guards / fresh ID minting in `ResolveAndInline`.
- Catalog digest persistence + background refresh.
- Span migration for remaining PG methods (legacy `Start` API).
- Live V4 sessions migration plan when prod swap happens (today
  flag-based path keeps V4 sessions on V4 until expiry).

## Quick reference for fresh-me (post-context-compact)

Pointers if you're starting this work cold:

- **Branch**: `v5`. **Last commit** before chunk-11 work:
  `3ae37cf` (`docs(v5): add cross-tenant chat/trace inspection gap`).
- **Local dev**: V5 backend on `:8084` (V4 production binary holds
  `:8082` in this monorepo). Vite frontend on `:5173`. Backend reads
  `project_v5/.env` (PORT + DB + Anthropic + OpenAI + tenant slug +
  model — mirrored from V4).
- **First reads**:
  1. This file — chunk-11 plan above.
  2. `docs/v5-engine-plan.md` — strategic snapshot of where V5 is.
  3. `docs/v5-known-gaps.md` — registry of known issues including
     the 4 render-quality gaps (chunk 12) and the Curator UI gap
     (chunk 13). Both are out of scope for chunk 11 but will land
     soon after.
  4. `docs/Updates/v5/README.md` — index of all session logs
     (chunks 1-10 closed).
- **Architectural framing**: shop-layer vs chat-layer. Action handlers
  + adjacency map are SHOP knowledge (LLM never picks). Chat decides
  visual shape only. This separation MUST stay clean as you build —
  any time the LLM would pick an action kind or a navigation target
  by inference, push back and re-route the decision into the shop
  layer.
- **What was just shipped** (chunks 9 + 10):
  - Chunk 9 (`aad071a`): tool surface, 7 system presets via registry,
    tree_map computation, pair-aware history trim fix.
  - Chunk 10 (`e9589b7`): scene-graph frontend renderer at
    `project_v5/frontend/`.
  - Docs (`450224f`, `5d17fe4`, `3ae37cf`): post-chunk status, render
    gaps, Curator UI gap.
- **Dev scripts** if you need them again:
  ```sh
  # backend
  export $(grep -v '^#' /Users/starknight/Keepstar_project/Keepstar_one_ultra/project_v5/.env | xargs)
  cd /Users/starknight/Keepstar_project/Keepstar_one_ultra/project_v5/backend
  go run ./cmd/server > /tmp/v5-logs/backend.log 2>&1 &

  # frontend
  cd /Users/starknight/Keepstar_project/Keepstar_one_ultra/project_v5/frontend
  npm run dev > /tmp/v5-logs/frontend.log 2>&1 &
  ```
