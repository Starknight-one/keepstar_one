# Expert System

Domain expertise for AI-assisted development.

## Concept

Experts are **mental models** of project domains — not documentation, not source of truth (code is).
They provide fast context access without parsing the entire codebase.

### ACT → LEARN → REUSE

```
ACT    →  Agent takes a useful action (builds, fixes, answers)
LEARN  →  Agent stores new information in its expertise file
REUSE  →  Agent uses that expertise on the next execution
```

The difference between a generic agent and an Agent Expert: **one executes and forgets, the other executes and learns**.

## Available Experts

### Backend (Hexagonal Architecture)

| Expert | Layer | Scope |
|--------|-------|-------|
| `backend-domain` | domain | Entities, types, errors |
| `backend-ports` | ports | Interfaces (contracts) |
| `backend-adapters` | adapters | PostgreSQL, Anthropic, OpenAI (embeddings), json_store, memory |
| `backend-usecases` | usecases | Business logic |
| `backend-handlers` | handlers | HTTP, middleware, routes |
| `backend-pipeline` | tools/prompts | Agent tools, LLM prompts, span/waterfall tracing |

### Frontend (Feature-Sliced Design)

| Expert | Layer | Scope |
|--------|-------|-------|
| `frontend-shared` | shared | API client, hooks |
| `frontend-entities` | entities | Atom, widget, formation |
| `frontend-features` | features | Chat, catalog, overlay |

## File Structure

Each expert has 3 files:

```
experts/<domain>/
├── expertise.yaml    # Structured knowledge (YAML)
├── question.md       # Answer questions (no code changes)
└── self-improve.md   # Sync with codebase
```

## Usage

### Ask Questions
```
/experts:backend-domain:question "What entities exist?"
/experts:backend-handlers:question "What endpoints are available?"
/experts:frontend-features:question "How does chat work?"
```

### Sync with Code
```
/experts:backend-usecases:self-improve
/sync-experts   # All experts
```

## Guidelines

- Keep each expertise ~100 lines
- Focus on: file locations, patterns, key structures
- Update after significant code changes
- Expertise matches hexagonal/FSD layers
