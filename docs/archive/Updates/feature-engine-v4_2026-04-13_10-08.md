# feature/engine-v4 — KeepstarCanvas Phase 4 (admin canvas UI shell + routing)

- **Branch**: `feature/engine-v4`
- **Date (UTC)**: 2026-04-13 10:08 UTC / 2026-04-13 13:08 MSK
- **Parent commit**: `5d561ea` (feat(v4): KeepstarCanvas Phase 3 — tenant design context in Agent2 prompt)
- **Commit sha**: `281e582`

## Context

Phases 1–3 built the full backend pipeline: admin CRUD for tenant presets /
components / tokens (Phase 1), V4 runtime loader with TTL cache (Phase 2),
and the `<tenant_design_context>` block in Agent2's system prompt with
version-aware invalidation (Phase 3). But there was no admin UI to actually
use any of this — the only way to create/publish presets was via curl.

Phase 4 delivers the canvas UI shell — the static 3-panel layout that all
subsequent phases (tldraw integration, inspector editing, component library)
will build on top of. The goal is to get tenants navigating to `/canvas`,
seeing their preset library, and creating new empty drafts — all wired to
the Phase 1 API endpoints.

## Approach

### 1. CanvasPage component (3-panel flex layout)

`features/canvas/CanvasPage.jsx` — single-file page component with three
panels in a flexbox row:

- **Left panel** (260px): preset library loaded from `GET /canvas/presets`,
  with a `+` button that toggles a create-draft form. Each preset shows
  name + status badge (draft/published). Clicking selects it. Also shows
  a design tokens summary count from `GET /canvas/tokens`.

- **Center panel** (flex: 1): top bar showing the selected preset name +
  status, canvas viewport with a placeholder card pointing to Phase 5
  (tldraw integration). Shows "Select a preset or create a new draft"
  when nothing is selected.

- **Right panel** (280px): inspector with 3 tabs (Properties / Design
  System / Data) using the shared `Tabs` component. Properties tab shows
  selected preset metadata (name, category, entity type, description,
  default replicate). Design System and Data tabs are stubs for Phase 6+.

### 2. Canvas CSS

`features/canvas/canvas.css` — ~250 lines of plain CSS using the project's
existing `--color-*` CSS variables. The shell goes edge-to-edge by negating
`main-content` padding. Status badges use amber for draft, green for
published.

### 3. Route + nav link wiring

- `App.jsx`: added `<Route path="canvas" element={<CanvasPage />} />`
  inside the protected `DashboardLayout` route group.
- `DashboardLayout.jsx`: added `Canvas` nav link with `Palette` icon from
  lucide-react, positioned between Conversations and Traces.

### 4. API integration

Uses the existing `api.get/post` from `shared/api/apiClient.js`. Two calls
on mount:
- `GET /canvas/presets` — loads the preset library
- `GET /canvas/tokens` — loads design tokens for the summary count

Draft creation via `POST /canvas/presets` with default values (category:
product, entityType: product, defaultReplicate: true, empty ops). The
created preset is appended to the local list and auto-selected.

## Files changed

| File | Kind | Notes |
|---|---|---|
| `project_admin/frontend/src/features/canvas/CanvasPage.jsx` | new | ~200 LOC; 3-panel shell, preset list, create draft, inspector tabs |
| `project_admin/frontend/src/features/canvas/canvas.css` | new | ~250 LOC; edge-to-edge layout, status badges, inspector styling |
| `project_admin/frontend/src/App.jsx` | modified | +1 import, +1 route (`/canvas`) |
| `project_admin/frontend/src/features/layout/DashboardLayout.jsx` | modified | +1 icon import (`Palette`), +1 nav link |

## Verification

**Build**: `cd project_admin/frontend && npx vite build` — clean, 1771
modules transformed.

**Manual verification** (live browser):

1. Started admin backend (`go run ./cmd/server/`) + frontend dev server
   (`npx vite --port 5174`).
2. Logged in as `cyber.k1slota@gmail.com`.
3. Navigated to `/canvas` — 3-panel shell renders correctly.
4. Sidebar shows "Canvas" link with Palette icon, highlighted as active.
5. Left panel shows "No presets yet" empty state.
6. Clicked `+` — create-draft form appeared with autofocused input.
7. Typed `test_promo_card`, clicked Create — preset appeared in left panel
   with `draft` badge.
8. Center panel updated: top bar shows `test_promo_card · draft`, viewport
   shows placeholder with "Selected: test_promo_card".
9. Right panel inspector Properties tab populated with name, category,
   entity type, description, default replicate.
10. Design System and Data tabs show stub placeholders.

## Known gaps / caveats

- **No tldraw yet.** Center panel is a placeholder — Phase 5 mounts
  `<Tldraw>` with custom `PresetTileShape`.
- **Inspector is read-only.** Properties tab displays preset metadata but
  doesn't edit it — Phase 6 adds editable fields with optimistic ops.
- **No delete/publish buttons.** The preset list shows items but has no
  context menu or action buttons — deferred to Phase 6.
- **No error toast.** Draft creation failures show a browser `alert()` —
  fine for now, will be replaced by a proper toast component.
- **Preset list not paginated.** Loads all presets at once. Expected <50
  presets per tenant, no concern.
- **Design tokens shown as count only.** The left panel shows "N tokens"
  but doesn't list individual tokens — Phase 8 (tokens switcher) will
  expand this into a full editor.
