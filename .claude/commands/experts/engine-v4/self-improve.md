# engine-v4 Self-Improve

Update the engine-v4 expertise from current code in `project_v4/backend/internal/engine_v4/`.

## Variables

USE_DIFF: $ARGUMENTS (true | false, default: false)
EXPERTISE: .claude/commands/experts/engine-v4/expertise.yaml
CODE_ROOT: project_v4/backend/internal/engine_v4/
LINE_LIMIT: 500

## Instructions

- Scan code, update YAML, keep under LINE_LIMIT.
- Preserve the high-level mental model (overview, pipeline steps, related_experts).
- Refresh anything tied to specific names/files (presets, ops types, default_ops funcs, tests).

## Workflow

### Step 1 — Read current expertise
Note which sections already exist so you only update, not duplicate.

### Step 2 — Choose scope
- USE_DIFF=true → run `git diff HEAD~10 -- project_v4/backend/internal/engine_v4/ project_v4/backend/internal/tools/tool_visual_assembly.go project_v4/backend/internal/prompts/prompt_compose_widgets.go --name-only` and focus on changed files.
- USE_DIFF=false → scan all files in CODE_ROOT.

### Step 3 — Refresh facts
Re-extract from code:
- **Presets**: list every `RegisterPreset(&Preset{ Name: "..."` in `presets_*.go`. Update `presets.{product,nav,system}.presets` arrays. Capture each preset's Description verbatim if it changed.
- **Ops types**: re-read the `OpType` constants in `types.go`. Update `types.op_type.values`.
- **Op struct fields**: re-read `Op` struct in `types.go`. Update `types.op_struct.fields`.
- **ExecuteInput fields**: re-read in `types.go`. Update `types.execute_input.fields`.
- **default_ops funcs**: `grep -n "^func" default_ops.go`. Update `default_ops.funcs`.
- **Tests**: `ls *_test.go`. Update `tests` array.
- **Pipeline steps**: re-read `engine.go` Execute method. If new steps added/reordered, update `entry_point.pipeline`.

### Step 4 — Capture cross-layer facts
Check that these still hold:
- `tool_visual_assembly.go` still calls `Engine.Execute` and threads `PresetResolver` from `tenant_preset_loader.go`.
- `prompt_compose_widgets.go` is still the Agent2 system prompt (filename may change).
- Module path `keepstar_v4/internal/domain` is still the only import (run `goimports`-style check or just grep imports in any .go file).
Update `caller_relationships` and `agent2_contract.prompt_file` if drift detected.

### Step 5 — Refresh gotchas
Add a new gotcha when:
- A non-obvious behavior surprised you while answering a question (LEARN step).
- A recent commit fixed a bug whose root cause was a hidden invariant (read `git log --since='4 weeks' -- project_v4/backend/internal/engine_v4/` for inspiration).
Remove a gotcha if the underlying behavior changed and the warning no longer applies.

### Step 6 — Refresh active workstreams
Read `docs/PRE_LAUNCH_TASKS.md`. Update `active_workstreams.waves_touching_engine_v4` to match current open waves.

### Step 7 — Report

```
engine-v4 expertise updated

Changes:
- Added: <items>
- Updated: <items>
- Removed: <items>

Lines: N / 500
```

## Constraints

- DO NOT exceed LINE_LIMIT.
- DO keep `overview` and `entry_point.pipeline` stable unless code actually changed them.
- DO update preset names verbatim from code — they are user-facing identifiers Agent2 emits.
- DO NOT invent presets that aren't in `presets_*.go`.
- DO NOT mix in V1/V2 facts from `project/backend/internal/engine/` — that engine has its own (legacy) expert.

## Output

Updated `expertise.yaml` plus a summary report.
