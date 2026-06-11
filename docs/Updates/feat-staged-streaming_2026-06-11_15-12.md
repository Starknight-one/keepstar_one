# Staged streaming for /pipeline — parity item 5.2

**Branch:** `feat/staged-streaming` (worktree; built off main @111f018)
**Context:** C1-parity track item 5.2 (`../V5_VS_C1_PARITY.md`): kill the 4-6s dead air between sending a chat message and the widget rendering. We do NOT stream LLM tokens (C1's approach — and the source of their JSON-breakage saga, see parity doc §6.5) and do NOT skip any agent (A5 rejected 2026-06-11, see v5-known-gaps.md). We narrate the deterministic stages instead.

## What changed

### Backend
- **`POST /api/v1/pipeline/stream`** (new, `handler_pipeline_stream.go`) — SSE-over-POST twin of `/pipeline`. Frames: `stage {"phase":"data_start"}` immediately → `stage {"phase":"data_done", signal, toolName, count, empty, bypassed, agent1Ms}` after Agent1 → terminal `result {<exact /pipeline JSON, spans included>}`. Mid-stream failures become an `error {status,message}` frame (409 session-killed / 500), never a silent hangup. Pre-stream errors (guard 429/503, bad body 400, tenant 500) mirror the old handler byte-for-byte. Guard `Allow`/`RecordCost` + trace persistence mirrored from `Pipeline()`. Old endpoint untouched.
- **`pipeline_execute.go`** — optional `OnStage func(StageEvent)` on the request (nil = identical old behavior), fired once between agents. `Empty` flag derived from the tool result's `"empty:"` prefix (both `catalog_search` and `_internal_state_filter` preserve stale data on 0 hits — count alone would lie).
- **`middleware_logging.go`** — `recordingWriter.Unwrap()`. Without it `http.Flusher` dies inside the logging middleware and the "stream" silently buffers until completion in prod while passing naive unit tests. Negative-proven: removing Unwrap fails `TestPipelineStreamIncrementalThroughMiddleware` (headers never arrive).
- **`handler_pipeline.go`** — `pipeline` field narrowed to a same-file `pipelineRunner` interface (testability; ctor unchanged).

### Widget
- **`client.js`** — `pipelineStreamRequest` (fetch + getReader, frame parser survives chunk splits and CRLF, terminal-frame reader.cancel, onStage isolated so a status-UI bug can't abort the turn) and `pipelineSmartRequest` with a **deliberately narrow fallback**: retry once via old `/pipeline` ONLY if the stream never really started (404 on older backend, connection refused before any frame). Once the server has spoken SSE — error frame or mid-stream break — the error surfaces as-is: the turn may already have run and been charged; re-running would double LLM spend and duplicate the session turn. (This policy was tightened after adversarial review; the original spec said "fall back on ANY throw" — reviewers correctly flagged the double-charge.)
- **`WidgetApp.jsx`** — transient `status` chat message: `Thinking…` → per-tool copy (`Found 12 products — building the view…` / `Narrowed down to 3 — updating…` / `Checking earlier in this chat…` / `Composing a response…`; `Nothing matched — composing an answer…` on empty). `pendingCards` state swaps the stale document for shimmer skeletons once the count is known (cap 6, floor: 0 → no skeletons, keep old doc). Status lines are removed wherever they sit on completion (a facet-filter error landing mid-turn can no longer orphan them). Success path extracted to `applyPipelineResponse` shared by stream/fallback.
- **`SkeletonCards.jsx` + widget.css** — skeleton row mimicking the product-card grid (280px, 4:3 image block, two bars, `kw-shimmer` keyframes), `kw-msg-status` muted style. All inside the shadow-DOM single stylesheet.

## Verification (all fresh)
- Backend: `go vet` clean; `go test ./... -count=1` all ok. New: 4 handler tests (frame order/shape; **incrementality through the real WithLogging chain with a blocking runner** — the data_done frame is asserted received BEFORE the runner unblocks; kill mid-stream → error{409}; rate-limit → plain 429, not SSE) + 4 usecase tests (nil no-op; fired once with full fields; empty-search stale count; empty state-filter).
- Frontend: vitest 63/63 (stream parsing across split chunks; error-frame throw; early-end throw; fallback matrix: 404→fallback, pre-frame network fail→fallback, frames-arrived→NO fallback, error-frame→NO fallback, both-fail rethrow; SkeletonCards N cards). `npm run build` green; `dist/widget.js` 231,923 B (+3.5 KB vs 228,431).
- Adversarial review (2 lenses): both verdicts approve; 1 major (fallback double-charge — **fixed**, policy narrowed + 4 new tests), CRLF nit fixed, reader-cancel nit fixed, skeleton floor fixed, status-orphan fixed, state_filter empty-case fixed. Accepted residual: tiny window where the connection dies before the first frame lands but after the server accepted (fallback would re-run the turn) — data_start is written within ms of accept, window is negligible; noted here for honesty.

## Not done / follow-ups
- Live prod smoke after deploy: stream a real turn (curl -N + widget devtools), verify Railway edge passes SSE unbuffered (X-Accel-Buffering set; nothing nginx-like in the image — expected fine).
- WidgetApp-level tests for status-message lifecycle (helpers tested indirectly; first app-level test left for a later slice).
- `bypassed`/`agent1Ms` fields are emitted but unused by the widget — reserved for a debug overlay.

## Live prod verification (post-deploy, 2026-06-11 ~15:30, main@2ed89de)

Streamed a real turn on tenant `pim-furniture-demo` ("show me office chairs"), frame timestamps via raw socket read (artifact: `assets/feat-staged-streaming_sse_smoke.txt`):

```
+0.36s  [stage]   {"phase":"data_start"}
+2.41s  [stage]   {"phase":"data_done","signal":"new_search: 50 items found",...}
+4.05s  [result]  preset=product_card  agent1Ms=2050  agent2Ms=1388
```

Frames arrive separately — Railway edge passes SSE through unbuffered. Note for future smokes: python `HTTPResponse.read(n)` BLOCKS until n bytes on chunked responses and fakes "buffering" — use `read1()` (first smoke run was that false alarm).
