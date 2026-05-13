# Chunk 7 — Agent1 port (NLU + data retrieval)

## Context

Chunks 1-6d shipped V5 end-to-end as far as Agent2 (rendering): scene-graph engine, presets/components, LLM adapter, ops applier, HTTP server with tx-safe state writes and span tracing. But the HTTP `/api/v1/pipeline` only runs Agent2 — clients have to pre-populate `state.Current.Data` themselves before each call. That's not a real product.

Agent1 is the data-retrieval half of the pipeline: it takes the user query, decides whether to fetch new catalog data or filter what's already loaded, and writes results to `state.Current.Data` + `state.Current.Meta`. Agent2 then renders. This chunk ports V4's Agent1 to V5 and wires the two-agent pipeline behind `/api/v1/pipeline`.

User hint: "Agent1 mostly stays the same — copy. Tools may change a bit because catalog access differs. Important: Agent1 also has prompt assembly like Agent2."

What "differs" in V5 catalog vs V4:
- **No pgvector / embeddings yet.** V4's `catalog_search` is hybrid (keyword + pgvector + RRF merge); V5 catalog has only keyword (`ListProducts` with multi-word AND-logic ILIKE). For chunk 7 we keep the V4 tool schema as-is (LLM keeps emitting `vector_query`) but the executor falls back to keyword-only. Vector / RRF / EmbeddingPort are explicitly deferred.
- **No services.** V5 catalog has `Products` only. `service` entity_type is dropped from the tool schema; if/when V5 grows services we add it back.
- **No `tenants.catalog_digest` column.** V4 pre-generates and stores the digest in the tenants table. V5 builds it on first use and caches in-process for the lifetime of the server (same pattern PromptCache uses today).

Everything else (single-turn tool loop, deterministic state-filter guard, history-lookup, prompt-cache placement, span instrumentation, append-only `conversation_history`) ports verbatim from V4.

## Approach

### Three new tools

All three register in the existing `tools.Registry` (sorted by name → byte-stable cache hashes). All three use V5's `ToolContext` (carries SessionID + TenantSlug).

