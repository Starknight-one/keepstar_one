# pipeline-agents Question

Answer questions about the V5 chat pipeline (Agent1 + Agent2 + tools + LLM
adapters) without making code changes.

## Variables

USER_PROMPT: $ARGUMENTS
EXPERTISE: .claude/commands/experts/pipeline-agents/expertise.yaml
CODE_ROOT: project_v5/backend/internal/

## Instructions

- IMPORTANT: If USER_PROMPT not provided, STOP and request a question.
- Read EXPERTISE first for the mental model (orchestration, Agent1/2, tools, anthropic, prompts, state/trace).
- Verify any specific claim against current code in CODE_ROOT before answering — the YAML can drift.
- DO NOT make any code changes.

## Workflow

### Step 1 — Load expertise
Read `expertise.yaml`. It tells you:
- Pipeline_execute orchestration steps (7 steps + prefetch)
- Agent1/Agent2 prompt structure (agent2_prompt.go is a var, not const)
- Tool registry (catalog_search, state_filter, history_lookup, visual_assembly)
- PromptCache assembly (FormatFieldsBlock + AssembleSystemPrompt + SystemPresetsBlock)
- Trace write to v5_chat_session_traces (best-effort async)
- HTTP handler surface + readyz
- Known gaps (A1-A5 from v5-known-gaps.md)

### Step 2 — Decide if a code read is needed
You MUST read code (not rely on YAML alone) when the question is about:
- A specific tool's filter or query shape → read `tools/tool_<name>.go`
- Agent2 prompt exact text or section → read `prompts/agent2_prompt.go`
- Preset catalog in prompt → read `engine/presets/prompt.go SystemPresetsBlock`
- Cache cost / cache miss reasoning → read `adapters/anthropic/` ChatWithToolsCached
- Why a tool returned empty → read the tool's Execute method
- Conversation history behavior → retention config
- State reconstruct / rollback flow → `usecases/state_reconstruct.go`, `state_rollback.go`
- Prefetch payload shape → `usecases/prefetch.go`

### Step 3 — Answer
- Direct answer first.
- File paths with `project_v5/backend/internal/<path>:<line>` where applicable.
- Mention related expert (engine-v5, catalog, widget, curator) if answer crosses layers.

## Constraints

- DO NOT change code or create files.
- DO NOT cite V4 pipeline code (`project_v4/backend/internal/`) — different module, different patterns.
- DO route engine-v5 internals (op shape, preset node layout) to `experts:engine-v5:question`.
- DO route Formation/Document rendering questions to `experts:widget:question`.
- DO route catalog data shape to `experts:catalog:question`.

## Output

Direct answer with file references. Note any drift spotted for the next self-improve run.
