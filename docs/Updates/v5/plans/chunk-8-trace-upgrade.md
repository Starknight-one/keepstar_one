# Chunk 8 — Trace upgrade: parent linkage, status, structured attrs

## Context

Chunk 6d shipped a working SpanCollector — every request gets a fresh collector via the logging middleware, hot adapters and use cases call `sc.Start(name)` to record timed spans, the pipeline handler returns the sorted span list in the response. Useful for «что вообще шло за этот turn», but the model is too thin for real diagnostics:

- **No parent/child** — flat list, hierarchy reconstructed by sorting `StartMs ASC, DurationMs DESC`. Heuristic, breaks under parallel goroutines and equal-millisecond starts. You can't tell from the JSON whether `agent1.llm` belongs to `agent1.execute` or to something else.
- **Only one freeform `Detail` string** per span — no way to attach `tokens.input=4110`, `cost_usd=0.012`, `rows=3`, `tenant_id=X`. Most interesting numbers (LLM token usage / cost) live in `pipelineResponse.usage` as a single sum across both agents; you can't see «Agent1 LLM cost $0.003, Agent2 cost $0.009» separately.
- **No status / error** — span succeeded? failed? what's the error? Currently can only tell by looking at logs (separate plumbing) or by seeing duration anomalies.
- **No request/turn correlation** at the span level — `request_id` is stamped only on log lines, not on spans, so when looking at a returned span list you can't easily cross-reference back.

Without these, the spans are good for «is anything firing» but not for «which call added the 800ms» or «which Anthropic call cost the most this turn». User flagged this directly: the trace was «так себе и чего-то явно не хватало».

