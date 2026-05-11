# widget Self-Improve

Update the widget expertise from current code in `project_v5/frontend/src/`.

## Variables

USE_DIFF: $ARGUMENTS (true | false, default: false)
EXPERTISE: .claude/commands/experts/widget/expertise.yaml
CODE_ROOT: project_v5/frontend/src/
LINE_LIMIT: 500

## Instructions

- Scan code, update YAML, keep under LINE_LIMIT.
- Preserve overview (Shadow DOM strategy, renderer architecture, embed pattern).
- Refresh anything tied to specific names/paths (node types dispatched, action kinds, format options, test count).

## Workflow

### Step 1 — Read current expertise
Note which sections exist so you only update, not duplicate.

### Step 2 — Choose scope
- USE_DIFF=true → `git diff HEAD~10 -- project_v5/frontend/src/ project_v5/frontend/vite.config.js --name-only`
- USE_DIFF=false → scan all files in CODE_ROOT.

### Step 3 — Refresh facts

**Node types dispatched in NodeRenderer.jsx** — re-read the switch statement. Update `renderer.node_renderer`. New type needs a component; missing component → console.warn + null.

**Frame.jsx drill logic** — re-read `computeDrillProps`. Update if drill detection heuristic changes. Cross-check with `usecases/prefetch.go` backend payload shape.

**format.js formats** — `grep -E '"currency"|"stars"|"date"' src/renderer/format.js`. Update `renderer.utilities.format_js.formats`.

**wrapper.js wrappers** — `grep -E 'case "' src/renderer/wrapper.js`. Update `renderer.utilities.wrapper_js.wrappers`.

**Action kinds in actionDispatch.js** — `grep -E 'case "' src/renderer/actionDispatch.js`. Update `renderer.utilities.action_dispatch.action_kinds`. New backend kind without a handler = silent no-op on click.

**RenderContext shape** — re-read `renderer/RenderContext.js`. Update `renderer.utilities.render_context.shape` if fields added/renamed.

**fillTemplate binding rules** — re-read `renderer/fillTemplate.js`. Update binding table. CRITICAL: must match backend `engine/binding.go` (text→content, image→fills[0].image).

**RendererErrorBoundary** — re-read. Update `renderer.error_boundary` if behavior changes (e.g. last-valid-state pattern added).

**Test count** — `ls project_v5/frontend/tests/*.test.jsx | wc -l` + total test count from last run. Update `tests.coverage`.

**widget.jsx apiBaseUrl detection** — re-read the detection logic. Update `entry_points.bootstrap.api_url_detection`.

### Step 4 — Cross-layer drift checks

- **fillTemplate (frontend) vs engine/binding.go (backend)** — binding key names and routing (content vs fills[0].image) must match. If backend changes which node key receives a fieldBinding, frontend fillTemplate breaks for instant drill.
- **Action kinds** — actionDispatch.js kinds vs engine/inject_actions.go InjectDefaultActions kinds. New injected kind without frontend handler = silent no-op on auto-injected buttons.
- **Node property keys** — Frame.jsx reads `node.layout.direction/gap/align/justify/wrap`, `node.width`, `node.action`. Text.jsx reads `node.content/format/wrapper/textStyle`. If engine-v5 renames these, rendering silently wrong.
- **Document shape** — SceneGraphRenderer expects `{version, children: [Node]}`. If engine-v5 changes Document envelope, null guard at line 15 catches it but shows blank.

### Step 5 — Refresh gotchas

`git log --since='4 weeks' -- project_v5/frontend/src/`.

Known persistent gotchas to re-verify each pass:
- Back button not implemented (A4 gap) — still true?
- No session cache (localStorage) — still true?
- Nested fieldBinding path silently null — still true?

### Step 6 — Report

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
- DO NOT cite V4 widget code (`project/frontend/`).
- DO NOT mix in admin frontend (`project_admin/frontend/`).

## Output

Updated `expertise.yaml` plus a summary report.
