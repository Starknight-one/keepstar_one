# Feature: Two-Agent Pipeline State + Storage (Phase 1)

## Feature Description

Implement foundational State and Storage layer for the Two-Agent Pipeline system. This includes:
- PostgreSQL tables for chat session state and deltas
- Go domain structures for State and Delta
- CRUD operations through a new StatePort interface
- PostgreSQL adapter implementing StatePort

This is Phase 1 from SPEC_TWO_AGENT_PIPELINE.md - the foundation that all other phases build upon.

## Objective

Enable persistent storage and retrieval of:
1. Chat session state (current materialized state with data, meta, template)
2. Delta history (action log for replay and rollback)
3. Support for future features: rollbacks, checkpoints, incremental updates

## Expertise Context

Expertise used:
- **backend**: Hexagonal architecture patterns, existing postgres adapter structure, migration patterns, port/adapter conventions

Key insights from expertise:
- Migrations use `CREATE TABLE IF NOT EXISTS` pattern in string constants
- Adapters implement ports with `New<Name>Adapter(client *Client)` constructor
- JSONB used for flexible metadata storage
- Foreign keys with `ON DELETE CASCADE` for cleanup
- Existing tables: `chat_sessions`, `chat_messages` in public schema

## Relevant Files

### Existing Files
- `project/backend/internal/adapters/postgres/postgres_client.go` - PostgreSQL client with connection pool
- `project/backend/internal/adapters/postgres/migrations.go` - Chat migration patterns
- `project/backend/internal/adapters/postgres/catalog_migrations.go` - Catalog migration patterns (schema example)
- `project/backend/internal/domain/session_entity.go` - Existing session domain entity
- `project/backend/internal/ports/cache_port.go` - Port interface example
- `project/backend/cmd/server/main.go` - Initialization flow

### New Files
- `project/backend/internal/domain/state_entity.go` - State, Delta, Meta, Action domain types
- `project/backend/internal/ports/state_port.go` - StatePort interface
- `project/backend/internal/adapters/postgres/state_migrations.go` - State tables migrations
- `project/backend/internal/adapters/postgres/postgres_state.go` - StatePort implementation

## Step by Step Tasks

IMPORTANT: Execute strictly in order.

### 1. Create Domain Entities

File: `project/backend/internal/domain/state_entity.go`

Define Go structs matching SPEC_TWO_AGENT_PIPELINE.md:

```go
package domain

import "time"

// TriggerType represents what initiated the delta
type TriggerType string

const (
    TriggerUserQuery    TriggerType = "USER_QUERY"
    TriggerWidgetAction TriggerType = "WIDGET_ACTION"
    TriggerSystem       TriggerType = "SYSTEM"
)

// ActionType represents the type of action performed
type ActionType string

const (
    ActionSearch   ActionType = "SEARCH"
    ActionFilter   ActionType = "FILTER"
    ActionSort     ActionType = "SORT"
    ActionLayout   ActionType = "LAYOUT"
    ActionRollback ActionType = "ROLLBACK"
)

// Action represents what happened in a delta
type Action struct {
    Type   ActionType             `json:"type"`
    Tool   string                 `json:"tool,omitempty"`
    Params map[string]interface{} `json:"params,omitempty"`
}

// ResultMeta contains metadata about the result (not raw data)
type ResultMeta struct {
    Count   int               `json:"count"`
    Fields  []string          `json:"fields"`
    Aliases map[string]string `json:"aliases,omitempty"`
}

// Delta represents a single change in state history
type Delta struct {
    Step      int                    `json:"step"`
    Trigger   TriggerType            `json:"trigger"`
    Action    Action                 `json:"action"`
    Result    ResultMeta             `json:"result"`
    Template  map[string]interface{} `json:"template,omitempty"`
    CreatedAt time.Time              `json:"created_at"`
}

// StateMeta contains metadata for Agent 2
type StateMeta struct {
    Count   int               `json:"count"`
    Fields  []string          `json:"fields"`
    Aliases map[string]string `json:"aliases,omitempty"`
}

// StateData contains raw data (products, etc.)
type StateData struct {
    Products []Product `json:"products,omitempty"`
}

// StateCurrent represents the materialized current state
type StateCurrent struct {
    Data     StateData              `json:"data"`
    Meta     StateMeta              `json:"meta"`
    Template map[string]interface{} `json:"template,omitempty"`
}

// SessionState represents the full state for a chat session
type SessionState struct {
    ID        string       `json:"id"`
    SessionID string       `json:"session_id"`
    Current   StateCurrent `json:"current"`
    Step      int          `json:"step"` // Current step number
    CreatedAt time.Time    `json:"created_at"`
    UpdatedAt time.Time    `json:"updated_at"`
}
```

