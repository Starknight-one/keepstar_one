# widget Question

Answer questions about the embeddable chat widget without making code changes.

## Variables

USER_PROMPT: $ARGUMENTS
EXPERTISE: .claude/commands/experts/widget/expertise.yaml
CODE_ROOT: project/frontend/src/

## Instructions

- IMPORTANT: If USER_PROMPT not provided, STOP and request a question.
- Read EXPERTISE first for the mental model (Shadow DOM, FSD layers, formation rendering, instant expand, session cache, theme).
- Verify any specific claim against current code in CODE_ROOT before answering — the YAML can drift.
- DO NOT make any code changes.

## Workflow

### Step 1 — Load expertise
Read `expertise.yaml`. It tells you:
- Shadow DOM isolation strategy and the ALL_CSS list in `widget.jsx`
- FSD layers (shared / entities / features) and what lives where
- FormationRenderer modes (composed / comparison / table / grid / list / single / carousel)
- Instant expand (fillFormation) and how it mirrors backend field getters
- Session cache (localStorage, 30 min TTL, what's saved/stripped)
- Theme system (CSS variables, marketplace)
- API layer (apiClient, endpoints)
- Known gotchas (CSS injection, hardcoded fallback URL, getField nested-field gap)

### Step 2 — Decide if a code read is needed
You MUST read code (not rely on YAML alone) when the question is about:
- Specific atom subtype rendering → read `entities/atom/AtomV2Renderer.jsx`
- Specific formation mode → read `entities/formation/FormationRenderer.jsx`
- Why an instant expand silently produces empty values → read `features/chat/model/fillFormation.js getField()` AND cross-check `project_v4/backend/internal/usecases/` getters
- Session cache shape → read `features/chat/sessionCache.js`
- Build/IIFE behavior → read `vite.config.js` and the IIFE wrapper at the bottom of `widget.jsx`
- Whether a behavior has a test → search for `*.test.{js,jsx}` (currently none — flag this)

### Step 3 — Answer
- Direct answer first.
- File paths with `project/frontend/src/<file>:<line>` where applicable.
- Mention the related expert (engine-v4, pipeline-agents, admin) if the answer crosses layers.
- For "why is this not rendering" / "why is value empty" questions, walk the path: API response → ChatPanel state → FormationRenderer dispatch → WidgetRenderer template → AtomRenderer.

## Constraints

- DO NOT change code or create files.
- DO NOT cite the legacy `project/backend/` code — that engine was deleted 2026-04-29; use `project_v4/backend/` for backend cross-references.
- DO NOT mix in admin frontend (`project_admin/frontend/`) — that's the `admin` expert.
- DO point at `experts:engine-v4:question` when the question is really about the Formation JSON producer rather than the consumer.

## Output

Direct answer with file references. Note any drift you spot between expertise.yaml and current code (so the next self-improve run can fix it).
