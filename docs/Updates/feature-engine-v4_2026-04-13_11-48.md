# feature/engine-v4 — KeepstarCanvas Phase 7 (components library + save-from-trace)

- **Branch**: `feature/engine-v4`
- **Date (UTC)**: 2026-04-13 11:48 UTC / 2026-04-13 14:48 MSK
- **Parent commit**: `b8b6f08` (docs(updates): KeepstarCanvas Phase 6 session log)
- **Commit sha**: `12c7bba`

## Context

Phase 6 delivered editable inspector + publish/delete, completing the
preset lifecycle in the admin UI. Phase 7 adds the components library to
the canvas and introduces save-from-trace — the ability to capture Agent2's
visual_assembly output from a production trace and save it as a new draft
preset for editing.

## Approach

### 1. Components section in left panel

Added a "COMPONENTS" section below the presets list in the left panel:
- Header with title + "+" create button
- Create form: name input, creates via `POST /canvas/components`
- Component list: name + category badge + hover-reveal delete button
- Delete via `DELETE /canvas/components/:id` with confirmation
- Loads components on mount via `GET /canvas/components`

### 2. Save-from-trace backend endpoint

New `POST /admin/api/canvas/capture` endpoint:
- Accepts `{"traceId": "uuid"}` body
- Loads trace via existing `TraceAdapter.Get(ctx, id)`
- Extracts `agent2.toolInput` from trace data (supports two JSON shapes:
  direct `agent2.toolInput` and `steps[].toolInput` array)
- Creates a new draft preset named `from_trace_<first8chars>`
- Populates: category=product, entityType=product, defaultReplicate from
  toolInput, ops from toolInput, description with trace reference
- Returns the created `TenantPreset`

### 3. "Save as preset" button in trace detail

Added to the Agent2 section in `TraceDetail.jsx`:
- Button appears next to the Agent2 tool name tag
- Calls `POST /canvas/capture` with the current trace ID
- On success: replaces button with green "Saved as <name>" tag
- Disabled state while capturing

### 4. CanvasHandler extension

Added `traces *postgres.TraceAdapter` field to `CanvasHandler` so the
capture endpoint can read traces. Updated `NewCanvasHandler` signature
to accept the trace adapter.

## Files changed

| File | Kind | Notes |
|---|---|---|
| `project_admin/backend/internal/handlers/handler_canvas.go` | modified | +traces field, +handleCapture, +extractToolInput helper, +toolInputPayload struct |
| `project_admin/backend/cmd/server/main.go` | modified | Pass traceAdapter to NewCanvasHandler, register /canvas/capture route |
| `project_admin/frontend/src/features/canvas/CanvasPage.jsx` | modified | +components state/CRUD, +Components section in left panel |
| `project_admin/frontend/src/features/canvas/canvas.css` | modified | +65 LOC: components section styling |
| `project_admin/frontend/src/features/traces/TraceDetail.jsx` | modified | +capturing state, +"Save as preset" button in Agent2 section |
| `project_admin/frontend/src/features/traces/traces.css` | modified | +capture button style |

## Verification

**Build**: Both `go build ./...` (admin backend) and `npx vite build`
(frontend) pass clean.

**Manual verification** (live browser):
1. Canvas page shows "COMPONENTS" section at bottom of left panel with
   "+" button and "No components yet" placeholder.
2. Both preset tiles visible on canvas, inspector works.
3. Traces page loads (no local traces available for capture test).
4. Zero console errors.

## Known gaps / caveats

- **Save-from-trace not tested end-to-end.** Requires production traces
  with Agent2 visual_assembly output. Backend compiles and follows the
  same patterns as existing handlers.
- **Components not on canvas.** Phase 7 plan mentions component tiles on
  canvas and drag-to-slot — deferred. Components are list-only in the
  left panel for now. The CRUD infrastructure is complete.
- **Component editing limited.** No inline ops editor or component-edit
  mode (tldraw page swap). Components can be created/deleted but not
  visually edited yet.
- **No tenant isolation on traces.** The plan notes that the admin trace
  adapter isn't tenant-filtered. The capture endpoint uses the tenant from
  the auth context to create the preset, but reads traces globally. A
  `JOIN chat_sessions` filter should be added for multi-tenant safety.
