# Reflections — Preset System & User Behavior Design

Chain of thought and design exploration for expanding presets and improving the display system.
Session: 2026-02-13 evening (~2.5 hours of discussion).

---

## Starting Point

**Pain:** Too little information displayed, no ability to check variations.

**Vision:** Everything the user does is asking to change the SET and FORMAT of displayed information. Presets are the flexible part — LLM calls tools with flags, backend does the heavy lifting. This abstraction should solve classes of problems, not individual features.

**Constraints:**
- Speed and cost matter — fewer tokens = cheaper, faster, more accurate
- Tools fill the cache on first message, then reads are fast
- LLM can't freestyle-compose (too many tokens, slow, hallucinations)
- Presets = hybrid sweet spot between LLM flexibility and code efficiency
- User cases are finite, 90% need simple cards + ability to modify what they see
- Voice module coming — some things easier as buttons than voice
- Services = special case of products, same system

---

## Phase 1: User Behavior Analysis

### Three Axes of User Behavior

All user actions decompose into three axes:

**Axis 1: Data Source (where information comes from)**
- Search — exists (Agent1 → catalog_search)
- Current screen — exists (state)
- Previously seen / liked — NOT exists
- Non-product info — NOT exists (FAQ, delivery, hours)

**Axis 2: Display Format (how information is shown) — WEAKEST AXIS**
- Field selection (on/off) — exists but rigid
- Display style (formatting) — exists
- Layout (grid/list/single) — exists but coupled to preset
- Comparison — NOT exists
- Sort/group — NOT exists
- Density control — NOT exists

**Axis 3: Actions (what user does)**
- Navigation expand/back — exists
- Like/save — NOT exists
- Widget actions (cart, variant select) — NOT exists
- Chat request = combination of axes (also an action)

**Key insight:** "Wiggly" edge cases are always COMBINATIONS of basic operations across these axes. "Show me those red bags I liked, but bigger" = Axis 1 (liked) + Axis 2 (product_card, large). The axes are finite, the combinations are what feel infinite.

---

## Phase 2: Current System Weaknesses

### Architecture Layers (as-is)
```
Atom (data + display formatting)
  ↓ inserted into
Slot/Cell of Preset
  ↓ preset = assembled widget
Layout (grid / list / carousel / single)
  ↓ arranges widgets on screen
```

### What Agent2 Controls Now
- Preset selection — 7 named presets (4 product + 3 service)
- Fields override — [{name, slot, display}] — full replacement of default fields
- Size override — tiny/small/medium/large
- Agent2 currently outputs ~50-70 tokens per call (already optimized)

### Mechanism Weaknesses

**A) Tool interface is not delta-friendly.** To add one field, Agent2 must re-specify ALL fields. Should be: `{add: ["description"]}`, not full re-specification.

**B) Mode locked to preset.** `product_grid` = always grid. Can't say "grid fields but as list." Layout should be independent.

**C) No priority or density concept.** No ranking of fields. No "show top N." No middle ground between 5 fields in grid and 10 in detail.

**D) Static field vocabulary.** 10 hardcoded field names. Entity attributes (color, size, material) are one blob. Agent2 can't address them individually. This is literally "too little information" — data EXISTS but mechanism can't surface it.

### Identified Shapes (Structurally Different Preset Types)

| Shape | Description | Status |
|-------|-------------|--------|
| **Card** | One entity = one widget, visual emphasis | Exists |
| **Row** | One entity = one row, compact text | Exists |
| **Detail** | One entity, all available info | Exists |
| **Comparison** | N entities side-by-side, fields aligned | Missing |
| **Table** | N entities × M fields, spreadsheet-like | Missing |

Gallery, "only photos" = Card/Row with field delta, not new shape.

---

## Phase 3: Tool-Train Concept

### Core Idea

Instead of Agent2 constructing configurations (fields, slots, displays) — give it a META-TOOL (tool-train) with composable primitives. Agent2 sets FLAGS. Backend resolves everything.

LLM task simplifies from "generate configuration" to "classify user intent → pick flags." Classification = what LLMs do best.

### Composable Primitives (Mixing Board Metaphor)

Each flag = one simple operation. COMBINATION = custom result nobody pre-designed. Like a mixing board: each slider is simple, but combinations produce infinite mixes.

