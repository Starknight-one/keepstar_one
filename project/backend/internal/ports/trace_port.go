package ports

import (
	"context"

	"keepstar/internal/domain"
)

// TracePort records and retrieves pipeline execution traces
type TracePort interface {
	// Record saves a completed pipeline trace and prints it to console
	Record(ctx context.Context, trace *domain.PipelineTrace) error

	// List returns recent traces, newest first
	List(ctx context.Context, limit int) ([]*domain.PipelineTrace, error)

	// Get returns a single trace by ID
	Get(ctx context.Context, traceID string) (*domain.PipelineTrace, error)
}
