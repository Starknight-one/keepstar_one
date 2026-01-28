# Build

Implement a plan from ADW/specs/ using expertise.

## Variables

PATH_TO_PLAN: $ARGUMENTS
CONFIG: ADW/adw.yaml
EXPERTS_DIR: .claude/commands/experts/

## Instructions

- IMPORTANT: If PATH_TO_PLAN not provided, STOP and request path
- Read CONFIG to understand project structure
- Implement the plan COMPLETELY, top to bottom
- Use expertise to understand patterns
- DO NOT stop until all steps are complete
- Finish with validation commands from CONFIG

## Workflow

### Step 1: Load Configuration
Read `ADW/adw.yaml` to get:
- Project paths
- Validation commands
- Expert list

### Step 2: Load Plan
- Read PATH_TO_PLAN
- Determine which experts are needed from "Expertise Context" section

### Step 3: Load Relevant Expertise
Read expertise files based on CONFIG:
- `EXPERTS_DIR/{expert}/expertise.yaml`

Understand:
- File structure
- Code patterns
- Dependencies

### Step 4: Execute Plan
For each step in the plan:
1. Read related files (from expertise)
2. Implement changes
3. Verify changes match project patterns

### Step 5: Run Validation
Execute validation commands from CONFIG:
- For each validation in `adw.yaml`:
  - `cd {path} && {cmd}`
- If errors - fix and repeat
- DO NOT stop with errors

### Step 6: Update Expertise (if needed)
If significant changes were made:
- Run `/experts:{domain}:self-improve true` for affected domains

## Constraints

- DO NOT: skip plan steps
- DO NOT: stop with validation errors
- DO: follow patterns from expertise
- DO: use existing code style
- DO: read commands from CONFIG

## Output

```
Build Complete

Plan: <path>
Steps completed: N/N

Changes:
- file1: description
- file2: description

Validation:
- command1: passed|failed
- command2: passed|failed

Files changed:
<git diff --stat>
```
