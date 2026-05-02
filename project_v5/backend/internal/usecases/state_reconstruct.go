package usecases

import (
	"context"
	"fmt"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// ReconstructStateUseCase replays the delta stream up to a target step to
// rebuild the materialized state. Used by rollback and by debug tooling.
type ReconstructStateUseCase struct {
	statePort ports.StatePort
}

func NewReconstructStateUseCase(statePort ports.StatePort) *ReconstructStateUseCase {
	return &ReconstructStateUseCase{statePort: statePort}
}

type ReconstructRequest struct {
	SessionID string
	ToStep    int
}

type ReconstructResponse struct {
	State      *domain.SessionState
	Deltas     []domain.Delta
	StepNow    int
	DeltaCount int
}

func (uc *ReconstructStateUseCase) Execute(ctx context.Context, req ReconstructRequest) (*ReconstructResponse, error) {
	deltas, err := uc.statePort.GetDeltasUntil(ctx, req.SessionID, req.ToStep)
	if err != nil {
		return nil, fmt.Errorf("get deltas until step %d: %w", req.ToStep, err)
	}

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
		View:      domain.ViewState{Mode: domain.ViewModeGrid},
		ViewStack: []domain.ViewSnapshot{},
		Step:      0,
	}

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

// applyDelta applies one delta to the state. Ported verbatim from V4 with the
// same simplified semantics — push/pop/rollback/remove cases are stubs that
// only update Step + meta. Improving applyDelta is a chunk-7 task; what's
// important here is determinism: replay 1..N twice and you get the same
// state.
func applyDelta(state *domain.SessionState, delta domain.Delta) *domain.SessionState {
	state.Step = delta.Step

	switch delta.DeltaType {
	case domain.DeltaTypeAdd:
		state.Current.Meta.Count = delta.Result.Count
		state.Current.Meta.Fields = delta.Result.Fields
		if delta.Result.Aliases != nil {
			for k, v := range delta.Result.Aliases {
				state.Current.Meta.Aliases[k] = v
			}
		}

	case domain.DeltaTypeUpdate:
		state.Current.Meta.Count = delta.Result.Count
		state.Current.Meta.Fields = delta.Result.Fields

	case domain.DeltaTypePush:
		// Push handled via ViewStack zone-write; payload (if any) lives in
		// delta.Template.

	case domain.DeltaTypePop:
		// Pop handled via ViewStack zone-write.

	case domain.DeltaTypeRollback:
		// Rollback target step is in delta.Action.Params.

	case domain.DeltaTypeRemove:
		// Remove not modeled in this simplified replay.
	}

	if delta.Template != nil {
		state.Current.Template = delta.Template
	}

	return state
}
