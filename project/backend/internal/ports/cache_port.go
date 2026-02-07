package ports

import (
	"context"

	"keepstar/internal/domain"
)

// CachePort defines the interface for caching
type CachePort interface {
	// GetSession returns a session by ID
	GetSession(ctx context.Context, id string) (*domain.Session, error)

	// SaveSession saves a session
	SaveSession(ctx context.Context, session *domain.Session) error

	// DeleteSession removes a session and all related data (state, deltas, traces)
	DeleteSession(ctx context.Context, id string) error

	// CacheProducts caches products for a session
	CacheProducts(ctx context.Context, sessionID string, products []domain.Product) error

	// GetCachedProducts returns cached products
	GetCachedProducts(ctx context.Context, sessionID string) ([]domain.Product, error)
}
