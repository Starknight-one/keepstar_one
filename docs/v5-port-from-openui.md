# V5 ← OpenUI: Port Plan (no DSL)

**Status**: proposal, 2026-05-05
**Scope**: how to graft OpenUI's reactive-runtime capabilities onto V5 **without** porting the OpenUI Lang DSL.
**Reader**: V5 owner deciding what to ship before first paying merchant.

---

## 1. Goal & non-goals

**Goal**: give V5 the two runtime capabilities that OpenUI has and V5 doesn't:

1. **Reactive client-side state** — variant pickers (size / color), filters, and form selections that change UI without a backend round-trip.
2. **Reactive data refresh** — when a client variable changes, declared data sources auto re-fetch (no Agent1/Agent2 hop).

**Non-goal**: porting the OpenUI Lang DSL. Agent2 keeps emitting JSON ops. The DSL is a 2-3 week project (parser, prompt rewrite, frontend renderer rewrite) for a token-efficiency gain that doesn't move the product needle pre-launch. The **runtime model** is what matters; the **wire format** can stay JSON.

---

## 2. Why this matters (two scenarios)

### Scenario A — variant picker on a single product page
User is on a sneaker detail. Picks size 42. Picks color black. Clicks "add to cart".

**Today (V5)**: every size/color click goes through Agent1 (intent classification) + Agent2 (visual_assembly with `mode: modify`). ~2s + ~$0.001 per click. Three clicks = 6s + $0.003 wasted on what is purely UI state.

**With reactive state**: size/color clicks update `$selectedSize` / `$selectedColor` in the browser. Zero backend traffic. Only "add to cart" hits the server.

### Scenario B — multi-step form with dependent fetches (Vlad's "adjacency forms")
Step 1: pick category. Step 2: pick brand (filtered by category). Step 3: pick model (filtered by brand). Submit.

**Today (V5)**: each step = full pipeline turn. State is on the server. No way to declare "step 2's options come from a query that depends on step 1's selection" — Agent1 would have to figure that out from the conversation each time.

