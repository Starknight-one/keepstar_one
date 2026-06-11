# feat/widget-render-document — Wave C: widget-side renderDocument API

**Date:** 2026-06-12
**Branch:** `feat/widget-render-document`

## Context

Wave C (canvas master plan): the admin canvas preview modal needs to render
engine documents *visually* instead of showing raw JSON. Frozen cross-repo
contract: the SAME `dist/widget.js` merchants embed (no second bundle)
exposes `window.KeepstarV5Widget.renderDocument(hostElement, doc, opts) ->
{ unmount() }`, plus an auto-mount guard
(`window.__KEEPSTAR_V5_WIDGET__.noAutoMount === true` skips the chat
widget mount). Admin backend/frontend halves of the contract live in the
`keepstar-admin` repo — this change is the v5 widget side only.

## Approach

- **`src/widget-preview.jsx` (new)** — exports `renderDocument`:
  - Reuses or attaches the shadow root (`host.shadowRoot ??
    host.attachShadow(...)`) — `attachShadow` twice throws, so repeat calls
    on the same host must reuse.
  - Clears prior shadow children, injects the same Inter font `<link>` +
    `ALL_CSS` `<style>` (same `widget.css?inline` mechanism as
    `widget.jsx`; Vite dedupes the module so CSS ships once).
  - Renders `<RenderContext.Provider value={defaultRenderContext}>`
    wrapping `<RendererErrorBoundary>` wrapping `<SceneGraphRenderer>`.
    The defaults are the existing safe no-ops — zero network, no session
    init, actions no-op.
  - Returns `{ unmount() }` — unmounts the React root and clears the
    shadow root.
- **`src/widget.jsx`** — two surgical additions:
  - `window.KeepstarV5Widget = { renderDocument }` assigned ALWAYS (before
    the mount decision), even when auto-mount is skipped.
  - Auto-mount guard: `if (devConfig.noAutoMount !== true) mount()` —
    flag absent/false → byte-for-byte today's behavior (merchant pages
    unaffected).
- **`src/renderer/RenderContext.js`** — exported the existing private
  `defaultCtx` as `defaultRenderContext` so the preview entry provides the
  same safe defaults without duplication. No behavior change.
- **`vite.config.js`** — `test.css: true` so `widget.css?inline` yields the
  real stylesheet text under vitest (default stubs CSS imports to `''`,
  which made the style-content assertion vacuous).

Deliberately NOT routed through `WidgetApp` — it fires `initSession`
(network) on mount; `renderDocument` goes straight to the pure renderer.

## Files changed

| File | Change |
|---|---|
| `project_v5/frontend/src/widget-preview.jsx` | new — `renderDocument` implementation |
| `project_v5/frontend/src/widget.jsx` | global assignment + auto-mount guard |
| `project_v5/frontend/src/renderer/RenderContext.js` | export `defaultRenderContext` (rename of private `defaultCtx`) |
| `project_v5/frontend/vite.config.js` | `test.css: true` |
| `project_v5/frontend/tests/widget-preview.test.jsx` | new — 5 contract tests |

## Verification

- `npm test -- --run`: **9 files / 68 tests passed** (63 pre-existing + 5
  new). New tests assert: fixture renders into shadow root with style +
  font link and **zero `fetch` calls** (spy); double `renderDocument` on
  one host doesn't throw and replaces content (1 style / 1 link, old text
  gone); `unmount()` leaves 0 shadow children; `noAutoMount: true` →
  `#keepstar-v5-widget` NOT appended but global still assigned; no flag →
  appended to `document.body` with shadow root, exactly as today.
- `npm run build`: `dist/widget.js 232.68 kB │ gzip: 72.58 kB`.
  Bundle markers verified by grep: `window.KeepstarV5Widget={renderDocument:ym}`
  and guard `noAutoMount!==!0&&w()` present in the minified IIFE.
- **Size delta:** 231,923 → 232,683 bytes (**+760 B, +0.3%**) — the
  preview entry pulls no new deps, just the wiring. (The builder's
  original claim of +4,252 B used a stale 228,431 baseline — reviewer
  caught it; main at merge-base builds 231,923 B.)
- `cd project_v5/backend && go build ./...` — OK (sanity no-op; backend
  untouched).

## Known gaps

- Old React roots from repeated `renderDocument` calls on the same host
  are discarded, not unmounted (detached, no DOM, no timers — inert). The
  admin preview modal calls `unmount()` on close/re-render per contract,
  so nothing accumulates in practice.
- `opts` is accepted per contract signature but currently unused — no
  options are defined in Wave C.
- Admin-side contract halves (render-config endpoint, preview modal
  script injection) land in `keepstar-admin`, not here.
