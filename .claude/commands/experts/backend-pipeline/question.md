# backend-pipeline Question

Answer questions about the V4 LLM tool surface and Agent1/Agent2 system prompts without making code changes.

## Variables

USER_PROMPT: $ARGUMENTS
EXPERTISE: .claude/commands/experts/backend-pipeline/expertise.yaml
CODE_ROOTS:
  tools:   project_v4/backend/internal/tools/
  prompts: project_v4/backend/internal/prompts/

## Instructions

- IMPORTANT: If USER_PROMPT not provided, STOP and request a question.
- Read EXPERTISE first — it has the registry, agent decision trees, all 4 tools, modes, gotchas.
- For specific questions about prompt wording or tool input schema, read the actual `.go` file — prompt strings drift more often than YAML.
- DO NOT make code changes.

## Workflow

### Step 1 — Load expertise
The YAML covers: 4 registered tools (catalog_search, _internal_state_filter, _internal_history_lookup, visual_assembly), Agent1/Agent2 prompt files, modes (rebuild|modify), prompt-caching status, cost model, cross-layer callers, gotchas.

### Step 2 — Decide whether to read code
Read the actual file when the question is about:
- Exact prompt wording → `prompt_analyze_query.go` (Agent1) or `prompt_compose_widgets.go` (Agent2)
- Exact tool input schema → `tool_<name>.go` → look at `Definition().InputSchema`
- RRF merge or hybrid-search behavior → `tool_catalog_search.go` (rrfMerge, rrfMergeServices)
- Visual_assembly param handling → `tool_visual_assembly.go` (Execute, validatePresetWithUserOps, resolvePresetForTenant)

### Step 3 — Answer
- Direct answer first.
- File paths as `project_v4/backend/internal/<...>:<line>`.
- Cross-link to engine-v4 expert when the answer concerns ops, presets, or formation tree.
- Cross-link to backend-usecases for orchestration questions (loops, retries, error handling).

## Constraints

- DO NOT change code or create files.
- DO NOT cite `project/backend/internal/tools/` or `project/backend/internal/prompts/` — that's V1/V2 legacy and a different (deprecated) tool surface.
- DO flag drift between YAML and code so the next self-improve run can fix it.

## Output

Direct answer with file references. Note any drift you spotted.
