# widget Self-Improve

Update the widget expertise from current code in `project/frontend/src/`.

## Variables

USE_DIFF: $ARGUMENTS (true | false, default: false)
EXPERTISE: .claude/commands/experts/widget/expertise.yaml
CODE_ROOT: project/frontend/src/
LINE_LIMIT: 500

## Instructions

- Scan code, update YAML, keep under LINE_LIMIT.
- Preserve overview (Shadow DOM strategy, FSD layers, embed pattern) unless code actually changed them.
- Refresh anything tied to specific names/paths (CSS files in ALL_CSS, atom subtypes, formation modes, getField keys, hook signatures).

## Workflow

### Step 1 — Read current expertise
Note which sections already exist so you only update, not duplicate.

### Step 2 — Choose scope
- USE_DIFF=true → run `git diff HEAD~10 -- project/frontend/src/ project/frontend/vite.config.js --name-only` and focus on changed files.
- USE_DIFF=false → scan all files in CODE_ROOT.

### Step 3 — Refresh facts (the high-drift surface)

**ALL_CSS list in `widget.jsx`** is the most common drift source. After ANY new CSS file added to a component:
- `grep -n "from './" project/frontend/src/widget.jsx` to see current imports.
- Confirm every component-level CSS file (e.g. `*.css` next to a `.jsx`) is either in ALL_CSS or intentionally excluded.
- Update `shadow_dom.css_strategy.files_inlined` to match.

**Formation modes in `FormationRenderer.jsx`** — re-read the modes block (`if mode === 'comparison'`, etc.). Update `formation_renderer.modes`. New modes are silently ignored by the renderer's default branch (falls through to `formation-list`) — flag in gotchas if a new mode is added without a corresponding render path.

**Atom subtypes/displays/formats/slots in `atomModel.js` and `AtomV2Renderer.jsx`**:
- `grep -E "subtype|display|format|slot" src/entities/atom/atomModel.js`.
- Update `atom_renderer.atom_model` arrays.
- New subtype without a render branch in AtomV2Renderer falls through to a default — likely visible in QA, but worth flagging.

**fillFormation getField keys** — re-read `src/features/chat/model/fillFormation.js getField()` switch. Update `instant_expand.field_getter.keys`. **CRITICAL**: this function MIRRORS Go's `productFieldGetter`/`serviceFieldGetter` in `project_v4/backend/internal/usecases/`. If they drift, instant expand silently breaks for new fields. Cross-check both.

**Session cache fields in `sessionCache.js saveSessionCache()`** — destructured argument list defines saved keys. Update `session_cache.saved_fields`.

**API endpoints in `apiClient.js`** — `grep -n "timedFetch\|fetch(" src/shared/api/apiClient.js`. Update `api_layer.endpoints`. Watch for new methods (currently sendChatMessage, getSession, initSession, expandView, navigateBack, sendAction).

**Build config in `vite.config.js`** — verify shadowDomCss plugin still suppresses `*.css` (without query). Verify build.lib.entry and build.lib.fileName.

### Step 4 — Capture cross-layer facts

These cross-layer relationships are the silent-failure surface — verify on each pass:

- **getField (frontend) vs productFieldGetter/serviceFieldGetter (backend)** — same key list, same null semantics. If backend adds a field, frontend instant expand misses it without erroring.
- **Formation JSON shape** — engine-v4 owns the producer (`engine_v4.BuildTreeMap` and downstream); FormationRenderer is the consumer. New widget shape (e.g. nested `sections.label`) needs both ends.
- **/widget/status endpoint** — guarded mount in widget.jsx. If backend removes or renames, widget fails closed (silently doesn't mount in prod).
- **VITE_API_URL injection** — start.sh injects in dev; project_v4/Dockerfile sets `/api/v1` in prod. apiClient.js fallback is hardcoded `localhost:8080` — known DRIFT after legacy backend snос.

### Step 5 — Refresh gotchas

Add a new gotcha when:
- A non-obvious behavior surprised you while answering a question (LEARN step).
- A recent commit fixed a bug whose root cause was a hidden invariant. Inspiration: `git log --since='4 weeks' -- project/frontend/`.

Remove a gotcha if the underlying behavior changed.

### Step 6 — Refresh hot files

Re-run: `find project/frontend/src -name "*.jsx" -o -name "*.js" -o -name "*.css" | xargs wc -l | sort -nr | head -10`. Update `active_workstreams.hot_files` if any new file crosses 250 LOC (signal of growth, possible split candidate).

### Step 7 — Report

```
widget expertise updated

Changes:
- Added: <items>
- Updated: <items>
- Removed: <items>

Lines: N / 500
```

## Constraints

- DO NOT exceed LINE_LIMIT.
- DO NOT mix in admin frontend facts (project_admin/frontend/) — that's the `admin` expert's scope.
- DO NOT cite project/backend/ — legacy, snесён 2026-04-29.
- DO update file:line references when files grow/shrink (atomic line numbers drift fast in this codebase).

## Output

Updated `expertise.yaml` plus a summary report.
