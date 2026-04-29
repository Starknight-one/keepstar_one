# pipeline-agents Self-Improve

Update the pipeline-agents expertise from current code in
`project_v4/backend/internal/{usecases,prompts,tools,adapters/anthropic,adapters/openai}/`.

## Variables

USE_DIFF: $ARGUMENTS (true | false, default: false)
EXPERTISE: .claude/commands/experts/pipeline-agents/expertise.yaml
CODE_ROOT: project_v4/backend/internal/
LINE_LIMIT: 500

## Instructions

- Scan code, update YAML, keep under LINE_LIMIT.
- Preserve overview (two-agent contract, span/trace model, cache strategy) unless code actually changed them.
- Refresh anything tied to specific names (tool names, system prompt sections, anthropic methods, span names).

## Workflow

### Step 1 — Read current expertise
Note which sections already exist so you only update, not duplicate.

### Step 2 — Choose scope
- USE_DIFF=true → run `git diff HEAD~10 -- project_v4/backend/internal/usecases project_v4/backend/internal/prompts project_v4/backend/internal/tools project_v4/backend/internal/adapters/anthropic project_v4/backend/internal/adapters/openai --name-only` and focus on changed files.
- USE_DIFF=false → scan all files in CODE_ROOT.

### Step 3 — Refresh facts (high-drift surfaces)

**Pipeline Execute steps in `usecases/pipeline_execute.go`** — re-read the Execute method. Update `orchestration.pipeline_steps`. New step (e.g. moderation, intent classification) must be inserted in correct order.

**Agent1ExecuteResponse fields** — `awk '/^type Agent1ExecuteResponse struct/,/^}/' usecases/agent1_execute.go`. Update `agent1.response_fields`. Same for Agent2.

**Tool registry contents** — re-read `tools/tool_registry.go NewRegistry()`. Update `tools.registered_tools`. Watch for new tools added (e.g. checkout, content_filter).

**System prompt sections in `prompts/prompt_compose_widgets.go`** — `grep -nE "^const|^var.*string|<[a-z_]+>" prompts/prompt_compose_widgets.go`. Update `agent2.system_prompt.sections`. Section reorder = cache invalidation; flag in gotchas.

**Cache breakpoints in `adapters/anthropic/anthropic_client.go ChatWithToolsCached`** — search for `cache_control` placements. Update `llm_adapters.anthropic.cache_points`. Adding/removing a breakpoint changes cost/latency profile.

**Span names** — `grep -nE 'sc\.Start\("|StageFromContext\("' project_v4/backend/internal/`. Update `llm_adapters.anthropic.span_instrumentation.spans`. Span names are parsed by trace UI — silent UI break if renamed.

**Field labels and design context loading in `usecases/agent2_execute.go`** — re-read `loadFieldLabels` and `formatDesignContextBlock`. If new field is added to DesignContextSnapshot in engine_v4, the formatter MUST be updated or it silently drops the field from the prompt.

**Retention config in `adapters/postgres/retention.go`** — re-read RetentionConfig defaults. Update `state_and_trace.retention.config`. Changes affect long-session behavior silently.

### Step 4 — Cross-layer drift checks

These are the silent-failure surfaces:

- **Tool definition order** — `tools/Registry.GetDefinitions()` MUST sort by name. If sort.Slice line is removed, prompt cache_read drops to 0% silently (only cost increase visible).
- **Engine.Execute call site** — only `tools/tool_visual_assembly.go` should call it. Direct calls from elsewhere bypass tenant preset resolution. Run `grep -rn "engine.Execute\|eng.Execute" project_v4/backend/internal/` to verify.
- **Tenant digest cache invalidation** — `agent1_execute.go buildSystemPromptWithDigest` memoizes per tenant via `sync.Map`. If digest schema changes, the cache must be cleared or new fields silently invisible to Agent1.
- **Conversation history trim** — retention.trimConversationHistory(20) is silent; long sessions lose early context. Verify the threshold matches what Agent1 system prompt expects to see.
- **Anthropic API version 2023-06-01** — pinned in client. Major version bump (e.g. 2024-xx) requires tracking new content block types in cache_types.go.

### Step 5 — Refresh gotchas

Add a new gotcha when:
- A non-obvious behavior surprised you while answering a question.
- A recent commit fixed a bug whose root cause was hidden behavior.

Inspiration: `git log --since='4 weeks' -- project_v4/backend/internal/usecases project_v4/backend/internal/prompts project_v4/backend/internal/tools`.

### Step 6 — Refresh hot files

Re-run: `find project_v4/backend/internal -name "*.go" ! -name "*_test.go" -exec wc -l {} \; | sort -nr | head -10`. Update `active_workstreams.hot_files`.

### Step 7 — Cross-check engine-v4 surface

The `agent2_contract` section here mirrors what's in engine-v4 expertise. If engine-v4 changed (new op type, new preset shape, new TreeMap shape), update `agent2.system_prompt` accordingly — Agent2's output IS engine-v4's input.

### Step 8 — Report

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
- DO NOT cite the legacy `project/backend/internal/` code — that pipeline was deleted 2026-04-29.
- DO NOT expand engine-v4 internals here — keep the engine-v4 expert as the source of truth for engine_v4 package.
- DO update tool argument schemas if they change in tool_registry — these are user-facing (Agent emits exactly what's in the tool definition).

## Output

Updated `expertise.yaml` plus a summary report.
