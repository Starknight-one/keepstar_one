# V2 — Builder flow spec

> Status: **LOCKED** (owner, 2026-07-29 — Q1–Q4 resolved, see §10)
> Scope: Part One of the product — the Builder. Part Two (Usage) appears only
> where it constrains Part One.
> Relation to V1: `../RUNTIME_SPEC.md` (runtime v1) stays the engine spec.
> V2 is the flow + the face. V1 is alive and deployed; V2 evolves it in place,
> no rewrite.

## 0. Product identity (context for every decision below)

Keepstar generates **AI-native software**. The back — data, operations,
automations — is real and assembled per business. The front is generative:
rendered to resolve the task at hand, with parts freezable so nothing is
endlessly re-generated for no reason. Every surface ships with its own
specialized chat agent (storefront chat, CRM chat) that can dig into the data
or perform actions on request — that is the identity, not a feature.

Two parts:

1. **Builder** — a new paying user lands in the landing chat, says what
   business they are and what they need. An onboarding workflow assembles the
   back config + picks a design system, shows it live, takes edits in chat,
   and soon hands over a fully working system.
2. **Usage** — the user works in the generated system and never has to come
   back; builder agents stay available for changes.

This run (V2) = make the Builder flow settle: build storefront + CRM for the
realtor case **well**, and make it look great. Per owner: "visually great"
and "flow works" are the same axis, not two.

## 1. Versioning

- `VERSIONS.md` at this repo's root — append-only log: version, what went in,
  decisions, leftovers.
- The builder page shows the version string (e.g. `v2.0`) in the footer,
  baked at build time. We test on prod; the footer answers "what is live
  right now".
- Current live state is retroactively **v1.0**. First deploy under this spec
  is **v2.0**.

## 2. The flow (happy path, realtor reference case)

1. New user arrives at the **builder page** (its current Railway home;
   the landing links to it). Work starts in its chat — no separate
   "signup then dashboard" detour.
2. They say who they are: *"Realtor business. I need a CRM and a storefront I
   can share with buyers and other realtors. Storefront must let people book
   showings or calls; those land in my CRM as new leads; I work the leads in
   the CRM."*
3. Blueprint match → assembly starts. The canvas is behind/around the chat;
   the chat floats over it (mock-demo pattern) and docks at the bottom.
4. **Staged reveal, honest.** Assembly is fast (blueprint), but it is
   revealed in real stages, narrated in sync: data model → storefront tab
   renders → CRM tab renders → the connecting link, demonstrated. No fake
   stagger, no "your storefront will appear shortly" text — if a part can be
   rendered, it is rendered, streamed onto the canvas.
5. Two tabs on the canvas: **Storefront** and **CRM**. Both are alive at
   first render because demo data is seeded as part of assembly.
6. **Auto-demonstration** of the invisible link: the builder itself fills the
   booking form on the Storefront tab, submits, the canvas switches to CRM,
   the new lead row appears highlighted. The user then pokes it by hand:
   uploads data, flips statuses, checks that a lead arrives.
7. **Iteration loop**: edits by chat + point-and-tell (click an element on
   the canvas → it becomes the referent of the next message). Visible edits
   re-render only the changed region with a short highlight; invisible edits
   are demonstrated (auto for new cross-surface links, on request otherwise).
8. **Handoff**: surface URLs issued, user leaves to use the system. Builder
   agents remain reachable for later changes.

Cost envelope: a full builder conversation cycle may cost $2–5 in model
spend. That is acceptable; per-turn cost is not the constraint, dead turns
and mis-aimed edits are.

## 3. Laws (converged 2026-07-29 — do not regress)

- **L1. Render, don't narrate.** If a piece can be shown, it is streamed onto
  the canvas. Text saying "X will appear" where X could have been rendered is
  a bug.
- **L2. The user looks at their app, never at a schema.** Workflow diagrams /
  operation graphs are under the hood, not a user-facing channel.
- **L3. Visible change → targeted re-render.** Only the changed region
  re-renders, with a brief "what changed" highlight. Full-screen re-render on
  edit is a bug. (Choreography: the MatterSurface mutation — dissolve region
  → re-birth changed.)
- **L4. Invisible change → demonstration.** Operations, automations,
  cross-surface links are shown by the builder executing a mini-scenario on
  demo data in front of the user (book → switch tab → lead appears). Never
  by prose alone.
- **L5. Point-and-tell.** Click on any canvas element selects it and enters
  chat context as the referent. Selection only — not an editor, no
  drag-and-drop.
- **L6. Demo data is part of the flow.** Seeded at assembly (listings, a few
  leads/contacts). Without it both tabs are dead and L4 has nothing to
  demonstrate. Seed packs are realistic, assembled **per business class**
  and ship with the blueprint (realty agency → realty pack); flagged demo,
  purged when real data arrives.
- **L7. Blueprint = a valid starting manifest** in the same language the
  conversation mutates. No privileged path; "give the core, talk through the
  rest" must work mechanically.
