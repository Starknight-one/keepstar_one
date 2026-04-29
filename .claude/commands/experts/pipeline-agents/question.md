# pipeline-agents Question

Answer questions about the V4 chat pipeline (Agent1 + Agent2 + tools + LLM
adapters) without making code changes.

## Variables

USER_PROMPT: $ARGUMENTS
EXPERTISE: .claude/commands/experts/pipeline-agents/expertise.yaml
CODE_ROOT: project_v4/backend/internal/

## Instructions

- IMPORTANT: If USER_PROMPT not provided, STOP and request a question.
- Read EXPERTISE first for the mental model (orchestration, Agent1/2 contracts, tools, anthropic, span/trace, prompts).
- Verify any specific claim against current code in CODE_ROOT before answering — the YAML can drift.
- DO NOT make any code changes.

## Workflow

### Step 1 — Load expertise
Read `expertise.yaml`. It tells you:
- Pipeline_execute orchestration steps and trace shape
- Agent1/Agent2 request/response shapes and prompt sections
- Tool registry contents (catalog_search, state_filter, history_lookup, visual_assembly)
- Anthropic adapter methods (ChatWithTools vs ChatWithToolsCached) and cache breakpoints
- State + trace + retention setup
- HTTP handler surface

### Step 2 — Decide if a code read is needed
You MUST read code (not rely on YAML alone) when the question is about:
- A specific tool's filter or query shape → read `tools/tool_<name>.go`
- A prompt section's exact text or example → read `prompts/prompt_*.go`
- Cache cost / cache miss reasoning → read `adapters/anthropic/anthropic_client.go ChatWithToolsCached`
- A span name or trace field → read `usecases/pipeline_execute.go` and the relevant Agent UseCase
- Why a tool returned 'empty' → read the tool's Execute method
- Conversation history behavior → read `adapters/postgres/retention.go trimConversationHistory`

### Step 3 — Answer
- Direct answer first.
- File paths with `project_v4/backend/internal/<file>:<line>` where applicable.
- Mention the related expert (engine-v4, catalog, widget) if the answer crosses layers.
- For "why is the cost so high" / "why is cache_read 0%" questions, walk: tool definition order → system prompt section ordering → cache_control placement → minimum-token threshold → static vs dynamic blocks.

## Constraints

- DO NOT change code or create files.
- DO NOT cite the legacy `project/backend/internal/usecases/pipeline_execute.go` — deleted 2026-04-29.
- DO route engine_v4 internals (op shape, preset details) to `experts:engine-v4:question` — pipeline-agents owns the wrapper, not the engine.
- DO route Formation rendering questions to `experts:widget:question` — that's the consumer.
- DO route catalog data shape to `experts:catalog:question` — that's the source.

## Output

Direct answer with file references. Note any drift you spot between expertise.yaml and current code (so the next self-improve run can fix it).