```
User: "покажи покрасивее с ценами крупно и без рейтинга"
Agent → flags: [emphasis_visual, field_price_prominent, hide_rating]
Backend resolves each flag → emergent custom result
```

---

## Phase 4: Three Dimensions of Visual Assembly (Breakthrough)

### The Realization

We're designing FREESTYLE MODE — what was postponed as "too complex." But if done right, nothing else is needed. Presets become saved configs within the freestyle system. Not "throw away presets for freestyle." Freestyle as FOUNDATION, presets as SHORTCUTS on top.

### Dimension 1: CONTEXT (what Agent2 knows)

What flows into Agent2 (from Miro sketch):
```
System prompt ──[Cache+Delta]──→
User request ────────────────────→  Agent2  →  Tool train
Previous context [Cache+Delta]──→
View right now ──[Cache+Delta]──→
View data now ───[Cache+Delta]──→
```

Contents:
- System prompt (tool-train definition, rules) — cached
- Current view meta (layout, visible fields, sizes) — delta from previous
- State data meta (entity count, available fields, value ranges via master format) — delta
- User request — new each turn
- History meta (previous turns: action + result, compact) — delta
- Screen dimensions — cached

Data flow problems:
- 300 products × 75 attributes = can't send raw. Need master format (ranges, enums, counts)
- History needs meta-representation (action + result per turn), not full replay
- "Are there purple phones with 5GB RAM?" = lookup against meta ranges, not raw data scan

### Dimension 2: MATERIAL (what Agent2 builds with)

**The ONLY building block is the Atom.**

- Widget = result of GROUPING atoms (an operation, not a building block)
- Formation = result of LAYING OUT groups (an operation, not a building block)
- Preset = a saved RECIPE (which atoms + which operations)
- "4 loose atoms without a widget" = atoms without grouping applied. Normal case, not exception.

Atom = {type, value, display, position}. Everything else is operations applied to atoms.

### Dimension 3: OPERATIONS (what Agent2 can do)

Starting set of operations (from sketch + analysis):

| Operation | What it does |
|-----------|-------------|
| **show/hide** | Create or remove an atom/group |
| **resize** | Change scale/emphasis (increase/decrease) |
| **reorder** | Change position (place before/after) |
| **restyle** | Change display format (h1→badge, price→price-lg) |
| **group/ungroup** | Assemble atoms into widget / dissolve |
| **layout** | How groups arrange on screen (grid/list/single/table) |

**The "too many variations" feeling comes from PARAMETERS, not operations.** Each operation is simple. But {which target} × {how much} × {relative to what} × {under what conditions} = combinatorial explosion. Parameters are resolved from CONDITIONS (Dim 1), not specified by agent.

**Gaps found in three base operations (show/hide, resize, reorder):**
- **Conditional operations** — "only red ones" = conditional show/hide based on DATA VALUES, not explicit toggle
- **Aggregate/compute** — "average price" = new data that doesn't exist on any entity
- **Relate** — "compare first and third" = structural relationship between entities

These may be additional primitives or modifiers on existing ones. TBD.

### Business Logic (from sketch)

When Agent2 makes a decision, it must resolve:
1. **Which atoms?** — ranked set exists, does it need updating? If no preset → global ranking
2. **Which formatting?** — anything beyond standard?
3. **What order and size?** — hard rules that can't be bypassed + user customization ("photo super big") that goes beyond ready variations but must not break anything
4. **What container?** — solo card, list, grid, composite? Custom wishes like "one big, others smaller"?
5. What else? (open question)

---

## Phase 5: Code vs Agent Split (The Layered Resolution Model)

### The Boundary Problem

- Deterministic tasks → code (fast, cheap, accurate)
- Ambiguous/creative tasks → agent (flexible, relevant, but slower/costlier)
- Too much code → rigid, agent is a glorified button
- Too much agent → 150 parameters, expensive, error-prone

### Three-Layer Resolution

**Layer 1: CODE resolves DEFAULTS.**
Based on context (entity count, screen size, entity type, business rules), code picks sensible defaults for EVERYTHING. Field ranking, layout choice, sizes, positions. "6 products, 1200px screen → grid, medium, top-5 fields by priority." No agent needed.

