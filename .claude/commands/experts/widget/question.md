# widget Question

Answer questions about the V5 embeddable chat widget without making code changes.

## Variables

USER_PROMPT: $ARGUMENTS
EXPERTISE: .claude/commands/experts/widget/expertise.yaml
CODE_ROOT: project_v5/frontend/src/

## Instructions

- IMPORTANT: If USER_PROMPT not provided, STOP and request a question.
- Read EXPERTISE first for the mental model (SceneGraphRenderer, NodeRenderer, action dispatch, fillTemplate, RenderContext).
- Verify any specific claim against current code in CODE_ROOT before answering — the YAML can drift.
- DO NOT make any code changes.

## Workflow

### Step 1 — Load expertise
Read `expertise.yaml`. It tells you:
- Renderer architecture (SceneGraphRenderer → NodeRenderer → Frame/Group/Text/Image/Ref)
- RendererErrorBoundary (new — catches render errors, resets on new document)
- format.js + wrapper.js + actionDispatch.js + fillTemplate.js
- RenderContext shape ({apiBaseUrl, tenantSlug, sessionId, prefetch, onUpdateDocument, onSearch})
- Action kinds (9 total — like, unlike, cart_add, cart_remove, drill_detail, back, external_link, search, open_category)
- Known gotchas (no session cache, no back button, nested fieldBinding, same-origin auto-detection)

### Step 2 — Decide if a code read is needed
You MUST read code (not rely on YAML alone) when the question is about:
- Specific node type rendering → read `renderer/nodes/<NodeType>.jsx`
- Why an action isn't firing → read `renderer/actionDispatch.js`
- Why drill-down shows blank → read `renderer/nodes/Frame.jsx computeDrillProps` + `renderer/fillTemplate.js`
- Why a format renders wrong → read `renderer/format.js formatValue()`
- Build/IIFE behavior → read `vite.config.js` + `widget.jsx`
- Error boundary behavior → read `renderer/RendererErrorBoundary.jsx`
- Whether a behavior has a test → check `tests/*.test.jsx`

### Step 3 — Answer
- Direct answer first.
- File paths with `project_v5/frontend/src/<file>:<line>` where applicable.
- Mention related expert (engine-v5, pipeline-agents, catalog) if answer crosses layers.
- For "why is this not rendering" questions, walk: pipeline response → WidgetApp state → SceneGraphRenderer → NodeRenderer dispatch → node component → format/wrapper.

## Constraints

- DO NOT change code or create files.
- DO NOT cite V4 widget code (`project/frontend/`) — different architecture (FSD vs flat, FormationRenderer vs SceneGraphRenderer).
- DO NOT mix in admin frontend (`project_admin/frontend/`) — that's the `admin` expert.
- DO point at `experts:engine-v5:question` when the question is about Document/node shape (the producer, not the consumer).

## Output

Direct answer with file references. Note any drift spotted for the next self-improve run.
