# Cold Start

Cold start: sync expertise and load project context.

## Instructions

Use at the beginning of a new session to load up-to-date context.

## Workflow

### Step 1: Sync All Experts
Update expertise for all domains:

**Backend:**
- Read `.claude/commands/experts/backend/expertise.yaml`
- Compare with `project/backend/cmd/server/main.go`, `project/backend/internal/handlers/*.go`, `project/backend/internal/domain/*.go`
- Update if discrepancies found

**Frontend:**
- Read `.claude/commands/experts/frontend/expertise.yaml`
- Compare with `project/frontend/src/App.jsx`, `project/frontend/src/shared/api/apiClient.js`, `project/frontend/src/features/chat/*.jsx`
- Update if discrepancies found

### Step 2: Prime
After syncing expertise:
```bash
git ls-files
```
Read README files if they exist:
- `README.md`

### Step 3: Summary
Output brief summary about the project and readiness to work.

## Output

```
Cold Start Complete

Expertise Sync:
- backend: [updated|no changes]
- frontend: [updated|no changes]

Project: Keepstar One Ultra
Stack: Go/Hexagonal + React/Vite/FSD
Status: Ready to work
```