- **L8. Every surface ships with its specialized chat.** Storefront chat,
  CRM chat — scoped agents over the same runtime. AI-native is the identity.
- **L9. Freeze is real.** Generated fragments persist; nothing is
  re-generated unless it changed. (Generation happens on change, projection
  on view.)
- **L10. Demonstration cadence:** auto on first assembly and on new
  cross-surface links; by button afterwards.
- **L11. The critical path hardens first.** Storefront form → operation →
  lead → CRM is the emotional climax of the flow; if it glitches once during
  a demonstration, trust in the whole builder dies. It gets run to
  "cannot not work" before any card gets prettier.

## 4. The realtor reference case

- **Storefront tab**: search, filters, listings. Listings are two-level:
  a *group* (complex/building) containing *units*, or a standalone object.
  Card on the storefront = group card with unit selection inside.
  Booking: showings and calls → become leads.
  Shareable: URL works for buyers and partner realtors.
- **CRM tab**: white canvas + floating menu plaque with sections —
  **Objects** / **Contacts** / **Tasks**. Tasks attach to a client↔object
  pair. New leads land here; statuses are advanced here (by hand and via the
  CRM chat).
- Both tabs carry their specialized chat (L8).

## 5. Visual language (source: `../mock-demo/app/src/scenes/`)

- `matter.tsx` — **MatterSurface**: matter-assembly spine. IN (birth), OUT
  (dissolve), and **Mutation** = OUT region → swap master → IN region. Maps
  1:1 to L3.
- `birthDesktop.tsx` — chat input center-stage → docks bottom, floats over
  canvas; think → assemble phase choreography.
- `birthView.tsx` — the reveal wave (header → map → listings), floating
  control rail as DOM over the canvas.
- `realEstateBirth.tsx` — the same choreography as live DOM + framer-motion
  stagger (spring, y+scale, staggerChildren).
- **Open (decide in the visual pass):** MatterSurface draws on canvas; real
  product blocks are live DOM. Working assumption: framer-motion stagger
  (realEstateBirth pattern) is the workhorse for real blocks; matter is
  reserved for hero moments (first birth, big mutations) — unless a
  DOM-adapted matter proves cheap.
- Theme: light blue `#5BA4D9` + orange `#F0924A`. No purple. All user-facing
  text in English.

## 6. Out of scope for V2

- Part Two beyond the presence of per-surface chats (freeze-management UI,
  agent-does-the-whole-task flows).
- Domain-specific operation packs (cosmetics etc.). Universal-operations
  rework is a separate track, not this run.
- Real CRM login/auth (surface tokens stay, said out loud in any handover).
- Billing, marketplace, multi-blueprint library work beyond the realtor case.

## 7. Open questions

*(all resolved at lock — see §10 Rulings log)*

## 8. Build order (coarse, provisional — discuss after the spec settles)

1. **B1 — critical path (L11):** scripted assembly → book → lead → CRM,
   e2e, run until boring. (Includes finishing V1 tails: surface URLs,
   `surface_links` binding, CRM beat live.)
2. **B2 — canvas shell:** tabs, chat-over-canvas, version footer.
3. **B3 — staged reveal:** honest stages, per-block streaming onto canvas.
4. **B4 — visual pass:** storefront + CRM presets on the design language of
   §5; demo data seeding.
5. **B5 — demonstration runs (L4/L10).**
6. **B6 — point-and-tell (L5).**
7. **B7 — polish loop** on the full realtor case, on prod, with version
   bumps per deploy.

## 9. Done =

The run closes when this walk passes on prod, twice in a row, and is filed
with artifacts (screens/JSON per the visible-verification rule):

1. Fresh browser, landing chat, realtor pitch in one message.
2. Staged reveal streams in; Storefront + CRM tabs alive on demo data.
3. Auto-demonstration: booking on Storefront → lead highlighted in CRM.
4. Three edits via chat, at least one via point-and-tell; each lands as a
   targeted re-render, no full-screen flash.
5. Surface URLs issued; the booking→lead path repeated by hand on the
   issued storefront URL.
6. Footer shows the shipped `v2.x`; `VERSIONS.md` entry written.

## 10. Rulings log (appended after LOCK)

- **R1 (2026-07-29, owner):** The builder stays at its current Railway home
  (v5 `/onboard` page, restyled). No embedding into `keepstar-landing`;
  the landing links to it.
- **R2 (2026-07-29, owner):** No "Data" tab. Data lives where it belongs
  per case — for realty, leads + objects are CRM sections.
- **R3 (2026-07-29, owner):** Demo data is realistic and assembled per
  business class as blueprint seed packs — "realty agency → here, touch
  what you're getting". (Folded into L6.)
- **R4 (2026-07-29, owner delegated):** Deploy split (serve instance vs
  builder instance, same image) is DEFERRED to the first real tenant.
  Until then one dev service; we run for "it works" first.
