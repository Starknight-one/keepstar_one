package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"keepstar/internal/domain"
	"keepstar/internal/logger"
)

// StateAdapter implements ports.StatePort
type StateAdapter struct {
	client *Client
	log    *logger.Logger
}

// NewStateAdapter creates a new StateAdapter
func NewStateAdapter(client *Client, log *logger.Logger) *StateAdapter {
	return &StateAdapter{client: client, log: log}
}

// CreateState creates a new state for a session
func (a *StateAdapter) CreateState(ctx context.Context, sessionID string) (*domain.SessionState, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("db.create_state")
		defer endSpan()
	}
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
		View: domain.ViewState{
			Mode: domain.ViewModeGrid,
		},
		ViewStack: []domain.ViewSnapshot{},
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

	err = a.client.pool.QueryRow(ctx, `
		INSERT INTO chat_session_state (session_id, current_data, current_meta, step, view_mode, view_stack, conversation_history)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at
	`, sessionID, dataJSON, metaJSON, state.Step, state.View.Mode, viewStackJSON, conversationHistoryJSON).Scan(
		&state.ID, &state.CreatedAt, &state.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create state: %w", err)
	}

	return state, nil
}

// GetState retrieves the current state for a session
func (a *StateAdapter) GetState(ctx context.Context, sessionID string) (*domain.SessionState, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("db.get_state")
		defer endSpan()
	}
	var state domain.SessionState
	var dataJSON, metaJSON, templateJSON, viewFocusedJSON, viewStackJSON, conversationHistoryJSON []byte
	var viewMode *string

	err := a.client.pool.QueryRow(ctx, `
		SELECT id, session_id, current_data, current_meta, current_template,
		       view_mode, view_focused, view_stack, conversation_history, step, created_at, updated_at
		FROM chat_session_state
		WHERE session_id = $1
	`, sessionID).Scan(
		&state.ID, &state.SessionID, &dataJSON, &metaJSON, &templateJSON,
		&viewMode, &viewFocusedJSON, &viewStackJSON, &conversationHistoryJSON,
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

	// Parse view state
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

	return &state, nil
}

// UpdateState updates the current materialized state
func (a *StateAdapter) UpdateState(ctx context.Context, state *domain.SessionState) error {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("db.update_state")
		defer endSpan()
	}
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

	_, err = a.client.pool.Exec(ctx, `
		UPDATE chat_session_state
		SET current_data = $1, current_meta = $2, current_template = $3,
		    view_mode = $4, view_focused = $5, view_stack = $6,
		    conversation_history = $7, step = $8, updated_at = NOW()
		WHERE session_id = $9
	`, dataJSON, metaJSON, templateJSON,
		state.View.Mode, viewFocusedJSON, viewStackJSON,
		conversationHistoryJSON, state.Step, state.SessionID)
	if err != nil {
		return fmt.Errorf("update state: %w", err)
	}

	return nil
}

// AddDelta appends a new delta to the session history.
// Step is auto-assigned as MAX(step)+1 for the session.
// Also updates chat_session_state.step to keep it in sync.
// Returns the assigned step number.
func (a *StateAdapter) AddDelta(ctx context.Context, sessionID string, delta *domain.Delta) (int, error) {
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

	// Use default values for new fields if not set
	source := delta.Source
	if source == "" {
		source = domain.SourceLLM
	}
	deltaType := delta.DeltaType
	if deltaType == "" {
		deltaType = domain.DeltaTypeAdd
	}

	// Auto-assign step and update state.step atomically
	var assignedStep int
	err = a.client.pool.QueryRow(ctx, `
		WITH next_step AS (
			SELECT COALESCE(MAX(step), 0) + 1 AS step
			FROM chat_session_deltas
			WHERE session_id = $1
		),
		inserted AS (
			INSERT INTO chat_session_deltas
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
		return 0, fmt.Errorf("add delta: %w", err)
	}

	// Update state.step to stay in sync
	if _, syncErr := a.client.pool.Exec(ctx, `
		UPDATE chat_session_state SET step = $1, updated_at = NOW()
		WHERE session_id = $2
	`, assignedStep, sessionID); syncErr != nil {
		a.log.Warn("sync state step", "session_id", sessionID, "step", assignedStep, "error", syncErr)
	}

	// Write back to delta struct so caller can see the assigned step
	delta.Step = assignedStep

	return assignedStep, nil
}

// UpdateData updates the data zone (products/services + meta) and creates a delta
func (a *StateAdapter) UpdateData(ctx context.Context, sessionID string, data domain.StateData, meta domain.StateMeta, info domain.DeltaInfo) (int, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("db.update_data")
		defer endSpan()
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
		UPDATE chat_session_state
		SET current_data = $1, current_meta = $2, updated_at = NOW()
		WHERE session_id = $3
	`, dataJSON, metaJSON, sessionID)
}

// UpdateTemplate updates the template zone and creates a delta
func (a *StateAdapter) UpdateTemplate(ctx context.Context, sessionID string, template map[string]interface{}, info domain.DeltaInfo) (int, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("db.update_template")
		defer endSpan()
	}
	templateJSON, err := json.Marshal(template)
	if err != nil {
		return 0, fmt.Errorf("marshal template: %w", err)
	}
	delta := info.ToDelta()
	return a.zoneWriteWithDelta(ctx, sessionID, delta, `
		UPDATE chat_session_state
		SET current_template = $1, updated_at = NOW()
		WHERE session_id = $2
	`, templateJSON, sessionID)
}

