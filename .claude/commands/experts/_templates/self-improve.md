# {DOMAIN} Self-Improve

Update {DOMAIN} expertise from codebase.

## Variables

USE_DIFF: $ARGUMENTS (true|false, default: false)
EXPERTISE: .claude/commands/experts/{DOMAIN}/expertise.yaml
CONFIG: ADW/adw.yaml

## Instructions

- Scan codebase for {DOMAIN} knowledge
- Update expertise.yaml with findings
- Keep within line limit from CONFIG

## Workflow

### Step 1: Load Current Expertise
Read EXPERTISE to understand current state.

### Step 2: Load Configuration
Read CONFIG to get:
- Paths for this domain
- Line limit

### Step 3: Scan Codebase
If USE_DIFF = true:
```bash
git diff HEAD~5 --name-only
```
Focus on changed files.

Otherwise, scan domain paths from CONFIG.

### Step 4: Extract Knowledge
For each relevant file:
- File purpose
- Key patterns
- Important structures
- Dependencies

### Step 5: Update Expertise
Update expertise.yaml:
- Keep format consistent
- Stay within line limit
- Focus on actionable knowledge

### Step 6: Report Changes

## Constraints

- DO NOT: exceed line limit
- DO: focus on patterns, not docs
- DO: include file paths
- DO: keep it actionable

## Output

```
{DOMAIN} Expertise Updated

Changes:
- Added: <what>
- Updated: <what>
- Removed: <what>

Lines: N / {limit}
```