1. **`catalog_search`** — fetches new data from the catalog.
   - Schema mirrors V4 minus `service` entity_type and the eight skin/PIM filters that don't yet exist in V5's `ProductFilter`. Final V5 schema: `vector_query` (string, required), `filters{brand, category, min_price, max_price, product_form, skin_type, concern, key_ingredient, routine_step, texture, target_area}`, `sort_by`, `sort_order`, `limit`. (V5's ProductFilter already has all eight PIM attrs — `internal/ports/catalog_port.go:23-44`.)
   - Executor: split `vector_query` into AND-logic keyword search (V5 `ListProducts` already does this in `Search` field), apply filters, sort, limit.
   - **No vector search, no RRF**: `Phase 0` (state read) → `Phase 1` (single keyword + filter SQL) → write to state. Returns `{count, products: [{id,name,...}], search_type: "keyword"}` to the LLM.
   - Writes `state.Current.Data.Products`, `state.Current.Meta` via `UpdateData` (zone-write with delta `action.Tool="catalog_search"`).
   - Prices in **rubles** at the LLM boundary (LLM sees rubles, V5 stores integer kopecks → multiply by 100 in the executor, V4 pattern at `tool_catalog_search.go:200-227`).

2. **`_internal_state_filter`** — pure in-memory subset of currently-loaded data.
   - Schema: `entity_type` (always "product" in V5 chunk 7), `brand`, `category`, `min_price`, `max_price`, `min_rating`, `text_match`.
   - Executor reads `state.Current.Data.Products`, applies case-insensitive contains on each filter, writes filtered subset back via `UpdateData`. No DB. Pricing in rubles same as above.
   - V4 has a "preserve original data on 0-result" path — port verbatim (don't blow away the screen if filter returned nothing; just record an empty delta).

3. **`_internal_history_lookup`** — reads deltas, returns matching tool-call summaries.
   - Schema: `query` (string), `last_n` (int, optional).
   - Executor calls `statePort.GetDeltas(ctx, sessionID)`, optionally slices last N, case-insensitive matches `query` against `delta.Action.Tool` and `delta.Path`. Returns formatted lines for the LLM.
   - Read-only. No state mutation.

### Catalog digest

New domain entity + builder + per-tenant in-process cache.

- `internal/domain/catalog_digest.go` — copy V4's `CatalogDigest` struct + `ToPromptText()` verbatim (~98 lines, no V5-specific changes).
- `internal/adapters/postgres/postgres_catalog.go` — port V4's `GenerateCatalogDigest` (lines 974+), but read from V5's catalog tables (same Postgres schema; the catalog itself is shared with V4 — see `CLAUDE.md` "Schemas: catalog, admin, logs"). One SQL query per shared filter (brand / category tree / product_form / skin_type / concern / key_ingredient / routine_step / texture / target_area) plus top-N brands and ingredients. Returns `*domain.CatalogDigest`. Behind a span `postgres.BuildCatalogDigest`.
- New port method on `CatalogPort`: `BuildCatalogDigest(ctx, tenantID string) (*domain.CatalogDigest, error)`. Drop V4's `GetCatalogDigest`/`SaveCatalogDigest` split — V5 builds on demand.

### Agent1 prompt + cache

- `internal/prompts/agent1_prompt.go` — V4's `Agent1SystemPrompt` constant (lines 11-52) + `BuildAgent1ContextPrompt(meta, currentConfig, actions, query)` (lines 75-114) ported verbatim. The `<state>` block includes `loaded_products`, `available_fields`, `liked_count`, `cart_count`, `current_display`. (V5 already has matching shapes in `domain.StateMeta` + `domain.StateActions` + `domain.RenderConfig` — verify field-by-field during port.)
- `Agent1PromptCache` — separate from the existing `PromptCache` (Agent2 uses that for `<fields>`). Same shape: `sync.Map[tenantSlug] → built prompt`. `GetOrBuild(ctx, tenantSlug)`:
  1. `catalogPort.GetTenantBySlug(ctx, slug)` → tenant ID.
  2. `catalogPort.BuildCatalogDigest(ctx, tenant.ID)` → digest.
  3. Concatenate `Agent1SystemPrompt + "\n\n<catalog>\n" + digest.ToPromptText() + "</catalog>\n"`.
  4. Cache + return.
  - On any error: return base prompt unchanged (fail-open — V4 pattern at `agent1_execute.go:407-417`).
- **Reuse note**: existing `internal/usecases/prompt_cache.go` is generic-enough but it's hard-wired to `Agent2SystemPrompt` + `<fields>` block. Cleanest is to extract the `sync.Map` caching behavior into a small helper or just write a parallel `Agent1PromptCache` (~30 lines). Going with the parallel struct — cleaner than parameterizing across two unrelated block builders.

### Cache threshold note (not a hard gate this chunk)

V4 Agent1 prefix (system + tools + digest, no conversation): static prompt ~520 chars (~130 tokens) + digest ~300-400 tokens + tool defs ~1500 tokens = ~2000-2050 tokens. That clears Haiku's documented 2048 floor but is well below Vlad's 4500-token V4-prod stability bar. V4 itself probably gets unreliable Agent1 cache hits on turn 1 and only stabilises once `conversation_history` grows.

For chunk 7 I am **not** going to grow Agent1 prompt to clear 4500 (would mean inventing rules + examples that don't exist in V4). Instead I'll extend `tokens/measurement_test.go` to count Agent1 prefix + log the gap so we can decide later whether to grow. Agent1's effective cost is dominated by output tokens (small) and the keyword-only path doesn't make latency-sensitive LLM calls anyway. Future chunk can grow the prompt if cost data warrants it.

### Agent1Execute use case

`internal/usecases/agent1_execute.go` — single-turn tool loop, mirrors V4's shape (`agent1_execute.go:56-313`).

```go
type Agent1Execute struct {
    llm           ports.LLMPort
    state         ports.StatePort
    catalog       ports.CatalogPort
    toolRegistry  *tools.Registry  // shared with Agent2; filtered by name prefix at call time
    promptCache   *Agent1PromptCache
    log           *slog.Logger
}

type Agent1ExecuteRequest struct {
    SessionID  string
    TenantSlug string
    UserQuery  string
}

type Agent1ExecuteResponse struct {
    ToolName       string                  // empty if no tool call
    ToolInput      map[string]interface{}
    ToolResult     *domain.ToolResult
    ProductsFound  int                     // for microcontext signal in pipeline
    Usage          domain.LLMUsage
    LatencyMs      int64
    EnrichedQuery  string                  // <state>+query as actually sent to LLM
}
```

Flow:
1. Span `agent1.execute` (top-level, defer end).
2. `state.GetState(ctx, sessionID)` (already spans `postgres.GetState`).
3. **Deterministic guard** — port V4's `agent1_execute.go:135-194`. If `Meta.ProductCount > 0` AND query has filter triggers (regex-ish word-list: "только X", "лишь X", "оставь", "дешевле N", "дороже N", "с рейтингом выше N") AND query does NOT have style-request words ("убери/покажи/добавь/без" + field name), bypass LLM and call `_internal_state_filter` directly via the registry. Returns immediately.
4. Build system prompt via `promptCache.GetOrBuild` (span `agent1.prompt`).
5. Build user message via `BuildAgent1ContextPrompt(meta, currentConfig, actions, userQuery)`.
6. Trim ConversationHistory (V4 does NOT trim — it relies on cache for cheap reads. V5 currently has no `ConversationHistory` consumer in Agent2 either; keep V4's append-only behavior. Conversation cache on `len(messages) > 1`.)
7. Tool defs: filter `toolRegistry.GetDefinitions()` by name prefix `"catalog_"` or `"_internal_"` (V4 pattern at `agent1_execute.go:374-385`).
8. `llm.ChatWithToolsCached(ctx, systemPrompt, messages, agent1Tools, CacheConfig{CacheSystem: true, CacheConversation: len(messages)>1, CacheTools: false, ToolChoice: "auto"})` — note `auto` not `any` (Agent1 may legitimately decline to call a tool — style requests). Span `agent1.llm`.
9. **First tool call only** (V4 single-turn). Span `agent1.tool.<name>`. On tool Go-error: retry once with empty input (V4 graceful pattern at `agent1_execute.go:301-309`). Tool-side `IsError` is informational, not retried.
10. Append `user`/`assistant`/`tool_result` triplet to `state.ConversationHistory` via `AppendConversation`.
11. Return response.

### Pipeline orchestrator

New use case `internal/usecases/pipeline_execute.go`. Replaces direct Agent2 call in the HTTP handler.

```go
type PipelineExecute struct {
    agent1 *Agent1Execute
    agent2 *Agent2Execute
    log    *slog.Logger
}

func (uc *PipelineExecute) Execute(ctx, req PipelineExecuteRequest) (*PipelineExecuteResponse, error) {
    // span "pipeline.execute"
    a1, err := uc.agent1.Execute(ctx, ...)            // mutates state.Current.Data
    // microcontext signal:
    //   "new_search: N items found"  if a1.ToolName == "catalog_search"
    //   "filtered: N items"           if a1.ToolName == "_internal_state_filter"
    //   "history: M matches"          if a1.ToolName == "_internal_history_lookup"
    //   "no_data_change"              otherwise (style request, bypassed)
    a2req := Agent2ExecuteRequest{..., UserQuery: req.Query, Microcontext: signal}
    a2, err := uc.agent2.Execute(ctx, a2req)          // mutates state.Current.Template
    return &PipelineExecuteResponse{
        Document:  a2.Document,
        ToolCalls: append(a1.ToolCalls, a2.ToolCalls...),  // surfaced in HTTP response
        Usage:     domain.LLMUsage{...sum...},
        LatencyMs: ...,
        Agent1Ms:  a1.LatencyMs,
        Agent2Ms:  a2.LatencyMs,
    }, nil
}
```

Microcontext is appended to the user message Agent2 sees, not to state. V4 pattern: lets Agent2 know "data just changed, you may want to re-render" without re-reading deltas.

`Agent2Execute` already accepts the microcontext concept implicitly (its prompt has rules like "if state changed, re-render"). Concrete plumbing: extend `Agent2ExecuteRequest` with optional `Microcontext string`; if non-empty, prepend `<microcontext>%s</microcontext>\n` to the user message. Agent2 prompt today doesn't mention microcontext — port V4's one-liner reference to the prompt (~50 chars).

### HTTP handler rewire

`internal/handlers/handler_pipeline.go` — swap dep from `*Agent2Execute` to `*PipelineExecute`. Response shape unchanged from chunk 6c (Document + ToolCalls + Usage + LatencyMs + Spans). Add `Agent1Ms` + `Agent2Ms` fields for breakdown.

`cmd/server/main.go` — add Agent1 wiring:
```go
registry.Register(tools.NewCatalogSearchTool(statePort, catalogPort))
registry.Register(tools.NewStateFilterTool(statePort))
registry.Register(tools.NewHistoryLookupTool(statePort))
agent1Cache := usecases.NewAgent1PromptCache(catalogPort)
agent1 := usecases.NewAgent1Execute(llm, statePort, catalogPort, registry, agent1Cache, log)
pipeline := usecases.NewPipelineExecute(agent1, agent2, log)
pipelineH := handlers.NewPipelineHandler(pipeline)
```

## Files to add / modify

| File | Status | Notes |
|---|---|---|
| `internal/domain/catalog_digest.go` | added | V4 port verbatim — `CatalogDigest` + `ToPromptText` |
| `internal/ports/catalog_port.go` | modified | + `BuildCatalogDigest(ctx, tenantID) (*domain.CatalogDigest, error)` |
| `internal/adapters/postgres/postgres_catalog.go` | modified | + `BuildCatalogDigest` (port V4 lines 974+, drop V5-absent columns); span `postgres.BuildCatalogDigest` |
| `internal/adapters/postgres/postgres_catalog_test.go` | modified | + integration test for digest builder against live Neon |
| `internal/tools/tool_catalog_search.go` | added | keyword-only port; LLM-ruble↔kopeck conversion; `UpdateData` zone-write |
| `internal/tools/tool_catalog_search_test.go` | added | unit + integration cases (filter-only / keyword / no-result) |
| `internal/tools/tool_state_filter.go` | added | in-memory filter; preserve-on-empty pattern |
| `internal/tools/tool_state_filter_test.go` | added | unit cases (brand / price / text_match / 0-result preserves) |
| `internal/tools/tool_history_lookup.go` | added | reads deltas; case-insensitive match on tool+path |
| `internal/tools/tool_history_lookup_test.go` | added | unit cases (last_n / 0-match) |
| `internal/prompts/agent1_prompt.go` | added | V4 system prompt verbatim + `BuildAgent1ContextPrompt` |
| `internal/usecases/agent1_prompt_cache.go` | added | per-tenant `sync.Map` cache, `GetOrBuild`, `Invalidate`; fail-open on errors |
| `internal/usecases/agent1_execute.go` | added | single-turn loop + deterministic guard + spans |
| `internal/usecases/agent1_execute_test.go` | added | unit cases: deterministic-guard hits / misses / LLM-path / retry-on-Go-error |
| `internal/usecases/pipeline_execute.go` | added | orchestrator + microcontext signal |
| `internal/usecases/agent2_execute.go` | modified | accept optional `Microcontext` in request; prepend to user message |
| `internal/handlers/handler_pipeline.go` | modified | dep type `*Agent2Execute` → `*PipelineExecute`; add `Agent1Ms` / `Agent2Ms` to response |
| `internal/handlers/handler_pipeline_live_test.go` | modified | end-to-end: send "Покажи 3 продукта" → expect Agent1 catalog_search + Agent2 visual_assembly + non-empty Document |
| `internal/engine/tokens/measurement_test.go` | modified | + Agent1 prefix measurement (no hard gate; logs gap to 4500) |
| `cmd/server/main.go` | modified | register 3 Agent1 tools; build Agent1PromptCache; wire PipelineExecute |
| `docs/v5-known-gaps.md` | modified | add: "Agent1 prefix < 4500 — accept turn-1 cache miss for now"; "Vector search / EmbeddingPort deferred"; "Services entity not in catalog_search" |
| `docs/Updates/v5/plans/chunk-7-agent1.md` | added | frozen plan |
| `docs/Updates/v5/v5_<UTC>.md` | added | session log |

## Critical files to read before coding

- `project_v4/backend/internal/usecases/agent1_execute.go:1-422` — reference shape
- `project_v4/backend/internal/tools/tool_catalog_search.go:1-549` — RRF code stays out, the rest of the structure (Phase 0 / state write / metadata return) ports
- `project_v4/backend/internal/tools/tool_state_filter.go:1-165` — full port
- `project_v4/backend/internal/tools/tool_history_lookup.go:1-82` — full port
- `project_v4/backend/internal/prompts/prompt_analyze_query.go:1-114` — full port
- `project_v4/backend/internal/domain/catalog_digest_entity.go:1-98` — full port
- `project_v4/backend/internal/adapters/postgres/postgres_catalog.go:974-1180` — `GenerateCatalogDigest` core; V5 only needs the build path (drop Get/Save)
- `project_v5/backend/internal/ports/catalog_port.go:1-60` — current ProductFilter shape; confirm matching V4 filter keys
- `project_v5/backend/internal/usecases/agent2_execute.go:81-205` — the use-case pattern this chunk mirrors

## Verification

```sh
cd project_v5/backend

# Static + unit
go build ./... && go build -tags=integration ./... && go build -tags="integration live" ./... && go build -tags=tokens ./...
go vet ./... && go vet -tags=integration ./... && go vet -tags="integration live" ./...
go test -count=1 ./...
# new tests pass; chunks 1-6d regression-free

# Token measurement (warns if Agent1 prefix < 4500; not a hard gate)
go test -tags=tokens -count=1 -v ./internal/engine/tokens/...

# Integration on Neon (no LLM): catalog_search keyword path + digest builder
TEST_DATABASE_URL=$DATABASE_URL go test -tags=integration -count=1 ./...

# Live HTTP smoke — full Agent1 → Agent2 pipeline
ANTHROPIC_API_KEY=$KEY TEST_DATABASE_URL=$DB go test -tags="integration live" -v -count=1 \
    ./internal/handlers/... -run TestHTTPLiveSmoke
# expect: turn 1 — Agent1 catalog_search → N products; Agent2 visual_assembly → Document
# expect: turn 2 with "оставь только COSRX" — deterministic guard fires, _internal_state_filter
# expect: spans include agent1.execute, agent1.llm, agent1.tool.catalog_search,
#         postgres.BuildCatalogDigest, agent2.execute, agent2.llm, agent2.tool.visual_assembly
```

## Commit plan — 3 commits

1. `feat(v5): catalog_search + state_filter + history_lookup tools (chunk 7)` — 3 tools + tests + ProductFilter ruble↔kopeck conversion + tool-registry wiring in main.go.
2. `feat(v5): Agent1 prompt + catalog digest + Agent1Execute use case (chunk 7)` — prompts/agent1_prompt.go, domain/catalog_digest.go, postgres_catalog BuildCatalogDigest, agent1_prompt_cache.go, agent1_execute.go (with deterministic guard) + tests; Agent1 wiring in main.go.
3. `feat(v5): pipeline orchestrator + microcontext + handler rewire (chunk 7)` — pipeline_execute.go, agent2 microcontext plumbing, handler_pipeline.go rewire, live HTTP test extended, known-gaps + session log.

Then `git push origin v5`.

## Out of scope (explicit deferral)

- **EmbeddingPort + pgvector + RRF** — V5 catalog_search is keyword-only this chunk. The `vector_query` parameter stays in the schema (LLM behavior unchanged) but the executor does keyword fallback. Future chunk wires OpenAI embeddings + pgvector + RRF merge.
- **Services** — V5 catalog has products only. `entity_type=service` is dropped from the schema. When V5 grows services, re-add the branch (~50 lines).
- **Catalog digest persistence** — V4 stores digest in `tenants.catalog_digest` and refreshes it via a background job. V5 builds on demand and caches in-process forever. If digest staleness becomes a real product issue, add `/api/v1/admin/invalidate-cache?tenant=X`.
- **Agent1 prompt-cache 4500-token gate** — measured + logged as a gap, not enforced. Decide whether to grow once we have real cost data from production traffic.
- **`<tenant_design_context>` block for Agent2** — still deferred (chunk 6 carryover); blocked on canvas-microservice handshake.
