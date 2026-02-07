# Feature: Delta State Management

## Feature Description
Расширение системы дельт для полноценного управления состоянием с историей изменений, разделением по источнику (user/llm/system), поддержкой отката на любой шаг и навигации back/forward.

**ADW_id**: x7k9m2p

## Objective
- Разделить дельты по источнику: `user` (действия пользователя), `llm` (действия агентов), `system` (автоматические)
- Поддержка отката на любой шаг через replay дельт
- ViewStack для навигации back/forward
- Подготовка к Feature 2 (Drill-down/Expand)

## Expertise Context
Expertise used:
- **backend-domain**: Существующие типы Delta, TriggerType, ActionType, SessionState
- **backend-ports**: StatePort с методами AddDelta, GetDeltas, GetDeltasSince
- **backend-adapters**: PostgreSQL реализация с таблицами chat_session_state, chat_session_deltas
- **backend-usecases**: Agent1/Agent2 создают дельты через StatePort

## Current State Analysis

### Что уже есть:
```go
// domain/state_entity.go
type Delta struct {
    Step      int
    Trigger   TriggerType  // USER_QUERY, WIDGET_ACTION, SYSTEM
    Action    Action
    Result    ResultMeta
    Template  map[string]interface{}
    CreatedAt time.Time
}

// TriggerType уже частично покрывает source
```

### Что нужно добавить:
1. **Source** — более явное разделение: `user`, `llm`, `system`
2. **ActorID** — какой агент или какое действие пользователя
3. **DeltaType** — тип изменения: `add`, `remove`, `update`, `push`, `pop`
4. **Path** — путь к изменяемым данным: `refs.products`, `view.mode`
5. **ViewStack** — стек для навигации back/forward
6. **ReconstructState** — восстановление state на любой шаг

## Relevant Files

### Existing Files (to modify)
| File | Purpose |
|------|---------|
| `project/backend/internal/domain/state_entity.go` | Расширить Delta, добавить ViewStack |
| `project/backend/internal/ports/state_port.go` | Добавить ReconstructStateAt |
| `project/backend/internal/adapters/postgres/postgres_state.go` | Реализовать новые методы |
| `project/backend/internal/adapters/postgres/state_migrations.go` | Миграция для новых полей |

### New Files
| File | Purpose |
|------|---------|
| `project/backend/internal/usecases/state_rollback.go` | RollbackUseCase для отката |
| `project/backend/internal/usecases/state_reconstruct.go` | Логика реконструкции state |

## Step by Step Tasks

### 1. Extend Delta Entity
**File**: `project/backend/internal/domain/state_entity.go`

Add to Delta struct:
```go
// DeltaSource identifies who initiated the change
type DeltaSource string

const (
    SourceUser   DeltaSource = "user"   // User actions (clicks, back, expand)
    SourceLLM    DeltaSource = "llm"    // LLM/Agent actions (search, render)
    SourceSystem DeltaSource = "system" // System actions (cleanup, TTL)
)

// DeltaType identifies the type of state change
type DeltaType string

const (
    DeltaTypeAdd    DeltaType = "add"    // Add data to state
    DeltaTypeRemove DeltaType = "remove" // Remove data from state
    DeltaTypeUpdate DeltaType = "update" // Update existing data
    DeltaTypePush   DeltaType = "push"   // Push to ViewStack
    DeltaTypePop    DeltaType = "pop"    // Pop from ViewStack
)

// Delta - add new fields
type Delta struct {
    Step      int                    `json:"step"`
    Trigger   TriggerType            `json:"trigger"`
    Source    DeltaSource            `json:"source"`     // NEW: user/llm/system
    ActorID   string                 `json:"actor_id"`   // NEW: "agent1", "agent2", "user_click", "user_back"
    DeltaType DeltaType              `json:"delta_type"` // NEW: add/remove/update/push/pop
    Path      string                 `json:"path"`       // NEW: "data.products", "view.mode", "viewStack"
    Action    Action                 `json:"action"`
    Result    ResultMeta             `json:"result"`
    Template  map[string]interface{} `json:"template,omitempty"`
    CreatedAt time.Time              `json:"created_at"`
}
```

### 2. Add ViewStack to SessionState
**File**: `project/backend/internal/domain/state_entity.go`

```go
// ViewMode represents how items are displayed
type ViewMode string

const (
    ViewModeGrid     ViewMode = "grid"
    ViewModeDetail   ViewMode = "detail"
    ViewModeList     ViewMode = "list"
    ViewModeCarousel ViewMode = "carousel"
)

// EntityRef is a reference to a product or service
type EntityRef struct {
    Type EntityType `json:"type"` // product, service
    ID   string     `json:"id"`
}

// ViewSnapshot captures a view state for back navigation
type ViewSnapshot struct {
    Mode      ViewMode    `json:"mode"`
    Focused   *EntityRef  `json:"focused,omitempty"` // Expanded item (if detail mode)
    Refs      []EntityRef `json:"refs"`              // What was shown
    Step      int         `json:"step"`              // Delta step when captured
    CreatedAt time.Time   `json:"created_at"`
}

// Add to SessionState:
type SessionState struct {
    ID        string         `json:"id"`
    SessionID string         `json:"session_id"`
    Current   StateCurrent   `json:"current"`
    View      ViewState      `json:"view"`       // NEW
    ViewStack []ViewSnapshot `json:"view_stack"` // NEW
    Step      int            `json:"step"`
    CreatedAt time.Time      `json:"created_at"`
    UpdatedAt time.Time      `json:"updated_at"`
}

// ViewState represents current view configuration
type ViewState struct {
    Mode    ViewMode   `json:"mode"`
    Focused *EntityRef `json:"focused,omitempty"`
}
```

