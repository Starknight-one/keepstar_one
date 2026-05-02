package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"keepstar_v5/internal/domain"
)

// pgExecutor is the subset of pgxpool.Pool / pgx.Tx that the AddDelta
// path uses. Lets the same SQL run either against the pool directly
// (public AddDelta) or inside a tx (zoneWriteWithDelta).
type pgExecutor interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// StateAdapter implements ports.StatePort against Postgres. Ported from V4
// with table names rewritten to v5_*, span tracing dropped, and the same
// atomicity caveats (zone+delta written as two separate Exec calls — see
// chunk-2 plan "Risks" section).
type StateAdapter struct {
	client *Client
	log    *slog.Logger
}

func NewStateAdapter(client *Client, log *slog.Logger) *StateAdapter {
	if log == nil {
		log = slog.Default()
	}
	return &StateAdapter{client: client, log: log}
}

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
		View:      domain.ViewState{Mode: domain.ViewModeGrid},
		ViewStack: []domain.ViewSnapshot{},
		Actions: domain.StateActions{
			LikedIds:  []string{},
			CartItems: []domain.CartItem{},
		},
		Step:      0,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	dataJSON, err := json.Marshal(state.Current.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal data: %w", err)
	}
	metaJSON, err := json.Marshal(state.Current.Meta)
	if err != nil {
		return nil, fmt.Errorf("marshal meta: %w", err)
	}
	viewStackJSON, err := json.Marshal(state.ViewStack)
	if err != nil {
		return nil, fmt.Errorf("marshal view stack: %w", err)
	}
	conversationHistoryJSON, err := json.Marshal(state.ConversationHistory)
	if err != nil {
		return nil, fmt.Errorf("marshal conversation history: %w", err)
	}
	agent2HistoryJSON, err := json.Marshal(state.Agent2History)
	if err != nil {
		return nil, fmt.Errorf("marshal agent2 history: %w", err)
	}
	actionsJSON, err := json.Marshal(state.Actions)
	if err != nil {
		return nil, fmt.Errorf("marshal actions: %w", err)
	}

	err = a.client.pool.QueryRow(ctx, `
		INSERT INTO v5_chat_session_state (session_id, current_data, current_meta, step, view_mode, view_stack, conversation_history, agent2_history, actions)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at
	`, sessionID, dataJSON, metaJSON, state.Step, state.View.Mode, viewStackJSON, conversationHistoryJSON, agent2HistoryJSON, actionsJSON).Scan(
		&state.ID, &state.CreatedAt, &state.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create state: %w", err)
	}
	return state, nil
}