### 2. Create StatePort Interface

File: `project/backend/internal/ports/state_port.go`

```go
package ports

import (
    "context"
    "keepstar/internal/domain"
)

// StatePort defines operations for session state persistence
type StatePort interface {
    // CreateState creates a new state for a session
    CreateState(ctx context.Context, sessionID string) (*domain.SessionState, error)

    // GetState retrieves the current state for a session
    GetState(ctx context.Context, sessionID string) (*domain.SessionState, error)

    // UpdateState updates the current materialized state
    UpdateState(ctx context.Context, state *domain.SessionState) error

    // AddDelta appends a new delta to the session history
    AddDelta(ctx context.Context, sessionID string, delta *domain.Delta) error

    // GetDeltas retrieves all deltas for a session (for replay)
    GetDeltas(ctx context.Context, sessionID string) ([]domain.Delta, error)

    // GetDeltasSince retrieves deltas from a specific step (for partial replay)
    GetDeltasSince(ctx context.Context, sessionID string, fromStep int) ([]domain.Delta, error)
}
```

### 3. Create State Migrations

File: `project/backend/internal/adapters/postgres/state_migrations.go`

```go
package postgres

import (
    "context"
    "fmt"
)

const migrationChatSessionState = `
CREATE TABLE IF NOT EXISTS chat_session_state (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
    current_data JSONB DEFAULT '{}',
    current_meta JSONB DEFAULT '{}',
    current_template JSONB,
    step INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(session_id)
);
`

const migrationChatSessionDeltas = `
CREATE TABLE IF NOT EXISTS chat_session_deltas (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
    step INTEGER NOT NULL,
    trigger VARCHAR(20) NOT NULL,
    action JSONB NOT NULL,
    result JSONB NOT NULL,
    template JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(session_id, step)
);
`

const migrationStateIndexes = `
CREATE INDEX IF NOT EXISTS idx_chat_session_state_session_id
    ON chat_session_state(session_id);
CREATE INDEX IF NOT EXISTS idx_chat_session_deltas_session_id
    ON chat_session_deltas(session_id);
CREATE INDEX IF NOT EXISTS idx_chat_session_deltas_session_step
    ON chat_session_deltas(session_id, step);
`

// RunStateMigrations executes state-related migrations
func (c *Client) RunStateMigrations(ctx context.Context) error {
    migrations := []string{
        migrationChatSessionState,
        migrationChatSessionDeltas,
        migrationStateIndexes,
    }

    for i, migration := range migrations {
        if _, err := c.pool.Exec(ctx, migration); err != nil {
            return fmt.Errorf("state migration %d failed: %w", i+1, err)
        }
    }

    return nil
}
```

### 4. Create PostgreSQL State Adapter

File: `project/backend/internal/adapters/postgres/postgres_state.go`

