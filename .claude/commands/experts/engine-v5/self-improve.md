# engine-v5 Self-Improve

Update the engine-v5 expertise from current code in `project_v5/backend/internal/engine/`.

## Variables

USE_DIFF: $ARGUMENTS (true | false, default: false)
EXPERTISE: .claude/commands/experts/engine-v5/expertise.yaml
CODE_ROOT: project_v5/backend/internal/engine/
LINE_LIMIT: 500

## Instructions

- Scan code, update YAML, keep under LINE_LIMIT.
- Preserve the high-level mental model (overview, pipeline_steps, node_system).
- Refresh anything tied to specific names/files (op types, preset list, value types, test coverage).

## Workflow

### Step 1 — Read current expertise
Note which sections already exist so you only update, not duplicate.

### Step 2 — Choose scope
- USE_DIFF=true → `git diff HEAD~10 -- project_v5/backend/internal/engine/ project_v5/backend/internal/tools/tool_visual_assembly.go project_v5/backend/internal/prompts/agent2_prompt.go --name-only` and focus on changed files.
- USE_DIFF=false → scan all files in CODE_ROOT.

### Step 3 — Refresh facts

**Node types** — re-read `engine/node_types.go` constants block. Update `node_system.types.*`. Note any new types added (they affect frontend renderer too — flag cross-layer in gotchas).

**Op types** — re-read `engine/apply_ops.go` or individual `op_*.go` files. Update `ops.types`. New op type = update Agent2 system prompt and widget expert too.

**Pipeline order** — re-read the visual_assembly tool execution in `project_v5/backend/internal/tools/tool_visual_assembly.go` to confirm step ordering. Update `pipeline_steps.order`.

**System presets** — re-read `engine/presets/presets.go SystemPresetSeeds` map keys. Update `presets.system_presets.*`. Also check `SystemPresetDefaultReplicate` for changes to replicate defaults.

**System components** — re-read `SystemComponentSeeds`. Update `presets.system_components`.

**Value type files** — `ls engine/value_*.go`. Update `value_types.files` if new value helpers added.

**Tests** — `ls engine/*_test.go`. Note coverage gaps.

**TreeMap shape** — re-read `engine/tree_map.go BuildTreeMap` output shape. Update `tree_map.shape`. Drift breaks Agent2's modify-mode context.

### Step 4 — Cross-layer drift checks

- **prompt.go → agent2_prompt.go** — SystemPresetsBlock generated from SystemPresetSeeds. If SystemPresetSeeds changes, verify SystemPresetDescriptions in `engine/presets/prompt.go` is updated too (manual sync required).
- **Node property keys** — `engine/binding.go` decides which node key receives fieldBinding result (content vs fills[0].image vs value). If this changes, frontend `renderer/fillTemplate.js` binding rules must match.
- **InjectDefaultActions action kinds** — `engine/inject_actions.go`. If new kinds added, `renderer/actionDispatch.js` in the frontend must handle them or they're silently ignored.
- **TreeMap shape** — consumed by Agent2 for modify mode. If BuildTreeMap output changes, `agent2_prompt.go` TREE_MAP section must be updated.

### Step 5 — Refresh gotchas

Add a new gotcha when:
- A non-obvious behavior surprised you during a question.
- A recent commit fixed a hidden invariant bug.

Inspiration: `git log --since='4 weeks' -- project_v5/backend/internal/engine/`.

### Step 6 — Refresh active workstreams

Read `docs/v5-known-gaps.md`. Update any active workstream section in expertise.yaml if gaps have been closed or new ones identified.

### Step 7 — Report

```
engine-v5 expertise updated

Changes:
- Added: <items>
- Updated: <items>
- Removed: <items>

Lines: N / 500
```

## Constraints

- DO NOT exceed LINE_LIMIT.
- DO NOT cite V4 engine (`project_v4/backend/internal/engine_v4/`) — completely different model.
- DO NOT invent preset names not in `engine/presets/presets.go SystemPresetSeeds`.
- DO update preset descriptions in `engine/presets/prompt.go SystemPresetDescriptions` when preset descriptions change — that's the live source, not just this YAML.

## Output

Updated `expertise.yaml` plus a summary report.
