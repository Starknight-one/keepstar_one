# 2026-07-27 — Re-anchor root docs to interface-runtime canon (return point)

## Context

Returning to the project after a pause. Two things changed since the
docs were last touched:

1. **Canon changed (2026-07-27, root `../MANIFESTO.md`):** Keepstar =
   **interface runtime** — data + operations + interface; anything can
   be visualized to resolve a user's question the moment they ask. LLM
   decides what to show and which operation to run; a deterministic
   engine binds real tenant data and executes. Chat widget is one door,
   the headless API is the other; commerce is one vertical, not the
   identity. Missing piece = the **operations layer** (catalog of
   validated operations with strict inputs/outputs) — main architecture
   front. Supersedes the June "conversion-led AI chat assistant" cut.
2. **July teardown:** all services were taken off Railway in July 2026
   (`v5-engine-production.up.railway.app` etc. are dead). Neon
   flat-moon is alive with data (3172 products, 35 tenants).

The working tree also carried uncommitted June edits (conversion-led
rewrite of CLAUDE.md / README.md / LAUNCH_ROADMAP.md) — finished
forward to the July canon instead of reverting.

## Changed

- **`CLAUDE.md`** — "What this is" rewritten to the runtime canon +
  current-state paragraph (Railway teardown, flat-moon alive, local run
  via `scripts/start_all.sh`); Dev-essentials Prod cell marked dead;
  Pointers: `../MANIFESTO.md` marked CANON (course 2026-07-27);
  `CANVAS_MASTER_PLAN.md`, `V5_VS_C1_PARITY.md`, `FINAL_PHASE_PLAN.md`,
  `SESSION_HANDOFF_2026-05-30.md` repointed to `../archive/` and marked
  HISTORICAL.
- **`README.md`** — intro rewritten to the same runtime framing;
  "Deployed services" table replaced with a "Deployment status" note
  (July teardown, DB alive, local mode); embed snippet made host-
  agnostic (no public deployment right now).
- **`docs/LAUNCH_ROADMAP.md`** — banner added at the top pointing to
  the new identity; body kept as-is (June conversion-led framing,
  operational content still valid).
- **`.claude/commands/experts/admin/expertise.yaml`** — harmless
  whitespace-only expert self-update, committed as found.

## Not touched

- CLAUDE.md routing / experts cycle / project rules / working rules.
- LAUNCH_ROADMAP.md body below the banner.
- Everything else in `docs/` (trackers, specs, historical audits).
- Other repos' docs; root `../MANIFESTO.md` itself (already the canon).

## Next

- Local bring-up of the stack (`scripts/start_all.sh` against
  flat-moon) and a live look at the system.
- Operations-layer architecture track (catalog format, two CRUD
  contours, onboarding flow) — open questions listed in
  `../MANIFESTO.md`.