**Layer 2: AGENT resolves DELTAS.**
Based on user request, agent modifies ONLY what needs changing. 1-5 parameters per request (90% of cases), up to 10 for complex requests.

```
Request: "покажи описание крупнее"
Agent sets: {show: ["description"], resize: {description: "large"}}
Code defaults: everything else stays from current view
```

**Layer 3: CODE applies CONSTRAINTS.**
After agent's deltas, code ensures nothing breaks. Image can't exceed viewport. Can't show 20 fields in tiny widget. Conflicting operations resolved deterministically.

### Key Principle

**Code handles the expected. Agent handles the unexpected.**

Field ranking = code. Deviation from ranking = agent.
Default layout = code. "Show as table" = agent.
Constraint enforcement = code. Creative combination = agent.

---

## Phase 6: Architecture Decision — Backend-First

### Critical Constraint

**Frontend = dumb renderer. Always.** This is a core architectural principle of Keepstar.

The entire resolution engine (defaults → deltas → constraints → layout → positioning) lives on the BACKEND. Backend sends fully assembled JSON. Frontend receives and draws. Period.

This means:
- Resolution engine = backend Go code
- Layout engine = backend Go code
- Constraint solver = backend Go code
- Field ranking = backend Go code
- Frontend = receives FormationWithData JSON, renders atoms by type/display

**Frontend almost doesn't change.** It already renders atoms by type. May need a few new display variants, but that's minor. All intelligence stays on backend.

### Refactor Scope

**What DOESN'T change (~70% of codebase):**
- Agent1 + catalog_search (entire data pipeline)
- State system (zones, deltas, turn tracking)
- Navigation (expand/back, formation stack, instant nav)
- Catalog, multitenancy, embeddings, vector search
- Widget shell (Shadow DOM, overlay, chat)
- Chat, stepper, infrastructure
- Admin panel

**What DOES change (one layer):**
- `tool_render_preset.go` → resolution engine (defaults → deltas → constraints)
- Preset registry → field ranking system + default configs
- Tool definitions → tool-train replaces `render_*_preset`
- Agent2 prompt → rewrite for tool-train interface
- Frontend: minimal changes (new display variants if needed)

### Incremental Implementation Plan

1. **Resolution engine on backend.** Current presets become "default configs" in new system. Tool-train works on top, but output = same FormationWithData. Frontend untouched. Validate that agent + deltas work.

2. **Expand atom rendering.** Add new display variants to frontend if needed. Flag-based: old templates vs new renderer. Validate visuals.

3. **Remove old path** when new system is stable.

Each step = working product. If step 1 fails → minimal rollback, just do more presets.

---

## Summary: What We're Building

**A backend engine for generating frontend per user request.**

Three dimensions:
1. **Context** — what Agent2 knows (meta, not raw data)
2. **Material** — atoms as the only building block
3. **Operations** — finite set of composable primitives

Three-layer resolution:
1. **Code defaults** — handles 90% deterministically
2. **Agent deltas** — handles the 10% that needs reasoning (1-5 params per request)
3. **Code constraints** — ensures nothing breaks

Architecture:
- ALL logic on backend
- Frontend = dumb renderer (unchanged principle)
- Presets = saved shortcut configs within the freestyle system
- Tool-train = meta-tool where agent sets flags, backend resolves

---

## Open Questions for Next Session

1. **Complete operation set** — are show/hide, resize, reorder, restyle, group, layout sufficient? What about conditional operations (filter by value), aggregation (average price), and relation (compare)?

2. **Field ranking system** — how to rank fields by priority? Business rules? Entity metadata? Per-tenant config? Does ranking update based on user behavior patterns?

3. **Master format for large catalogs** — how to represent 300×75 as compact meta? What level of detail does Agent2 need?

4. **Tool-train parameter design** — concrete syntax. What does the tool definition look like? What does a typical agent call look like? Validate that 1-5 params covers 90%.

5. **Constraint solver scope** — what rules? How complex? Can it be a simple rule engine or needs something more?

6. **Composition cases** — "detail card + 2 small widgets + 4 loose atoms" — how does tool-train express composite layouts?

7. **Backward compatibility** — can current 7 presets work as-is within the new system during transition?

---

*Last updated: 2026-02-13 evening*
*Status: Design phase complete for today. Core architecture agreed. Next session: prototype resolution engine + tool-train parameter space on backend.*