### 3. Extend StatePort Interface
**File**: `project/backend/internal/ports/state_port.go`

```go
type StatePort interface {
    // Existing methods...
    CreateState(ctx context.Context, sessionID string) (*domain.SessionState, error)
    GetState(ctx context.Context, sessionID string) (*domain.SessionState, error)
    UpdateState(ctx context.Context, state *domain.SessionState) error
    AddDelta(ctx context.Context, sessionID string, delta *domain.Delta) error
    GetDeltas(ctx context.Context, sessionID string) ([]domain.Delta, error)
    GetDeltasSince(ctx context.Context, sessionID string, fromStep int) ([]domain.Delta, error)

    // NEW: Reconstruction
    GetDeltasUntil(ctx context.Context, sessionID string, toStep int) ([]domain.Delta, error)

    // NEW: ViewStack operations
    PushView(ctx context.Context, sessionID string, snapshot *domain.ViewSnapshot) error
    PopView(ctx context.Context, sessionID string) (*domain.ViewSnapshot, error)
    GetViewStack(ctx context.Context, sessionID string) ([]domain.ViewSnapshot, error)
}
```

### 4. Update Database Migration
**File**: `project/backend/internal/adapters/postgres/state_migrations.go`

Add migration for new fields:
```sql
-- Add new columns to chat_session_deltas
ALTER TABLE chat_session_deltas
    ADD COLUMN IF NOT EXISTS source VARCHAR(20) DEFAULT 'llm',
    ADD COLUMN IF NOT EXISTS actor_id VARCHAR(50),
    ADD COLUMN IF NOT EXISTS delta_type VARCHAR(20) DEFAULT 'add',
    ADD COLUMN IF NOT EXISTS path VARCHAR(100);

-- Add view state columns to chat_session_state
ALTER TABLE chat_session_state
    ADD COLUMN IF NOT EXISTS view_mode VARCHAR(20) DEFAULT 'grid',
    ADD COLUMN IF NOT EXISTS view_focused JSONB,
    ADD COLUMN IF NOT EXISTS view_stack JSONB DEFAULT '[]';

-- Index for filtering by source
CREATE INDEX IF NOT EXISTS idx_chat_session_deltas_source
    ON chat_session_deltas(session_id, source);
```

### 5. Implement StateAdapter Extensions
**File**: `project/backend/internal/adapters/postgres/postgres_state.go`

Update `AddDelta` to include new fields:
```go
func (a *StateAdapter) AddDelta(ctx context.Context, sessionID string, delta *domain.Delta) error {
    actionJSON, _ := json.Marshal(delta.Action)
    resultJSON, _ := json.Marshal(delta.Result)
    var templateJSON []byte
    if delta.Template != nil {
        templateJSON, _ = json.Marshal(delta.Template)
    }

    _, err := a.client.pool.Exec(ctx, `
        INSERT INTO chat_session_deltas
            (session_id, step, trigger, source, actor_id, delta_type, path, action, result, template)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
    `, sessionID, delta.Step, delta.Trigger,
       delta.Source, delta.ActorID, delta.DeltaType, delta.Path,
       actionJSON, resultJSON, templateJSON)

    return err
}
```

Implement new methods:
```go
func (a *StateAdapter) GetDeltasUntil(ctx context.Context, sessionID string, toStep int) ([]domain.Delta, error) {
    // SELECT ... WHERE session_id = $1 AND step <= $2 ORDER BY step ASC
}

func (a *StateAdapter) PushView(ctx context.Context, sessionID string, snapshot *domain.ViewSnapshot) error {
    // UPDATE ... SET view_stack = view_stack || $1::jsonb
}

func (a *StateAdapter) PopView(ctx context.Context, sessionID string) (*domain.ViewSnapshot, error) {
    // Get last element, remove it, return
}
```

### 6. Create State Reconstruct UseCase
**File**: `project/backend/internal/usecases/state_reconstruct.go`

```go
type ReconstructStateUseCase struct {
    statePort ports.StatePort
}

func NewReconstructStateUseCase(statePort ports.StatePort) *ReconstructStateUseCase {
    return &ReconstructStateUseCase{statePort: statePort}
}

type ReconstructRequest struct {
    SessionID string
    ToStep    int // Which step to reconstruct to
}

type ReconstructResponse struct {
    State   *domain.SessionState
    Deltas  []domain.Delta // Deltas applied
    StepNow int
}

// Execute reconstructs state at a specific step
func (uc *ReconstructStateUseCase) Execute(ctx context.Context, req ReconstructRequest) (*ReconstructResponse, error) {
    // 1. Get base state (step 0)
    // 2. Get deltas until req.ToStep
    // 3. Apply deltas sequentially
    // 4. Return reconstructed state
}
```

