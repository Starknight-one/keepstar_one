package usecases

import (
	"context"

	"keepstar/internal/domain"
	"keepstar/internal/ports"
)

// ComposeWidgetsUseCase handles Agent 2: widget composition
type ComposeWidgetsUseCase struct {
	llm ports.LLMPort
}

// NewComposeWidgetsUseCase creates a new use case
func NewComposeWidgetsUseCase(llm ports.LLMPort) *ComposeWidgetsUseCase {
	return &ComposeWidgetsUseCase{llm: llm}
}

// Execute runs the widget composition
func (uc *ComposeWidgetsUseCase) Execute(ctx context.Context, query string, products []domain.Product) ([]domain.Widget, *domain.Formation, error) {
	// TODO: implement
	return nil, nil, nil
}