func (a *StateAdapter) GetState(ctx context.Context, sessionID string) (*domain.SessionState, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		end := sc.Start("postgres.GetState")
		defer end()
	}
	var state domain.SessionState
	var dataJSON, metaJSON, templateJSON, viewFocusedJSON, viewStackJSON, conversationHistoryJSON, agent2HistoryJSON, actionsJSON []byte
	var viewMode *string

	err := a.client.pool.QueryRow(ctx, `
		SELECT id, session_id, current_data, current_meta, current_template,
		       view_mode, view_focused, view_stack, conversation_history, agent2_history, actions, step, created_at, updated_at
		FROM v5_chat_session_state
		WHERE session_id = $1
	`, sessionID).Scan(
		&state.ID, &state.SessionID, &dataJSON, &metaJSON, &templateJSON,
		&viewMode, &viewFocusedJSON, &viewStackJSON, &conversationHistoryJSON, &agent2HistoryJSON, &actionsJSON,
		&state.Step, &state.CreatedAt, &state.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, domain.ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get state: %w", err)
	}

	if len(dataJSON) > 0 {
		if err := json.Unmarshal(dataJSON, &state.Current.Data); err != nil {
			a.log.Warn("unmarshal state data", "session_id", sessionID, "error", err)
		}
	}
	if len(metaJSON) > 0 {
		if err := json.Unmarshal(metaJSON, &state.Current.Meta); err != nil {
			a.log.Warn("unmarshal state meta", "session_id", sessionID, "error", err)
		}
	}
	if len(templateJSON) > 0 {
		if err := json.Unmarshal(templateJSON, &state.Current.Template); err != nil {
			a.log.Warn("unmarshal state template", "session_id", sessionID, "error", err)
		}
	}

	if viewMode != nil {
		state.View.Mode = domain.ViewMode(*viewMode)
	} else {
		state.View.Mode = domain.ViewModeGrid
	}
	if len(viewFocusedJSON) > 0 {
		if err := json.Unmarshal(viewFocusedJSON, &state.View.Focused); err != nil {
			a.log.Warn("unmarshal view focused", "session_id", sessionID, "error", err)
		}
	}
	if len(viewStackJSON) > 0 {
		if err := json.Unmarshal(viewStackJSON, &state.ViewStack); err != nil {
			a.log.Warn("unmarshal view stack", "session_id", sessionID, "error", err)
		}
	}
	if len(conversationHistoryJSON) > 0 {
		if err := json.Unmarshal(conversationHistoryJSON, &state.ConversationHistory); err != nil {
			a.log.Warn("unmarshal conversation history", "session_id", sessionID, "error", err)
		}
	}
	if len(agent2HistoryJSON) > 0 {
		if err := json.Unmarshal(agent2HistoryJSON, &state.Agent2History); err != nil {
			a.log.Warn("unmarshal agent2 history", "session_id", sessionID, "error", err)
		}
	}
	if len(actionsJSON) > 0 {
		if err := json.Unmarshal(actionsJSON, &state.Actions); err != nil {
			a.log.Warn("unmarshal actions", "session_id", sessionID, "error", err)
		}
	}
	if state.Actions.LikedIds == nil {
		state.Actions.LikedIds = []string{}
	}
	if state.Actions.CartItems == nil {
		state.Actions.CartItems = []domain.CartItem{}
	}

	return &state, nil
}

func (a *StateAdapter) UpdateState(ctx context.Context, state *domain.SessionState) error {
	dataJSON, err := json.Marshal(state.Current.Data)
	if err != nil {
		return fmt.Errorf("marshal data: %w", err)
	}
	metaJSON, err := json.Marshal(state.Current.Meta)
	if err != nil {
		return fmt.Errorf("marshal meta: %w", err)
	}
	templateJSON, err := json.Marshal(state.Current.Template)
	if err != nil {
		return fmt.Errorf("marshal template: %w", err)
	}
	viewFocusedJSON, err := json.Marshal(state.View.Focused)
	if err != nil {
		return fmt.Errorf("marshal view focused: %w", err)
	}
	viewStackJSON, err := json.Marshal(state.ViewStack)
	if err != nil {
		return fmt.Errorf("marshal view stack: %w", err)
	}
	conversationHistoryJSON, err := json.Marshal(state.ConversationHistory)
	if err != nil {
		return fmt.Errorf("marshal conversation history: %w", err)
	}
	agent2HistoryJSON, err := json.Marshal(state.Agent2History)
	if err != nil {
		return fmt.Errorf("marshal agent2 history: %w", err)
	}
	actionsJSON, err := json.Marshal(state.Actions)
	if err != nil {
		return fmt.Errorf("marshal actions: %w", err)
	}

	_, err = a.client.pool.Exec(ctx, `
		UPDATE v5_chat_session_state
		SET current_data = $1, current_meta = $2, current_template = $3,
		    view_mode = $4, view_focused = $5, view_stack = $6,
		    conversation_history = $7, agent2_history = $8, actions = $9, step = $10, updated_at = NOW()
		WHERE session_id = $11
	`, dataJSON, metaJSON, templateJSON,
		state.View.Mode, viewFocusedJSON, viewStackJSON,
		conversationHistoryJSON, agent2HistoryJSON, actionsJSON, state.Step, state.SessionID)
	if err != nil {
		return fmt.Errorf("update state: %w", err)
	}
	return nil
}

