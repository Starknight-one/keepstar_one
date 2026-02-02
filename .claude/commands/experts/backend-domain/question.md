# backend-domain Question

Answer questions about backend-domain without making code changes.

## Variables

USER_PROMPT: $ARGUMENTS
EXPERTISE: .claude/commands/experts/backend-domain/expertise.yaml

## Instructions

- IMPORTANT: If USER_PROMPT not provided, STOP and request question
- Read EXPERTISE for context
- Answer based on expertise + codebase exploration
- DO NOT make any code changes

## Workflow

### Step 1: Load Expertise
Read expertise.yaml to understand:
- Project structure
- Patterns
- Key files

### Step 2: Analyze Question
Determine what information is needed.

### Step 3: Explore if Needed
If expertise doesn't have the answer:
- Search relevant files
- Read code to understand

### Step 4: Answer
Provide clear, concise answer with:
- Direct answer
- Relevant file paths
- Code examples if helpful

## Constraints

- DO NOT: change any code
- DO NOT: create files
- DO: provide accurate information
- DO: reference specific files/lines

## Output

Answer the question directly. Include file references.
