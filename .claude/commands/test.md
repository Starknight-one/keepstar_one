# Test

Run validation commands from config.

## Variables

TARGET: $ARGUMENTS (optional: specific domain or "all")
CONFIG: ADW/adw.yaml
EXPERTS_DIR: .claude/commands/experts/

## Instructions

- Read CONFIG to get validation commands
- If TARGET not provided, run all validations
- Report results clearly

## Workflow

### Step 1: Load Configuration
Read `ADW/adw.yaml` to get:
- Validation commands list
- Project paths

### Step 2: Run Validations
For each validation in CONFIG:

```yaml
validation:
  - name: "Name"
    path: "{backend}"  # Resolve from paths section
    cmd: "command"
    required: true|false
```

Execute:
```bash
cd {resolved_path} && {cmd}
```

Skip validations with empty `cmd`.

### Step 3: Integration Check (if available)
If CONFIG has integration commands, run them.

### Step 4: Report Results

## Constraints

- DO NOT: change code
- DO: report all issues
- DO: suggest fixes
- DO: read all commands from CONFIG

## Output

```
Test Results

Validation:
- {name}: passed|failed|skipped
  {output if failed}

Summary:
- Passed: N
- Failed: N
- Skipped: N

Recommendations:
- [list of fixes if any]
```
