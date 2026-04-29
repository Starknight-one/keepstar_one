# backend-pipeline Self-Improve

Update the V4 backend-pipeline expertise from current code in `project_v4/backend/internal/tools/` and `project_v4/backend/internal/prompts/`.

## Variables

USE_DIFF: $ARGUMENTS (true | false, default: false)
EXPERTISE: .claude/commands/experts/backend-pipeline/expertise.yaml
CODE_ROOTS:
  tools:    project_v4/backend/internal/tools/
  prompts:  project_v4/backend/internal/prompts/
  registry: project_v4/backend/internal/tools/tool_registry.go
LINE_LIMIT: 250

## Instructions

- Scan code, update YAML, keep under LINE_LIMIT.
- Preserve mental model: registry / agent1 / agent2 / tools / cross_layer_callers / gotchas.
- Refresh anything tied to specific names, file paths, or behaviors.

## Workflow

### Step 1 — Read current expertise
Note which sections already exist so you only update, not duplicate.

### Step 2 — Choose scope
- USE_DIFF=true →
  ```bash
  git diff HEAD~10 --name-only -- project_v4/backend/internal/tools/ project_v4/backend/internal/prompts/
  ```
  Focus on changed files.
- USE_DIFF=false → scan all files in CODE_ROOTS.

### Step 3 — Refresh facts

**Registry**
- Re-read `tool_registry.go` `NewRegistry(...)`. List every `r.Register(...)` call. Update `registry.registered_tools`.
- Note any tool file in `internal/tools/` that is NOT registered — list it under `registry.not_registered` (legacy, ignore).

**Tool definitions**
For each `tool_*.go` file:
- Re-read `Definition()` — capture `Name`, top-level fields in `InputSchema.properties`. Update the matching `tools.<tool>.inputs` block.
- If a new tool was added → add a new `tools.<name>` entry mirroring the existing structure.
- If a tool was removed → drop the entry.

**Agent prompts**
- `prompt_analyze_query.go` (`Agent1SystemPrompt` const) — re-read first 80 lines. Update `agent1.decision_tree` if the FILTER/SEARCH/STYLE rules changed.
- `prompt_compose_widgets.go` (`Agent2ToolSystemPrompt` const) — re-read MODE section + PARAMETERS section. Update `agent2.modes` and `agent2.required_param`.

**Visual_assembly specifics**
- `tool_visual_assembly.go` Execute → check `mode` handling (rebuild vs modify), `validatePresetWithUserOps`, `resolvePresetForTenant`. If new helpers added, list under `tools.visual_assembly.helpers`.

### Step 4 — Capture cross-layer drift
Verify these still hold (read each file briefly):
- `internal/usecases/agent1_execute.go` — Agent1 loop
- `internal/usecases/agent2_execute.go` — Agent2 single-call
- `internal/usecases/pipeline_execute.go` — orchestrator
- `internal/handlers/handler_pipeline.go` — `POST /api/v1/pipeline`
Update `cross_layer_callers` if file moved or renamed.

### Step 5 — Refresh gotchas
Add a new gotcha when:
- A non-obvious behavior surprised you while answering a question (LEARN step).
- A recent commit fixed a bug whose root cause was a hidden invariant. Try:
  ```bash
  git log --since='4 weeks' --oneline -- project_v4/backend/internal/tools/ project_v4/backend/internal/prompts/
  ```
Remove a gotcha when its underlying behavior changed.

### Step 6 — Active workstreams
Skim `docs/PRE_LAUNCH_TASKS.md`. Update `active_workstreams.notes` to match currently-active waves touching tools or prompts (typical: B2, B4, B7).

### Step 7 — Report

```
backend-pipeline expertise updated

Changes:
- Added: <items>
- Updated: <items>
- Removed: <items>

Lines: N / 250
```

## Constraints

- DO NOT exceed LINE_LIMIT.
- DO keep `overview` and `agent2.pattern` stable unless code actually changed them.
- DO record tool names verbatim from `Definition().Name` — these are the strings the LLM emits.
- DO NOT mix in legacy V1/V2 facts (RenderProductPresetTool, FreestyleTool, GetCachePaddingTools, mock_tools.go) — those belong to `project/backend/` and are NOT registered in V4.
- DO NOT scan `ADW/adw.yaml` for line limits — that file is stale and out of sync (use the LINE_LIMIT here instead).

## Output

Updated `expertise.yaml` plus a summary report.
