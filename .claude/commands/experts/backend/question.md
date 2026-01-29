---
allowed-tools: Bash, Read, Grep, Glob
description: Answer questions about Go backend architecture, hexagonal layers, and API without coding
argument-hint: [question]
---

# Backend Expert - Question Mode

Answer the user's question by analyzing the Go hexagonal architecture backend implementation. This prompt provides information about the backend layer without making any code changes.

## Variables

USER_QUESTION: $1
EXPERTISE_PATH: .claude/commands/experts/backend/expertise.yaml

## Instructions

- IMPORTANT: This is a question-answering task only - DO NOT write, edit, or create any files
- Focus on: hexagonal layers, handlers, usecases, adapters, domain entities, API
- If the question requires code changes, explain the approach conceptually without implementing
- Validate information from EXPERTISE_PATH against the codebase before answering
- Remember: there's active code (working) and hexagonal stubs (not active yet)

## Workflow

1. **Load Expertise**
   - Read EXPERTISE_PATH to understand architecture and patterns
   - Note layer rules (domain → ports → usecases → handlers)
   - Identify active code vs hexagonal stubs

2. **Validate Against Codebase**
   - Read relevant files mentioned in expertise
   - Confirm information is accurate and up-to-date

3. **Answer Question**
   - Provide direct answer to USER_QUESTION
   - Include file paths where relevant
   - Use code snippets to illustrate patterns
   - Clarify if referring to active or stub code

## Report

Provide answer with:

- **Direct Answer**: Clear response to the question
- **Supporting Evidence**: File paths and code snippets
- **Architecture Context**: How this fits into hexagonal layers
- **Related Files**: Other files that may be relevant

**Example:**

```
Question: "How do I add a new use case?"

Answer:
Use cases live in internal/usecases/ and contain business logic.

Pattern:
1. Create file: internal/usecases/{domain}_{action}.go
2. Define struct with port dependencies
3. Implement Execute() method
4. Inject via constructor in cmd/server/main.go

Example structure:
type MyUseCase struct {
    llm   ports.LLMPort
    cache ports.CachePort
}

func NewMyUseCase(llm ports.LLMPort, cache ports.CachePort) *MyUseCase {
    return &MyUseCase{llm: llm, cache: cache}
}

func (uc *MyUseCase) Execute(ctx context.Context, req Request) (*Response, error) {
    // Business logic here
}

Key Files:
- internal/usecases/README.md (patterns)
- internal/usecases/chat_analyze_query.go (example)
- internal/ports/*.go (interfaces to depend on)

Note: Currently hexagonal structure contains stubs. Active code is in root main.go.
```
