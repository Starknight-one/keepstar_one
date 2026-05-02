package ports

import (
	"context"

	"keepstar_v5/internal/domain"
)

// StatePort defines operations for session state persistence. Mirrors V4
// surface 1:1; V5 only changes the Template zone content (scene-graph
// engine.Document JSON instead of Formation).
type StatePort interface {
	// Lifecycle
	CreateState(ctx context.Context, sessionID string) (*domain.SessionState, error)
	GetState(ctx context.Context, sessionID string) (*domain.SessionState, error)
	UpdateState(ctx context.Context, state *domain.SessionState) error

	// Delta stream — append-only history. AddDelta auto-assigns Step
	// monotonically per session and returns the assigned step. Delta.Step is
	// ignored on input.
	AddDelta(ctx context.Context, sessionID string, delta *domain.Delta) (int, error)
	GetDeltas(ctx context.Context, sessionID string) ([]domain.Delta, error)
	GetDeltasSince(ctx context.Context, sessionID string, fromStep int) ([]domain.Delta, error)
	GetDeltasUntil(ctx context.Context, sessionID string, toStep int) ([]domain.Delta, error)

	// Zone writes — update the zone column AND append a delta. Returns the
	// assigned step.
	UpdateData(ctx context.Context, sessionID string, data domain.StateData, meta domain.StateMeta, info domain.DeltaInfo) (int, error)
	UpdateTemplate(ctx context.Context, sessionID string, template map[string]interface{}, info domain.DeltaInfo) (int, error)
	UpdateView(ctx context.Context, sessionID string, view domain.ViewState, stack []domain.ViewSnapshot, info domain.DeltaInfo) (int, error)
	UpdateActions(ctx context.Context, sessionID string, actions domain.StateActions, info domain.DeltaInfo) (int, error)

	// Append-only history zones (no delta written — used for LLM cache).
	AppendConversation(ctx context.Context, sessionID string, messages []domain.LLMMessage) error
	AppendAgent2History(ctx context.Context, sessionID string, messages []domain.LLMMessage) error

	// View stack — explicit push/pop semantics.
	PushView(ctx context.Context, sessionID string, snapshot *domain.ViewSnapshot) error
	PopView(ctx context.Context, sessionID string) (*domain.ViewSnapshot, error)
	GetViewStack(ctx context.Context, sessionID string) ([]domain.ViewSnapshot, error)
}
