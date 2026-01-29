package usecases

import (
	"context"

	"keepstar/internal/ports"
)

// AnalyzeQueryUseCase handles Agent 1: query analysis
type AnalyzeQueryUseCase struct {
	llm ports.LLMPort
}

// NewAnalyzeQueryUseCase creates a new use case
func NewAnalyzeQueryUseCase(llm ports.LLMPort) *AnalyzeQueryUseCase {
	return &AnalyzeQueryUseCase{llm: llm}
}

// Execute runs the query analysis
func (uc *AnalyzeQueryUseCase) Execute(ctx context.Context, query string, sessionID string) (*ports.AnalyzeQueryResponse, error) {
	// TODO: implement
	return nil, nil
}
