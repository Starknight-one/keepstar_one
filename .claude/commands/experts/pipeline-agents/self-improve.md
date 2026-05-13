# pipeline-agents Self-Improve

Update pipeline-agents expertise from current code in
`project_v5/backend/internal/{usecases,prompts,tools,adapters/anthropic,adapters/openai}/`.

## Variables

USE_DIFF: $ARGUMENTS (true | false, default: false)
EXPERTISE: .claude/commands/experts/pipeline-agents/expertise.yaml
CODE_ROOT: project_v5/backend/internal/
LINE_LIMIT: 500

## Instructions

- Scan code, update YAML, keep under LINE_LIMIT.
- Preserve overview (two-agent contract, span/trace model, cache strategy) unless code changed.
- Refresh anything tied to specific names (tool names, prompt sections, anthropic methods, span names).

## Workflow

### Step 1 — Read current expertise
Note which sections exist so you only update, not duplicate.

### Step 2 — Choose scope
- USE_DIFF=true → `git diff HEAD~10 -- project_v5/backend/internal/usecases project_v5/backend/internal/prompts project_v5/backend/internal/tools project_v5/backend/internal/adapters/anthropic --name-only`
- USE_DIFF=false → scan all files in CODE_ROOT.

### Step 3 — Refresh facts

**Pipeline steps** — re-read `usecases/pipeline_execute.go` Execute method. Update `orchestration.pipeline_steps`. New step must be inserted in correct order.

**Agent1/Agent2 response fields** — awk struct defs from `agent1_execute.go`, `agent2_execute.go`. Update response_fields.

**Tool registry** — re-read `tools/` directory. Update `tools.registered_tools`. Watch for new tools.

**Agent2 prompt structure** — read `prompts/agent2_prompt.go`. Verify it's still a `var` (not `const`). Check `buildAgent2SystemPrompt()` injection — confirm `agent2PromptPart1 + presets.SystemPresetsBlock + agent2PromptPart2` pattern holds. Update `agent2.system_prompt.sections` if sections reordered.

**SystemPresetsBlock** — read `engine/presets/prompt.go`. Update `agent2.prompt_assembly` note if the generation function changes.

**PromptCache** — re-read `usecases/prompt_cache.go`. Update `agent2.prompt_cache` if methods change.

**Cache breakpoints in anthropic adapter** — search for `cache_control` in `adapters/anthropic/`. Update `llm_adapters.anthropic.cache_strategy`.

**New usecases files** — `ls project_v5/backend/internal/usecases/*.go`. Note any new files (prefetch, state_reconstruct, state_rollback, span_helper have been added before). Update `orchestration.related_files`.

**Trace table name** — verify `v5_chat_session_traces` is still the trace table. Update if renamed.

**Known gaps** — re-read `docs/v5-known-gaps.md`. Update `active_known_gaps.items` — close items that are fixed, add new ones.

### Step 4 — Cross-layer drift checks

- **visual_assembly tool → engine-v5** — only `tools/tool_visual_assembly.go` should call engine functions. Run `grep -rn "Materialise\|ApplyOps\|ExpandReplicates" project_v5/backend/internal/` to verify no rogue callers.
- **Agent2SystemPrompt preset catalog** — list `engine/presets/presets.go SystemPresetSeeds` keys. Confirm `engine/presets/prompt.go SystemPresetDescriptions` covers all seeds. If a seed has no description entry, it appears in the prompt with empty description — flag in gotchas.
- **Tool definitions order** — `tools/Registry.GetDefinitions()` must sort by name. Cache-busting if removed.
- **v5_chat_session_traces schema** — if curator side reads specific columns, schema changes cascade. Cross-check with `curator/` adapter code.

### Step 5 — Refresh gotchas

`git log --since='4 weeks' -- project_v5/backend/internal/usecases project_v5/backend/internal/prompts project_v5/backend/internal/tools`.

### Step 6 — Report

```
pipeline-agents expertise updated

Changes:
- Added: <items>
- Updated: <items>
- Removed: <items>

Lines: N / 500
```

## Constraints

- DO NOT exceed LINE_LIMIT.
- DO NOT expand engine-v5 internals — keep engine-v5 expert as source of truth for engine package.
- DO update tool argument schemas if they change — Agent2 emits exactly what's in the tool definition.

## Output

Updated `expertise.yaml` plus a summary report.
