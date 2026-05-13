# Chunk 2 — port V4 state + delta-stream into V5

> Resumed after /compact. Chunk 1 (v9 → Go engine port) is committed
> (`5a0a89e` on branch `v5`); detailed plan preserved at
> `docs/Updates/v5/plans/chunk-1-engine-port.md`.

## Context

V5 takes v9's TypeScript scene-graph engine as foundation and ports it to Go.
V4 stays in `project_v4/` as production until V5 is ready. Chunk 1 ported the
engine (~2850 LOC Go in `project_v5/backend/internal/engine/`, 60+ tests pass,
`Node = map[string]any`).

This chunk ports V4's sectional session state + append-only delta-stream
into V5. Vlad's reminder: *"дельта важна для всего ваще"* — delta-stream is
foundational (rollback, reconstruction, multi-actor coordination, trace,
navigation). Don't underweight it.

The state shape is unchanged from V4 except for one field: the `Template`
zone in V4 holds a Formation JSON; in V5 it holds a scene-graph
`engine.Document` (or its serialized JSON form).

## User decisions

- **Q1 — PG tables**: separate `v5_chat_session_state`, `v5_state_deltas`.
  V4 chat keeps its tables, V5 writes its own. No shared row collision risk.
- **Q2 — DATABASE_URL**: same as V4 (single Neon Postgres, single env var).
  V5 just creates new tables next to V4 ones.
- **Q3 — LLMMessage**: port verbatim from V4 (`tool_entity.go` lines 25-31:
  Role + Content + ToolCalls + ToolResult). Cache-control hints stay
  implicit in adapter layer, deferred to chunk 6.

## What we port (V4 sources)

| V4 file | LOC | What it gives us |
|---|---|---|
| `internal/domain/state_entity.go` | 188 | SessionState, StateCurrent, StateData, StateMeta, ViewState, ViewSnapshot, StateActions, CartItem, EntityRef, ViewMode, Delta, DeltaInfo, ActionType, DeltaType, DeltaSource, TriggerType + all string constants |
| `internal/domain/tool_entity.go` (lines 10-31) | ~25 | ToolCall, ToolResult, LLMMessage (used by ConversationHistory + Agent2History) |
| `internal/domain/product_entity.go` | ? | Product (StateData.Products) |
| `internal/domain/service_entity.go` | ? | Service (StateData.Services) |
| `internal/ports/state_port.go` | 63 | StatePort interface — 14 methods |
| `internal/adapters/postgres/postgres_state.go` | 621 | PG adapter — 14 method impls + scanDeltas helper + zoneWriteWithDelta helper |
| `internal/adapters/postgres/state_migrations.go` | 110 | DDL — 8 incremental migrations folded into 1 fresh migration for V5 |
| `internal/usecases/state_reconstruct.go` | 124 | applyDelta + Execute (replay) |
| `internal/usecases/state_rollback.go` | 113 | Execute (rollback to step N + write rollback delta) |

Total to port: ~1.25K LOC, mostly verbatim.

## Key adaptation for V5

`StateCurrent.Template` field type = `map[string]interface{}` keeps the
JSON-shaped storage unchanged. The *content* changes: it now holds an
`engine.Document` serialized to JSON (chunk 1 round-trip-tests this shape).

Chunk 2 stores Template as `map[string]interface{}` (transport shape).
A typed wrapper that marshals/unmarshals to `*engine.Document` is deferred
to chunk 3 when binding kicks in.

## Scope

In:
1. Domain types in `project_v5/backend/internal/domain/`
2. `StatePort` interface in `project_v5/backend/internal/ports/`
3. Postgres adapter + migration in `project_v5/backend/internal/adapters/postgres/`
4. Reconstruct + rollback usecases in `project_v5/backend/internal/usecases/`
5. Tests at every layer

Out:
- Binding layer (slot↔field, `<fields>` block, ProductToMap) — chunk 3
- LLM adapter wiring — chunk 6
- HTTP handlers — chunk 6
- Pipeline usecases (Agent1/Agent2/Pipeline) — chunk 6
- Span tracing helper — re-add when usecases need it (chunk 6)

## Key changes vs V4

1. **Table names** prefixed `v5_` — `v5_chat_sessions`, `v5_chat_session_state`,
   `v5_chat_session_deltas`. We need our own `v5_chat_sessions` (FK source for
   state.session_id) since V4 deletes cascade to V4 sessions only. **Decision**:
   port a minimal `v5_chat_sessions` (id UUID PK, tenant_id, created_at,
   updated_at) — no other columns until chunk 6 needs them.
2. **Single fresh migration** instead of V4's 8 incremental ALTER TABLE
   migrations. Just one `v5_state_001_init.sql` with the final schema baked in.
3. **No `domain.SpanFromContext` calls** in adapter — chunk 1 didn't port the
   tracer. Skip the `if sc := domain.SpanFromContext(ctx); sc != nil { ... }`
   blocks; re-add when Span port comes back in chunk 6.
4. **No `domain.ErrSessionNotFound`** — port it as part of domain (it's a
   sentinel error, ~3 LOC). Same with any other domain.Err* found while porting.
