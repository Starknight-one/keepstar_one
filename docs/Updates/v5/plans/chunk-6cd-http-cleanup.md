# Chunks 6c + 6d — HTTP server + ops applier + cleanup

## Context

Chunks 6a + 6b shipped: SDK adapter, tool registry, visual_assembly (preset+replicate end-to-end), Agent2 prompt-builder (cache gate cleared, V5 cacheable prefix 4540 ≥ 4500), Agent2Execute use case, live smoke test green ($0.002/2-turns vs V4 $0.0021).

What's still missing before V5 can be hit by a real client:

- **HTTP layer** — `cmd/server/main.go` is a stub. There's no way to call Agent2 over HTTP, no session/init endpoint, no DI wiring for migrations.
- **Ops applier** — `visual_assembly` declares `ops` in its schema and the prompt teaches the LLM how to use them, but the tool currently logs+ignores. With this gone, LLM can do preset+replicate but can't tweak (color, format, layout, delete a child, insert a CTA). Not blocking minimum-viable production but blocking real interactivity.
- **Transaction-level race in state writes** — `zoneWriteWithDelta` does the zone UPDATE and `AddDelta` as two separate `pool.Exec` calls; if `AddDelta` fails, state and audit log diverge. `AddDelta` itself has a TOCTOU race on `MAX(step)` under concurrent writers per session — the unique constraint protects integrity but the caller doesn't retry.
- **Tracer port** — V4 has `domain.SpanFromContext` + `SpanCollector` in-context for the `/debug/traces` waterfall. V5 PG adapters don't carry it; the feature itself isn't critical for MVP but tracing the LLM call chain is useful.

This combined chunk closes all four. **6c lands the HTTP layer + ops applier; 6d lands the cleanup.** Each ships independently and pushes the v5 branch when done — if 6d blows up, 6c is already on origin.

After this combined chunk: a `curl POST localhost:8082/api/v1/pipeline` against running V5 returns a Document JSON, and the engine state writes are tx-safe under concurrent load.

---

## Chunk 6c — HTTP + ops applier

### What lands

1. Config loader from env vars
2. `cmd/server/main.go` real entrypoint with full DI + migrations + graceful shutdown
3. Three HTTP endpoints — `/api/v1/session/init`, `/api/v1/session/{id}`, `/api/v1/pipeline`
4. Three middleware — logging (request_id + slog), CORS (open for chat widget), tenant (X-Tenant-Slug header → context)
5. Ops applier — translates JSON op array from `visual_assembly.input.ops` into `engine.Command` instances and executes via `CommandHistory`
6. visual_assembly tool wired to call the applier (the FIXME from 6b)
7. HTTP integration test (build tag `integration`) hitting a running test server

`<tenant_design_context>` block — **still deferred** to a later chunk. Hardcoded preset catalog in the prompt works for MVP; adding the dynamic block now would require designing cache-invalidation hooks (V4 pattern: tenant version snapshot bumps on canvas write, prompt cache invalidates) and that's a separate pile of work. Document as known gap; revisit when canvas-microservice ships.

### Files to add / modify

