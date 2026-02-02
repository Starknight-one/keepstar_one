# Validate Spec

Validate a spec against actual codebase before implementation.

## Variables

SPEC_PATH: $ARGUMENTS
CONFIG: ADW/adw.yaml
EXPERTS_DIR: .claude/commands/experts/

## Instructions

- IMPORTANT: If SPEC_PATH not provided, STOP and request path
- Read the spec and identify all integration points
- Verify each integration point against actual code
- Report issues that would cause implementation bugs

## Workflow

### Step 1: Load Spec
Read SPEC_PATH and extract:
- Files to create/modify
- External APIs used
- Database operations
- Port/adapter interactions

### Step 2: Load Relevant Expertise
Read expertise files, especially:
- `gotchas` section
- `external_apis` section
- `integration_patterns` section

### Step 3: Verify Data Types
For each port method mentioned in spec:
1. Read actual port interface
2. Check parameter types (UUID vs string vs slug)
3. Flag mismatches

Example check:
```
Spec says: ListProducts(tenantID="nike", ...)
Reality: ListProducts requires UUID, not slug
Issue: Must call GetTenantBySlug first
```

### Step 4: Verify Database Constraints
For each database operation:
1. Read migration files for table
2. Check foreign key constraints
3. Flag missing prerequisites

Example check:
```
Spec says: Create state for session
Reality: chat_session_state has FK to chat_sessions
Issue: Must create session in chat_sessions first
```

### Step 5: Verify External APIs
For each external API call:
1. Read actual adapter implementation
2. Check API version headers
3. Check request/response format
4. Flag outdated info

Example check:
```
Spec says: anthropic-version: 2024-06-01
Reality: Current working version is 2023-06-01
Issue: Wrong API version
```

### Step 6: Verify SQL/Filter Logic
For each search/filter operation:
1. Read adapter implementation
2. Check how conditions combine (AND/OR)
3. Flag logic issues

Example check:
```
Spec says: search by brand AND query
Reality: Both conditions via AND = empty result if query doesn't match
Issue: Need to handle brand-only vs query-only cases
```

### Step 7: Generate Report

## Output Format

```
Spec Validation Report
======================
Spec: <path>
Status: PASS | ISSUES FOUND

Issues Found: N

1. [DATA_TYPE] <description>
   - Spec assumes: <what spec says>
   - Reality: <what code does>
   - Fix: <how to fix spec>

2. [FOREIGN_KEY] <description>
   ...

3. [API_VERSION] <description>
   ...

4. [SQL_LOGIC] <description>
   ...

Recommendations:
- <actionable fix 1>
- <actionable fix 2>

If PASS:
  Ready for /build

If ISSUES FOUND:
  Fix spec before /build
```

## Constraints

- DO NOT: modify spec (only report)
- DO NOT: modify code
- DO: read actual implementation files
- DO: be specific about issues
- DO: provide actionable fixes