**Adjacency tree** (V5's `SystemAdjacency` map) handles **deterministic** transitions like grid → detail. Form transitions are **input-dependent** — you can't prefetch step 2 because you don't know what the user will pick in step 1. This needs a **reactive Query model**, not prefetch.

**With reactive Query**: step 2's options is `Query("catalog_brands", {category: $selectedCategory}, [])`. When user picks `$selectedCategory`, the engine auto-invalidates that query and fetches new options. No LLM in the loop until submit.

These two scenarios are why OpenUI looks competitive — not because of the DSL, but because of the runtime.

---

## 3. OpenUI's runtime model (6 primitives, condensed)

Read once. Quoted file refs from the OpenUI repo (thesysdev/openui, main branch).

| # | Primitive | What it does | OpenUI file |
|---|---|---|---|
| 1 | **Reactive Store** | `Map<string, any>` with observer subscription. ~100 lines. `set()` notifies subscribers via `useSyncExternalStore`. | `packages/lang-core/src/runtime/store.ts:13-58` |
| 2 | **State declaration** | `$varName = initValue` parsed as a `state` AST statement; registered in store on first render; persists across LLM turns (never overwritten by new defaults) | `packages/lang-core/src/parser/ast.ts:109-113`, `store.ts:58-74` |
| 3 | **Query w/ dependency tracking** | `Query(tool, args, defaults, refreshInterval?)` keeps args as **AST**, not evaluated values. Re-evaluated each render with current `$bindings`. Cache-key changes trigger re-fetch. | `packages/lang-core/src/runtime/queryManager.ts:142-255` |
| 4 | **Mutation w/ explicit fire** | `Mutation(tool, args)` registers but does NOT fire. Only `@Run(mutationRef)` inside an Action plan invokes it. Concurrent calls rejected (no double-submit). | `queryManager.ts:295-362` |
| 5 | **Action plan (sequential steps)** | `Action([@Run(m), @Set($x, val), @Reset($y), @Run(query)])`. Halts on Mutation failure; Query refetch never halts. | `packages/react-lang/src/hooks/useOpenUIState.ts:216-249`, `parser/builtins.ts:115-132` |
| 6 | **Form field binding** | `Input("email", ...)` writes to `store.contact.email`. `Select(..., $selectedSize)` writes to `$selectedSize` directly. Same hook (`useStateField`) handles both. | `packages/react-lang/src/hooks/useStateField.ts` |

**Key insight from reading their evaluator**: dependencies are tracked **by walking the argument AST and finding `StateRef` nodes**. No signals, no proxies. On each render, the evaluator collects `$ref` names; QueryManager builds a cache key from `(toolName + args + deps)`; if it differs from prior, re-fetch.

That's the entire reactive system. ~500 lines of TypeScript total, no exotic deps.

---

## 4. V5 today — what's there, what's missing

### Already there (verified in code)
- **Scene-graph engine** with 5 ops (insert/update/delete/move/override). `engine/apply_ops.go:47-145`
- **Server-side data binding** via `fieldBinding` attribute, `dataIndex` inheritance for replicate. `engine/binding.go:59-145`
- **Component refs + system registry** with DB-miss fallback. `domain/component.go`, hotfix 15.5
- **9-kind closed action vocabulary** (like / unlike / cart_add / cart_remove / drill_detail / back / open_category / external_link / search). `domain/user_action.go`
- **Frontend action dispatch** with optimistic drill via prefetch. `frontend/src/renderer/actionDispatch.js:26-50`
- **Adjacency prefetch** (1-level) for deterministic transitions. `engine/presets/SystemAdjacency`
- **`variable_resolver.go`** — but it resolves **design-token variables** (theme colors), NOT reactive client state. Different concept; don't confuse.
- **Server-side SessionState** with `state.Current.Data`, `state.Current.Template`, `ViewStack`, `Actions`. `domain/state.go:90-102`

### Missing
- **No client-side reactive variables.** All state is server-side; every interaction goes through pipeline.
- **No declarative client-side data refresh.** Filtering "products under 5000₽" goes through Agent1 → `state_filter` → Agent2 → render.
- **No form node type.** No way to collect multi-field input client-side and submit as a payload.
- **No action composition.** Each user-action is one kind; can't chain "submit then refresh then reset".
- **No streaming render.** `frontend/src/api/client.js:23-37` waits for full JSON.

The streaming gap is real but **separate** from this doc. This doc is about the reactive layer.

---

## 5. Port plan — minimum viable adoption

Adopt only what gives the two scenarios in §2. Skip everything else.

### P0 — without these, no variant picker, no client forms

**P0.1 — `$variable` references in node props**

Extend the V5 binding model: a node prop value can be `{$ref: "selectedSize"}` instead of a literal. At render time, the frontend resolves the ref against a client-side store.

- Today: `{type: "text", content: "Size 42"}`
- After: `{type: "text", content: {$ref: "selectedSize"}}`

The backend (Agent2) emits the `$ref` shape; the engine doesn't try to resolve it server-side; the frontend renderer reads the store.

**P0.2 — Client-side reactive store (frontend only)**

Port OpenUI's `createStore()` essentially verbatim. ~100 lines of JS. Lives in `project_v5/frontend/src/runtime/store.js` (new dir).

- Map<string, any>
- `get(name)`, `set(name, value)`, `subscribe(fn)`, `getSnapshot()`
- React adapter via `useSyncExternalStore`

**P0.3 — `set_var` action kind**

Add a 10th action kind to V5's vocabulary: `set_var`. Frontend-only. Click handler updates the store.

```json
{"kind": "set_var", "var": "selectedSize", "value": "42"}
```

Wire into `actionDispatch.js`. No backend involvement.

**P0.4 — Form node + `submit_form` action**

A `Form` is a Frame with `formName` attribute. Inputs inside know their parent form (React context). `submit_form` action collects all child input values into a payload and POSTs to a backend tool.

```json
{"kind": "submit_form", "formName": "contact", "tool": "send_message"}
```

This is the only place where a form interaction hits the backend. Everything before submit is client-side.

### P1 — for variant pickers with dynamic data

**P1.1 — Reactive `Query` declaration on a node**

A node can carry a `dataSource` attribute:
```json
{
  "type": "frame",
  "dataSource": {
    "tool": "catalog_search",
    "args": {"category": {"$ref": "selectedCategory"}},
    "defaults": {"results": []}
  },
  "children": [...]
}
```

Frontend QueryManager (port from OpenUI):
- Walks `args` for `$ref` nodes → dependency list
- Builds cache key from (tool + resolved-args + deps)
- On store change, if any dep is in dependency list and cache key differs → POST to backend, swap result in
- `defaults` rendered immediately

Result is exposed under `dataSource.id` (a generated id) and accessible via `{$query: "id", path: "results"}` in child nodes.

**P1.2 — Loading state**

Each query has a `status` (idle/loading/success/error) that subscribers can read. Show spinner / fade existing data while loading.

### P2 — nice but not required for v1

**P2.1 — Action composition (sequence)**

Already have 9 action kinds. Allow `kind: "sequence"` whose payload is `[action1, action2, ...]`. Frontend executes in order, halts on backend-action failure.

```json
{"kind": "sequence", "steps": [
  {"kind": "cart_add", "id": "..."},
  {"kind": "set_var", "var": "selectedSize", "value": null}
]}
```

**P2.2 — `@Reset` semantics**

`{"kind": "reset_var", "vars": ["selectedSize", "selectedColor"]}` — restore to declared defaults (declared where? a top-level `state` block in the document).

---

## 6. Special: forms & adjacency

Two transition models. Different solutions:

| Transition type | Example | Today (V5) | After port |
|---|---|---|---|
| **Deterministic** (target known at render time) | Click product card → detail page | Adjacency prefetch (works) | No change |
| **Input-dependent** (target depends on client selection) | Pick category → see brands for that category | Pipeline round-trip every step | Reactive Query on `$selectedCategory` |
| **Pure UI state** (no data, no fetch) | Pick size → highlight chosen variant | Pipeline round-trip | `set_var` + `$ref` in style |

For Vlad's mental model: **adjacency is server-side prefetch for known graph edges. Reactive Query is client-side cache invalidation for input-dependent edges.** Both are needed; they're complementary, not alternatives.

For multi-step product configurators (size → color → engraving → checkout), the right architecture is:
1. Initial Agent2 turn renders the whole multi-step Form node with all `$bindings` declared and all reactive `Query` declarations wired
2. User completes the form entirely client-side; selections trigger reactive query refetches but no full pipeline turns
3. Submit fires `submit_form` → backend tool processes (cart_add) → optionally returns new ops to update UI (success page)

This is exactly OpenUI's pattern from `e-commerce-product.oui`, just expressed as JSON ops instead of DSL statements.

---

## 7. Concrete file plan

### Backend (Go)

**New domain types** — `project_v5/backend/internal/domain/reactive.go` (new):
- `RefValue` — `{$ref: string}` envelope on prop values
- `QueryDecl` — `{tool, args, defaults}` envelope on node `dataSource`
- `FormDecl` — `formName` attribute discriminator

**Action vocab extension** — `domain/user_action.go`:
- Add `UserActionSetVar`, `UserActionResetVar`, `UserActionSubmitForm`, `UserActionSequence`
- `IsBackendHandled()` returns true only for `submit_form` (and existing like/cart)

**Engine — accept new attributes** — `engine/binding.go`, `engine/apply_ops.go`:
- `BindData` skips nodes whose `content` is a `RefValue` (don't try to resolve server-side)
- Validation: `RefValue` and `QueryDecl` propagate through ops without coercion

**Agent2 prompt** — `prompts/agent2_prompt.go`:
- Add a section "Client-side state" with 3-5 examples
- Add `set_var` / `submit_form` to the action enum in tool def
- Heuristic: "if this is a variant picker, declare `$selectedSize` and use `set_var`; do NOT call visual_assembly on every click"

**Tool** — `tools/tool_visual_assembly.go`:
- Schema: allow `dataSource`, `formName` attributes on nodes; `set_var`/`submit_form`/`sequence`/`reset_var` in action enum

### Frontend (JS/React)

**New runtime** — `project_v5/frontend/src/runtime/` (new dir):
- `store.js` — port of OpenUI's createStore, ~100 lines
- `queryManager.js` — port for client-side queries, ~150 lines (skip OpenUI's polling/refresh-interval; not needed)
- `evaluator.js` — resolve `{$ref: "x"}` and `{$query: "id", path: "..."}` against store + queryManager

**New context** — `renderer/StoreContext.js`:
- Provides store + queryManager to entire tree
- Hook `useResolvedValue(value)` that handles literals, $refs, $queries

**Renderer changes** — `renderer/nodes/Text.jsx`, `Image.jsx`:
- Wrap `content` access through `useResolvedValue`
- Re-renders on store change automatically (subscription via useSyncExternalStore)

**Form support** — `renderer/nodes/Form.jsx` (new):
- React Context provides `formName` to child Inputs
- Tracks field values in store under `<formName>.<fieldName>`

**Input nodes** — `renderer/nodes/Input.jsx`, `Select.jsx`, `RadioGroup.jsx` (new):
- Read/write either to a `$binding` (if prop is `{$ref: "x"}`) or to form state

**Action dispatch** — `renderer/actionDispatch.js`:
- Add `set_var` / `reset_var` / `submit_form` / `sequence` handlers
- `submit_form`: collect form payload from store, POST to `/api/v1/actions` with the named tool

### Tests

- Engine: `binding_test.go` — RefValue passes through unchanged
- Frontend: vitest fixtures with reactive variant picker (size/color)
- HTTP smoke: full flow — render variant page, simulate clicks, verify no pipeline calls until submit

---

## 8. Effort & sequencing

Time is calendar hours of focused coding by one developer. Add ~30% buffer.

| Item | Effort | Risk |
|---|---|---|
| P0.1 RefValue passthrough in engine + tests | 2h | low |
| P0.2 Client store + React adapter + tests | 3h | low |
| P0.3 `set_var` action + dispatch | 2h | low |
| P0.4 Form node + submit_form action | 4h | medium (form payload shape) |
| Agent2 prompt section + examples | 3h | medium (LLM behavior tuning) |
| P1.1 Client QueryManager + dataSource attr | 6h | medium (cache key, race conditions) |
| P1.2 Loading state UI | 2h | low |
| End-to-end smoke test | 3h | low |
| **Subtotal P0+P1** | **25h** | |
| P2.1 Sequence action | 2h | low |
| P2.2 reset_var | 1h | low |

**~3-4 days of focused work** for the full P0+P1 set. Realistic with interruptions: 5-7 days.

---

## 9. What NOT to port

| OpenUI feature | Why skip |
|---|---|
| The DSL (parser, prompt syntax) | Weeks of work; runtime parity is the win, not language parity |
| `@Each` / `@Filter` / `@Sort` builtins | V5 has `replicate: true` which covers @Each; filtering can be SQL-side via `state_filter` |
| Edit-mode merging by statement name | V5's modify-mode + ops is cleaner |
| OpenUI's component library (Card, Charts, etc.) | V5 has tenant presets; different concept |
| Streaming render | Different doc, different problem |
| Server-side store for $bindings | V5's SessionState already persists across turns; client store is per-message |

---

## 10. Decision matrix for the 10-day sprint

You have ~10 days to first paying merchant. Three options:

**Option A — ship V5 as-is, defer this port.**
Pro: zero new code, focus 10 days on A1/A3/A4 + chat fix + demo + outreach.
Con: variant pickers and forms on product pages will feel laggy (~2s per click). Mitigation: don't demo product pages with variants in v1; only browse/search/comparison flows.

**Option B — ship P0 only (no reactive Query).**
Pro: variant pickers (`$selectedSize` + `set_var`) work instantly; form submission works. ~10-15h of work.
Con: still no dependent dropdowns; multi-step configurators not on offer.

**Option C — ship P0 + P1 (full reactive).**
Pro: feature-parity with OpenUI on the two scenarios that matter. Demo can include a multi-step form.
Con: ~25h, eats 3-4 of the 10 days. Risk of burning sprint on infrastructure instead of product polish.

**Recommendation**: Option A for first paying merchant. Use the demo to validate that customers actually care about variant-picker latency. If they do, ship Option B in the post-launch first week. P1 is post-revenue.

The trap to avoid: porting reactive runtime instead of fixing A1 (greeting), A3 (pagination), A4 (back button), and the "chat doesn't work yet" blocker. Those gate the demo; this doc is for after the demo lands.

---

## 11. References

- OpenUI repo: https://github.com/thesysdev/openui
- OpenUI runtime: `packages/lang-core/src/runtime/{store,queryManager,evaluator}.ts`
- OpenUI form hooks: `packages/react-lang/src/hooks/{useStateField,useOpenUIState,useFormValidation}.ts`
- V5 known gaps: `docs/v5-known-gaps.md`
- V5 plan: `docs/v5-engine-plan.md`