| File | Status | Purpose |
|---|---|---|
| `project_v5/backend/internal/config/config.go` | added | `Config` struct + `Load()` reading PORT, DATABASE_URL, ANTHROPIC_API_KEY, LLM_MODEL, TENANT_SLUG, LOG_LEVEL |
| `project_v5/backend/internal/handlers/middleware_logging.go` | added | request_id + structured slog + write to stdout |
| `project_v5/backend/internal/handlers/middleware_cors.go` | added | open CORS for the chat widget — Allow-Origin "*", Allow-Methods POST/OPTIONS, Allow-Headers content-type + X-Tenant-Slug |
| `project_v5/backend/internal/handlers/middleware_tenant.go` | added | reads `X-Tenant-Slug` header, falls back to TENANT_SLUG env, validates via CatalogPort.GetTenantBySlug, stores resolved tenant in context |
| `project_v5/backend/internal/handlers/handler_session.go` | added | POST /api/v1/session/init: insert v5_chat_sessions row + statePort.CreateState; returns `{sessionId, tenant{slug, name}}`. GET /api/v1/session/{id}: read state, return shape for debugging |
| `project_v5/backend/internal/handlers/handler_pipeline.go` | added | POST /api/v1/pipeline: parse `{sessionId, query}` → call Agent2Execute (with tenant from context) → return `{document, toolCalls, usage, latencyMs}` |
| `project_v5/backend/internal/handlers/routes.go` | added | net/http.ServeMux setup + middleware composition |
| `project_v5/backend/internal/handlers/handler_pipeline_integration_test.go` | added | spins up the server in-process via `httptest.NewServer`, hits `/api/v1/pipeline` with a real-tenant query against live Neon + Haiku |
| `project_v5/backend/cmd/server/main.go` | rewritten | real DI: config → logger → DB → run all migrations → adapters → ports → llm client → registry → use cases → handlers → http.Server with graceful shutdown |
| `project_v5/backend/internal/engine/apply_ops.go` | added | `ApplyOps(doc, ops)` translates each op to `engine.Command` and executes via a fresh `CommandHistory`. Maintains a `$ref → node_id` map across ops in one batch (matches V4's pattern) |
| `project_v5/backend/internal/engine/apply_ops_test.go` | added | unit tests covering each op (insert / update / delete / move / override) + chained `$ref` resolution |
| `project_v5/backend/internal/tools/tool_visual_assembly.go` | modified | replace the FIXME no-op with a call to `engine.ApplyOps(doc, ops)`. Pipeline order: Materialise → ApplyOps (NEW) → ExpandReplicates → ResolveAndInline → BindData |
| `docs/Updates/v5/plans/chunk-6cd-http-cleanup.md` | added | frozen plan |
| `docs/Updates/v5/v5_<UTC>.md` | added | session log |

### Ops applier — concrete contract

```go
// ApplyOps applies a list of ops (parsed from visual_assembly.input.ops)
// to doc, in order. Returns the first error encountered; subsequent ops
// are NOT applied (V4 fail-fast pattern). The $ref → node id map is
// scoped to one ApplyOps call and discarded after — refs don't survive
// across separate tool calls (V5's run-binding cache, deferred to later
// chunks, will lift this).
//
// Each op object shape: {op, target?, parent?, ref?, after?, props?}
//   - op:     "insert" | "update" | "delete" | "move" | "override"
//   - target: node id to update / delete / move / override-target
//   - parent: parent id (or "$ref-name") for insert / move
//   - ref:    local binding name; subsequent ops can reference via "$ref"
//   - props:  property bag for insert / update / override
//   - after:  sibling id to insert/move after (chunk 6d if needed)
func ApplyOps(doc *Document, ops []map[string]any) error
```

`$ref` resolution: when an op carries `"ref": "cta"`, the applier records `cta → <created node id>` after a successful Insert. Later ops' `parent: "$cta"` are rewritten to the actual id.

Override: V4's override didn't exist. v9 has SetOverrideCommand (descendant override) and SetRootOverrideCommand (root override). For 6c, support `"op": "override"` with `target` = ref node id and `props` = `{descendantId: {key: value}}`. Rare; document as advanced use.

### main.go DI shape

```go
func main() {
  ctx, cancel := signalContext()
  defer cancel()

  cfg := config.MustLoad()
  log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))

  client, err := postgres.NewClient(ctx, cfg.DatabaseURL); fatal(err)
  defer client.Close()

  // Run all V5 migrations on startup.
  fatal(client.RunStateMigrations(ctx))
  fatal(client.RunPresetMigrations(ctx))
  fatal(client.RunComponentMigrations(ctx))

  // Adapters → ports.
  catalogPort   := postgres.NewCatalogAdapter(client)
  statePort     := postgres.NewStateAdapter(client, log)
  presetPort    := postgres.NewPresetAdapter(client)
  componentPort := postgres.NewComponentAdapter(client)
  fdPort        := postgres.NewFieldDefinitionAdapter(client)

  // LLM.
  llm := anthropic.NewClient(cfg.AnthropicAPIKey, cfg.LLMModel)

  // Tools + use cases.
  registry := tools.NewRegistry()
  registry.Register(tools.NewVisualAssemblyTool(statePort, presetPort, componentPort))
  promptCache := usecases.NewPromptCache(fdPort, "product")
  agent2 := usecases.NewAgent2Execute(llm, statePort, registry, promptCache)

  // Handlers.
  sessionH  := handlers.NewSessionHandler(statePort, catalogPort)
  pipelineH := handlers.NewPipelineHandler(agent2)

  mux := handlers.RegisterRoutes(sessionH, pipelineH)
  chain := middleware.WithLogging(log)(middleware.WithCORS(middleware.WithTenant(catalogPort, cfg.TenantSlug)(mux)))

  srv := &http.Server{Addr: ":" + cfg.Port, Handler: chain, ReadTimeout: 15 * time.Second, WriteTimeout: 120 * time.Second}
  go gracefulShutdown(ctx, srv, log)
  log.Info("V5 listening", "port", cfg.Port)
  fatal(srv.ListenAndServe())
}
```

### HTTP endpoints — concrete shapes

**POST /api/v1/session/init**
- Headers: `X-Tenant-Slug: <slug>` (or fall back to TENANT_SLUG env)
- Request body: empty
- Response: `{ "sessionId": "uuid", "tenant": { "slug": "...", "name": "..." } }`
- Side effect: inserts v5_chat_sessions row + statePort.CreateState

**GET /api/v1/session/{id}**
- Returns: `{ "sessionId": "...", "step": N, "current": {...}, "view": {...} }`
- For debugging / restoration

**POST /api/v1/pipeline**
- Headers: `X-Tenant-Slug: <slug>`
- Request body: `{ "sessionId": "...", "query": "..." }`
- Response: `{ "document": {...}, "toolCalls": [...], "usage": {...}, "latencyMs": N }`
- Internally: agent2.Execute(ctx, Agent2ExecuteRequest{SessionID, TenantSlug, UserQuery: query})

### Verification

```sh
cd project_v5/backend

# Build + vet
go build ./... && go build -tags=integration ./... && go build -tags="integration live" ./...
go vet ./... && go vet -tags=integration ./... && go vet -tags="integration live" ./...
go test -count=1 ./...

# Integration on Neon (no LLM)
TEST_DATABASE_URL=$DATABASE_URL go test -tags=integration -count=1 ./...

# Live HTTP smoke
ANTHROPIC_API_KEY=$KEY DATABASE_URL=$DB go run ./cmd/server &
SERVER_PID=$!
sleep 1
SESSION=$(curl -sX POST localhost:8082/api/v1/session/init \
  -H "X-Tenant-Slug: hey-babes-cosmetics" | jq -r .sessionId)
curl -sX POST localhost:8082/api/v1/pipeline \
  -H "X-Tenant-Slug: hey-babes-cosmetics" \
  -H "Content-Type: application/json" \
  -d "{\"sessionId\":\"$SESSION\",\"query\":\"Show me 3 products\"}" | jq .
kill $SERVER_PID
```

### Commit plan (6c) — 4 commits

1. `feat(v5): config + middleware + handlers + DI in main.go (chunk 6c)` — config/, handlers/{logging,cors,tenant}/, handler_session, handler_pipeline, routes, cmd/server/main.go
2. `feat(v5): ops applier — JSON ops → engine.Command (chunk 6c)` — engine/apply_ops.go + apply_ops_test.go
3. `feat(v5): wire ops applier into visual_assembly tool (chunk 6c)` — modify tool_visual_assembly.go, drop the FIXME
4. `chore(v5): live HTTP integration test against Neon + Haiku (chunk 6c)` — handler_pipeline_integration_test.go

Then `git push origin v5`.

---

## Chunk 6d — cleanup

### What lands

1. **`zoneWriteWithDelta` transaction wrap** — zone UPDATE + AddDelta in one `pgx.Tx`; on error, rollback. State and audit log can no longer diverge.
2. **`AddDelta` retry** — caller-side retry-with-backoff on the UNIQUE(session_id, step) constraint violation. Up to 3 attempts; each backoff 10-50ms.
3. **Tracer port** — `domain.SpanCollector` interface + `domain.SpanFromContext(ctx)` helper; PG adapter methods + Agent2Execute + tool execution emit named spans. Spans collected in-memory, returned in PipelineResponse for debugging (no `/debug/traces` UI yet — that's a later chunk).
4. **Re-add span calls** to the chunk-2 PG state methods that originally had them (annotated with NOTE comments back then).

LLMMessage cache_control hint plumbing — already covered by chunk 6a's adapter; nothing to do.

### Files to add / modify

| File | Status | Purpose |
|---|---|---|
| `project_v5/backend/internal/adapters/postgres/postgres_state.go` | modified | `zoneWriteWithDelta` wraps zone UPDATE + AddDelta in `client.pool.BeginTx(ctx, ...)`; rollback on error. `AddDelta` adds 3-attempt retry with 10-50ms backoff on unique constraint violation |
| `project_v5/backend/internal/adapters/postgres/postgres_state_test.go` | added | unit + integration tests covering tx semantics + retry behavior; uses fake `pgx.Rows` for unit, live Neon for integration |
| `project_v5/backend/internal/domain/span.go` | added | `Span` struct + `SpanCollector` interface (Start, Spans, Anchor) + `WithSpanCollector(ctx, sc)`/`SpanFromContext(ctx)` context helpers — V4-aligned |
| `project_v5/backend/internal/adapters/postgres/postgres_state.go` | modified | re-add `domain.SpanFromContext(ctx)` calls on `GetState`, `AddDelta`, `UpdateData`, `UpdateTemplate` (annotated as deferred in chunk 2's NOTE comments) |
| `project_v5/backend/internal/adapters/postgres/postgres_catalog.go` | modified | add spans on `ListProducts`, `GetProduct`, `GetTenantBySlug` |
| `project_v5/backend/internal/adapters/postgres/postgres_preset.go` | modified | add spans on `GetPublishedPreset`, `ListPublishedPresets` |
| `project_v5/backend/internal/adapters/postgres/postgres_component.go` | modified | add spans on `GetPublishedComponent`, `ListPublishedComponents` |
| `project_v5/backend/internal/usecases/agent2_execute.go` | modified | wrap stages (`agent2`, `agent2.prompt`, `agent2.llm`, `agent2.tool`) in named spans |
| `project_v5/backend/internal/handlers/middleware_logging.go` | modified | attach a fresh `SpanCollector` to context for each request, log final span list at request end |
| `docs/v5-known-gaps.md` | modified | close the `zoneWriteWithDelta tx` row, close the `AddDelta race` row, close the `Span tracing` row |

### Tx wrap — concrete shape

```go
func (a *StateAdapter) zoneWriteWithDelta(
    ctx context.Context, sessionID string,
    zoneSQL string, zoneArgs []any,
    info domain.DeltaInfo,
) (int, error) {
    tx, err := a.client.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
    if err != nil { return 0, err }
    defer tx.Rollback(ctx) // no-op after Commit
    if _, err := tx.Exec(ctx, zoneSQL, zoneArgs...); err != nil {
        return 0, fmt.Errorf("zone update: %w", err)
    }
    step, err := a.addDeltaTx(ctx, tx, sessionID, info.ToDelta())
    if err != nil { return 0, err }
    if err := tx.Commit(ctx); err != nil { return 0, err }
    return step, nil
}
```

`addDeltaTx` is a tx-aware variant of AddDelta. Public AddDelta keeps the bare `pool.Exec` path with retry — used by callers outside zoneWriteWithDelta (rare; today only `state_rollback.go`).

### AddDelta retry — concrete shape

```go
func (a *StateAdapter) AddDelta(ctx context.Context, sessionID string, delta *domain.Delta) (int, error) {
    const maxAttempts = 3
    var lastErr error
    for attempt := 0; attempt < maxAttempts; attempt++ {
        step, err := a.addDeltaOnce(ctx, sessionID, delta)
        if err == nil { return step, nil }
        if !isUniqueViolation(err) { return 0, err } // not a race; bail
        lastErr = err
        time.Sleep(time.Duration(10+rand.Intn(40)) * time.Millisecond)
    }
    return 0, fmt.Errorf("AddDelta exhausted retries: %w", lastErr)
}
```

`isUniqueViolation` matches PG SQLSTATE 23505. `addDeltaOnce` is the existing CTE INSERT.

### Span port — concrete shape

```go
// domain/span.go
type Span struct {
    Name      string
    StartMs   int64
    EndMs     int64
    Detail    string
}
type SpanCollector struct {
    mu     sync.Mutex
    anchor time.Time
    spans  []Span
}
func NewSpanCollector() *SpanCollector { ... }
func (sc *SpanCollector) Start(name string) func(detail ...string) { ... } // returns end-fn
func (sc *SpanCollector) Spans() []Span { ... } // sorted by StartMs

type ctxKey struct{}
func WithSpanCollector(ctx context.Context, sc *SpanCollector) context.Context { ... }
func SpanFromContext(ctx context.Context) *SpanCollector { ... } // nil-safe
```

Adapters that emit spans use the nil-safe pattern:
```go
if sc := domain.SpanFromContext(ctx); sc != nil {
    end := sc.Start("postgres.GetState")
    defer end()
}
```

### Verification

```sh
go build ./... && go vet ./...
go test -count=1 ./...
TEST_DATABASE_URL=$DATABASE_URL go test -tags=integration -count=1 ./...
# all green; new tx + retry tests pass; existing 14+ integration green

# Live HTTP smoke (re-run from 6c) — should now report span list in pipeline response
curl -sX POST localhost:8082/api/v1/pipeline \
  -H "X-Tenant-Slug: hey-babes-cosmetics" \
  -H "Content-Type: application/json" \
  -d '{"sessionId":"...","query":"Show 3 products"}' | jq .spans
# spans: [{name: "agent2", ...}, {name: "agent2.prompt", ...}, {name: "agent2.llm", ...}, {name: "postgres.GetState", ...}, ...]
```

### Commit plan (6d) — 3 commits

1. `fix(v5): wrap zoneWriteWithDelta in tx + AddDelta retry on unique-violation (chunk 6d)` — postgres_state.go + tests
2. `feat(v5): tracer port + SpanCollector + spans on adapters/usecases (chunk 6d)` — domain/span.go + 5 adapter files + agent2_execute.go + middleware_logging.go
3. `docs(v5): chunk 6c+6d session log + frozen plan + close known-gaps`

Then `git push origin v5`.

---

## Combined verification

```sh
cd project_v5/backend
go build ./... && go build -tags=integration ./... && go build -tags="integration live" ./...
go vet ./... && go vet -tags=integration ./...
go test -count=1 ./...

TEST_DATABASE_URL=$DATABASE_URL go test -tags=integration -count=1 ./...

# Full live: server + 2-turn HTTP conversation
ANTHROPIC_API_KEY=$KEY DATABASE_URL=$DB go run ./cmd/server &
PID=$!; sleep 2
SESSION=$(curl -sX POST localhost:8082/api/v1/session/init -H "X-Tenant-Slug: hey-babes-cosmetics" | jq -r .sessionId)
curl -sX POST localhost:8082/api/v1/pipeline \
  -H "X-Tenant-Slug: hey-babes-cosmetics" \
  -d "{\"sessionId\":\"$SESSION\",\"query\":\"Show 3 products\"}" | jq '{toolCalls, usage, spans: .spans|length}'
curl -sX POST localhost:8082/api/v1/pipeline \
  -H "X-Tenant-Slug: hey-babes-cosmetics" \
  -d "{\"sessionId\":\"$SESSION\",\"query\":\"Now show 5\"}" | jq '{usage}'
# turn 2 should show non-zero cache_read
kill $PID
```

---

## Risk + time

- **6c**: ~3-4 hours focused work. Risk: middleware ordering, tenant resolution edge cases. Mitigated by mirroring V4's middleware chain.
- **6d**: ~2 hours. Risk: tx semantics under concurrent writes (test by simulating). Tracer port is mostly mechanical re-add.
- Combined session: ~5-6 hours of clock time, ≈1-2 Vlad-time hours of review + commit.
- If 6d blows up, 6c is on origin and the user can already hit V5 over HTTP.

---

## Out of scope (after 6c+6d)

- `<tenant_design_context>` block — still deferred. Hardcoded preset catalog in prompt works for MVP.
- `/debug/traces` UI — V4 has it; V5 collects spans but doesn't expose a waterfall page yet. Add when it becomes useful.
- Agent1 (catalog_search / state_filter) — V5 production deployment can ship without it; clients call `/api/v1/pipeline` directly with `state.Current.Data` populated upstream (or via a separate seed endpoint).
- Frontend renderer port — separate work, not part of chunk 6.
