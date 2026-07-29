# Versions

> Append-only. One entry per shipped version. The builder footer shows the
> live version string; this file is its history.

## v2.0 — wave 1 (2026-07-29) — **REVERTED same night (owner call)**

Code rolled back to `1a8e0c6` (the 2026-07-28 live-proven state). `V2_SPEC.md`
kept as canon. Wave-1 branches (`v2/*`) retained locally, unmerged.

- Spec: `V2_SPEC.md` (LOCKED 2026-07-29). Wave 1 shipped B1 + B2/B3 + B4:
  critical-path fixes (auto-issue surface URLs on render, surface_links
  binds real URLs, adopt_presets stage-time sanitize), canvas shell (tabs
  Builder/Storefront/CRM with live iframes, chat dock over canvas, honest
  per-block stagger reveal, v2.0 footer), realty demo seed pack
  (`seed_demo_data` step, 12 listings / 7 leads, demo-flagged, guarded).
- Log: `docs/Updates/v2-wave1_2026-07-29.md`. Wave 2 next: B4 visual pass,
  B5 demonstration runs, B6 point-and-tell, demo purge, matter hero moments.

## v1.0 — runtime v1 (2026-07-27 → 28, retroactive)

- The Keepstar Interface Runtime v1: one engine, three forms
  (storefront / crm / onboarding). M1 operations plane, M2 entity plane +
  secured runtime, M3 onboarding end to end + live-hardening pass.
- Live on Railway `selfless-tranquility`/dev @ `1a8e0c6`.
- Known tails: `docs/Updates/runtime-v1_2026-07-28_handoff.md`.