This chunk extends the model and migrates the hot call sites. Scope is deliberately narrow — pure model + use-case migration, no `/debug/traces` UI page (that's still chunk 12, the actual rendering).

After this chunk: `pipelineResponse.spans[]` looks like a real trace — every span has `id` + `parent_id`, LLM spans carry `tokens.input` / `cache_read` / `cost_usd`, Postgres spans carry `rows` / `query_kind`, errored spans carry `status="error"` + `error="…"`.

## Approach

### Extended `Span` shape

```go
type SpanStatus string

const (
    SpanStatusOK    SpanStatus = "ok"
    SpanStatusError SpanStatus = "error"
)

type Span struct {
    ID         string         `json:"id"`                       // monotonic "s-N" inside collector
    ParentID   string         `json:"parent_id,omitempty"`      // empty for root spans
    Name       string         `json:"name"`
    StartMs    int64          `json:"start_ms"`
    EndMs      int64          `json:"end_ms"`
    DurationMs int64          `json:"duration_ms"`
    Status     SpanStatus     `json:"status,omitempty"`         // "ok" | "error"
    Error      string         `json:"error,omitempty"`          // populated when Status=error
    Attrs      map[string]any `json:"attrs,omitempty"`          // tokens.input, rows, tenant_id, etc.
    Detail     string         `json:"detail,omitempty"`         // legacy; kept for back-compat
}
```

### New ctx-aware API on `SpanCollector`

```go
// StartSpan begins a new span as a child of whatever span is currently
// "active" in ctx (returned ctx propagates this span as the new parent).
// Caller MUST invoke handle.End() — typically via defer — to record
// duration and finalise the span. Status defaults to "ok"; SetError
// flips to "error".
func (sc *SpanCollector) StartSpan(ctx context.Context, name string) (context.Context, *SpanHandle)

type SpanHandle struct { /* ...unexported... */ }

func (h *SpanHandle) SetAttr(key string, value any)
func (h *SpanHandle) SetAttrs(attrs map[string]any)
func (h *SpanHandle) SetError(err error)
func (h *SpanHandle) End()
```

Parent tracking uses a separate context key (`spanParentIDCtxKey{}`) carrying the active span's ID. `StartSpan` reads it, stamps the new span's ParentID, then writes the new span's ID into the returned ctx. Nested `StartSpan` calls automatically pick that up — no caller needs to thread parent IDs manually.

### Backward compatibility

Existing `Start(name) func(detail ...string)` stays — every current call site keeps working. New API is `StartSpan(ctx, name)`. Migration is opt-in:
- **Migrate hot paths now**: `pipeline.execute`, `agent1.execute` / `agent1.llm` / `agent1.tool.*` / `agent1.prompt`, `agent2.execute` / `agent2.prompt` / `agent2.llm` / `agent2.tool.*`, `postgres.GetState` / `UpdateData` / `UpdateTemplate` / `ListProducts` / `BuildCatalogDigest`. These are the spans whose attrs and parent linkage matter most.
- **Leave the rest on legacy Start**: `postgres.GetTenantBySlug`, `GetProduct`, `GetPublishedPreset`, `ListPublishedPresets`, `GetPublishedComponent`, `ListPublishedComponents` — secondary; can migrate piecemeal in future chunks if specific traces are confusing.

### Helper in `usecases`

The current helper:

```go
func startSpan(ctx context.Context, name string) func() { ... }
```

Gets a sibling:

```go
// withSpan is the new ctx-aware helper. Returns updated ctx + handle.
// Nil-safe: when there's no SpanCollector in ctx, returns a no-op handle
// and the original ctx — so callers don't need to special-case the nil
// path. Pattern:
//
//	ctx, span := withSpan(ctx, "agent1.llm")
//	defer span.End()
//	span.SetAttrs(map[string]any{"model": ..., "tokens.input": ...})
//	if err != nil { span.SetError(err) }
func withSpan(ctx context.Context, name string) (context.Context, *domain.SpanHandle)
```

### LLM-span attrs (most valuable)

After the `llm.ChatWithToolsCached` call in Agent1 / Agent2, before the span ends, set:

| key                   | source                              |
|-----------------------|-------------------------------------|
| `model`               | resp.Usage.Model                    |
| `tokens.input`        | resp.Usage.InputTokens              |
| `tokens.output`       | resp.Usage.OutputTokens             |
| `tokens.cache_read`   | resp.Usage.CacheReadInputTokens     |
| `tokens.cache_creation` | resp.Usage.CacheCreationInputTokens |
| `cost_usd`            | resp.Usage.CostUSD                  |
| `tool_calls`          | len(resp.ToolCalls)                 |
| `stop_reason`         | resp.StopReason                     |

This means looking at the trace JSON you can see «agent1.llm: $0.003, 4110 input, 6244 cache_read» vs «agent2.llm: $0.009, 5000 input, 6244 cache_read». Today both numbers are squashed into one Usage struct in pipelineResponse.

### Postgres-span attrs

Selected hot paths only:

| span                       | attrs                                            |
|----------------------------|--------------------------------------------------|
| `postgres.ListProducts`    | `tenant_id`, `rows`, `limit`, `has_search` (bool) |
| `postgres.GetState`        | `session_id`                                     |
| `postgres.UpdateData`      | `session_id`, `step`                             |
| `postgres.UpdateTemplate`  | `session_id`, `step`                             |
| `postgres.BuildCatalogDigest` | `tenant_id`, `total_products`                 |

### Tool-span attrs

`agent1.tool.<name>` / `agent2.tool.<name>`:

| key       | source                          |
|-----------|---------------------------------|
| `is_error`| result.IsError                  |
| `tool_name` | tc.Name (redundant with Name but useful for filters) |

### Pipeline-span attrs

`pipeline.execute` (root):

| key         | source                                           |
|-------------|--------------------------------------------------|
| `request_id`| request_id from ctx (filled in handler)          |
| `agent1_ms` | a1.LatencyMs                                     |
| `agent2_ms` | a2.LatencyMs                                     |
| `microcontext` | composed signal string                        |

### Span ID generation

`s-{N}` where N is a monotonic counter inside SpanCollector (incremented under the existing mutex). Cheap, short, deterministic per request. Not globally unique — but spans are scoped to one request anyway, and the request_id (via `pipeline.execute.attrs.request_id`) gives the global anchor.

## Files to add / modify

| File | Status | Notes |
|---|---|---|
| `internal/domain/span.go` | modified | + Span fields (ID, ParentID, Status, Error, Attrs); + StartSpan / SpanHandle; legacy Start untouched |
| `internal/domain/span_test.go` | modified | + 4 new tests: parent linkage / SetAttr / SetError / nested-no-parent-fallback |
| `internal/usecases/agent2_execute.go` | modified | startSpan → withSpan migration on agent2.execute / .prompt / .llm / .tool.*; LLM attrs |
| `internal/usecases/agent1_execute.go` | modified | startSpan → withSpan migration; LLM attrs; tool-error status; deterministic-guard span attrs |
| `internal/usecases/pipeline_execute.go` | modified | startSpan → withSpan; pipeline.execute attrs (agent1_ms, agent2_ms, microcontext) |
| `internal/adapters/postgres/postgres_state.go` | modified | GetState / UpdateData / UpdateTemplate use new API + attrs (session_id, step) |
| `internal/adapters/postgres/postgres_catalog.go` | modified | ListProducts / BuildCatalogDigest use new API + attrs (tenant_id, rows, total_products) |
| `internal/handlers/handler_pipeline.go` | modified | stamp request_id attr on root pipeline span via `domain.SpanFromContext` after Pipeline.Execute returns |
| `internal/handlers/handler_pipeline_live_test.go` | modified | assert `agent2.llm` attrs has `tokens.input` > 0 + `cost_usd` > 0 + `parent_id` non-empty for nested spans |
| `docs/v5-known-gaps.md` | modified | + entry: "/debug/traces UI deferred to chunk 12 — spans are richer now but no waterfall page yet" |
| `docs/Updates/v5/plans/chunk-8-trace-upgrade.md` | added | frozen plan |
| `docs/Updates/v5/v5_<UTC>.md` | added | session log |

## Critical files to read before coding

- `internal/domain/span.go` — current types + nil-safe pattern (lines 14-108)
- `internal/usecases/agent2_execute.go:90-145, 196-205` — the canonical use case-side span pattern
- `internal/handlers/middleware_logging.go:40-43` — where SpanCollector enters ctx
- `internal/adapters/postgres/postgres_state.go:101-104, 350-353, 371-374` — adapter span pattern

## Verification

```sh
cd project_v5/backend

# Static + unit
go build ./... && go build -tags=integration ./... && go build -tags="integration live" ./... && go build -tags=tokens ./...
go vet ./... && go vet -tags=integration ./... && go vet -tags="integration live" ./...
go test -count=1 ./...
# new span tests pass; chunks 1-7 regression-free

# Integration on Neon (no LLM)
TEST_DATABASE_URL=$DATABASE_URL go test -tags=integration -count=1 ./...
# state + catalog adapter spans now carry attrs; assertions verify

# Live HTTP smoke — verify rich spans
ANTHROPIC_API_KEY=$KEY TEST_DATABASE_URL=$DB go test -tags="integration live" -v -count=1 \
    ./internal/handlers/... -run TestHTTPLiveSmoke
# expect: pipeline.execute root has agent1_ms / agent2_ms / request_id attrs;
#         agent2.llm.attrs has tokens.input + cost_usd > 0;
#         postgres.ListProducts.attrs has tenant_id + rows + has_search;
#         all child spans (agent1.execute, agent1.llm, postgres.GetState) have parent_id set;
#         no span has Status=error in the happy-path run
```

## Commit plan — 3 commits

1. `feat(v5): SpanCollector — parent linkage, status, structured attrs (chunk 8)` — `internal/domain/span.go` + `span_test.go` (model only, no migration; legacy Start untouched)
2. `feat(v5): use cases switch to withSpan + LLM attrs on agent1/agent2 spans (chunk 8)` — agent1_execute / agent2_execute / pipeline_execute migrate; tool-span and LLM-span attrs
3. `feat(v5): postgres adapter attrs + handler request_id stamp + live test assertions (chunk 8)` — postgres_state / postgres_catalog hot paths; handler_pipeline.go stamps request_id; live test extended; known-gaps + session log

Then `git push origin v5`.

## Out of scope (explicit deferral)

- **`/debug/traces` waterfall UI** — still chunk 12. This chunk only enriches the data; rendering it as a useful page is its own work.
- **Migrating every legacy `sc.Start` site** — secondary postgres methods (`GetTenantBySlug`, `GetProduct`, preset / component reads) keep the legacy API. Migrate piecemeal when those traces become useful.
- **OpenTelemetry export** — V5 spans are not exported via OTLP / Jaeger. In-process only, surfaced via pipeline response. Future chunk if production needs distributed tracing across services.
- **Span sampling** — every request collects every span. Fine at current traffic; revisit if p99 latency budget gets tight.
- **Adapter-level retry visibility** — `addDeltaWithRetry` wraps in one span; individual retry attempts aren't separate child spans. Add `attempts: N` attr in the same span if retry is exercised. (Wired into `UpdateData` attr: `retry_attempts`.)
