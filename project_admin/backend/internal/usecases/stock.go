package usecases

import (
	"context"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/ports"
)

type StockUseCase struct {
	catalog ports.AdminCatalogPort
}

func NewStockUseCase(catalog ports.AdminCatalogPort) *StockUseCase {
	return &StockUseCase{catalog: catalog}
}

type BulkStockRequest struct {
	Items []domain.StockUpdate `json:"items"`
}

func (uc *StockUseCase) BulkUpdate(ctx context.Context, tenantID string, req BulkStockRequest) (int, error) {
	return uc.catalog.BulkUpdateStock(ctx, tenantID, req.Items)
}
