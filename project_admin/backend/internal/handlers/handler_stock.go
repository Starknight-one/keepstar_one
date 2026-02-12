package handlers

import (
	"encoding/json"
	"net/http"

	"keepstar-admin/internal/usecases"
)

type StockHandler struct {
	stock *usecases.StockUseCase
}

func NewStockHandler(stock *usecases.StockUseCase) *StockHandler {
	return &StockHandler{stock: stock}
}

func (h *StockHandler) HandleBulkUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}

	tenantID := TenantID(r.Context())

	var req usecases.BulkStockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if len(req.Items) == 0 {
		writeError(w, http.StatusBadRequest, "items required")
		return
	}

	updated, err := h.stock.BulkUpdate(r.Context(), tenantID, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "stock update failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]int{"updated": updated})
}
