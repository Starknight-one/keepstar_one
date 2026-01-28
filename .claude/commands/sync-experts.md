# Sync Experts

Update all expert knowledge from codebase.

## Variables

CONFIG: ADW/adw.yaml
EXPERTS_DIR: .claude/commands/experts/

## Instructions

- Read CONFIG to get expert list
- For each expert, run self-improve
- Report changes

## Workflow

### Step 1: Load Configuration
Read `ADW/adw.yaml` to get:
- List of experts
- Their paths

### Step 2: Sync Each Expert
For each expert in CONFIG:
1. Check if `EXPERTS_DIR/{expert}/` exists
2. If exists, run self-improve logic:
   - Read current expertise.yaml
   - Compare with actual code in expert's paths
   - Update if needed

### Step 3: Report

## Output

```
Experts Synced

- {expert}: [updated|no changes|not found]
  {brief change description if updated}

Total: N experts
Updated: M
```
