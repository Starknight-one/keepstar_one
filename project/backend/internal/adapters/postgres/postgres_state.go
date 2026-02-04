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
		View: domain.ViewState{
			Mode: domain.ViewModeGrid,
		},
		ViewStack: []domain.ViewSnapshot{},
		Step:      0,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	dataJSON, _ := json.Marshal(state.Current.Data)
	metaJSON, _ := json.Marshal(state.Current.Meta)
	viewStackJSON, _ := json.Marshal(state.ViewStack)

	err := a.client.pool.QueryRow(ctx, `
		INSERT INTO chat_session_state (session_id, current_data, current_meta, step, view_mode, view_stack)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`, sessionID, dataJSON, metaJSON, state.Step, state.View.Mode, viewStackJSON).Scan(
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
	var dataJSON, metaJSON, templateJSON, viewFocusedJSON, viewStackJSON []byte
	var viewMode *string

	err := a.client.pool.QueryRow(ctx, `
		SELECT id, session_id, current_data, current_meta, current_template,
		       view_mode, view_focused, view_stack, step, created_at, updated_at
		FROM chat_session_state
		WHERE session_id = $1
	`, sessionID).Scan(
		&state.ID, &state.SessionID, &dataJSON, &metaJSON, &templateJSON,
		&viewMode, &viewFocusedJSON, &viewStackJSON,
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

	// Parse view state
	if viewMode != nil {
		state.View.Mode = domain.ViewMode(*viewMode)
	} else {
		state.View.Mode = domain.ViewModeGrid
	}
	if len(viewFocusedJSON) > 0 {
		json.Unmarshal(viewFocusedJSON, &state.View.Focused)
	}
	if len(viewStackJSON) > 0 {
		json.Unmarshal(viewStackJSON, &state.ViewStack)
	}

	return &state, nil
}

// UpdateState updates the current materialized state
func (a *StateAdapter) UpdateState(ctx context.Context, state *domain.SessionState) error {
	dataJSON, _ := json.Marshal(state.Current.Data)
	metaJSON, _ := json.Marshal(state.Current.Meta)
	templateJSON, _ := json.Marshal(state.Current.Template)
	viewFocusedJSON, _ := json.Marshal(state.View.Focused)
	viewStackJSON, _ := json.Marshal(state.ViewStack)

	_, err := a.client.pool.Exec(ctx, `
		UPDATE chat_session_state
		SET current_data = $1, current_meta = $2, current_template = $3,
		    view_mode = $4, view_focused = $5, view_stack = $6,
		    step = $7, updated_at = NOW()
		WHERE session_id = $8
	`, dataJSON, metaJSON, templateJSON,
		state.View.Mode, viewFocusedJSON, viewStackJSON,
		state.Step, state.SessionID)
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
	actionJSON, _ := json.Marshal(delta.Action)
	resultJSON, _ := json.Marshal(delta.Result)
	var templateJSON []byte
	if delta.Template != nil {
		templateJSON, _ = json.Marshal(delta.Template)
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
	err := a.client.pool.QueryRow(ctx, `
		WITH next_step AS (
			SELECT COALESCE(MAX(step), 0) + 1 AS step
			FROM chat_session_deltas
			WHERE session_id = $1
		),
		inserted AS (
			INSERT INTO chat_session_deltas
				(session_id, step, trigger, source, actor_id, delta_type, path, action, result, template)
			SELECT $1, next_step.step, $2, $3, $4, $5, $6, $7, $8, $9
			FROM next_step
			RETURNING step
		)
		SELECT step FROM inserted
	`, sessionID, delta.Trigger,
		source, delta.ActorID, deltaType, delta.Path,
		actionJSON, resultJSON, templateJSON).Scan(&assignedStep)
	if err != nil {
		return 0, fmt.Errorf("add delta: %w", err)
	}

	// Update state.step to stay in sync
	_, _ = a.client.pool.Exec(ctx, `
		UPDATE chat_session_state SET step = $1, updated_at = NOW()
		WHERE session_id = $2
	`, assignedStep, sessionID)

	// Write back to delta struct so caller can see the assigned step
	delta.Step = assignedStep

	return assignedStep, nil
}

// GetDeltas retrieves all deltas for a session
func (a *StateAdapter) GetDeltas(ctx context.Context, sessionID string) ([]domain.Delta, error) {
	return a.GetDeltasSince(ctx, sessionID, 0)
}

// GetDeltasSince retrieves deltas from a specific step
func (a *StateAdapter) GetDeltasSince(ctx context.Context, sessionID string, fromStep int) ([]domain.Delta, error) {
	rows, err := a.client.pool.Query(ctx, `
		SELECT step, trigger, source, actor_id, delta_type, path, action, result, template, created_at
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
		SELECT step, trigger, source, actor_id, delta_type, path, action, result, template, created_at
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
		var source, actorID, deltaType, path *string

		err := rows.Scan(&d.Step, &trigger, &source, &actorID, &deltaType, &path,
			&actionJSON, &resultJSON, &templateJSON, &d.CreatedAt)
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
		json.Unmarshal(actionJSON, &d.Action)
		json.Unmarshal(resultJSON, &d.Result)
		if len(templateJSON) > 0 {
			json.Unmarshal(templateJSON, &d.Template)
		}

		deltas = append(deltas, d)
	}

	return deltas, nil
}

// PushView pushes a view snapshot onto the navigation stack
func (a *StateAdapter) PushView(ctx context.Context, sessionID string, snapshot *domain.ViewSnapshot) error {
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
		json.Unmarshal(viewStackJSON, &viewStack)
	}

	if len(viewStack) == 0 {
		return nil, nil // Empty stack
	}

	// Pop the last element
	lastSnapshot := viewStack[len(viewStack)-1]
	viewStack = viewStack[:len(viewStack)-1]

	// Update the stack
	newStackJSON, _ := json.Marshal(viewStack)
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
		json.Unmarshal(viewStackJSON, &viewStack)
	}

	return viewStack, nil
}
