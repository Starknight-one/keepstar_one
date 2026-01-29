package jsonstore

import (
	"context"

	"keepstar/internal/domain"
)

// ProductStore implements ports.SearchPort using JSON file
type ProductStore struct {
	filePath string
	products []domain.Product
}

// NewProductStore creates a new JSON product store
func NewProductStore(filePath string) *ProductStore {
	return &ProductStore{
		filePath: filePath,
	}
}

// Search implements SearchPort.Search
func (s *ProductStore) Search(ctx context.Context, params map[string]any) ([]domain.Product, error) {
	// TODO: implement
	return nil, nil
}

// GetByID implements SearchPort.GetByID
func (s *ProductStore) GetByID(ctx context.Context, id string) (*domain.Product, error) {
	// TODO: implement
	return nil, nil
}
