package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/usecases"
)

type ProductsHandler struct {
	products *usecases.ProductsUseCase
	log      *logger.Logger
}

func NewProductsHandler(products *usecases.ProductsUseCase, log *logger.Logger) *ProductsHandler {
	return &ProductsHandler{products: products, log: log}
}

func (h *ProductsHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("handler.products_list")
		defer endSpan()
	}

	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}

	tenantID := TenantID(ctx)
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	if limit <= 0 {
		limit = 25
	}

	filter := domain.AdminProductFilter{
		Search:     q.Get("search"),
		CategoryID: q.Get("categoryId"),
		Limit:      limit,
		Offset:     offset,
	}

	products, total, err := h.products.List(ctx, tenantID, filter)
	if err != nil {
		h.log.FromContext(ctx).Error("products_list_failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list products")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"products": products,
		"total":    total,
	})
}

func (h *ProductsHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("handler.products_get")
		defer endSpan()
	}

	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}

	tenantID := TenantID(ctx)
	productID := extractID(r.URL.Path, "/admin/api/products/")

	product, err := h.products.Get(ctx, tenantID, productID)
	if err != nil {
		writeError(w, http.StatusNotFound, "product not found")
		return
	}

	writeJSON(w, http.StatusOK, product)
}

func (h *ProductsHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("handler.products_update")
		defer endSpan()
	}

	if r.Method != http.MethodPut {
		writeError(w, http.StatusMethodNotAllowed, "PUT only")
		return
	}

	tenantID := TenantID(ctx)
	productID := extractID(r.URL.Path, "/admin/api/products/")

	var update domain.ProductUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if err := h.products.Update(ctx, tenantID, productID, update); err != nil {
		h.log.FromContext(ctx).Error("product_update_failed", "product_id", productID, "error", err)
		writeError(w, http.StatusNotFound, "product not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *ProductsHandler) HandleCategories(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("handler.categories_list")
		defer endSpan()
	}

	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}

	cats, err := h.products.GetCategories(ctx)
	if err != nil {
		h.log.FromContext(ctx).Error("categories_list_failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list categories")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"categories": cats})
}

func extractID(path, prefix string) string {
	id := strings.TrimPrefix(path, prefix)
	id = strings.TrimSuffix(id, "/")
	return id
}
