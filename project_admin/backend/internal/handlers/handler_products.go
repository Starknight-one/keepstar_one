package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
	"keepstar-admin/internal/usecases"
)

type ProductsHandler struct {
	products *usecases.ProductsUseCase
	audit    ports.AuditPort // optional; nil means no audit writes
	log      *logger.Logger
}

func NewProductsHandler(products *usecases.ProductsUseCase, audit ports.AuditPort, log *logger.Logger) *ProductsHandler {
	return &ProductsHandler{products: products, audit: audit, log: log}
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

	// Snapshot before-state for audit diff. Best-effort — if Get fails, the
	// update still proceeds and we just lose the field-level diff.
	var before *domain.Product
	if h.audit != nil {
		before, _ = h.products.Get(ctx, tenantID, productID)
	}

	if err := h.products.Update(ctx, tenantID, productID, update); err != nil {
		h.log.FromContext(ctx).Error("product_update_failed", "product_id", productID, "error", err)
		writeError(w, http.StatusNotFound, "product not found")
		return
	}

	if h.audit != nil {
		changes := diffProductUpdate(before, update)
		if len(changes) > 0 {
			if err := h.audit.LogHuman(ctx, tenantID, UserID(ctx),
				domain.EntityKindListing, productID, domain.AuditActionUpdate, changes); err != nil {
				h.log.FromContext(ctx).Warn("audit_log_failed", "error", err)
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// diffProductUpdate computes a per-field old→new diff between the snapshot
// and the partial update. Fields absent from the update aren't recorded.
func diffProductUpdate(before *domain.Product, u domain.ProductUpdate) map[string]domain.FieldChange {
	changes := map[string]domain.FieldChange{}
	if before == nil {
		// No snapshot — record only the new values, audit gets one-sided info.
		if u.DisplayName != nil {
			changes["displayName"] = domain.FieldChange{New: *u.DisplayName}
		}
		if u.Price != nil {
			changes["price"] = domain.FieldChange{New: *u.Price}
		}
		if u.Stock != nil {
			changes["stock"] = domain.FieldChange{New: *u.Stock}
		}
		if u.Rating != nil {
			changes["rating"] = domain.FieldChange{New: *u.Rating}
		}
		if u.Description != nil {
			changes["description"] = domain.FieldChange{New: *u.Description}
		}
		return changes
	}
	if u.DisplayName != nil && before.DisplayName != *u.DisplayName {
		changes["displayName"] = domain.FieldChange{Old: before.DisplayName, New: *u.DisplayName}
	}
	if u.Price != nil && before.Price != *u.Price {
		changes["price"] = domain.FieldChange{Old: before.Price, New: *u.Price}
	}
	if u.Stock != nil && before.StockQuantity != *u.Stock {
		changes["stock"] = domain.FieldChange{Old: before.StockQuantity, New: *u.Stock}
	}
	if u.Rating != nil && before.Rating != *u.Rating {
		changes["rating"] = domain.FieldChange{Old: before.Rating, New: *u.Rating}
	}
	if u.Description != nil && before.Description != *u.Description {
		changes["description"] = domain.FieldChange{Old: before.Description, New: *u.Description}
	}
	return changes
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

	tenantID := TenantID(ctx)
	cats, err := h.products.GetCategories(ctx, tenantID)
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
