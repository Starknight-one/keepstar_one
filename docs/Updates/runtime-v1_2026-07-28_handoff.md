# Runtime v1 — build + live-hardening handoff (2026-07-27 → 28)

> The resume-point doc. Read this + `../../RUNTIME_SPEC.md` (canonical build
> spec, rulings R1–R18 + final owner decisions) to pick up with zero context.
> Everything below is deployed on Railway `selfless-tranquility`/dev.

## What exists now (all live-verified unless marked otherwise)

**The Keepstar Interface Runtime v1**: one engine, three forms (storefront /
crm / onboarding — a "form" = prompts + operations + presets + data scope;
owner canon: forms are data, never hardcode three).

Live-proven end-to-end on tenant **blue-harbor-realty**, assembled entirely
by conversation on the `/onboard` page:

| Beat | Status | Evidence |
|---|---|---|
| Password gate (`Bb8tj13k`) + onboarding session | ✅ | 200/403, cookie, resumable |
| Turn 1–2: proposal with interleaved blocks | ✅ | 7 blocks: text/registration_form/uploader_card/design_system_preview; manifest {staged:5} on the wire |
| Multi-op staging by conversation | ✅ | create_tenant + 2×value_set/entity + enable_operations + define_automation in one turn |
| Registration form submit = approve | ✅ | 200; auto-stages `register_user` if the model didn't, auto-applies the whole manifest; tenant `blue-harbor-realty` (type "real estate agency"), owner user, entities Lead + Property Showing in DB |
| Tokenless CSV upload = approve | ✅ | 202 → completed: processed 10, projectionRows 10, invalidated true |
| Storefront search over uploaded data | ✅ | product_card in 3.1s, same turn as upload — no restart |
| Operation invoke with typed contracts | ✅ | `capture_lead` rejected a non-E.164 phone with a human reason, then created lead `Maria Santos / status new` (value-set default) |
| Conversation-born operations | ✅ | instances named by the model: find_leads / capture_lead / advance_lead / notify_staff |
| R14 role gate | ✅ | visitor invoking find_leads → "denied … requires role staff" (fixed live, see commits) |
| Real per-block SSE streaming | ✅ built + unit-tested | blocks stream as generated (`event: block`); visually verify in the browser |
| CRM beat (surface token → staff chat → lead) | ⚠️ NOT live-run | blocked by the surface-URLs tail below |

## The run in numbers

- 2 workflows (7 + 23 agents) + ~10 hand-fixes during the live pass.
- Commits this session (ultra `main`): `1b5c6e5` split layout + chips +
  turn-1 fallback · `eb6328d` owner-feedback pass (unlock, 3-case flow,
  contextual chips, incremental streaming; 45 files) · `83805b6` security
  review (credential scrub in audit, auto-apply serialization) · `9e461da`
  e2e (`e2e/demo_flow_test.go`, runnable vs Railway with env) · `39de990` +
  `07136e7` register auto-stage (+persist before apply-reload) · `309e693` +
  `409b82b` ingest-door auto-stage · `e8a91af` + `0341e61` staff gate on
  entity queries · `f84e0a0` adopt_presets skip-unknown.
- Admin `main`: `8350e2c` (M3 preset families) — no changes in this pass.

## The owner's laws encoded this run (do not regress)

1. **A user action must never depend on the model having called a tool.**
   Form submit = approve; file upload = approve; missing staged step for a
   rendered artifact → the server stages it deterministically (persisted
   BEFORE the apply path reloads state — that was a real bug).
2. **Cosmetic failures must not keep the workspace dead**: unknown preset
   names are skipped with `skipped[]`, the apply chain continues.
3. **Entity reads are staff-scoped** even though the shared `query` template
   is visitor-visible for catalog search (gate lives in the executor).
4. Keep haiku; fix flow/prompts/UX, not the model.

## Known tails (ranked)

1. **Surface URLs not issued on the Blue Harbor session**: the model
   narrated "here are your links" without applying; direct `apply_manifest`
   then halted on invented preset name `lead_cards` (fix `f84e0a0` deployed
   AFTER that attempt). One `apply_manifest` re-run — or a fresh onboarding
   session — should now pass. **Systemic candidate:** auto-apply zero-input
   steps when their block renders (same law as forms/uploads).
2. **CRM beat unverified live** (`/crm/{slug}?k=`): waiting on №1.
3. `surface_links` preset renders without the actual URLs (binding gap).
4. `meta_adopt_presets` needs stage-time name validation against the
   library (model invents names).
5. Turn-1 proposals are sometimes thin (deterministic fallback catches the
   empty case; richness still dice-driven).
6. Notification row from automation `alert_new_lead` not verified.
7. Workflow leftovers: resume chip-shape quirk (prefer `manifestStatus`
   over the full manifest in OnboardingShell), gofmt drift in
   `anthropic/cost.go`, streamed-text-then-invalid edge.
8. Onepager (`../../PITCH/onepager_preseed.html`) final; `_v1_backup` kept.

## Owner test flow (browser)

0. Once: `/etc/hosts` → `69.46.46.41 v5-engine-dev.up.railway.app
   curator-dev.up.railway.app` (or VPN).
1. https://v5-engine-dev.up.railway.app/onboard → password → **use a fresh
   agency name** (slug collisions with blue-harbor-realty/realtor-agency).
2. Describe the business → watch blocks stream in (canvas left / chat
   right). Thin answer → just send the next message.
3. Accept (chip appears when a plan is staged) → registration form in chat
   (any email) → success plaque. Must work in ONE submit.
4. Upload `Keepstar_project/demo-data/listings.csv` → "10 processed" →
   storefront search finds them immediately.
5. Ask for the links. If the bot narrates without links — say "apply it";
   if still stuck, that's tail №1 — report.
6. Storefront URL: search, cards, booking form (phone must be
   +5521999990000-style — E.164 validation is real).
7. CRM URL (`?k=`): "any new leads?" → Maria Santos (or your booking) →
   "mark it contacted".
8. Curator (`curator-dev.up.railway.app`): per-turn traces + operation runs.

## Resume pointers (fresh session)

- Spec: `RUNTIME_SPEC.md` (root). Canon: `MANIFESTO.md`. This doc: state.
- e2e: `project_v5/backend/e2e/demo_flow_test.go` (env E2E_BASE_URL,
  E2E_ADMIN_URL, ONBOARDING_PASSWORD, ADMIN_SERVICE_KEY, E2E_DATABASE_URL).
- Env on Railway dev already set: ONBOARDING_PASSWORD / ADMIN_BASE_URL /
  ADMIN_SERVICE_KEY (both services) / PUBLIC_BASE_URL. Replicas pinned 1.
- Next work, in order: tails 1–3 → full CRM beat live → owner's browser
  walk → M4 hardening (rate-limit proof, curator additions, demo-day
  checklist, rollback drill).
