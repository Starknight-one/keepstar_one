# SDLC

Full SDLC pipeline: /feature -> /build -> /test with automated reporting.

## Variables

USER_PROMPT: $ARGUMENTS
CONFIG: ADW/adw.yaml

## Instructions

- IMPORTANT: If USER_PROMPT not provided, STOP and request feature description
- Execute the SDLC orchestrator
- Output the pipeline report

## Workflow

### Step 1: Validate Input
- Ensure USER_PROMPT is provided
- If empty, respond: "Please provide a feature description: /sdlc \"your feature description\""

### Step 2: Check Orchestrator
Check if Go orchestrator exists and is built:
```bash
ls ADW/sdlc 2>/dev/null || (cd ADW && go build -o sdlc sdlc.go)
```

If no sdlc.go exists, fall back to manual execution.

### Step 3: Execute Pipeline

**With orchestrator:**
```bash
cd ADW && ./sdlc "USER_PROMPT"
```

**Without orchestrator (fallback):**
1. Run `/feature "USER_PROMPT"`
2. Get spec path from output
3. Run `/build {spec_path}`
4. Run `/test`

### Step 4: Report Results
- Display the pipeline report
- Include spec path for reference
- Include manual testing instructions from plan

## Output

```
SDLC Pipeline Complete

Feature: <short summary>
Spec: ADW/specs/feature-<name>.md
Status: success|failed

Validation:
- <result 1>
- <result 2>

Manual test:
- <instruction from spec>
```
