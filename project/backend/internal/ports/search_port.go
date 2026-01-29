package ports

import (
	"context"

	"keepstar/internal/domain"
)

// SearchPort defines the interface for product search
type SearchPort interface {
	// Search finds products by parameters
	Search(ctx context.Context, params map[string]any) ([]domain.Product, error)

	// GetByID returns a single product
	GetByID(ctx context.Context, id string) (*domain.Product, error)
}