```go
package postgres

import (
    "context"
    "encoding/json"
    "fmt"
    "time"

    "github.com/jackc/pgx/v5"
    "keepstar/internal/domain"
)

// StateAdapter implements ports.StatePort
type StateAdapter struct {
    client *Client
}

// NewStateAdapter creates a new StateAdapter
func NewStateAdapter(client *Client) *StateAdapter {
    return &StateAdapter{client: client}
}

// CreateState creates a new state for a session
func (a *StateAdapter) CreateState(ctx context.Context, sessionID string) (*domain.SessionState, error) {
    state := &domain.SessionState{
        SessionID: sessionID,
        Current: domain.StateCurrent{
            Data: domain.StateData{},
            Meta: domain.StateMeta{
                Count:   0,
                Fields:  []string{},
                Aliases: make(map[string]string),
            },
        },
        Step:      0,
        CreatedAt: time.Now(),
        UpdatedAt: time.Now(),
    }

    dataJSON, _ := json.Marshal(state.Current.Data)
    metaJSON, _ := json.Marshal(state.Current.Meta)

    err := a.client.pool.QueryRow(ctx, `
        INSERT INTO chat_session_state (session_id, current_data, current_meta, step)
        VALUES ($1, $2, $3, $4)
        RETURNING id, created_at, updated_at
    `, sessionID, dataJSON, metaJSON, state.Step).Scan(
        &state.ID, &state.CreatedAt, &state.UpdatedAt,
    )
    if err != nil {
        return nil, fmt.Errorf("create state: %w", err)
    }

    return state, nil
}

// GetState retrieves the current state for a session
func (a *StateAdapter) GetState(ctx context.Context, sessionID string) (*domain.SessionState, error) {
    var state domain.SessionState
    var dataJSON, metaJSON, templateJSON []byte

    err := a.client.pool.QueryRow(ctx, `
        SELECT id, session_id, current_data, current_meta, current_template, step, created_at, updated_at
        FROM chat_session_state
        WHERE session_id = $1
    `, sessionID).Scan(
        &state.ID, &state.SessionID, &dataJSON, &metaJSON, &templateJSON,
        &state.Step, &state.CreatedAt, &state.UpdatedAt,
    )
    if err == pgx.ErrNoRows {
        return nil, domain.ErrSessionNotFound
    }
    if err != nil {
        return nil, fmt.Errorf("get state: %w", err)
    }

    if len(dataJSON) > 0 {
        json.Unmarshal(dataJSON, &state.Current.Data)
    }
    if len(metaJSON) > 0 {
        json.Unmarshal(metaJSON, &state.Current.Meta)
    }
    if len(templateJSON) > 0 {
        json.Unmarshal(templateJSON, &state.Current.Template)
    }

    return &state, nil
}

// UpdateState updates the current materialized state
func (a *StateAdapter) UpdateState(ctx context.Context, state *domain.SessionState) error {
    dataJSON, _ := json.Marshal(state.Current.Data)
    metaJSON, _ := json.Marshal(state.Current.Meta)
    templateJSON, _ := json.Marshal(state.Current.Template)

    _, err := a.client.pool.Exec(ctx, `
        UPDATE chat_session_state
        SET current_data = $1, current_meta = $2, current_template = $3,
            step = $4, updated_at = NOW()
        WHERE session_id = $5
    `, dataJSON, metaJSON, templateJSON, state.Step, state.SessionID)
    if err != nil {
        return fmt.Errorf("update state: %w", err)
    }

    return nil
}

// AddDelta appends a new delta to the session history
func (a *StateAdapter) AddDelta(ctx context.Context, sessionID string, delta *domain.Delta) error {
    actionJSON, _ := json.Marshal(delta.Action)
    resultJSON, _ := json.Marshal(delta.Result)
    var templateJSON []byte
    if delta.Template != nil {
        templateJSON, _ = json.Marshal(delta.Template)
    }

    _, err := a.client.pool.Exec(ctx, `
        INSERT INTO chat_session_deltas (session_id, step, trigger, action, result, template)
        VALUES ($1, $2, $3, $4, $5, $6)
    `, sessionID, delta.Step, delta.Trigger, actionJSON, resultJSON, templateJSON)
    if err != nil {
        return fmt.Errorf("add delta: %w", err)
    }

    return nil
}

// GetDeltas retrieves all deltas for a session
func (a *StateAdapter) GetDeltas(ctx context.Context, sessionID string) ([]domain.Delta, error) {
    return a.GetDeltasSince(ctx, sessionID, 0)
}

// GetDeltasSince retrieves deltas from a specific step
func (a *StateAdapter) GetDeltasSince(ctx context.Context, sessionID string, fromStep int) ([]domain.Delta, error) {
    rows, err := a.client.pool.Query(ctx, `
        SELECT step, trigger, action, result, template, created_at
        FROM chat_session_deltas
        WHERE session_id = $1 AND step >= $2
        ORDER BY step ASC
    `, sessionID, fromStep)
    if err != nil {
        return nil, fmt.Errorf("get deltas: %w", err)
    }
    defer rows.Close()

    var deltas []domain.Delta
    for rows.Next() {
        var d domain.Delta
        var actionJSON, resultJSON, templateJSON []byte
        var trigger string

        err := rows.Scan(&d.Step, &trigger, &actionJSON, &resultJSON, &templateJSON, &d.CreatedAt)
        if err != nil {
            return nil, fmt.Errorf("scan delta: %w", err)
        }

        d.Trigger = domain.TriggerType(trigger)
        json.Unmarshal(actionJSON, &d.Action)
        json.Unmarshal(resultJSON, &d.Result)
        if len(templateJSON) > 0 {
            json.Unmarshal(templateJSON, &d.Template)
        }

        deltas = append(deltas, d)
    }

    return deltas, nil
}
```

### 5. Integrate Migrations in main.go

Update `project/backend/cmd/server/main.go`:

Add call to `RunStateMigrations` after existing migrations:

