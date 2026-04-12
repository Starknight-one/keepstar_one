# feature/engine-v4 — KeepstarCanvas Phase 6 (inspector editing + publish/delete)

- **Branch**: `feature/engine-v4`
- **Date (UTC)**: 2026-04-13 10:56 UTC / 2026-04-13 13:56 MSK
- **Parent commit**: `a31e8f7` (docs(updates): KeepstarCanvas Phase 5 session log)
- **Commit sha**: `4c6ea5c`

## Context

Phase 5 delivered tldraw integration with read-only preset tiles and an
inspector that displayed preset metadata but couldn't edit it. The only way
to modify presets was via curl. Phase 6 makes the inspector fully editable
and adds publish/delete actions — tenants can now manage their preset
library entirely from the canvas UI.

## Approach

### 1. Editable inspector fields

Replaced the read-only `<div>` values in the Properties tab with input
controls:

- **Name**: `<input>` with live-as-you-type local state + `onBlur` persist
- **Category**: `<select>` with options product/service/nav/system
- **Entity Type**: `<select>` with options product/service
- **Description**: `<textarea>` with live local + `onBlur` persist
- **Default Replicate**: `<input type="checkbox">` with immediate persist

### 2. Optimistic updates

Every field edit:
1. Updates the local `presets` state immediately (React re-render)
2. Updates the tldraw tile shape props via `editor.updateShapes()` —
   the tile card reflects changes instantly (name, category, description,
   replicate badge)
3. Persists via `PUT /canvas/presets/:id` in background
4. On failure: rolls back both local state and tile props, shows alert

Text fields (name, description) use a dual strategy: `onChange` for instant
visual feedback, `onBlur` for the API call — avoids hammering the backend
on every keystroke.

### 3. Publish button

Green "Publish" button in the top bar, visible only for draft presets.
Calls `POST /canvas/presets/:id/publish` which:
- Flips the version status to published
- Bumps `admin.tenant_design_context_version` (invalidates Agent2 prompt)
- Updates all three surfaces: tile badge (amber→green), top bar badge,
  left panel badge
- Hides itself after success (published presets show only Delete)

### 4. Delete button

Red outline "Delete" button. Shows a confirm dialog, then:
- Calls `DELETE /canvas/presets/:id` via the new `api.delete()` method
- Removes the tile from the tldraw canvas
- Removes the preset from the left panel list
- Clears the selection

### 5. New draft → canvas sync

Phase 5 gap: creating a draft added it to the left panel but not the
canvas. Fixed — `handleCreateDraft` now also calls `editor.createShapes()`
to add a new tile at the next grid position.

### 6. API client extension

Added `api.delete(path)` to the shared `apiClient.js`.

## Files changed

| File | Kind | Notes |
|---|---|---|
| `project_admin/frontend/src/features/canvas/CanvasPage.jsx` | modified | Editable inspector, publish/delete, optimistic updates, new-draft tile sync, editor ref |
| `project_admin/frontend/src/features/canvas/canvas.css` | modified | +98 LOC: publish/delete buttons, select, textarea, checkbox, saving indicator |
| `project_admin/frontend/src/shared/api/apiClient.js` | modified | +1 line: `api.delete()` method |

## Verification

**Build**: `cd project_admin/frontend && npx vite build` — clean, 2744
modules transformed.

**Manual verification** (live browser):

1. Navigated to `/canvas` — tile and inspector render correctly.
2. Clicked tile → inspector shows editable fields (input, selects,
   textarea, checkbox).
3. Clicked "Publish" → tile badge changed draft→published, top bar +
   left panel updated, Publish button hidden.
4. Clicked "+" → typed "hero_banner" → Create → new tile appeared on
   canvas next to existing one, left panel shows both presets.
5. Zero console errors throughout.

## Known gaps / caveats

- **No undo/redo for inspector edits.** tldraw's built-in undo handles
  shape moves/resizes but not inspector field changes. Would need a
  command queue — deferred.
- **No ops editor.** Inspector edits metadata only (name, category, etc.).
  The ops array itself isn't editable from the UI yet — would need a
  structured tree editor or JSON editor. Deferred to a future phase.
- **Name field allows empty.** No client-side validation beyond the
  backend's `validatePresetName`. Could add inline validation later.
- **Published presets still editable.** Inspector allows editing metadata
  on published presets. Backend creates a new draft revision on edit,
  which is correct behavior, but the UI doesn't show the version split.
