# Cold Start

Initialize session: check config, sync expertise, load context.

## Variables

CONFIG: ADW/adw.yaml
EXPERTS_DIR: .claude/commands/experts/

## Instructions

Use at the beginning of a new session to load up-to-date context.

## Workflow

### Step 1: Check Configuration
Read `ADW/adw.yaml`:
- If missing, suggest running `/init`
- If empty paths, note that project structure needs setup

### Step 2: Sync Available Experts
For each expert in CONFIG experts list:
1. Check if `EXPERTS_DIR/{expert}/expertise.yaml` exists
2. If exists and has content, verify against codebase
3. If empty or outdated, note for user

### Step 3: Project Overview
```bash
git ls-files | head -50
```

Read README.md if exists.

### Step 4: Summary
Output brief summary about the project and readiness.

## Output

```
Cold Start Complete

Config: ADW/adw.yaml [found|missing]

Project: {name from config}
Stack: {detected stack}
Paths:
  - Backend: {path} [exists|missing]
  - Frontend: {path} [exists|missing]

Expertise:
- {expert}: [ready|empty|outdated]

Status: Ready | Needs /init | Needs setup
```
