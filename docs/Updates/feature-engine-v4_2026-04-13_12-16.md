# feature/engine-v4 — KeepstarCanvas Phase 8 (design tokens editor + themes)

- **Branch**: `feature/engine-v4`
- **Date (UTC)**: 2026-04-13 12:16 UTC / 2026-04-13 15:16 MSK
- **Parent commit**: `9977e5d` (docs(updates): KeepstarCanvas Phase 7 session log)
- **Commit sha**: `b89bd9c`

## Context

Phase 7 added the components library and save-from-trace backend. Phase 8
completes the canvas admin frontend by adding a full Design System tab to
the inspector — the design tokens editor with theme support. This is the
final phase of the KeepstarCanvas 8-phase admin plan.

Design tokens allow tenants to define their brand-level variables (colors,
radii, spacing, fonts, shadows) that Phase 3 will inject into Agent2's
`<tenant_design_context>` block. Theme variants (e.g. "season:halloween")
let a tenant maintain parallel token sets for seasonal or contextual
overrides.

## Approach

### 1. Design System tab in inspector

Added third tab "Design System" to the right panel inspector (alongside
Properties and Data). Shows regardless of whether a preset is selected —
tokens are tenant-level, not preset-level.

### 2. Token create form

"+" button toggles a compact inline form:
- **Category dropdown**: color, radius, spacing, font, shadow, other
- **Name input**: token identifier (e.g. "accent", "card", "body")
- **Value input**: the token value (e.g. "#FFD600", "12px", "Inter")
- **Theme axis** (optional): dimension name (e.g. "season", "mode")
- **Theme value** (optional): variant within the axis (e.g. "halloween", "dark")
- **Save button**: POSTs to `POST /canvas/tokens`

### 3. Token display

Tokens rendered grouped by category, each group with a header:
- Token name in semibold
- Color swatch (14x14 rounded square) for hex color tokens
- Value in monospace
- Theme badge (purple pill, "axis:value") for themed tokens
- Hover-reveal delete button (x) per row

### 4. Theme axes summary

When any token has a `themeAxis`, a "THEME AXES" section renders above the
token list showing purple tag pills for each unique axis.

### 5. Upsert logic

Frontend `handleUpsertToken` does smart replacement: if a token with the
same name + category + themeAxis + themeValue already exists in local state,
it replaces in-place. Otherwise appends. This matches the backend's upsert
behavior.

### 6. Token count in left panel

A footer section "DESIGN TOKENS — N tokens" shows at the bottom of the
left panel, giving a quick summary without switching to the Design System
tab.

## Files changed

| File | Kind | Notes |
|---|---|---|
| `project_admin/frontend/src/features/canvas/CanvasPage.jsx` | modified | +164 LOC: tokens state, newToken form state, tokensByCategory/themeAxes memos, handleUpsertToken, handleDeleteToken, Design System tab JSX, token count footer |
| `project_admin/frontend/src/features/canvas/canvas.css` | modified | +116 LOC: token form, theme axes tags, token category groups, token rows, color swatches, monospace values, theme badges |

## Verification

**Build**: `npx vite build` passes clean (2744 modules, 3.54s).

**Manual verification** (live browser at localhost:5174/canvas):
1. Design System tab shows "DESIGN TOKENS" header with "+" button and
   empty state placeholder.
2. Created color token `accent=#FFD600` — renders under "COLOR" category
   with yellow swatch and monospace value. Token count "1 tokens" in left
   panel.
3. Created themed token `accent=#FF6B00` with `season:halloween` — renders
   with orange swatch, "season:halloween" purple badge. "THEME AXES"
   section shows "season" tag. Count updates to "2 tokens".
4. Deleted the halloween variant — token removed, theme axes section
   disappears, count back to "1 tokens".
5. Zero console errors throughout all operations.

## Known gaps / caveats

- **Theme axes are read-only display.** No UI to filter by theme or toggle
  theme variants. Phase 3's `<tenant_design_context>` will thread all theme
  variants to Agent2, which picks based on context.
- **No token editing.** Tokens can be created and deleted but not edited
  inline. Workaround: create a new token with the same name/category/theme
  to upsert-replace.
- **Category is fixed dropdown.** Custom categories beyond the 6 predefined
  ones (color/radius/spacing/font/shadow/other) require code change.
- **No token validation.** Values are free-form strings — no color picker,
  no CSS value validation. Intentional: tokens carry arbitrary values that
  the rendering engine interprets.

## KeepstarCanvas — all 8 phases complete

| Phase | Commit | Summary |
|---|---|---|
| 1 | `4ed581c` | Preset CRUD backend (6 tables, handlers, routes) |
| 2 | `de81801` | V4 tenant preset loader (resolve + cache) |
| 3 | *next* | Agent2 `<tenant_design_context>` prompt injection |
| 4 | `2345...` | tldraw canvas shell (3-panel layout, tabs) |
| 5 | `af8d...` | tldraw integration (preset tiles, selection sync) |
| 6 | `4c6ea5c` | Inspector editing + publish/delete |
| 7 | `12c7bba` | Components library + save-from-trace |
| 8 | `b89bd9c` | Design tokens editor + themes |

Admin frontend is feature-complete for the self-service loop. Next
milestone: Phase 3 (Agent2 prompt injection) to close the full
admin→publish→Agent2→chat pipeline.
