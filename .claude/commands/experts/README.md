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

| Expert | Domain | Architecture |
|--------|--------|--------------|
| `backend` | Go API, hexagonal layers, handlers, usecases | Hexagonal (Ports & Adapters) |
| `frontend` | React, components, hooks, widgets | Feature-Sliced Design |

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
/experts:backend:question "How do I add a new use case?"
/experts:frontend:question "Where are widgets rendered?"
```

### Sync with Code
```
/experts:backend:self-improve true    # With git diff
/experts:frontend:self-improve false  # Without git diff
/sync-experts                         # All experts
```

## Writing Expertise

### expertise.yaml Format

```yaml
overview:
  description: "Brief description"
  architecture: "Hexagonal | Feature-Sliced"

project_structure:
  root: "path/"
  active_code: {}      # Working code
  new_structure: {}    # Stubs/future

layer_rules:
  layer_name:
    imports: "What can import"
    contains: "What goes here"

core_implementation:
  # Detailed file mapping

patterns:
  naming: {}
  structure: {}

run_commands:
  start: "command"
  build: "command"

migration_status:
  description: "Current state"
  next_steps: []
```

### Guidelines

- Keep it concise — mental model, not docs
- Focus on: file locations, patterns, key structures
- Respect line limits (300 lines max)
- Update after significant code changes

## ACT → LEARN → REUSE Cycle

```
/feature → /build    # ACT: Write code
/sync-experts        # LEARN: Update expertise
Next session         # REUSE: Start with knowledge
```

Each cycle adds knowledge. Sessions start smarter over time.
