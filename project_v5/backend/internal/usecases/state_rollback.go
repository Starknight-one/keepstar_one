package usecases

import (
	"context"
	"fmt"
	"time"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// RollbackUseCase pops the state back to a prior step by reconstructing from
// the delta stream and writing a rollback delta to record what happened.
type RollbackUseCase struct {
	statePort     ports.StatePort
	reconstructUC *ReconstructStateUseCase
}

func NewRollbackUseCase(statePort ports.StatePort) *RollbackUseCase {
	return &RollbackUseCase{
		statePort:     statePort,
		reconstructUC: NewReconstructStateUseCase(statePort),
	}
}

type RollbackRequest struct {
	SessionID string
	ToStep    int
	Source    domain.DeltaSource
	ActorID   string
}

type RollbackResponse struct {
	State         *domain.SessionState
	RolledBack    int
	FromStep      int
	ToStep        int
	RollbackDelta *domain.Delta
}

func (uc *RollbackUseCase) Execute(ctx context.Context, req RollbackRequest) (*RollbackResponse, error) {
	currentState, err := uc.statePort.GetState(ctx, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get current state: %w", err)
	}

	if req.ToStep < 0 {
		return nil, fmt.Errorf("invalid rollback step: %d", req.ToStep)
	}
	if req.ToStep >= currentState.Step {
		return nil, fmt.Errorf("cannot rollback forward: current step %d, target %d", currentState.Step, req.ToStep)
	}

	reconstructResp, err := uc.reconstructUC.Execute(ctx, ReconstructRequest{
		SessionID: req.SessionID,
		ToStep:    req.ToStep,
	})
	if err != nil {
		return nil, fmt.Errorf("reconstruct state at step %d: %w", req.ToStep, err)
	}

	rollbackDelta := &domain.Delta{
		Trigger:   domain.TriggerSystem,
		Source:    req.Source,
		ActorID:   req.ActorID,
		DeltaType: domain.DeltaTypeRollback,
		Path:      "state",
		Action: domain.Action{
			Type: domain.ActionRollback,
			Params: map[string]interface{}{
				"from_step": currentState.Step,
				"to_step":   req.ToStep,
			},
		},
		Result: domain.ResultMeta{
			Count:  reconstructResp.State.Current.Meta.Count,
			Fields: reconstructResp.State.Current.Meta.Fields,
		},
		CreatedAt: time.Now(),
	}

	if _, err := uc.statePort.AddDelta(ctx, req.SessionID, rollbackDelta); err != nil {
		return nil, fmt.Errorf("add rollback delta: %w", err)
	}

	reconstructResp.State.ID = currentState.ID
	reconstructResp.State.SessionID = req.SessionID
	reconstructResp.State.Step = rollbackDelta.Step
	reconstructResp.State.UpdatedAt = time.Now()

	if err := uc.statePort.UpdateState(ctx, reconstructResp.State); err != nil {
		return nil, fmt.Errorf("update state: %w", err)
	}

	return &RollbackResponse{
		State:         reconstructResp.State,
		RolledBack:    currentState.Step - req.ToStep,
		FromStep:      currentState.Step,
		ToStep:        req.ToStep,
		RollbackDelta: rollbackDelta,
	}, nil
}
