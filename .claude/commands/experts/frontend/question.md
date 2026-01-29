---
allowed-tools: Bash, Read, Grep, Glob
description: Answer questions about React frontend, components, hooks, and widgets without coding
argument-hint: [question]
---

# Frontend Expert - Question Mode

Answer the user's question by analyzing the React feature-sliced frontend implementation. This prompt provides information about the frontend layer without making any code changes.

## Variables

USER_QUESTION: $1
EXPERTISE_PATH: .claude/commands/experts/frontend/expertise.yaml

## Instructions

- IMPORTANT: This is a question-answering task only - DO NOT write, edit, or create any files
- Focus on: React components, hooks, entities (atom/widget/message), features (chat/overlay)
- If the question requires code changes, explain the approach conceptually without implementing
- Validate information from EXPERTISE_PATH against the codebase before answering
- Remember: there's active code (App.jsx, Chat.jsx) and feature-sliced stubs

## Workflow

1. **Load Expertise**
   - Read EXPERTISE_PATH to understand architecture and patterns
   - Note layer rules (shared → entities → features → app)
   - Identify active code vs feature-sliced stubs

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
- **Architecture Context**: How this fits into feature-sliced layers
- **Related Files**: Other files that may be relevant

**Example:**

```
Question: "How do I render a widget?"

Answer:
Widgets are rendered via WidgetRenderer which composes Atoms.

Pattern (feature-sliced stubs):
1. Widget has type and atoms array
2. WidgetRenderer switches on widget.type
3. Each widget type renders its atoms via AtomRenderer

Flow:
MessageBubble → WidgetRenderer → AtomRenderer

Key Files:
- src/entities/widget/WidgetRenderer.jsx
- src/entities/widget/widgetModel.js (WidgetType enum)
- src/entities/atom/AtomRenderer.jsx
- src/entities/atom/atomModel.js (AtomType enum)

Note: Currently feature-sliced structure contains stubs.
Active chat code is in src/components/Chat.jsx.
```
