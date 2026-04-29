# engine-v4 Question

Answer questions about the V4 ops-driven UI assembly engine without making code changes.

## Variables

USER_PROMPT: $ARGUMENTS
EXPERTISE: .claude/commands/experts/engine-v4/expertise.yaml
CODE_ROOT: project_v4/backend/internal/engine_v4/

## Instructions

- IMPORTANT: If USER_PROMPT not provided, STOP and request a question.
- Read EXPERTISE first for the mental model (presets, ops, binding, pipeline).
- Verify any specific claim against current code in CODE_ROOT before answering — the YAML can drift.
- DO NOT make any code changes.

## Workflow

### Step 1 — Load expertise
Read `expertise.yaml`. It tells you:
- The 8-step Execute pipeline
- The 12 presets (6 product, 3 nav, 3 system) and what fields they bind
- The Op shape (insert/update/delete/move/replace) and ref system
- Tenant preset overrides via PresetResolver
- Known gaps (B3/B4/B7) and gotchas

### Step 2 — Decide if a code read is needed
You MUST read code (not rely on YAML alone) when the question is about:
- A specific preset's exposed refs or atom layout → read the relevant `presets_*.go`
- Op semantics edge cases → read `ops.go` / `expand.go`
- Binding behavior with missing/mismatched fields → read `binding.go`
- Whether a behavior is covered by a test → list `*_test.go`

### Step 3 — Answer
- Direct answer first.
- File paths with `project_v4/backend/internal/engine_v4/<file>.go:<line>` where applicable.
- Mention the related expert (backend-pipeline, backend-domain, admin-backend) if the answer crosses layers.

## Constraints

- DO NOT change code or create files.
- DO NOT cite the legacy `project/backend/internal/engine/` code — that is the V1/V2 engine and is NOT what this expert covers.
- DO point at the test files when the user asks "how do I know X works".

## Output

Direct answer with file references. Note any drift you spot between expertise.yaml and current code (so the next self-improve run can fix it).
