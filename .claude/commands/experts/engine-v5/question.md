# engine-v5 Question

Answer questions about the V5 scene-graph engine without making code changes.

## Variables

USER_PROMPT: $ARGUMENTS
EXPERTISE: .claude/commands/experts/engine-v5/expertise.yaml
CODE_ROOT: project_v5/backend/internal/engine/

## Instructions

- IMPORTANT: If USER_PROMPT not provided, STOP and request a question.
- Read EXPERTISE first for the mental model (property-bag nodes, 5 op types, pipeline order, preset registry, binding).
- Verify any specific claim against current code in CODE_ROOT before answering — the YAML can drift.
- DO NOT make any code changes.

## Workflow

### Step 1 — Load expertise
Read `expertise.yaml`. It tells you:
- Node type system (map[string]any, 14 node types, key helpers)
- 5 op types and op shape (insert/update/delete/move/override — NOT replace)
- Pipeline order (materialise → ops → replicate → resolve_inline → bind → inject_actions)
- Preset + component registry (9 system presets, 2 components, prompt.go auto-gen)
- Binding (fieldBinding → content / fills[0].image)
- TreeMap shape Agent2 receives in modify mode
- Known gotchas (component-internal ids, registry miss silent no-op, missing SystemComponentRegistry history)

### Step 2 — Decide if a code read is needed
You MUST read code (not rely on YAML alone) when the question is about:
- A specific preset's node tree → read `engine/presets/seed/<preset>.json`
- Op semantics edge cases → read `engine/op_<type>.go`
- Binding behavior for a specific node type → read `engine/binding.go`
- TreeMap shape details → read `engine/tree_map.go`
- Value type behavior (color/fill/layout parsing) → read `engine/value_*.go`
- Whether a behavior has a test → list `engine/*_test.go`

### Step 3 — Answer
- Direct answer first.
- File paths with `project_v5/backend/internal/engine/<file>.go:<line>` where applicable.
- Mention related expert (pipeline-agents, widget, catalog) if the answer crosses layers.

## Constraints

- DO NOT change code or create files.
- DO point at test files when the user asks "how do I know X works".
- DO flag when you spot drift between expertise.yaml and current code.

## Output

Direct answer with file references. Note any drift spotted for the next self-improve run.