// addDeltaMaxRetries caps the number of attempts when AddDelta hits the
// UNIQUE(session_id, step) constraint under concurrent writers. Each
// retry waits a randomised 10–50ms.
const addDeltaMaxRetries = 3

// AddDelta inserts a delta with auto-assigned step (MAX(step)+1 per
// session) and syncs v5_chat_session_state.step.
//
// Race handling: the CTE reads MAX(step) and INSERTs in one statement,
// but two concurrent writers per session can still read the same
// MAX(step) under READ COMMITTED. The UNIQUE(session_id, step)
// constraint guarantees integrity — one INSERT errors with SQLSTATE
// 23505. AddDelta now retries up to addDeltaMaxRetries times with a
// short randomised backoff before surfacing the error.
//
// The state.step sync UPDATE is best-effort and runs after a successful
// delta INSERT — failures are logged, not surfaced.
func (a *StateAdapter) AddDelta(ctx context.Context, sessionID string, delta *domain.Delta) (int, error) {
	step, err := a.addDeltaWithRetry(ctx, a.client.pool, sessionID, delta)
	if err != nil {
		return 0, err
	}
	if _, syncErr := a.client.pool.Exec(ctx, `
		UPDATE v5_chat_session_state SET step = $1, updated_at = NOW()
		WHERE session_id = $2
	`, step, sessionID); syncErr != nil {
		a.log.Warn("sync state step", "session_id", sessionID, "step", step, "error", syncErr)
	}
	delta.Step = step
	return step, nil
}

// addDeltaWithRetry calls addDeltaOnce against the given executor (pool
// or tx) up to addDeltaMaxRetries times on unique-constraint violation.
// Other errors bail immediately.
func (a *StateAdapter) addDeltaWithRetry(ctx context.Context, exec pgExecutor, sessionID string, delta *domain.Delta) (int, error) {
	var lastErr error
	for attempt := 0; attempt < addDeltaMaxRetries; attempt++ {
		step, err := a.addDeltaOnce(ctx, exec, sessionID, delta)
		if err == nil {
			return step, nil
		}
		if !isUniqueViolation(err) {
			return 0, fmt.Errorf("add delta: %w", err)
		}
		lastErr = err
		a.log.Debug("AddDelta unique-violation; retrying",
			"session_id", sessionID, "attempt", attempt+1, "max", addDeltaMaxRetries)
		// Randomised 10-50ms backoff so retries don't lock-step.
		time.Sleep(time.Duration(10+rand.Intn(40)) * time.Millisecond)
	}
	return 0, fmt.Errorf("add delta: exhausted %d retries: %w", addDeltaMaxRetries, lastErr)
}

// addDeltaOnce runs a single CTE INSERT against the provided executor
// (pool or tx). Returns the assigned step on success; passes the
// underlying pgx error through on failure (caller decides retry policy).
func (a *StateAdapter) addDeltaOnce(ctx context.Context, exec pgExecutor, sessionID string, delta *domain.Delta) (int, error) {
	actionJSON, err := json.Marshal(delta.Action)
	if err != nil {
		return 0, fmt.Errorf("marshal action: %w", err)
	}
	resultJSON, err := json.Marshal(delta.Result)
	if err != nil {
		return 0, fmt.Errorf("marshal result: %w", err)
	}
	var templateJSON []byte
	if delta.Template != nil {
		var tErr error
		templateJSON, tErr = json.Marshal(delta.Template)
		if tErr != nil {
			return 0, fmt.Errorf("marshal template: %w", tErr)
		}
	}

	source := delta.Source
	if source == "" {
		source = domain.SourceLLM
	}
	deltaType := delta.DeltaType
	if deltaType == "" {
		deltaType = domain.DeltaTypeAdd
	}

	var assignedStep int
	err = exec.QueryRow(ctx, `
		WITH next_step AS (
			SELECT COALESCE(MAX(step), 0) + 1 AS step
			FROM v5_chat_session_deltas
			WHERE session_id = $1
		),
		inserted AS (
			INSERT INTO v5_chat_session_deltas
				(session_id, step, trigger, source, actor_id, delta_type, path, action, result, template, turn_id)
			SELECT $1, next_step.step, $2, $3, $4, $5, $6, $7, $8, $9, $10
			FROM next_step
			RETURNING step
		)
		SELECT step FROM inserted
	`, sessionID, delta.Trigger,
		source, delta.ActorID, deltaType, delta.Path,
		actionJSON, resultJSON, templateJSON, delta.TurnID).Scan(&assignedStep)
	if err != nil {
		return 0, err // unwrapped so retry can detect 23505
	}
	return assignedStep, nil
}

