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
