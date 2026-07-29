# Spec (run-level)

Create or evolve a **run spec** — the document for a multi-feature run:
a version of the product (V2, V3…), a flow overhaul, a milestone arc.
Sits one level above `/feature`: every item in the run spec's Build order
is later expanded into its own `ADW/specs/feature-*.md` via `/feature`.

## Variables

USER_PROMPT: $ARGUMENTS
CONFIG: ADW/adw.yaml
EXPERTS_DIR: .claude/commands/experts/

## When to use which

- `/spec` — a run: many features, UX laws, visual language, weeks of work,
  owner iterates on the document itself.
- `/feature` — one bounded feature, detailed enough for autonomous `/build`.

## Instructions

- If USER_PROMPT not provided, STOP and ask what run this spec is for.
- The spec is a **conversation artifact**: it is written AFTER convergence
  in dialogue with the owner, not instead of it. If positions haven't been
  discussed yet, discuss first (positions + questions), write second.
- Save to repo root as `<NAME>_SPEC.md` (e.g. `V2_SPEC.md`) — owner-visible
  and git-tracked, never only in memory or chat.
- English, like all dev docs. Time estimates in minutes, not sessions.
- The spec is **append-mostly**: laws and decisions get numbers and dates;
  superseded text is marked superseded, not silently deleted. Cite numbers
  (L3, Q2, R14) instead of relitigating.

## Required sections

```md
# <Name> — <one-line what this run is>

> Status: draft | review | LOCKED (owner, date)
> Scope: what this run IS. One sentence.
> Relation: which specs this builds on / supersedes (link them).

## 0. Context / identity
Why this run exists; the product framing every decision below serves.
Short — a screen, not a chapter.

## 1. Laws (numbered L1..Ln, dated at the header — do not regress)
The converged owner decisions, each one sentence of rule + at most two of
rationale. These survive the run; features come and go, laws don't.
A law that can't be violated by a concrete future commit is not a law —
rewrite it until it can be.

## 2. Reference case
The ONE concrete scenario this run must nail, end to end, in user terms.
Not an abstraction: named business, named tabs, named actions.

## 3. Design / visual language  (if the run touches the face)
Pointers to reference implementations (file paths), palette, motion
language. Working assumptions marked as such.

## 4. Out of scope
Explicit, with one-line rationale each. This section prevents the run from
gaining a second head mid-flight.

## 5. Open questions (numbered Q1..Qn — each owned by the owner)
Every question ships with a proposal. A question without a proposal is
homework dumped on the owner.

## 6. Build order (B1..Bn, coarse)
Ordered items, each one later expands into `/feature`. Dependency notes
where order is forced. The critical path (the beat that kills trust if it
glitches) goes FIRST, before any polish.

## 7. Done =
What must be live-proven for the run to close: the demo walk that passes,
the artifacts filed (per visible-verification rule), the version string
that ships. "Done" that can't be demonstrated on prod is not done.

## 8. Rulings log (appended during the run)
Dated owner decisions made after LOCK, RUNTIME_SPEC-style:
`R<n> (date): <decision>. Supersedes: <what>.`
```

## Constraints

- DO NOT: include implementation steps, file-by-file plans, code — that's
  `/feature` territory.
- DO NOT: average conflicting positions into mush; surface the conflict as
  an open question with a recommendation.
- DO: keep it under ~250 lines. Longer means it's absorbing feature specs.
- DO: update `VERSIONS.md` when the run's version ships.

## Output

```
Run Spec Created/Updated

File: <NAME>_SPEC.md
Status: draft|review|LOCKED
Laws: N · Open questions: N · Build items: N
Next: owner review pass → resolve Q's → LOCK → /feature per build item
```
