package usecases

import (
	"context"

	"keepstar/internal/domain"
	"keepstar/internal/ports"
)

// ExecuteSearchUseCase handles product search
type ExecuteSearchUseCase struct {
	search ports.SearchPort
	cache  ports.CachePort
}

// NewExecuteSearchUseCase creates a new use case
func NewExecuteSearchUseCase(search ports.SearchPort, cache ports.CachePort) *ExecuteSearchUseCase {
	return &ExecuteSearchUseCase{
		search: search,
		cache:  cache,
	}
}

// Execute runs the product search
func (uc *ExecuteSearchUseCase) Execute(ctx context.Context, sessionID string, params map[string]any) ([]domain.Product, error) {
	// TODO: implement
	return nil, nil
}
