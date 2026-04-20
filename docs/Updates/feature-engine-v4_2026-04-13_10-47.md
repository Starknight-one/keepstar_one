# feature/engine-v4 — KeepstarCanvas Phase 5 (tldraw integration + preset tiles)

- **Branch**: `feature/engine-v4`
- **Date (UTC)**: 2026-04-13 10:47 UTC / 2026-04-13 13:47 MSK
- **Parent commit**: `f526393` (docs(updates): KeepstarCanvas Phase 4 session log)
- **Commit sha**: `0b7c314`

## Context

Phase 4 delivered the 3-panel canvas UI shell — left panel (preset library),
center panel (placeholder), right panel (inspector). The center panel was a
static card saying "Canvas editor will render here (Phase 5: tldraw
integration)". Phase 5 replaces that placeholder with a live tldraw canvas
and introduces `PresetTileShapeUtil` — a custom tldraw shape that renders
each tenant preset as a visual card on the canvas.

## Approach

### 1. tldraw dependency

Installed `tldraw@3.15.6` (latest v3 stable). tldraw provides the canvas
engine (pan, zoom, select, resize) and custom shape registration via
`shapeUtils` prop on the `<Tldraw>` component.

### 2. PresetTileShapeUtil (custom shape)

`features/canvas/PresetTileShape.jsx` — extends `BaseBoxShapeUtil` from
tldraw. Defines a `preset-tile` shape type with props:

- `w`, `h` — dimensions (required by BaseBoxShapeUtil for geometry)
- `presetId`, `name`, `category`, `description`, `status` — preset metadata
- `defaultReplicate`, `opsCount` — behavioral metadata

The `component()` method renders a styled card inside `HTMLContainer`:
- Header: preset name + status badge (amber for draft, green for published)
- Body: category label + replicate indicator + description (2-line clamp)
- Footer: ops count or "empty"

`indicator()` renders a rounded selection rectangle.

### 3. CanvasPage tldraw integration

Rewired `CanvasPage.jsx`:

- Imports `Tldraw`, `useEditor`, `createShapeId` from `tldraw` + CSS
- `SHAPE_UTILS` array registered once (module-level, stable reference)
- `<Tldraw>` mounted in the center panel viewport with `hideUi` (no
  toolbar/menus — admin canvas is preset-focused, not a drawing tool)
- Inner `CanvasInner` component (rendered inside `<Tldraw>`) uses
  `useEditor()` hook:
  - **Shape creation**: on mount, creates `preset-tile` shapes from the
    loaded presets array, laid out in a 4-column grid (260x160 tiles, 32px
    gap). Deduplicates by `presetId` to handle hot-reload.
  - **Selection sync**: subscribes to tldraw store changes; when a
    `preset-tile` shape is selected, calls `onSelectPreset(presetId)` which
    updates the parent state → top bar + inspector populate.
  - **Camera persistence**: saves camera `{x, y, z}` to localStorage on
    every user-driven change. Restores on mount; falls back to `zoomToFit`
    if no saved position.
- Left panel preset clicks set `selectedId` state, which the inspector reads
  via `useMemo` lookup.

### 4. CSS updates

Updated `.canvas-viewport` from flex centering to `position: relative` +
`overflow: hidden` so tldraw fills the container via absolute positioning.

## Files changed

| File | Kind | Notes |
|---|---|---|
| `project_admin/frontend/src/features/canvas/PresetTileShape.jsx` | new | Custom tldraw shape: preset-tile with card UI |
| `project_admin/frontend/src/features/canvas/CanvasPage.jsx` | modified | Replaced placeholder with `<Tldraw>` + `CanvasInner`; selection wiring; camera persistence |
| `project_admin/frontend/src/features/canvas/canvas.css` | modified | Viewport containment for tldraw |
| `project_admin/frontend/package.json` | modified | +tldraw@^3.15.6 |
| `project_admin/frontend/package-lock.json` | modified | Lock file update (~3k lines, tldraw + @tldraw/* deps) |

## Verification

**Build**: `cd project_admin/frontend && npx vite build` — clean, 2744
modules transformed.

**Manual verification** (live browser):

1. Started admin backend + frontend dev server.
2. Logged in as `cyber.k1slota@gmail.com`.
3. Navigated to `/canvas` — 3-panel shell with tldraw canvas in center.
4. `test_promo_card` preset rendered as a tile on the canvas: name, draft
   badge, "product · replicate", "empty" footer.
5. Clicked the tile — blue selection handles appeared, top bar updated to
   "test_promo_card · draft", inspector Properties tab populated (Name,
   Category, Entity Type, Description, Default Replicate).
6. Clicked the preset in the left panel — same inspector update, tile
   remains selected on canvas.
7. Zero console errors after the `w`/`h` fix (initial attempt missed
   declaring `w`/`h` in `presetTileShapeProps` — tldraw v3's
   `BaseBoxShapeUtil` requires them in the props schema).
8. tldraw pan/zoom works (scroll to zoom, drag to pan).

## Known gaps / caveats

- **hideUi leaves tldraw watermark.** The "MADE WITH TLDRAW" badge persists
  in the bottom-right corner. Acceptable for admin-only tool.
- **No canvas → left panel sync.** Clicking a tile updates the inspector,
  but doesn't visually highlight the corresponding left panel item via
  tldraw (left panel highlight works via React state, not tldraw selection).
- **No new preset → canvas sync.** Creating a new draft from the left panel
  adds it to the list but doesn't create a tile on the canvas (would need
  editor ref in the parent). Deferred to Phase 6.
- **Tile shape is read-only.** `canEdit()` returns false. Phase 6 will add
  editable fields via the inspector, not direct tile editing.
- **Camera persistence is global.** Uses a single localStorage key, not
  per-tenant. Fine for single-tenant admin sessions.
- **Large chunk warning.** tldraw adds ~550KB gzipped to the bundle. Could
  be code-split via dynamic import in a future optimization pass.