// isUniqueViolation reports whether err is a Postgres unique-constraint
// violation (SQLSTATE 23505). Used by AddDelta retry logic.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

func (a *StateAdapter) UpdateData(ctx context.Context, sessionID string, data domain.StateData, meta domain.StateMeta, info domain.DeltaInfo) (int, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		end := sc.Start("postgres.UpdateData")
		defer end()
	}
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return 0, fmt.Errorf("marshal data: %w", err)
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return 0, fmt.Errorf("marshal meta: %w", err)
	}
	delta := info.ToDelta()
	return a.zoneWriteWithDelta(ctx, sessionID, delta, `
		UPDATE v5_chat_session_state
		SET current_data = $1, current_meta = $2, updated_at = NOW()
		WHERE session_id = $3
	`, dataJSON, metaJSON, sessionID)
}

func (a *StateAdapter) UpdateTemplate(ctx context.Context, sessionID string, template map[string]interface{}, info domain.DeltaInfo) (int, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		end := sc.Start("postgres.UpdateTemplate")
		defer end()
	}
	templateJSON, err := json.Marshal(template)
	if err != nil {
		return 0, fmt.Errorf("marshal template: %w", err)
	}
	delta := info.ToDelta()
	return a.zoneWriteWithDelta(ctx, sessionID, delta, `
		UPDATE v5_chat_session_state
		SET current_template = $1, updated_at = NOW()
		WHERE session_id = $2
	`, templateJSON, sessionID)
}

func (a *StateAdapter) UpdateView(ctx context.Context, sessionID string, view domain.ViewState, stack []domain.ViewSnapshot, info domain.DeltaInfo) (int, error) {
	viewFocusedJSON, err := json.Marshal(view.Focused)
	if err != nil {
		return 0, fmt.Errorf("marshal view focused: %w", err)
	}
	viewStackJSON, err := json.Marshal(stack)
	if err != nil {
		return 0, fmt.Errorf("marshal view stack: %w", err)
	}
	delta := info.ToDelta()
	return a.zoneWriteWithDelta(ctx, sessionID, delta, `
		UPDATE v5_chat_session_state
		SET view_mode = $1, view_focused = $2, view_stack = $3, updated_at = NOW()
		WHERE session_id = $4
	`, view.Mode, viewFocusedJSON, viewStackJSON, sessionID)
}

func (a *StateAdapter) UpdateActions(ctx context.Context, sessionID string, actions domain.StateActions, info domain.DeltaInfo) (int, error) {
	actionsJSON, err := json.Marshal(actions)
	if err != nil {
		return 0, fmt.Errorf("marshal actions: %w", err)
	}
	delta := info.ToDelta()
	return a.zoneWriteWithDelta(ctx, sessionID, delta, `
		UPDATE v5_chat_session_state
		SET actions = $1, updated_at = NOW()
		WHERE session_id = $2
	`, actionsJSON, sessionID)
}

func (a *StateAdapter) AppendConversation(ctx context.Context, sessionID string, messages []domain.LLMMessage) error {
	historyJSON, err := json.Marshal(messages)
	if err != nil {
		return fmt.Errorf("marshal conversation: %w", err)
	}
	_, err = a.client.pool.Exec(ctx, `
		UPDATE v5_chat_session_state
		SET conversation_history = $1, updated_at = NOW()
		WHERE session_id = $2
	`, historyJSON, sessionID)
	if err != nil {
		return fmt.Errorf("append conversation: %w", err)
	}
	return nil
}