5. **pgx version**: use `github.com/jackc/pgx/v5` (same as V4's `go.mod`).
   Add `github.com/google/uuid` if scanning UUID into string (V4 uses string).

## Steps

### Step 0 — go.mod deps
Add to `project_v5/backend/go.mod`:
- `github.com/jackc/pgx/v5` (pin to V4's exact version — read V4's go.mod)
- `github.com/google/uuid` (only if `pgx` UUID scan needs explicit type)

`go mod tidy` after Step 1 once imports are real.

### Step 1 — Port domain types
Files (roughly verbatim from V4):
- `internal/domain/state.go` — SessionState, StateCurrent, StateData, StateMeta,
  ViewState, ViewSnapshot, StateActions, CartItem, EntityRef, ViewMode +
  EntityType (port from V4's product_entity.go if separate)
- `internal/domain/delta.go` — Delta, DeltaInfo, Action, ResultMeta + all
  TriggerType / DeltaSource / DeltaType / ActionType constants
- `internal/domain/llm_message.go` — ToolCall, ToolResult, LLMMessage
- `internal/domain/product.go` — Product (verbatim from V4 `product_entity.go`)
- `internal/domain/service.go` — Service (verbatim from V4 `service_entity.go`)
- `internal/domain/errors.go` — ErrSessionNotFound + any other ports
- Tests: JSON round-trip for SessionState (with non-trivial Current.Template
  scene-graph), Delta JSON round-trip, DeltaInfo.ToDelta() sets CreatedAt

### Step 2 — Port StatePort interface
File: `internal/ports/state_port.go` — 14 methods, identical signatures to V4
but `keepstar_v5/internal/domain` import. No tests (interface only).

### Step 3 — Postgres client + adapter
- `internal/adapters/postgres/client.go` — minimal Client wrapper around
  `*pgxpool.Pool`. Port what V4's `postgres.Client` exposes that StateAdapter
  uses (just `pool` field + Connect helper).
- `internal/adapters/postgres/migrations.go` — runs DDL once
- `internal/adapters/postgres/state_migrations.go` — single SQL file with the
  final V5 schema (no incremental ALTERs). Embedded as string constant.
- `internal/adapters/postgres/postgres_state.go` — port from V4 verbatim
  with: (a) table names → `v5_*`, (b) drop SpanFromContext blocks, (c) update
  imports.
- Tests: skip integration unless `TEST_DATABASE_URL` env var is set; default
  to mock-style tests of the SQL builders + delta scan logic. **At minimum**:
  one test that verifies CreateState → AddDelta → GetDeltasUntil returns
  deltas in order. Mark integration test with build tag `//go:build integration`.

### Step 4 — Reconstruct + rollback usecases
- `internal/usecases/state_reconstruct.go` — port verbatim. The applyDelta
  switch is V4's; we keep the same simplified semantics for now.
- `internal/usecases/state_rollback.go` — port verbatim.
- Tests:
  - TestApplyDelta — sequence of 5 deltas (add → update → push → pop → add)
    reconstructs a deterministic state with right Step/Meta/Template.
  - TestRollback — given a fake StatePort, rollback to step 2 of 5 produces
    state-at-step-2 + writes a rollback delta + bumps Step.

### Step 5 — Smoke test
Single file `internal/usecases/state_smoke_test.go` (or in postgres adapter
with integration tag) wiring CreateState → UpdateData → UpdateTemplate →
PushView → PopView → AddDelta(rollback) → ReconstructStateUseCase.Execute(toStep=2)
→ verify final state matches expected. Pure in-memory mock StatePort is fine
if no test DB.

### Step 6 — Session log + chunk plan
- Copy `~/.claude/plans/spicy-dreaming-waffle.md` (this file) to
  `docs/Updates/v5/plans/chunk-2-state-delta.md` (frozen plan record).
- Write session log `docs/Updates/v5/v5_<date>_<time>.md` after commit per
  `docs/Updates/v5/README.md` template.

## Files to create

```
project_v5/backend/
├── go.mod                               # +pgx/v5, +google/uuid
├── go.sum
└── internal/
    ├── domain/
    │   ├── state.go                     # ~180 LOC
    │   ├── delta.go                     # ~110 LOC
    │   ├── llm_message.go               # ~25 LOC
    │   ├── product.go                   # ported from V4
    │   ├── service.go                   # ported from V4
    │   ├── errors.go                    # sentinel errors
    │   ├── state_test.go                # round-trip
    │   └── delta_test.go                # round-trip + ToDelta
    ├── ports/
    │   └── state_port.go                # 14 methods, ~63 LOC
    ├── adapters/postgres/
    │   ├── client.go                    # minimal pool wrapper
    │   ├── migrations.go                # runner
    │   ├── state_migrations.go          # DDL: v5_chat_sessions + v5_chat_session_state + v5_chat_session_deltas
    │   ├── postgres_state.go            # ~600 LOC port
    │   └── postgres_state_test.go       # SQL builder + scan tests; integration behind tag
    └── usecases/
        ├── state_reconstruct.go         # ~125 LOC port
        ├── state_rollback.go            # ~115 LOC port
        ├── state_reconstruct_test.go    # 5-step sequence
        ├── state_rollback_test.go       # rollback to step N
        └── state_smoke_test.go          # full lifecycle with mock port
```

Plus:
- `docs/Updates/v5/plans/chunk-2-state-delta.md` (this plan, copied from
  `~/.claude/plans/`)
- `docs/Updates/v5/v5_<date>_<time>.md` (session log on commit)

## SQL — fresh V5 schema

```sql
-- v5_state_001_init.sql

CREATE TABLE IF NOT EXISTS v5_chat_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_v5_chat_sessions_tenant_id
    ON v5_chat_sessions(tenant_id);

CREATE TABLE IF NOT EXISTS v5_chat_session_state (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES v5_chat_sessions(id) ON DELETE CASCADE,
    current_data JSONB DEFAULT '{}',
    current_meta JSONB DEFAULT '{}',
    current_template JSONB,                    -- holds engine.Document JSON
    view_mode VARCHAR(20) DEFAULT 'grid',
    view_focused JSONB,
    view_stack JSONB DEFAULT '[]',
    conversation_history JSONB DEFAULT '[]',
    agent2_history JSONB DEFAULT '[]',
    actions JSONB DEFAULT '{"likedIds":[],"cartItems":[]}',
    step INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(session_id)
);

CREATE INDEX IF NOT EXISTS idx_v5_chat_session_state_session_id
    ON v5_chat_session_state(session_id);

CREATE TABLE IF NOT EXISTS v5_chat_session_deltas (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES v5_chat_sessions(id) ON DELETE CASCADE,
    step INTEGER NOT NULL,
    trigger VARCHAR(20) NOT NULL,
    source VARCHAR(20) DEFAULT 'llm',
    actor_id VARCHAR(50),
    delta_type VARCHAR(20) DEFAULT 'add',
    path VARCHAR(100),
    action JSONB NOT NULL,
    result JSONB NOT NULL,
    template JSONB,
    turn_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(session_id, step)
);

CREATE INDEX IF NOT EXISTS idx_v5_chat_session_deltas_session_id
    ON v5_chat_session_deltas(session_id);
CREATE INDEX IF NOT EXISTS idx_v5_chat_session_deltas_session_step
    ON v5_chat_session_deltas(session_id, step);
CREATE INDEX IF NOT EXISTS idx_v5_chat_session_deltas_source
    ON v5_chat_session_deltas(session_id, source);
```

## Verification

```sh
cd project_v5/backend
go build ./...                       # exit 0
go test ./internal/...               # all unit tests pass
TEST_DATABASE_URL=$DATABASE_URL go test -tags=integration ./internal/adapters/postgres/...
                                     # only when test DB available
```

Acceptance:
- StatePort fully implemented (14 methods)
- 5-step delta sequence reconstructs deterministically (unit test)
- Rollback to step N produces equivalent state to applyDelta over deltas 1..N
- Postgres tx semantics: zone-update + delta-write happen sequentially in
  zoneWriteWithDelta; AddDelta auto-assigns step monotonically
- Migration applies cleanly to fresh DB (manual or integration test)

## Risks

- **No tx wrapper in V4 zoneWriteWithDelta**: V4's `zoneWriteWithDelta` does
  two separate `pool.Exec` calls without `BEGIN`/`COMMIT`. If zone update
  succeeds but AddDelta fails, state and delta-stream diverge. Decision:
  port verbatim (don't fix V4 bugs in chunk 2 — Vlad said *"в4 имеет
  килотонны багов"*; bug-for-bug port preserves behaviour we can compare
  against). Note in known gaps; revisit when LLM adapter wires up.
- **applyDelta is incomplete in V4**: Push/Pop/Rollback/Remove cases are
  stubs. We port the stubs verbatim; reconstruction works for add/update +
  template-zone replays. Improving applyDelta is a chunk-7 task.
- **Span tracing absent in V5**: `domain.SpanFromContext` blocks dropped.
  Re-add when chunk 6 wires tracer port.
- **Tx isolation around AddDelta's MAX(step) race**: V4's CTE `next_step`
  has a TOCTOU between MAX read and INSERT. Concurrent writers in same
  session could pick the same step. UNIQUE(session_id, step) protects
  integrity (one INSERT errors out) but caller must retry. Port verbatim;
  document; revisit if we ever have concurrent zone writes (unlikely
  per-session in chat).

## Out of scope (revisit later)

- TX-wrapping zone+delta writes
- Span tracing port
- Improving applyDelta semantics
- LLMMessage cache-control hints
- HTTP handlers / Agent1 / Agent2 / pipeline orchestration

## Resume protocol after another /compact

1. Read this file (`~/.claude/plans/spicy-dreaming-waffle.md`).
2. Read `docs/v5-engine-plan.md`.
3. Read `docs/Updates/v5/plans/chunk-1-engine-port.md`.
4. Read `project_v5/backend/internal/engine/document.go` to remember the
   shape Template column will store (chunk 3 will type-wrap it).
5. Cross-check user decisions in this plan against any follow-up messages.
6. Continue with whatever step is next per Step 1-6 above.
