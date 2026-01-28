# Accept

Commit and push changes after successful /build and /test.

## Variables

MESSAGE: $ARGUMENTS (optional commit message)

## Instructions

- Run final validation
- Create descriptive commit
- Push to remote

## Workflow

### Step 1: Check Status
```bash
git status
git diff --stat
```

If no changes, report and exit.

### Step 2: Final Validation
Run `/test` to ensure everything passes.

### Step 3: Create Commit
If MESSAGE provided, use it.
Otherwise, generate from changes:
- Analyze changed files
- Create conventional commit message

```bash
git add -A
git commit -m "{message}"
```

### Step 4: Push
```bash
git push
```

## Output

```
Changes Accepted

Commit: {hash}
Message: {message}
Files: {count}

Pushed to: {remote/branch}
```
