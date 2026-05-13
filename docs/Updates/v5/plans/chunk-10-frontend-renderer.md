# Chunk 10 — Frontend renderer for V5 scene-graph

> Closes P0-B items 4 + 5 from `docs/v5-engine-plan.md`. Depends on
> chunk 9 (tool surface unblocked + presets seeded). NEW frontend
> directory at `project_v5/frontend/` — `project/frontend/` (V4 widget)
> stays untouched.

## Context

V5 backend emits a v9 scene-graph (`engine.Document` with arbitrary
nesting of frame / text / image / ref nodes + `format` / `wrapper` /
`textStyle` / `mediaStyle` / `layout` props). The chat widget today
(`project/frontend/`) renders ONLY V4 Formation (3-level Formation /
Widget / Atom). It has no idea what to do with a scene-graph.

Without a renderer, no human can verify any V5 output, no smoke test
against real prompts is meaningful, no decision about prod swap can be
made. This is the gate before P0-C (interaction) and P1 (deploy).

User decision: **new frontend at `project_v5/frontend/`** — V5 is one
service (backend + frontend). V4 frontend stays runnable in
`project/frontend/` until V5 fully replaces.

## Approach — minimum viable for «I can see what V5 makes»

Not aiming for feature-parity with V4 frontend in this chunk. Just:
- Take V5 pipeline response → render to DOM that looks reasonable.
- No actions, no session restore, no prefetch (those are P0-C).
- No Yoga-WASM (defer; flexbox-CSS is enough for today's preset shapes).

### Part A — project scaffold

**`project_v5/frontend/`** — new directory mirroring `project/frontend/`
structure but trimmed:

```
project_v5/frontend/
├── package.json          # vite + react 19 + nothing fancy
├── vite.config.js        # IIFE bundle, shadow DOM target
├── index.html            # dev test page
├── public/
│   └── widget.html       # standalone test page that loads the widget
└── src/
    ├── widget.jsx        # entry: mount in shadow DOM
    ├── WidgetApp.jsx     # ChatPanel + render area shell
    ├── widget.css        # imported as ?inline → injected into shadow root
    ├── api/
    │   ├── client.js     # POST /api/v1/pipeline + /session/init
    │   └── config.js     # base URL from data-api script attr
    ├── renderer/
    │   ├── SceneGraphRenderer.jsx  # walks Document.Children
    │   ├── NodeRenderer.jsx        # dispatch by node.type
    │   ├── nodes/
    │   │   ├── Frame.jsx           # flex container
    │   │   ├── Group.jsx           # plain wrapper
    │   │   ├── Text.jsx            # text + format + wrapper
    │   │   ├── Image.jsx           # img with object-fit
    │   │   └── Ref.jsx             # render resolved children (backend already inlined)
    │   ├── format.js               # currency / stars / percent / number / date
    │   └── wrapper.js              # badge / tag / pill / button / link / alert
    └── chat/
        ├── ChatPanel.jsx           # narrow right-column input + history
        └── MessageList.jsx
```

Match V4 conventions:
- React 19, Vite 7, IIFE bundle output.
- Shadow DOM (mode: open) for style isolation.
- CSS imported as `?inline` and injected into shadow root.
- Brand: `#5BA4D9` light blue + `#F0924A` orange. **No purple.** White
  bg, text `#1a1a1a`. Layout: 360px chat right column + flex-1 widget
  area on the left.

### Part B — scene-graph renderer

**`SceneGraphRenderer.jsx`:**

- Props: `document` (the V5 `Document` with `children: Node[]`).
- Iterates `document.children`, dispatches each to `NodeRenderer`.
- Top-level layout: vertical stack with gap, max-width 1200, centered
  in widget area (mirror V4 `widget-display-area`).

**`NodeRenderer.jsx`:**

- Props: `node`.
- Switch on `node.type`:
  - `frame` → `Frame`
  - `group` → `Group`
  - `text` → `Text`
  - `image` → `Image`
  - `ref` → `Ref` (just renders resolved children — backend already
    inlined them via `ResolveAndInline`)
  - any other type → log warn + render nothing
- Recursion: container nodes map their `children` through
  `NodeRenderer` again.

**`Frame.jsx`:**

- Reads `node.layout`:
  - `direction: "row" | "column"` → `flex-direction`
  - `gap: "xs" | "sm" | "md" | "lg" | "xl"` → CSS variable
    `--gap-{size}`
  - `align`, `justify` if present
- Reads `node.style` (background, padding, etc) if present.
- Recursively renders children.

**`Text.jsx`:**

- Reads `node.content` (raw value — string, number, array).
- Reads `node.format` → applies `format.js` transform:
  - `currency` → «$12.99» (locale-aware via Intl.NumberFormat)
  - `stars` → «★★★★☆ 4.5»
  - `stars-compact` → «★ 4.5»
  - `stars-text` → «4.5/5»
  - `percent` → «12.5%»
  - `number` → integer
  - `date` → «2 days ago» (relative) or absolute
  - `text` (default) → as-is
- Reads `node.wrapper` → wraps via `wrapper.js`:
  - `badge` → `<span class="kw-badge">`
  - `tag` → `<span class="kw-tag">`
  - `pill` → `<span class="kw-pill">`
  - `button` → `<button class="kw-button">`
  - `link` → `<a class="kw-link">`
  - `alert` → `<div class="kw-alert kw-alert-{variant}">`
- Reads `node.textStyle` → inline styles (fontSize / fontWeight / color /
  lineClamp / lineHeight).

**`Image.jsx`:**

- Reads `node.fills[0].url` (V5 image-fill shape after chunk-5.5 fix).
- Reads `node.mediaStyle.aspectRatio` → CSS `aspect-ratio`.
- Reads `node.mediaStyle.objectFit` → CSS `object-fit`.
- Fallback: `node.content` if `fills` absent (some atoms set content
  directly to a URL).

**`Ref.jsx`:**

- Backend has already inlined the component children via
  `ResolveAndInline` — so a `ref` node by the time it reaches the
  frontend is just a wrapper around its resolved `children`. Renderer
  treats it like a transparent group.

### Part C — API client

**`api/client.js`:**

- `pipelineRequest({ tenantSlug, sessionId, query })` → POST
  `/api/v1/pipeline` body `{tenant_slug, session_id, query}`.
- `initSession({ tenantSlug })` → POST `/api/v1/session/init`.
- Reads base URL from `data-api` attribute on the embed `<script>` tag
  (mirror V4). Dev: `http://localhost:8082`.
- Returns parsed pipeline response: `{state, spans, ...}` — extracts
  `state.current.template` as the Document.

### Part D — chat shell (minimal)

**`ChatPanel.jsx`** — narrow 360px right column:
- Input box + send button.
- Message history above input.
- On send: call `client.pipelineRequest`, append response to history,
  hand the new template to `WidgetApp` for left-side render.

**`WidgetApp.jsx`:**
- Holds `currentDocument` state.
- Renders `ChatPanel` (right) + `SceneGraphRenderer` (left).

### Part E — dev test page + smoke test

**`public/widget.html`:**

```html
<script src="./widget.js" data-tenant="hey-babes-cosmetics"
        data-api="http://localhost:8082"></script>
```

Open in browser, type a prompt, see widget render. Manual visual check.

**Smoke test** (Vitest + jsdom):
- Mock fetch returning a known V5 Document.
- Mount `<SceneGraphRenderer>` with the document.
- Assert: title text content, image src, badge wrapper element exists,
  N replicated cards present.

Three test fixtures:
- `fixtures/product-card-grid.json` — 3 product_card replicated (chunk
  9 output).
- `fixtures/product-detail.json` — single product detail (chunk 9).
- `fixtures/empty-not-found.json` — empty state.

### Part F — slog (browser console) observability

Per «логи чтобы трейсы смотреть можно было» — frontend side:
- On every render: `console.debug('[v5-renderer] document', {nodeCount,
  topLevelTypes, hasFormat, hasWrappers})`.
- On any unrecognised node type: `console.warn('[v5-renderer] unknown
  node type', node.type, node.id)`.
- Pipeline response: log `spans` summary (count, total ms) so we can
  inspect from devtools without a debug page.

## Files added (planned)

All under `project_v5/frontend/` — new directory.

| File | Notes |
|---|---|
| `package.json` / `vite.config.js` / `index.html` | scaffold (mirror V4) |
| `src/widget.jsx` / `WidgetApp.jsx` / `widget.css` | shadow DOM mount, layout shell |
| `src/api/client.js` / `config.js` | V5 pipeline + session client |
| `src/renderer/SceneGraphRenderer.jsx` | top-level walker |
| `src/renderer/NodeRenderer.jsx` | dispatcher |
| `src/renderer/nodes/Frame.jsx` | flex container |
| `src/renderer/nodes/Group.jsx` | wrapper |
| `src/renderer/nodes/Text.jsx` | text + format + wrapper |
| `src/renderer/nodes/Image.jsx` | image with media style |
| `src/renderer/nodes/Ref.jsx` | transparent (backend resolved already) |
| `src/renderer/format.js` | currency / stars / percent / number / date |
| `src/renderer/wrapper.js` | badge / tag / pill / button / link / alert |
| `src/chat/ChatPanel.jsx` / `MessageList.jsx` | minimal chat shell |
| `public/widget.html` | dev embed test page |
| `tests/renderer.test.jsx` | jsdom smoke + 3 fixtures |
| `tests/fixtures/*.json` | 3 backend responses |
| `docs/v5-engine-plan.md` | mark P0-B 4 + 5 done |
| `docs/Updates/v5/plans/chunk-10-frontend-renderer.md` | this plan |
| `docs/Updates/v5/v5_2026-05-03_<HH-MM>.md` | session log |
| `CLAUDE.md` | add `project_v5/frontend/` to layout table |

## Verification

```sh
cd project_v5/frontend
npm install
npm run build        # IIFE bundle builds
npm test             # vitest jsdom smoke

# Live browser check (manual):
# 1. cd project_v5/backend && go run ./cmd/server (port 8082)
# 2. cd project_v5/frontend && npm run dev (Vite dev server)
# 3. Open http://localhost:5173/widget.html
# 4. Type «покажи 3 крема» — see grid of product cards on left
# 5. Type «детали первого» — see product detail
# 6. Type «найди что-то для сухой кожи» — see whatever Agent2 builds
# 7. Open devtools → check spans logged on console
```

Acceptance criteria:
- `npm run build` produces a single IIFE `widget.js` that mounts in
  shadow DOM.
- Vitest smoke passes for all 3 fixtures.
- Manual browser check: typing a prompt produces something visible that
  resembles a product grid (not blank, not console-error spam).
- Console shows `[v5-renderer]` debug lines correlating with each turn.
- No purple anywhere (memory: brand is light-blue + orange).

## Known gaps after this chunk

- **No actions** (LIKE / CART buttons) — buttons render but clicks do
  nothing. P0-C item 7-8.
- **No drill-down / prefetch** — clicking a card doesn't expand to
  detail. P0-C item 9.
- **No back navigation** — P0-C item 10-11.
- **No session restore from localStorage** — F5 resets. P0-C scope.
- **No Yoga-WASM layout** — flexbox-CSS only. Defer until a preset
  needs auto-sizing.
- **Limited node types** — `rectangle` / `ellipse` / `line` / `polygon`
  / `path` / `note` / `prompt` / `context` / `icon_font` not implemented.
  Today's presets only use frame / group / text / image / ref. Add as
  needed.
- **No constraints rendering** (W8 / C1 / C3) — V5 backend doesn't
  produce them yet (P2 item 20). When it does, frontend may need to
  read additional layout hints.

## Coordination with chunk 9

- Chunk 9 ships first; produces 7 system presets in DB-fallback registry.
- Chunk 10 fixtures should ideally come from real chunk-9 output — run
  the live test, capture the response JSON, save as fixture. Single
  source of truth for «what the renderer must handle».

## What lands next

P0-C interaction loop (items 7-13) — actions, drill-down, prefetch,
nav, session, V5 route prefix. Vlad has «есть идея как оформить
красиво» — pause for design discussion before chunk 11.
