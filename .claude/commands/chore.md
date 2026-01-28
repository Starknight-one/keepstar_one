# Chore

Create a plan for technical task (refactoring, dependencies, etc).

## Variables

USER_PROMPT: $ARGUMENTS
CONFIG: ADW/adw.yaml
EXPERTS_DIR: .claude/commands/experts/

## Instructions

- IMPORTANT: If USER_PROMPT not provided, STOP and request task description
- Chore = technical work that doesn't add user-facing features
- Create plan in `ADW/specs/chore-<name>.md`

## Workflow

### Step 1: Load Configuration
Read `ADW/adw.yaml` for project structure.

### Step 2: Analyze Task
Determine type:
- Refactoring
- Dependency update
- Configuration change
- Documentation
- Performance improvement

### Step 3: Create Plan
Create `ADW/specs/chore-<name>.md`:

```md
# Chore: <task name>

## Description
<what needs to be done>

## Rationale
<why this is needed>

## Tasks

### 1. <Task>
- action

### N. Validation

## Validation Commands
<!-- From ADW/adw.yaml -->

## Notes
<risks, considerations>
```

## Output

```
Chore Plan Created

File: ADW/specs/chore-<name>.md
Type: <refactoring|deps|config|docs|perf>
Scope: <affected areas>
```
