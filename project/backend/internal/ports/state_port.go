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

	// GetDeltasUntil retrieves deltas up to and including a specific step (for reconstruction)
	GetDeltasUntil(ctx context.Context, sessionID string, toStep int) ([]domain.Delta, error)

	// PushView pushes a view snapshot onto the navigation stack
	PushView(ctx context.Context, sessionID string, snapshot *domain.ViewSnapshot) error

	// PopView pops and returns the last view snapshot from the navigation stack
	PopView(ctx context.Context, sessionID string) (*domain.ViewSnapshot, error)

	// GetViewStack retrieves the entire view stack for a session
	GetViewStack(ctx context.Context, sessionID string) ([]domain.ViewSnapshot, error)
}
