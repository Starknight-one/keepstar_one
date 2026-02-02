# Stone

Finalize work: sync context, commit, push, update changelog.

## Instructions

Use after implementation is verified (tests pass, manual testing done).
This command "sets in stone" the current state.

## Workflow

### Step 1: Sync Expertise
Run `/sync-experts` to update all expertise files:

Backend (hexagonal layers):
- `backend-domain`, `backend-ports`, `backend-adapters`
- `backend-usecases`, `backend-handlers`, `backend-pipeline`

Frontend (FSD layers):
- `frontend-shared`, `frontend-entities`, `frontend-features`

### Step 2: Sync README
Run `/sync-readme`:
- Check all project README files
- Update any that are outdated

### Step 3: Update Changelog
Edit `docs/UPDATES.md`:
- Add new section with current date/time
- List what was done in this session
- Keep it concise

Format:
```markdown
## YYYY-MM-DD HH:MM

### [Category]
- What was done
- Another change
```

### Step 4: Stage and Commit
```bash
git add -A
git status
```

Create commit with summary of changes:
```bash
git commit -m "$(cat <<'EOF'
feat/fix/chore: brief description

- detail 1
- detail 2

Co-Authored-By: Claude Opus 4.5 <noreply@anthropic.com>
EOF
)"
```

### Step 5: Push
Check current branch:
```bash
git branch --show-current
```

If on `main`:
```bash
git push origin main
```

If on feature branch:
```bash
git push origin <branch-name>
git checkout main
git merge <branch-name>
git push origin main
```

## Output

```
Stone Complete

Expertise: [synced|no changes]
README: [synced|no changes]
Changelog: Updated (docs/UPDATES.md)

Commit: <commit-hash>
Message: <commit-message>

Pushed to: main
```
