# Feature

Create a plan for a new feature using project expertise.

## Variables

USER_PROMPT: $ARGUMENTS
CONFIG: ADW/adw.yaml
EXPERTS_DIR: .claude/commands/experts/

## Instructions

- IMPORTANT: If USER_PROMPT not provided, STOP and request feature description
- Read CONFIG to understand project structure
- Use expertise to understand project context and patterns
- Feature is new functionality that adds user value
- Plan must be detailed enough for autonomous implementation via /build
- Save plan to `ADW/specs/feature-<kebab-case-name>.md`

## Workflow

### Step 1: Load Configuration
Read `ADW/adw.yaml` to get:
- Project paths (backend, frontend, database)
- Validation commands
- Available experts

### Step 2: Load Expertise Context
Read relevant expertise based on CONFIG experts list:
- `EXPERTS_DIR/{expert}/expertise.yaml`

Determine which experts are relevant based on USER_PROMPT.

### Step 3: Check for Reference-Based UI
**TRIGGER**: If USER_PROMPT contains "by reference" or similar:
1. Look for ui-spec agent in `.claude/agents/`
2. If exists, call it to extract UI specs
3. Include extracted specs in plan

### Step 4: Analyze Requirements
- Break down USER_PROMPT into specific requirements
- Determine complexity: simple | medium | complex
- Determine affected layers based on CONFIG paths

### Step 5: Explore Codebase
Using expertise context:
- Find relevant files
- Understand existing patterns
- Identify extension points

### Step 6: Design Solution
- Think through architectural approach
- Identify dependencies between steps
- Consider edge cases and error handling

### Step 7: Create Plan Document
Create `ADW/specs/feature-<name>.md`:

```md
# Feature: <feature name>

## Feature Description
<detailed description>

## Objective
<what will be achieved>

## Expertise Context
Expertise used:
- {expert}: <key insights>

## Relevant Files

### Existing Files
- `path/to/file` - why relevant

### New Files (if needed)
- `path/to/new/file` - what it will contain

## Step by Step Tasks
IMPORTANT: Execute strictly in order.

### 1. <Task name>
- specific action
- specific action

### N. Validation
- Run validation commands from config

## Validation Commands
<!-- Read from ADW/adw.yaml validation section -->

## Acceptance Criteria
- [ ] Criterion 1
- [ ] Criterion 2

## Notes
<additional context>
```

## Constraints

- DO NOT: implement code, only plan
- DO: use expertise for context
- DO: be specific in steps
- DO: follow existing project patterns
- DO: read validation commands from CONFIG

## Output

```
Feature Plan Created

File: ADW/specs/feature-<name>.md
Complexity: simple|medium|complex
Layers: <from config>
Steps: N
Experts used: <list>
```