func (a *StateAdapter) AppendAgent2History(ctx context.Context, sessionID string, messages []domain.LLMMessage) error {
	historyJSON, err := json.Marshal(messages)
	if err != nil {
		return fmt.Errorf("marshal agent2 history: %w", err)
	}
	_, err = a.client.pool.Exec(ctx, `
		UPDATE v5_chat_session_state
		SET agent2_history = $1, updated_at = NOW()
		WHERE session_id = $2
	`, historyJSON, sessionID)
	if err != nil {
		return fmt.Errorf("append agent2 history: %w", err)
	}
	return nil
}

// zoneWriteWithDelta runs a zone UPDATE and the matching delta INSERT
// inside a single transaction (READ COMMITTED) — chunk-6d hygiene. Prior
// to this, the two writes ran as separate Exec calls and could diverge
// on AddDelta failure (zone written, no audit row).
//
// addDeltaWithRetry is invoked inside the tx. If the unique-step race
// triggers within the tx context, the retry loop kicks in; on terminal
// failure the tx rolls back and the zone update is undone.
//
// state.step sync UPDATE is also part of the tx so it can never lag the
// delta INSERT.
func (a *StateAdapter) zoneWriteWithDelta(ctx context.Context, sessionID string, delta *domain.Delta, zoneSQL string, zoneArgs ...interface{}) (int, error) {
	tx, err := a.client.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) // no-op after Commit

	if _, err := tx.Exec(ctx, zoneSQL, zoneArgs...); err != nil {
		return 0, fmt.Errorf("zone update: %w", err)
	}
	step, err := a.addDeltaWithRetry(ctx, tx, sessionID, delta)
	if err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE v5_chat_session_state SET step = $1, updated_at = NOW()
		WHERE session_id = $2
	`, step, sessionID); err != nil {
		return 0, fmt.Errorf("sync step: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	delta.Step = step
	return step, nil
}

func (a *StateAdapter) GetDeltas(ctx context.Context, sessionID string) ([]domain.Delta, error) {
	return a.GetDeltasSince(ctx, sessionID, 0)
}

func (a *StateAdapter) GetDeltasSince(ctx context.Context, sessionID string, fromStep int) ([]domain.Delta, error) {
	rows, err := a.client.pool.Query(ctx, `
		SELECT step, trigger, source, actor_id, delta_type, path, action, result, template, turn_id, created_at
		FROM v5_chat_session_deltas
		WHERE session_id = $1 AND step >= $2
		ORDER BY step ASC
	`, sessionID, fromStep)
	if err != nil {
		return nil, fmt.Errorf("get deltas: %w", err)
	}
	defer rows.Close()
	return a.scanDeltas(rows)
}

func (a *StateAdapter) GetDeltasUntil(ctx context.Context, sessionID string, toStep int) ([]domain.Delta, error) {
	rows, err := a.client.pool.Query(ctx, `
		SELECT step, trigger, source, actor_id, delta_type, path, action, result, template, turn_id, created_at
		FROM v5_chat_session_deltas
		WHERE session_id = $1 AND step <= $2
		ORDER BY step ASC
	`, sessionID, toStep)
	if err != nil {
		return nil, fmt.Errorf("get deltas until: %w", err)
	}
	defer rows.Close()
	return a.scanDeltas(rows)
}

// deltaRows is the minimal pgx.Rows surface scanDeltas uses. Lets us pass a
// fake in unit tests without mocking the whole 9-method pgx.Rows interface.
type deltaRows interface {
	Next() bool
	Scan(dest ...any) error
}

func (a *StateAdapter) scanDeltas(rows deltaRows) ([]domain.Delta, error) {
	var deltas []domain.Delta
	for rows.Next() {
		var d domain.Delta
		var actionJSON, resultJSON, templateJSON []byte
		var trigger string
		var source, actorID, deltaType, path, turnID *string

		err := rows.Scan(&d.Step, &trigger, &source, &actorID, &deltaType, &path,
			&actionJSON, &resultJSON, &templateJSON, &turnID, &d.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan delta: %w", err)
		}

		d.Trigger = domain.TriggerType(trigger)
		if source != nil {
			d.Source = domain.DeltaSource(*source)
		}
		if actorID != nil {
			d.ActorID = *actorID
		}
		if deltaType != nil {
			d.DeltaType = domain.DeltaType(*deltaType)
		}
		if path != nil {
			d.Path = *path
		}
		if turnID != nil {
			d.TurnID = *turnID
		}
		if err := json.Unmarshal(actionJSON, &d.Action); err != nil {
			a.log.Warn("unmarshal delta action", "step", d.Step, "error", err)
		}
		if err := json.Unmarshal(resultJSON, &d.Result); err != nil {
			a.log.Warn("unmarshal delta result", "step", d.Step, "error", err)
		}
		if len(templateJSON) > 0 {
			if err := json.Unmarshal(templateJSON, &d.Template); err != nil {
				a.log.Warn("unmarshal delta template", "step", d.Step, "error", err)
			}
		}
		deltas = append(deltas, d)
	}
	return deltas, nil
}

func (a *StateAdapter) PushView(ctx context.Context, sessionID string, snapshot *domain.ViewSnapshot) error {
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	_, err = a.client.pool.Exec(ctx, `
		UPDATE v5_chat_session_state
		SET view_stack = view_stack || $1::jsonb,
		    updated_at = NOW()
		WHERE session_id = $2
	`, snapshotJSON, sessionID)
	if err != nil {
		return fmt.Errorf("push view: %w", err)
	}
	return nil
}

func (a *StateAdapter) PopView(ctx context.Context, sessionID string) (*domain.ViewSnapshot, error) {
	var viewStackJSON []byte
	err := a.client.pool.QueryRow(ctx, `
		SELECT view_stack
		FROM v5_chat_session_state
		WHERE session_id = $1
	`, sessionID).Scan(&viewStackJSON)
	if err == pgx.ErrNoRows {
		return nil, domain.ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get view stack: %w", err)
	}

	var viewStack []domain.ViewSnapshot
	if len(viewStackJSON) > 0 {
		if err := json.Unmarshal(viewStackJSON, &viewStack); err != nil {
			a.log.Warn("unmarshal view stack in pop", "session_id", sessionID, "error", err)
		}
	}
	if len(viewStack) == 0 {
		return nil, nil
	}

	lastSnapshot := viewStack[len(viewStack)-1]
	viewStack = viewStack[:len(viewStack)-1]

	newStackJSON, err := json.Marshal(viewStack)
	if err != nil {
		return nil, fmt.Errorf("marshal view stack: %w", err)
	}
	_, err = a.client.pool.Exec(ctx, `
		UPDATE v5_chat_session_state
		SET view_stack = $1,
		    updated_at = NOW()
		WHERE session_id = $2
	`, newStackJSON, sessionID)
	if err != nil {
		return nil, fmt.Errorf("update view stack: %w", err)
	}
	return &lastSnapshot, nil
}

func (a *StateAdapter) GetViewStack(ctx context.Context, sessionID string) ([]domain.ViewSnapshot, error) {
	var viewStackJSON []byte
	err := a.client.pool.QueryRow(ctx, `
		SELECT view_stack
		FROM v5_chat_session_state
		WHERE session_id = $1
	`, sessionID).Scan(&viewStackJSON)
	if err == pgx.ErrNoRows {
		return nil, domain.ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get view stack: %w", err)
	}

	var viewStack []domain.ViewSnapshot
	if len(viewStackJSON) > 0 {
		if err := json.Unmarshal(viewStackJSON, &viewStack); err != nil {
			a.log.Warn("unmarshal view stack", "session_id", sessionID, "error", err)
		}
	}
	return viewStack, nil
}
