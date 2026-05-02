# Chunk 6 — LLM in the Loop

## Context

After chunks 1-5.5 V5 has the engine, sectional state, binding, presets, components, format/wrapper props, and a cache-aware token measurement infrastructure — but no LLM, no HTTP, no Agent2 turn. This chunk closes that gap.

V4 chose **raw HTTP** for its Anthropic adapter. That decision created a long tail of pain — every new feature (count_tokens, cache_control variants, prompt-cache metrics, future thinking/computer-use blocks) had to be re-implemented by hand and was error-prone. **V5 explicitly diverges**: we use the **official Anthropic Go SDK** (`github.com/anthropics/anthropic-sdk-go@v1.38.0+`). Cost: one external dep. Benefit: typed cache_control, typed tool blocks, built-in count_tokens, built-in retry/backoff, free upgrades to new model features.

Chunk 6 stays large no matter what, so we split it into four sub-chunks. Each ships independently; each commits + pushes the v5 branch at the end.

- **6a** — Anthropic SDK adapter shell + LLMPort + replace tokens stub. **Lands the SDK and proves it works against count_tokens + a smoke message call.** No Agent2, no HTTP yet.
- **6b** — Agent2Execute use case + visual_assembly V5 tool + prompt-builder + tenant-design-context loader. **First real LLM turn against V5, invoked from a test entry point.** HARD GATE: cacheable prefix ≥ 4500 tokens.
- **6c** — HTTP server + handlers (`/api/v1/pipeline`, `/session/init`, `/session/{id}`) + DI of all migrations + middleware (logging/CORS/tenant) + graceful shutdown. **First end-to-end pipeline call over HTTP.**
- **6d** — `zoneWriteWithDelta` tx fix + `AddDelta` retry/advisory-lock + Tracer port + spans on PG adapters + LLMMessage cache_control hint plumbing if not already covered by 6a's adapter.

This document plans **6a in concrete detail** (we execute next) and **6b/6c/6d as sketches** (concrete plans land in their own plan-mode session).

---

## Chunk 6a — Anthropic SDK adapter shell

### What lands

A new `adapters/anthropic` package in V5 wrapping the official SDK behind a `LLMPort` interface. One method exposed for now — the cached chat-with-tools call Agent2 needs. Plus a `count_tokens` method, replacing our chunk-5.5 HTTP stub. Smoke test: make a minimal real call to Haiku, assert non-empty response.

### Files to add

| File | Purpose |
|---|---|
| `project_v5/backend/internal/ports/llm_port.go` | LLMPort interface + CacheConfig + ChatRequest/ChatResponse types |
| `project_v5/backend/internal/domain/llm_usage.go` | LLMUsage struct (InputTokens / OutputTokens / CacheCreationInputTokens / CacheReadInputTokens / CostUSD / Model). `domain.LLMMessage` already exists from chunk 2 — reuse |
| `project_v5/backend/internal/adapters/anthropic/client.go` | Wraps `anthropic.NewClient`, holds model + apiKey + sync.Map for prompt cache hints if needed |
| `project_v5/backend/internal/adapters/anthropic/chat.go` | `ChatWithToolsCached` — converts domain types → SDK params, places cache_control per CacheConfig flags, calls `client.Messages.New`, maps response back to domain |
| `project_v5/backend/internal/adapters/anthropic/count_tokens.go` | `CountInputTokens` — calls `client.Messages.CountTokens`. Same signature as the chunk-5.5 stub so tokens/measurement_test.go can switch to it without test-code changes |
| `project_v5/backend/internal/adapters/anthropic/cost.go` | Per-model pricing table (Haiku 4.5 / Sonnet 4.6 / Opus 4.7) + `Calculate(usage LLMUsage) float64` |
| `project_v5/backend/internal/adapters/anthropic/chat_smoke_test.go` | Live smoke test (build tag `live`) — gated on ANTHROPIC_API_KEY, calls Haiku with a 1-line user msg, asserts non-empty response. Minimal token cost, intended for manual verification only |
| `project_v5/backend/internal/adapters/anthropic/cache_placement_test.go` | Unit test verifying our cache_control placement matches the V4-proven pattern (tools last, system first, history second-to-last) |

### Files to modify

