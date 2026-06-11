# V5 Agent2 tool-call format — token optimization decision (ADR)

> Date: 2026-05-29. Context: evaluate replacing the JSON `ops` tool args with a
> compact DSL (à la v9 batch_design / Thesys Lang) to cut Agent2 OUTPUT tokens.
> Method: multi-agent workflow (23 agents) — investigate emission/tokens, map
> blast radius, port-check v9 DSL, research LLM reliability of string-DSL-in-arg,
> design 3 variants + judge panel (4 lenses), adversarial verify ×3.

## TL;DR — MEASURE FIRST; the big win already exists; micropresets beat a rewrite

- **The large DSL win is ALREADY in the engine.** preset + ops + server-side
  binding already gives ~**12.7×** compression on the dominant path
  (`{preset, replicate:3}` ≈ 28 tok vs ~356 tok hand-rolled) and the LLM never
  emits data values. We did not "forget" the DSL — we built its payoff.
- A compact wire format adds only a **~18–25% envelope trim** (OUTPUT-only),
  **not** a multiplier. props are copied verbatim (`apply_ops.go:93
  node := Node(props)`) so the byte-bulk on freestyle/compose does NOT shrink.
- **String-DSL-in-a-tool-arg: rejected.** Reliability research is decisive — a
  bespoke grammar inside a string field loses JSON-schema/constrained-decoding
  validation, is off the model's trained distribution, and a single malformed
  retry re-bills a full turn (dwarfs ~25 saved tokens). Net usually negative.
- **Winner of the 3 designs = shorthand-JSON** (stay valid JSON, abbreviate
  envelope keys only). But all 3 adversarial verifiers returned
  **`prefer-alternative`**: win is real but marginal and fragile.
- **Cheaper, bigger, safer lever (3/3 verifiers): grow preset/micropreset
  coverage** — move freestyle (356 tok) asks onto the preset path (28 tok) =
  the 12.7× lever, on exactly the expensive shapes, with **zero** new parsing,
  **zero** cache-bust, **zero** correctness hazard.
- **Recommendation: MEASURE FIRST** (no output-token baseline exists; the
  call-shape distribution is unmeasured). Gate any build on real data.

## Honest token numbers (per call shape)

| Shape | Now (OUTPUT tok) | shorthand-JSON | Saving |
|---|---|---|---|
| 1 preset-only (50–60% of calls) | ~28 | ~10–12 | ~50% (~14 tok) — but rule-1 "ALWAYS pass mode" erodes it |
| 2–3 preset+ops | 92 / 146 | −15–25% on envelope | modest (props dominate) |
| 4–5 freestyle (rare) | 356 / 632 | −~21% | low ceiling (props native) |
| 6 compose | ~580 | −15–20% | low ceiling |
| 7–8 modify | 88 / 208 | −22–49% | good (small props) |

**Blended: ~127.5 → ~95–105 tok/call ≈ 18–25% OUTPUT reduction (~22–30 tok/call).**
On Haiku 4.5 output ($5/Mtok) is comparable-to-larger than cache-read input
($0.10/Mtok), so it IS real money — but small absolute, and contingent on the
(unmeasured) distribution. The 2–4× framing was the preset-vs-handroll win,
which already exists and this change does not touch.

## Why string-DSL was rejected (reliability)

Constrained decoding / strict mode validates the JSON envelope, NOT the contents
of a string field. A DSL stuffed in a string arg is generated as unconstrained
free-form text (the regime models err in) with no decode-time rejection, while
being more out-of-distribution than complex JSON. JSONSchemaBench: unconstrained
validity falls to 13–38% on non-trivial grammars; strict/structured pushes JSON
to ~100%. Retry math: one malformed retry ≈ 0.10×prefix + full output re-gen >>
the compact saving. Verdict: less reliable AND usually net-negative tokens.

## Risks of the shorthand-JSON aggressive variant (must neutralize)

