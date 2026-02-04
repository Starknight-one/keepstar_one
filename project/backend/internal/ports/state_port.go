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

	// AddDelta appends a new delta to the session history.
	// Step is auto-assigned (next sequential step for the session).
	// Returns the assigned step number. Delta.Step is ignored on input.
	AddDelta(ctx context.Context, sessionID string, delta *domain.Delta) (int, error)

	// GetDeltas retrieves all deltas for a session (for replay)
	GetDeltas(ctx context.Context, sessionID string) ([]domain.Delta, error)

	// GetDeltasSince retrieves deltas from a specific step (for partial replay)
	GetDeltasSince(ctx context.Context, sessionID string, fromStep int) ([]domain.Delta, error)

	// GetDeltasUntil retrieves deltas up to and including a specific step (for reconstruction)
	GetDeltasUntil(ctx context.Context, sessionID string, toStep int) ([]domain.Delta, error)

	// Zone writes — update zone columns + create delta atomically

	// UpdateData updates the data zone (products/services + meta) and creates a delta
	UpdateData(ctx context.Context, sessionID string, data domain.StateData, meta domain.StateMeta, info domain.DeltaInfo) (int, error)

	// UpdateTemplate updates the template zone and creates a delta
	UpdateTemplate(ctx context.Context, sessionID string, template map[string]interface{}, info domain.DeltaInfo) (int, error)

	// UpdateView updates the view zone (mode, focused, stack) and creates a delta
	UpdateView(ctx context.Context, sessionID string, view domain.ViewState, stack []domain.ViewSnapshot, info domain.DeltaInfo) (int, error)

	// AppendConversation updates conversation history (no delta — append-only for LLM cache)
	AppendConversation(ctx context.Context, sessionID string, messages []domain.LLMMessage) error

	// PushView pushes a view snapshot onto the navigation stack
	PushView(ctx context.Context, sessionID string, snapshot *domain.ViewSnapshot) error

	// PopView pops and returns the last view snapshot from the navigation stack
	PopView(ctx context.Context, sessionID string) (*domain.ViewSnapshot, error)

	// GetViewStack retrieves the entire view stack for a session
	GetViewStack(ctx context.Context, sessionID string) ([]domain.ViewSnapshot, error)
}