```go
// After RunMigrations and RunCatalogMigrations
if err := pgClient.RunStateMigrations(ctx); err != nil {
    log.Fatalf("Failed to run state migrations: %v", err)
}
```

### 6. Create Integration Test

File: `project/backend/internal/adapters/postgres/postgres_state_test.go`

```go
package postgres_test

import (
    "context"
    "testing"
    "time"

    "keepstar/internal/adapters/postgres"
    "keepstar/internal/domain"
)

func TestStateAdapter_CreateAndGetState(t *testing.T) {
    // Skip if no DATABASE_URL
    ctx := context.Background()

    // This test requires a running database
    // Run with: go test -v ./internal/adapters/postgres/ -run TestStateAdapter

    client, err := postgres.NewClient(ctx, testDatabaseURL())
    if err != nil {
        t.Skip("Database not available")
    }
    defer client.Close()

    adapter := postgres.NewStateAdapter(client)

    // Create state
    state, err := adapter.CreateState(ctx, testSessionID())
    if err != nil {
        t.Fatalf("CreateState failed: %v", err)
    }

    if state.Step != 0 {
        t.Errorf("Expected step 0, got %d", state.Step)
    }

    // Get state
    retrieved, err := adapter.GetState(ctx, state.SessionID)
    if err != nil {
        t.Fatalf("GetState failed: %v", err)
    }

    if retrieved.ID != state.ID {
        t.Errorf("ID mismatch")
    }
}

func TestStateAdapter_AddAndGetDeltas(t *testing.T) {
    ctx := context.Background()

    client, err := postgres.NewClient(ctx, testDatabaseURL())
    if err != nil {
        t.Skip("Database not available")
    }
    defer client.Close()

    adapter := postgres.NewStateAdapter(client)
    sessionID := testSessionID()

    // Create state first
    _, err = adapter.CreateState(ctx, sessionID)
    if err != nil {
        t.Fatalf("CreateState failed: %v", err)
    }

    // Add delta
    delta := &domain.Delta{
        Step:    1,
        Trigger: domain.TriggerUserQuery,
        Action: domain.Action{
            Type:   domain.ActionSearch,
            Tool:   "search_products",
            Params: map[string]interface{}{"query": "ноутбуки"},
        },
        Result: domain.ResultMeta{
            Count:  10,
            Fields: []string{"name", "price", "rating"},
        },
        CreatedAt: time.Now(),
    }

    err = adapter.AddDelta(ctx, sessionID, delta)
    if err != nil {
        t.Fatalf("AddDelta failed: %v", err)
    }

    // Get deltas
    deltas, err := adapter.GetDeltas(ctx, sessionID)
    if err != nil {
        t.Fatalf("GetDeltas failed: %v", err)
    }

    if len(deltas) != 1 {
        t.Errorf("Expected 1 delta, got %d", len(deltas))
    }

    if deltas[0].Action.Tool != "search_products" {
        t.Errorf("Tool mismatch")
    }
}

func testDatabaseURL() string {
    // Return from env or skip
    return "" // Set via DATABASE_URL env
}

func testSessionID() string {
    return "test-session-" + time.Now().Format("20060102150405")
}
```

### 7. Validation

Run validation commands:

```bash
cd project/backend && go build ./...
cd project/backend && go test ./...
```

Verify:
- Migrations execute without errors
- State CRUD operations work
- Deltas are stored and retrieved correctly

## Validation Commands

From ADW/adw.yaml:
- `cd project/backend && go build ./...` (required)
- `cd project/backend && go test ./...` (optional)

## Acceptance Criteria

- [ ] `chat_session_state` table created with correct schema
- [ ] `chat_session_deltas` table created with correct schema
- [ ] Domain types (State, Delta, Action, ResultMeta) defined
- [ ] StatePort interface defined
- [ ] StateAdapter implements StatePort
- [ ] CreateState creates new state for session
- [ ] GetState retrieves existing state
- [ ] UpdateState updates state (data, meta, template, step)
- [ ] AddDelta stores delta with all fields
- [ ] GetDeltas retrieves deltas in order
- [ ] GetDeltasSince supports partial replay
- [ ] Migrations integrated in main.go
- [ ] Backend builds without errors
- [ ] Backend tests pass

## Notes

- Tables use `ON DELETE CASCADE` to clean up when session deleted
- JSONB used for flexible data/meta/template storage
- Step field enables ordered delta replay
- Unique constraint on (session_id, step) prevents duplicate deltas
- This is foundation for Phase 2 (Agent 1 + Tool) which will use StatePort
