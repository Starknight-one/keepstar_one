# Commit

Create git commit without push.

## Variables

MESSAGE: $ARGUMENTS (optional commit message)

## Instructions

- Stage and commit changes
- Do not push

## Workflow

### Step 1: Check Status
```bash
git status
git diff --stat
```

If no changes, report and exit.

### Step 2: Generate Message
If MESSAGE not provided:
- Analyze changed files
- Generate conventional commit message
- Format: type(scope): description

Types: feat, fix, chore, docs, style, refactor, test

### Step 3: Commit
```bash
git add -A
git commit -m "{message}"
```

## Output

```
Committed

Hash: {hash}
Message: {message}
Files: {count}
```
