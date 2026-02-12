package handlers

import (
	"encoding/json"
	"net/http"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/usecases"
)

type StockHandler struct {
	stock *usecases.StockUseCase
	log   *logger.Logger
}

func NewStockHandler(stock *usecases.StockUseCase, log *logger.Logger) *StockHandler {
	return &StockHandler{stock: stock, log: log}
}

func (h *StockHandler) HandleBulkUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("handler.stock_bulk_update")
		defer endSpan()
	}

	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}

	tenantID := TenantID(ctx)

	var req usecases.BulkStockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if len(req.Items) == 0 {
		writeError(w, http.StatusBadRequest, "items required")
		return
	}

	reqLog := h.log.FromContext(ctx)

	updated, err := h.stock.BulkUpdate(ctx, tenantID, req)
	if err != nil {
		reqLog.Error("stock_bulk_update_failed", "items", len(req.Items), "error", err)
		writeError(w, http.StatusInternalServerError, "stock update failed")
		return
	}

	reqLog.Info("stock_bulk_updated", "items", len(req.Items), "affected", updated)
	writeJSON(w, http.StatusOK, map[string]int{"updated": updated})
}