### 7. Create Rollback UseCase
**File**: `project/backend/internal/usecases/state_rollback.go`

```go
type RollbackUseCase struct {
    statePort      ports.StatePort
    reconstructUC  *ReconstructStateUseCase
}

type RollbackRequest struct {
    SessionID string
    ToStep    int    // Step to rollback to
    Source    domain.DeltaSource // Who initiated (user/system)
}

type RollbackResponse struct {
    State       *domain.SessionState
    RolledBack  int // Number of steps rolled back
    NewStep     int
}

// Execute rolls back state to a previous step
func (uc *RollbackUseCase) Execute(ctx context.Context, req RollbackRequest) (*RollbackResponse, error) {
    // 1. Reconstruct state at req.ToStep
    // 2. Create rollback delta (DeltaType = "rollback", records what was undone)
    // 3. Update current state
    // 4. Return new state
}
```

### 8. Update Agent1/Agent2 to Use New Delta Fields
**File**: `project/backend/internal/usecases/agent1_execute.go`

When creating delta, populate new fields:
```go
delta := &domain.Delta{
    Step:      state.Step + 1,
    Trigger:   domain.TriggerUserQuery,
    Source:    domain.SourceLLM,          // NEW
    ActorID:   "agent1",                   // NEW
    DeltaType: domain.DeltaTypeAdd,        // NEW
    Path:      "data.products",            // NEW
    Action: domain.Action{
        Type:   domain.ActionSearch,
        Tool:   toolName,
        Params: toolInput,
    },
    Result: domain.ResultMeta{...},
}
```

### 9. Validation
Run validation commands:
```bash
cd project/backend && go build ./...
cd project/backend && go test ./...
```

## Database Schema (after migration)

```
chat_session_state
├── id UUID (PK)
├── session_id UUID (FK → chat_sessions, UNIQUE)
├── current_data JSONB
├── current_meta JSONB
├── current_template JSONB
├── view_mode VARCHAR(20)      -- NEW
├── view_focused JSONB         -- NEW
├── view_stack JSONB           -- NEW
├── step INTEGER
├── created_at, updated_at

chat_session_deltas
├── id UUID (PK)
├── session_id UUID (FK)
├── step INTEGER
├── trigger VARCHAR(20)
├── source VARCHAR(20)         -- NEW: user/llm/system
├── actor_id VARCHAR(50)       -- NEW: agent1, user_click, etc.
├── delta_type VARCHAR(20)     -- NEW: add/remove/update/push/pop
├── path VARCHAR(100)          -- NEW: data.products, view.mode
├── action JSONB
├── result JSONB
├── template JSONB
├── created_at
└── UNIQUE(session_id, step)
```

## Validation Commands

```bash
cd project/backend && go build ./...
cd project/backend && go test ./...
cd project/frontend && npm run build
cd project/frontend && npm run lint
```

## Acceptance Criteria

- [ ] DeltaSource enum (user/llm/system) added to Delta
- [ ] ActorID field identifies which agent or user action
- [ ] DeltaType enum (add/remove/update/push/pop) added
- [ ] Path field tracks what part of state changed
- [ ] ViewStack stores navigation history
- [ ] ViewSnapshot captures view state for back navigation
- [ ] StatePort.GetDeltasUntil implemented
- [ ] StatePort.PushView/PopView implemented
- [ ] ReconstructStateUseCase can rebuild state at any step
- [ ] RollbackUseCase can revert to previous step
- [ ] Agent1/Agent2 populate new delta fields
- [ ] Database migration adds new columns
- [ ] All tests pass

## Notes

### Gotchas (from expertise)
- `session_id` must be valid UUID (FK to chat_sessions)
- Create session via CachePort before creating state
- ErrSessionNotFound returned when state doesn't exist
- Price stored in kopecks (int), not rubles

### Design Decisions
1. **Source vs Trigger**: `Trigger` describes what initiated (query, click, system), `Source` describes who (user, llm, system). Both are useful for different analytics.

2. **Path format**: Use dot notation (`data.products`, `view.mode`, `viewStack`) for consistency. Not implementing full JSON path — keep simple.

3. **ViewStack in DB**: Stored as JSONB array in chat_session_state. Could be separate table, but array is simpler for back/forward nav.

4. **Rollback creates delta**: When rolling back, we create a new delta of type "rollback" that records what was undone. This preserves full history.

### Dependencies for Feature 2 (Drill-down)
This feature provides:
- ViewStack for navigation history
- ViewMode for grid/detail/list states
- Push/Pop operations for back button
- EntityRef for tracking focused item

Feature 2 will add:
- ExpandUseCase (uses PushView)
- BackUseCase (uses PopView)
- Frontend DetailView component