1. **Default-mode = silent destructive bug.** Dropping `mode` and defaulting to
   rebuild turns a forgotten modify into a rebuild that **wipes the on-screen
   view** (`tool_visual_assembly.go:163–204`). → **Keep `mode` REQUIRED**;
   abbreviate only key+values (`m` + `reb`/`mod`).
2. **`o`/`o` collision** (ops-array vs op-verb at different depths). → use **`v`
   for the verb**.
3. **Retry asymmetry** is the kill switch: one malformed retry
   (`agent2_execute.go:324–331`) ≈ a full extra turn ≈ wipes 15–25 calls of
   savings. Malformed rate must not rise >~1–3pp.
4. **Cache-bust once:** editing the tool schema + the 8 in-prompt examples busts
   the ephemeral cached prefix (`tool_visual_assembly.go:47`, `chat.go:79–88`).
   Net-measure input delta + output delta together.

## The better lever — micropreset coverage (preferred by all 3 verifiers)

Add 2–3 micropresets / preset tags for the common freestyle & compose patterns
(e.g. list-row card, hero+gallery+CTA, compact). Rule 3 (`agent2_prompt.go:395`)
already routes preset-first; widening coverage shifts expensive Shapes 4–6
(356–632 tok) into the cheap Shape-1 family (28 tok) — the real 12.7× win, where
absolute tokens are largest, with no DSL/parsing/cache/correctness risk.

## Recommended path

**Step 0 — MEASURE (gate, ~1 day).** Extend the token harness
(`internal/engine/tokens/measurement_test.go`, build tag `tokens`; note: it
currently measures INPUT only and `convertMessages` drops tool_use —
`count_tokens.go:71–78` — so **new output-side plumbing is required**). Measure:
(a) OUTPUT tokens per shape (run the 8 canonical calls through `count_tokens` as
assistant tool_use), current vs shorthand; (b) the REAL call-shape distribution
from captured `agent2.llm` spans (`agent2_execute.go:175–184`); (c) malformed +
retry rate over ≥100 prompts A/B, with **zero tolerance** for silent
modify→rebuild. **Also measure preset-coverage gaps** (how much freestyle volume
a micropreset would absorb).
- **Gate:** proceed to a format change only if blended OUTPUT saving ≥15% AND it
  is non-trivial net of the re-cached input prefix AND malformed/retry ≤ current.
  Otherwise → fund micropresets instead.

**If the gate clears — ship the LOW-RISK variant only (7 steps, ~24–36h):**
1. MEASURE FIRST (gate) — 6–10h — `internal/engine/tokens/measurement_test.go` (+ output plumbing)
2. Normalization shim `normalizeShorthand(input)` at `tool_visual_assembly.go:127` (always-on, tolerant of native keys; **does not default mode**) — 3–4h — new `tool_visual_assembly_shim_test.go`
3. Tool schema → abbreviated keys, `mode` stays REQUIRED, `v` for verb — 2–3h — `tool_visual_assembly.go:60–99`
4. Agent2 prompt: rewrite 8 examples + OPS VOCABULARY to short keys; keep rule-1; re-run `go test -tags=tokens` (prefix shrinks — verify ≥4500 bar) — 4–6h — `agent2_prompt.go:25–421`
5. Update LLM-facing tests only (engine tests unchanged — they build maps post-shim) — 2–3h — `tool_visual_assembly_test.go`, `agent2_prompt_test.go`
6. Feature flag `AGENT2_SHORTHAND` for A/B (shim always-on; flag flips only what the LLM is taught) — 2–3h — `config/config.go`, `agent2_execute.go:153–168`
7. Rollout + post-deploy measurement vs baseline; Updates log — 3–4h

## Caveat — this is an optimization, not what makes v5 work

The thing standing between v5 and "works superbly" is the A-series gaps
(greeting, modify-vs-rebuild, back, layout) and search quality — NOT the token
format. Token format is a margin/COGS optimization worth doing AFTER the engine
is reliable and AFTER measurement. Do not let it block the demo path.

Source: workflow `wf_38c1ac23-423` (full result in session task `wctudtijd`).