- `project_v5/backend/go.mod` — add `github.com/anthropics/anthropic-sdk-go v1.38.0` (or latest at execution time)
- `project_v5/backend/internal/engine/tokens/count_tokens.go` — **delete** (becomes adapter-owned). Or convert it to a thin re-export that delegates to the SDK adapter, so the test file doesn't need to change imports
- `project_v5/backend/internal/engine/tokens/measurement_test.go` — switch `CountInputTokens` import to the new adapter package; otherwise no behavioural change

### LLMPort interface (concrete)

```go
type LLMPort interface {
    // Cached chat-with-tools — Agent2 production path. CacheConfig flags
    // toggle cache_control on tools / system / history blocks.
    ChatWithToolsCached(
        ctx context.Context,
        systemPrompt string,
        messages []domain.LLMMessage,
        tools []domain.ToolDefinition,
        cfg CacheConfig,
    ) (*domain.LLMResponse, error)

    // Count tokens for a fully-assembled request (system + tools + msgs).
    // Used by tokens/measurement_test.go and by Agent2 to gate prompts.
    CountInputTokens(
        ctx context.Context,
        systemPrompt string,
        messages []domain.LLMMessage,
        tools []domain.ToolDefinition,
    ) (int, error)
}

type CacheConfig struct {
    CacheTools        bool
    CacheSystem       bool
    CacheConversation bool
    ToolChoice        string // "auto" (default) | "any" | "tool:<name>"
}
```

Other V4 LLMPort methods (`Chat`, `ChatWithTools`, `ChatWithUsage`) — do **not** port. Production Agent2 only uses the cached variant; YAGNI on the others.

### Cache-control placement (port the V4-proven pattern via SDK types)

V4 places `cache_control: {"type":"ephemeral"}` on:
1. The **last tool** in the tools array (when `CacheTools`)
2. The **system block** (when `CacheSystem`)
3. The **second-to-last message** in the messages array (when `CacheConversation` and ≥ 2 msgs)

In SDK types this becomes:
- Tools: build `[]anthropic.ToolUnionParam` and assign `CacheControl: anthropic.NewCacheControlEphemeralParam()` to the last entry
- System: `MessageNewParams.System = []anthropic.TextBlockParam{{ Text: prompt, CacheControl: ephemeral }}`
- History: walk `[]anthropic.MessageParam`, find `len-2`, attach cache_control on its content blocks

Unit test (`cache_placement_test.go`) constructs synthetic input, runs the placement logic, asserts the resulting params struct has cache_control on exactly the expected blocks. No live API call.

### Smoke test (`chat_smoke_test.go`, build tag `live`)

```go
//go:build live
// Run: ANTHROPIC_API_KEY=... go test -tags=live ./internal/adapters/anthropic/...
```

- Construct adapter with Haiku 4.5 model id (read from env or hard-coded constant for now).
- Call `ChatWithToolsCached` with a tiny system prompt ("You are a helpful assistant.") and one user message ("Reply with just the word OK.").
- Assert response.Text is non-empty + Usage.InputTokens > 0.
- Total cost: ~$0.0001 — negligible.

Build tag `live` keeps it out of CI / regular `go test ./...`.

### Replace the chunk-5.5 token-counting stub

`tokens/count_tokens.go` becomes a one-line wrapper that delegates to the new `adapters/anthropic.CountInputTokens` — preserves the existing test API surface so `measurement_test.go` doesn't need code changes, but the underlying implementation is now SDK-typed. Re-run the cache-aware measurement test to confirm numbers match what we measured in chunk 5.5.

### Verification

```sh
cd project_v5/backend

# Build + vet
go mod tidy
go build ./... && go build -tags=integration ./... && go build -tags=live ./...
go vet ./...

# Unit tests (cache placement + chat structure)
go test ./internal/adapters/anthropic/...
# all green

# Live smoke
ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY go test -tags=live -v ./internal/adapters/anthropic/...
# logs: response text + token usage; assertion only that we got back ≥ 1 token

# Re-run cache-aware token measurement (now via SDK)
ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY go test -tags=tokens -v ./internal/engine/tokens/...
# numbers match chunk-5.5 baseline (V4 ~7235, V5 ~3705 input tokens)

# Existing test suites still green
go test ./...
TEST_DATABASE_URL=$DATABASE_URL go test -tags=integration -count=1 ./...
```

