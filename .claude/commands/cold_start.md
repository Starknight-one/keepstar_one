# Cold Start

Cold start: load expertise and project context.

## Instructions

Use at the beginning of a new session to load up-to-date context.

## Workflow

### Step 1: Load Expertise (Quick Scan)
Read all expertise files to prime context:

**Backend (hexagonal):**
- `.claude/commands/experts/backend-domain/expertise.yaml`
- `.claude/commands/experts/backend-ports/expertise.yaml`
- `.claude/commands/experts/backend-usecases/expertise.yaml`
- `.claude/commands/experts/backend-handlers/expertise.yaml`
- `.claude/commands/experts/backend-pipeline/expertise.yaml`

**Frontend (FSD):**
- `.claude/commands/experts/frontend-shared/expertise.yaml`
- `.claude/commands/experts/frontend-entities/expertise.yaml`
- `.claude/commands/experts/frontend-features/expertise.yaml`

Skip adapters unless needed (large file).

### Step 2: Prime
```bash
git status
git log --oneline -5
```
Read `README.md` if exists.

### Step 3: Summary
Output brief summary.

## Output

```
Cold Start Complete

Expertise Loaded:
- backend: domain, ports, usecases, handlers, pipeline
- frontend: shared, entities, features

Project: Keepstar One Ultra
Branch: {current branch}
Status: Ready to work
```
