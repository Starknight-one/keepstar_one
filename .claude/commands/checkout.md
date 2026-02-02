# Checkout

Create a new git branch for implementing a spec.

## Variables

SPEC_PATH: $ARGUMENTS

## Instructions

- IMPORTANT: If SPEC_PATH not provided, STOP and request path to spec file
- Parse spec filename to generate branch name
- Create and checkout new branch
- Confirm the switch

## Workflow

### Step 1: Validate Input
If SPEC_PATH is empty:
```
Error: Spec path required

Usage: /checkout ADW/specs/feature-<name>.md

Available specs:
<list files in ADW/specs/>
```

### Step 2: Validate Spec Exists
- Check if SPEC_PATH file exists
- If not, show error and list available specs

### Step 3: Parse Branch Name
Extract branch name from spec filename:
- `feature-<name>.md` -> `feature/<name>`
- `bug-<name>.md` -> `bugfix/<name>`
- `chore-<name>.md` -> `chore/<name>`
- `migration-<name>.md` -> `migration/<name>`
- Other: `spec/<filename-without-extension>`

Examples:
- `ADW/specs/feature-two-agent-state-storage.md` -> `feature/two-agent-state-storage`
- `ADW/specs/bug-login-error.md` -> `bugfix/login-error`

### Step 4: Check Current State
- Run `git status` to ensure working directory is clean
- If there are uncommitted changes, warn user and ask to proceed or abort

### Step 5: Create and Checkout Branch
```bash
git checkout -b <branch-name>
```

### Step 6: Confirm Success
Show the result of the operation.

## Output

```
Branch Created

Spec: <spec-path>
Branch: <branch-name>
Base: <previous-branch>

Ready to build. Run:
  /build <spec-path>
```

## Error Cases

### Uncommitted changes
```
Warning: Uncommitted changes detected

<git status output>

Proceed anyway? Changes will carry to new branch.
```

### Branch already exists
```
Error: Branch '<name>' already exists

Options:
1. Switch to existing: git checkout <name>
2. Delete and recreate: git branch -D <name> && git checkout -b <name>
```
