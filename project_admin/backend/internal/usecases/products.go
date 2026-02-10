package usecases

import (
	"context"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/ports"
)

type ProductsUseCase struct {
	catalog ports.AdminCatalogPort
}

func NewProductsUseCase(catalog ports.AdminCatalogPort) *ProductsUseCase {
	return &ProductsUseCase{catalog: catalog}
}

func (uc *ProductsUseCase) List(ctx context.Context, tenantID string, filter domain.AdminProductFilter) ([]domain.Product, int, error) {
	return uc.catalog.ListProducts(ctx, tenantID, filter)
}

func (uc *ProductsUseCase) Get(ctx context.Context, tenantID string, productID string) (*domain.Product, error) {
	return uc.catalog.GetProduct(ctx, tenantID, productID)
}

func (uc *ProductsUseCase) Update(ctx context.Context, tenantID string, productID string, update domain.ProductUpdate) error {
	return uc.catalog.UpdateProduct(ctx, tenantID, productID, update)
}

func (uc *ProductsUseCase) GetCategories(ctx context.Context) ([]domain.Category, error) {
	return uc.catalog.GetCategories(ctx)
}