### 6a commits (4)

1. `feat(v5): add anthropic-sdk-go dep + LLMPort interface (chunk 6a)` — go.mod, go.sum, llm_port.go, llm_usage.go domain
2. `feat(v5): anthropic adapter — ChatWithToolsCached + CountInputTokens (chunk 6a)` — adapters/anthropic/{client,chat,count_tokens,cost}.go + cache_placement_test.go
3. `chore(v5): switch tokens measurement to SDK count_tokens (chunk 6a)` — tokens/count_tokens.go delegated to adapter, test still passes with same numbers
4. `docs(v5): chunk 6a session log + plan freeze`

Then `git push origin v5`.

---

## Chunk 6b — Agent2 + first real LLM turn (sketch)

### What lands

The `Agent2Execute` use case ported from V4, adapted for the V5 scene-graph engine. The first end-to-end "user query → LLM turn → ops applied → bound document" runs against V5 in a test (no HTTP yet). **HARD GATE**: V5 system + tools cacheable prefix ≥ 4500 tokens, otherwise prompt caching is unstable and V5 loses cost-wise (chunk 5.5 measurement showed +70.5% over a 10-turn conversation if we miss the threshold).

### Major files

- `project_v5/backend/internal/domain/tool.go` — `ToolDefinition`, `ToolCall`, `ToolResult`, `ToolContext` (port from V4)
- `project_v5/backend/internal/tools/tool_registry.go` — sorted-by-name registry for cache stability
- `project_v5/backend/internal/tools/tool_visual_assembly.go` — V5 visual_assembly tool: parses ops + preset + replicate, runs `Materialise → ApplyOps → ExpandReplicates → ResolveAndInline → BindData`, writes resulting Document to `state.Current.Template`
- `project_v5/backend/internal/prompts/prompt_compose_widgets.go` — V5 Agent2 system prompt body. Grow from current 5637 byte / 1556 token sketch to the production prompt: port V4's BUILDING / COMPOSING / MODIFYING examples, FIELD BINDING rules, ANTI-PATTERNS, decision rules. Target: prompt + tools ≥ 4500 cacheable tokens
- `project_v5/backend/internal/usecases/agent2_execute.go` — port V4 Agent2Execute: state retrieval → buildSystemPrompt → trim history → ChatWithToolsCached → execute tool → write to state → return
- `project_v5/backend/internal/usecases/prompt_cache.go` — sync.Map memoisation per tenant slug + version, identical contract to V4
- `project_v5/backend/internal/usecases/format_fields_block.go` — `<fields entity="product">` block from `FieldDefinitionPort` (ported from V4 `formatFieldsBlock`)
- `project_v5/backend/internal/usecases/format_design_context.go` — `<tenant_design_context version="N">` block reading `v5_presets` + `v5_components` (instead of V4's admin.tenant_*)
- `project_v5/backend/internal/usecases/agent2_smoke_test.go` — driven by `go test`, no HTTP, talks to live LLM + live Neon, asserts: tool was called, ops applied, bound Document carries product names

### Hard gate
After the prompt body lands, re-run `go test -tags=tokens` and confirm:
- V5 cacheable prefix ≥ 4500 tokens (the threshold)
- 10-turn effective cost V5 < V4 (the actual goal)

If the gate fails: iterate on prompt content (more useful examples, more decision rules — content the LLM benefits from, not filler) until it passes.

### 6b commits

5-6 commits (vocabulary + tool + prompt + use case + test + docs).

---

## Chunk 6c — HTTP server + handlers + DI (sketch)

### What lands

`cmd/server/main.go` becomes a real entrypoint. The first HTTP endpoint that ships: `POST /api/v1/pipeline` — Agent2-only path (no Agent1 yet, since V5 hasn't ported `catalog_search` / `state_filter`). For testing, a mock Agent1 stub fills `state.Data` from a hard-coded query → catalog adapter call.

### Major files

- `project_v5/backend/cmd/server/main.go` — DI: config → logger → DB → migrations (state, preset, component) → adapters → ports → use cases → handlers → http.Server with graceful shutdown
- `project_v5/backend/internal/config/config.go` — env var loader: PORT, DATABASE_URL, ANTHROPIC_API_KEY, LLM_MODEL, TENANT_SLUG, LOG_LEVEL
- `project_v5/backend/internal/handlers/handler_pipeline.go` — `POST /api/v1/pipeline`: parse body → resolve tenant from `X-Tenant-Slug` → call Agent2 → return Document JSON
- `project_v5/backend/internal/handlers/handler_session.go` — `POST /api/v1/session/init` + `GET /api/v1/session/{id}` — minimum needed to drive a real conversation
- `project_v5/backend/internal/handlers/middleware_logging.go` — request_id + structured slog + SpanCollector in context
- `project_v5/backend/internal/handlers/middleware_cors.go` — open CORS for chat widget
- `project_v5/backend/internal/handlers/middleware_tenant.go` — resolves tenant from `X-Tenant-Slug` header, falls back to TENANT_SLUG env, stores in context
- `project_v5/backend/internal/handlers/routes.go` — net/http.ServeMux setup
- `project_v5/backend/internal/handlers/handler_pipeline_integration_test.go` — spins up the server + hits `/api/v1/pipeline` with a real-tenant query

Skipped for now (port later if needed): `/navigation/expand`, `/navigation/back`, `/actions`, `/testbench`, `/debug/traces`. Frontend port is its own work item — V5 server starts emitting scene-graph and we'll handle the frontend side separately.

### 6c commits

5-6 commits.

---

## Chunk 6d — Cleanup + caching plumbing (sketch)

### What lands

The known-gap items deferred since chunks 2-3:

- **`zoneWriteWithDelta` transaction**: wrap zone UPDATE + AddDelta in a single `BEGIN/COMMIT` so they can't diverge. File: `project_v5/backend/internal/adapters/postgres/postgres_state.go`.
- **`AddDelta` retry**: caller-side retry with backoff on the unique-constraint violation (or pg_advisory_xact_lock at session level). Same file.
- **Tracer port + spans**: re-add `domain.SpanFromContext` to all PG adapter methods (catalog, state, preset, component, field_definition). Define a `domain.SpanCollector` interface + a context helper. Wire from request-id middleware. Revives the V4 `/debug/traces` waterfall feature down the line.
- **LLMMessage cache_control hint plumbing**: if 6a doesn't already wire this through to messages (the V4 pattern marks the second-to-last message), close the gap here.

### 6d commits

3-4 commits.

---

## Push policy

**At the end of every sub-chunk** (6a, 6b, 6c, 6d), commit the logical groups (per the per-chunk commit list above) and **push the v5 branch**:

```sh
git push origin v5
```

Confirm `git rev-parse origin/v5` matches local. Sub-chunks land independently; if any one stalls, the previous ones are already on origin.

---

## Verification (end of chunk 6 overall)

```sh
cd project_v5/backend
go build ./... && go build -tags=integration ./... && go build -tags=live ./...
go vet ./... && go vet -tags=integration ./... && go vet -tags=live ./...
go test ./...

# Integration on Neon
TEST_DATABASE_URL=$DATABASE_URL go test -tags=integration -count=1 ./...

# Live LLM smoke (manual, not CI)
ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY go test -tags=live -v ./...

# End-to-end HTTP smoke against running server
go run ./cmd/server &
curl -X POST http://localhost:8082/api/v1/pipeline \
  -H "X-Tenant-Slug: heybabescosmetics" \
  -H "Content-Type: application/json" \
  -d '{"sessionId":"...","query":"show me 3 lipsticks"}'
# returns a v5 Document JSON
```

---

## Time + risk

- 6a: small, well-scoped (~1-2 hours). Risk: SDK API differences vs my expectations — mitigated by the smoke test catching breakage early.
- 6b: largest sub-chunk. Risk: prompt-engineering iteration to clear the cache threshold while staying useful — budget for 2-3 iterations of the prompt body.
- 6c: medium. Risk: tenant resolution + middleware order — mitigated by mirroring V4.
- 6d: medium. Risk: tx semantics under concurrent writers — mitigated by a focused integration test before declaring closed.

If any sub-chunk blows up, the previous one is already on `origin/v5` and the project doesn't lose work.