// UpdateView updates the view zone (mode, focused, stack) and creates a delta
func (a *StateAdapter) UpdateView(ctx context.Context, sessionID string, view domain.ViewState, stack []domain.ViewSnapshot, info domain.DeltaInfo) (int, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("db.update_view")
		defer endSpan()
	}
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
		UPDATE chat_session_state
		SET view_mode = $1, view_focused = $2, view_stack = $3, updated_at = NOW()
		WHERE session_id = $4
	`, view.Mode, viewFocusedJSON, viewStackJSON, sessionID)
}

// AppendConversation updates conversation history (no delta — append-only for LLM cache)
func (a *StateAdapter) AppendConversation(ctx context.Context, sessionID string, messages []domain.LLMMessage) error {
	historyJSON, err := json.Marshal(messages)
	if err != nil {
		return fmt.Errorf("marshal conversation: %w", err)
	}
	_, err = a.client.pool.Exec(ctx, `
		UPDATE chat_session_state
		SET conversation_history = $1, updated_at = NOW()
		WHERE session_id = $2
	`, historyJSON, sessionID)
	if err != nil {
		return fmt.Errorf("append conversation: %w", err)
	}
	return nil
}

// zoneWriteWithDelta executes a zone UPDATE + AddDelta in sequence.
// AddDelta auto-assigns step and syncs state.step.
func (a *StateAdapter) zoneWriteWithDelta(ctx context.Context, sessionID string, delta *domain.Delta, zoneSQL string, zoneArgs ...interface{}) (int, error) {
	// 1. Execute zone update
	_, err := a.client.pool.Exec(ctx, zoneSQL, zoneArgs...)
	if err != nil {
		return 0, fmt.Errorf("zone update: %w", err)
	}
	// 2. Add delta (step auto-assigned, state.step synced)
	step, err := a.AddDelta(ctx, sessionID, delta)
	if err != nil {
		return 0, fmt.Errorf("add delta: %w", err)
	}
	return step, nil
}

// GetDeltas retrieves all deltas for a session
func (a *StateAdapter) GetDeltas(ctx context.Context, sessionID string) ([]domain.Delta, error) {
	return a.GetDeltasSince(ctx, sessionID, 0)
}

// GetDeltasSince retrieves deltas from a specific step
func (a *StateAdapter) GetDeltasSince(ctx context.Context, sessionID string, fromStep int) ([]domain.Delta, error) {
	rows, err := a.client.pool.Query(ctx, `
		SELECT step, trigger, source, actor_id, delta_type, path, action, result, template, turn_id, created_at
		FROM chat_session_deltas
		WHERE session_id = $1 AND step >= $2
		ORDER BY step ASC
	`, sessionID, fromStep)
	if err != nil {
		return nil, fmt.Errorf("get deltas: %w", err)
	}
	defer rows.Close()

	return a.scanDeltas(rows)
}

// GetDeltasUntil retrieves deltas up to and including a specific step (for reconstruction)
func (a *StateAdapter) GetDeltasUntil(ctx context.Context, sessionID string, toStep int) ([]domain.Delta, error) {
	rows, err := a.client.pool.Query(ctx, `
		SELECT step, trigger, source, actor_id, delta_type, path, action, result, template, turn_id, created_at
		FROM chat_session_deltas
		WHERE session_id = $1 AND step <= $2
		ORDER BY step ASC
	`, sessionID, toStep)
	if err != nil {
		return nil, fmt.Errorf("get deltas until: %w", err)
	}
	defer rows.Close()

	return a.scanDeltas(rows)
}

// scanDeltas is a helper to scan delta rows
func (a *StateAdapter) scanDeltas(rows pgx.Rows) ([]domain.Delta, error) {
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

// PushView pushes a view snapshot onto the navigation stack
func (a *StateAdapter) PushView(ctx context.Context, sessionID string, snapshot *domain.ViewSnapshot) error {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("db.push_view")
		defer endSpan()
	}
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}

	_, err = a.client.pool.Exec(ctx, `
		UPDATE chat_session_state
		SET view_stack = view_stack || $1::jsonb,
		    updated_at = NOW()
		WHERE session_id = $2
	`, snapshotJSON, sessionID)
	if err != nil {
		return fmt.Errorf("push view: %w", err)
	}

	return nil
}

// PopView pops and returns the last view snapshot from the navigation stack
func (a *StateAdapter) PopView(ctx context.Context, sessionID string) (*domain.ViewSnapshot, error) {
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("db.pop_view")
		defer endSpan()
	}
	var viewStackJSON []byte

	// Get current view stack
	err := a.client.pool.QueryRow(ctx, `
		SELECT view_stack
		FROM chat_session_state
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
		return nil, nil // Empty stack
	}

	// Pop the last element
	lastSnapshot := viewStack[len(viewStack)-1]
	viewStack = viewStack[:len(viewStack)-1]

	// Update the stack
	newStackJSON, err := json.Marshal(viewStack)
	if err != nil {
		return nil, fmt.Errorf("marshal view stack: %w", err)
	}
	_, err = a.client.pool.Exec(ctx, `
		UPDATE chat_session_state
		SET view_stack = $1,
		    updated_at = NOW()
		WHERE session_id = $2
	`, newStackJSON, sessionID)
	if err != nil {
		return nil, fmt.Errorf("update view stack: %w", err)
	}

	return &lastSnapshot, nil
}

// GetViewStack retrieves the entire view stack for a session
func (a *StateAdapter) GetViewStack(ctx context.Context, sessionID string) ([]domain.ViewSnapshot, error) {
	var viewStackJSON []byte

	err := a.client.pool.QueryRow(ctx, `
		SELECT view_stack
		FROM chat_session_state
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
