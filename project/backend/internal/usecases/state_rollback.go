package usecases

import (
	"context"
	"fmt"
	"time"

	"keepstar/internal/domain"
	"keepstar/internal/ports"
)

// RollbackUseCase handles rolling back state to a previous step
type RollbackUseCase struct {
	statePort     ports.StatePort
	reconstructUC *ReconstructStateUseCase
}

// NewRollbackUseCase creates a new rollback use case
func NewRollbackUseCase(statePort ports.StatePort) *RollbackUseCase {
	return &RollbackUseCase{
		statePort:     statePort,
		reconstructUC: NewReconstructStateUseCase(statePort),
	}
}

// RollbackRequest is the input for rollback operation
type RollbackRequest struct {
	SessionID string
	ToStep    int               // Step to rollback to
	Source    domain.DeltaSource // Who initiated (user/system)
	ActorID   string            // Actor identifier (e.g., "user_back", "system_cleanup")
}

// RollbackResponse is the output from rollback
type RollbackResponse struct {
	State       *domain.SessionState
	RolledBack  int // Number of steps rolled back
	FromStep    int // Original step before rollback
	ToStep      int // New step after rollback
	RollbackDelta *domain.Delta // The delta recording the rollback
}

// Execute rolls back state to a previous step
func (uc *RollbackUseCase) Execute(ctx context.Context, req RollbackRequest) (*RollbackResponse, error) {
	// Get current state to know what we're rolling back from
	currentState, err := uc.statePort.GetState(ctx, req.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get current state: %w", err)
	}

	// Validate rollback target
	if req.ToStep < 0 {
		return nil, fmt.Errorf("invalid rollback step: %d", req.ToStep)
	}
	if req.ToStep >= currentState.Step {
		return nil, fmt.Errorf("cannot rollback forward: current step %d, target %d", currentState.Step, req.ToStep)
	}

	// Reconstruct state at target step
	reconstructResp, err := uc.reconstructUC.Execute(ctx, ReconstructRequest{
		SessionID: req.SessionID,
		ToStep:    req.ToStep,
	})
	if err != nil {
		return nil, fmt.Errorf("reconstruct state at step %d: %w", req.ToStep, err)
	}

	// Create rollback delta to record what was undone
	rollbackDelta := &domain.Delta{
		Step:      currentState.Step + 1,
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

	// Save the rollback delta
	if err := uc.statePort.AddDelta(ctx, req.SessionID, rollbackDelta); err != nil {
		return nil, fmt.Errorf("add rollback delta: %w", err)
	}

	// Update the current state with reconstructed state
	// Note: We keep the new step number (rollbackDelta.Step)
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
