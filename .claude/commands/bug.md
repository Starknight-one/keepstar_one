# Bug

Create a plan for fixing a bug.

## Variables

USER_PROMPT: $ARGUMENTS
CONFIG: ADW/adw.yaml
EXPERTS_DIR: .claude/commands/experts/

## Instructions

- IMPORTANT: If USER_PROMPT not provided, STOP and request bug description
- Investigate the issue using expertise
- Create fix plan in `ADW/specs/bug-<name>.md`

## Workflow

### Step 1: Load Configuration
Read `ADW/adw.yaml` for project structure.

### Step 2: Load Expertise
Read relevant expertise to understand:
- Where the bug might be
- Related code patterns
- Testing approach

### Step 3: Investigate
- Search for related code
- Understand the flow
- Identify root cause

### Step 4: Create Fix Plan
Create `ADW/specs/bug-<name>.md`:

```md
# Bug Fix: <bug name>

## Bug Description
<what is broken>

## Root Cause
<why it's broken>

## Fix Plan

### 1. <Fix step>
- action

### N. Validation
- Run validation commands

## Validation Commands
<!-- From ADW/adw.yaml -->

## Acceptance Criteria
- [ ] Bug is fixed
- [ ] No regressions
```

## Output

```
Bug Fix Plan Created

File: ADW/specs/bug-<name>.md
Root cause: <brief>
Affected: <files/components>
```
