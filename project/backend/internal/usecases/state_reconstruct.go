package usecases

import (
	"context"
	"fmt"

	"keepstar/internal/domain"
	"keepstar/internal/ports"
)

// ReconstructStateUseCase reconstructs session state at any given step
type ReconstructStateUseCase struct {
	statePort ports.StatePort
}

// NewReconstructStateUseCase creates a new reconstruct use case
func NewReconstructStateUseCase(statePort ports.StatePort) *ReconstructStateUseCase {
	return &ReconstructStateUseCase{statePort: statePort}
}

// ReconstructRequest is the input for state reconstruction
type ReconstructRequest struct {
	SessionID string
	ToStep    int // Which step to reconstruct to
}

// ReconstructResponse is the output from reconstruction
type ReconstructResponse struct {
	State      *domain.SessionState
	Deltas     []domain.Delta // Deltas that were applied
	StepNow    int            // Final step after reconstruction
	DeltaCount int            // Number of deltas applied
}

// Execute reconstructs state at a specific step by replaying deltas
func (uc *ReconstructStateUseCase) Execute(ctx context.Context, req ReconstructRequest) (*ReconstructResponse, error) {
	// Get deltas up to the target step
	deltas, err := uc.statePort.GetDeltasUntil(ctx, req.SessionID, req.ToStep)
	if err != nil {
		return nil, fmt.Errorf("get deltas until step %d: %w", req.ToStep, err)
	}

	// Build base state (step 0)
	state := &domain.SessionState{
		SessionID: req.SessionID,
		Current: domain.StateCurrent{
			Data: domain.StateData{
				Products: []domain.Product{},
				Services: []domain.Service{},
			},
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
	}

	// Apply each delta sequentially
	for _, delta := range deltas {
		state = applyDelta(state, delta)
	}

	return &ReconstructResponse{
		State:      state,
		Deltas:     deltas,
		StepNow:    state.Step,
		DeltaCount: len(deltas),
	}, nil
}

// applyDelta applies a single delta to the state
// This is a simplified implementation - in production, this would
// need to handle all delta types and paths properly
func applyDelta(state *domain.SessionState, delta domain.Delta) *domain.SessionState {
	state.Step = delta.Step

	switch delta.DeltaType {
	case domain.DeltaTypeAdd:
		// For add deltas, update counts from result meta
		state.Current.Meta.Count = delta.Result.Count
		state.Current.Meta.Fields = delta.Result.Fields
		if delta.Result.Aliases != nil {
			for k, v := range delta.Result.Aliases {
				state.Current.Meta.Aliases[k] = v
			}
		}

	case domain.DeltaTypeUpdate:
		// Update meta counts
		state.Current.Meta.Count = delta.Result.Count
		state.Current.Meta.Fields = delta.Result.Fields

	case domain.DeltaTypePush:
		// Push is handled via ViewStack operations
		// The actual snapshot would be in delta.Template
		if delta.Template != nil {
			// ViewStack push recorded in delta
		}

	case domain.DeltaTypePop:
		// Pop is handled via ViewStack operations

	case domain.DeltaTypeRollback:
		// Rollback restores to a previous state
		// The target step is in delta.Action.Params

	case domain.DeltaTypeRemove:
		// Remove data from state
	}

	// Apply template if present
	if delta.Template != nil {
		state.Current.Template = delta.Template
	}

	return state
}
