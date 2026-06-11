# feat/system-presets-list — system preset library listing (Wave A, v5 side)

**Date:** 2026-06-11
**Branch:** `feat/system-presets-list` (off `main` @ 7a873b6)
**Scope:** v5 backend only (`project_v5/backend`). Admin proxy half of the
contract lands in `keepstar-admin` separately.

## Context

Wave A of the canvas master plan needs the admin canvas to show a
registration picker over the system preset library. Until now the
library's taxonomy (what family a preset belongs to, what the
components are for) lived only implicitly in seed maps + the Agent2
prompt text. This session ships the v5 half of the frozen cross-repo
contract: a single internal endpoint that returns every system preset
and component with name, Agent2-visible description, family category,
default replicate flag, and kind.

## Approach

- **Taxonomy lives in the presets package**, next to the existing seed
  maps (`SystemPresetSeeds`, `SystemPresetDefaultReplicate`):
  - `SystemPresetCategory` — the frozen family map (cards ×6,
    details ×3, states ×2, narrative ×1).
  - `SystemComponentDescriptions` — short Agent2-style descriptions for
    the two reusable components, keyed by seed key.
  - `SystemComponentPublicNames` — seed key → public wire name. The
    contract names the components `component_price_rating` /
    `component_brand_badge` (matching the seed file names), but the
    `SystemComponentSeeds` keys are the engine-internal RefNode targets
    (`price-rating-root` / `brand-badge-root`). An explicit map keeps
    the engine detail off the wire instead of leaking node ids.
- **Endpoint** `GET /api/v1/internal/presets/system` on the existing
  `PresetHandler`, registered in the internal block of `routes.go`.
  Same `checkInternalKey` gate as cache-invalidate/preview (503 when
  `V5_INTERNAL_KEY` unset, 403 on mismatch); exempt from `WithTenant`
  via the existing `/api/v1/internal/` prefix rule — no middleware
  change needed. Response: `{"presets":[{name, description, category,
  defaultReplicate, kind}, ...]}`, 14 entries, sorted by name.
- **Minute-fix:** `systemPresetFallback` in the postgres adapter
  hardcoded `Description: "system-default preset"`; it now reads
  `SystemPresetDescriptions` (generic string kept only as a guard for a
  name missing from the map — which the taxonomy test prevents).
- **Untouched on purpose:** `prompt.go` / `SystemPresetsBlock` / the
  Agent2 prompt path — the cached prompt prefix is byte-identical;
  `TestAgent2SystemPromptCacheBudget` passes untouched.

## Files changed

- `project_v5/backend/internal/engine/presets/presets.go` — three new
  taxonomy maps (category, component descriptions, component public
  names).
- `project_v5/backend/internal/engine/presets/taxonomy_test.go` — NEW:
  `TestSystemPresetTaxonomyComplete` — every seed has a canonical
  category / description / public name AND no orphan taxonomy entries
  (both directions, so seeds and taxonomy cannot drift apart).
- `project_v5/backend/internal/handlers/handler_presets.go` —
  `systemPresetEntry` wire struct + `ListSystem` handler.
- `project_v5/backend/internal/handlers/routes.go` — route in the
  internal block + endpoint-catalog doc comment line.
- `project_v5/backend/internal/handlers/handler_presets_test.go` —
  `TestListSystemAuthGate` (503 unset / 403 wrong key),
  `TestListSystemContract` (decodes the raw JSON body: 14 entries in
  exact sorted order, exact 5-field set per entry, canonical
  categories, non-empty descriptions, spot-checks for
  product_card/details/states/narrative/component_price_rating, and
  the description == `SystemPresetDescriptions` text — same text the
  Agent2 prompt block injects).
- `project_v5/backend/internal/adapters/postgres/postgres_preset.go` —
  fallback description fix (line ~109).
- `project_v5/backend/internal/adapters/postgres/postgres_preset_test.go`
  — NEW: `TestSystemPresetFallbackDescription` +
  `TestSystemPresetFallbackUnknownName`. Note: the task said "extend
  the nearest existing test", but the only existing tests of this path
  are `-tags=integration` (live Neon) and constructed WITHOUT the
  registry; `systemPresetFallback` needs no DB, so a new in-package
  unit test was the honest home — it runs in the default suite.

## Verification (asserted output)

`go vet ./...` → exit 0, no output.

`go test ./... -count=1` tail:

```
ok  	keepstar_v5/internal/adapters/postgres	0.798s
ok  	keepstar_v5/internal/domain	1.073s
ok  	keepstar_v5/internal/engine	1.769s
ok  	keepstar_v5/internal/engine/presets	2.068s
ok  	keepstar_v5/internal/handlers	2.396s
ok  	keepstar_v5/internal/prompts	2.872s
ok  	keepstar_v5/internal/tools	2.573s
ok  	keepstar_v5/internal/usecases	2.316s
TEST EXIT: 0
```

New tests + the untouched cache budget, verbose:

```
--- PASS: TestListSystemAuthGate (0.00s)
    --- PASS: TestListSystemAuthGate/key_unset_→_503 (0.00s)
    --- PASS: TestListSystemAuthGate/wrong_key_→_403 (0.00s)
--- PASS: TestListSystemContract (0.00s)
--- PASS: TestSystemPresetTaxonomyComplete (0.00s)
--- PASS: TestSystemPresetFallbackDescription (0.00s)
--- PASS: TestSystemPresetFallbackUnknownName (0.00s)
--- PASS: TestAgent2SystemPromptCacheBudget (0.00s)
```

What the contract test actually asserts (not just "200 OK"): the raw
response body decodes to exactly 14 entries whose `name` values equal,
index by index, the frozen sorted list (component_brand_badge …
text_explainer); each entry has exactly the 5 contract fields; every
category ∈ {cards, details, components, states, narrative}; every
description non-empty; product_card = cards/replicate=true/preset and
component_price_rating = components/false/component.

## Known gaps / open questions

- **Component naming divergence (resolved, flag for reviewer):** the
  frozen contract's component names (`component_*`) ≠ registry seed
  keys (`*-root`). Resolved via `SystemComponentPublicNames`; if the
  admin side ever needs to address components by name against other v5
  APIs (e.g. preview), it must be aware those APIs speak the seed-key
  names, not the public names.
- The admin proxy (`GET /admin/api/canvas/presets/system`) is NOT in
  this repo — keepstar-admin task.
- The generic `"system-default preset"` fallback string in
  `systemPresetFallback` is now dead code in practice (taxonomy guard
  forces map completeness); kept as a defensive guard per task spec.
- Branch not pushed (per task instruction).

## Post-review (2026-06-12 night, main session)

Review verdict: **approve**. Follow-up commit `192a88e`: TestListSystemContract now pins the FULL 14-row name→{category,replicate,kind} table (reviewer proved a flipped category survived the spot-check version by mutation); taxonomy guard extended to SystemPresetDescriptions both directions — the postgres fallback comment is now accurate. Cross-repo e2e (admin proxy → this endpoint → Library UI) smoke-proven on a Neon branch, see keepstar-admin/Updates/2026-06-11_waveA-system-library.md.
