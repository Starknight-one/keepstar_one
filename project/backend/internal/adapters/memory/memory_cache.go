package memory

import (
	"context"
	"sync"

	"keepstar/internal/domain"
)

// Cache implements ports.CachePort using in-memory storage
type Cache struct {
	mu       sync.RWMutex
	sessions map[string]*domain.Session
	products map[string][]domain.Product
}

// NewCache creates a new in-memory cache
func NewCache() *Cache {
	return &Cache{
		sessions: make(map[string]*domain.Session),
		products: make(map[string][]domain.Product),
	}
}

// GetSession implements CachePort.GetSession
func (c *Cache) GetSession(ctx context.Context, id string) (*domain.Session, error) {
	// TODO: implement
	return nil, nil
}

// SaveSession implements CachePort.SaveSession
func (c *Cache) SaveSession(ctx context.Context, session *domain.Session) error {
	// TODO: implement
	return nil
}

// CacheProducts implements CachePort.CacheProducts
func (c *Cache) CacheProducts(ctx context.Context, sessionID string, products []domain.Product) error {
	// TODO: implement
	return nil
}

// GetCachedProducts implements CachePort.GetCachedProducts
func (c *Cache) GetCachedProducts(ctx context.Context, sessionID string) ([]domain.Product, error) {
	// TODO: implement
	return nil, nil
}
