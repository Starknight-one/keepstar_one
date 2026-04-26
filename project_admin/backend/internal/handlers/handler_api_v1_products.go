// Package handlers — public REST API v1 for products (M10).
//
// X-API-Key auth via APIKeyMiddleware. Tenant scope is taken from the
// resolved key, not from the request path. Bulk push (POST /products) is
// intentionally a tenant-only listing update — full master pipeline (match
// cascade, candidates, junk detection) is harvester territory and deferred
// to M4d.
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

type APIv1ProductsHandler struct {
	products *usecases.ProductsUseCase
	log      *logger.Logger
}

func NewAPIv1ProductsHandler(products *usecases.ProductsUseCase, log *logger.Logger) *APIv1ProductsHandler {
	return &APIv1ProductsHandler{products: products, log: log}
}

// GET /api/v1/products?limit=50&offset=0&search=&categoryId=
func (h *APIv1ProductsHandler) HandleCollection(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.list(w, r)
	case http.MethodPost:
		writeError(w, http.StatusNotImplemented, "bulk push not available until harvester (M4d) ships")
	default:
		writeError(w, http.StatusMethodNotAllowed, "GET or POST only")
	}
}

func (h *APIv1ProductsHandler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset, _ := strconv.Atoi(q.Get("offset"))
	if offset < 0 {
		offset = 0
	}
	filter := domain.AdminProductFilter{
		Search:     q.Get("search"),
		CategoryID: q.Get("categoryId"),
		Limit:      limit,
		Offset:     offset,
	}
	products, total, err := h.products.List(r.Context(), TenantID(r.Context()), filter)
	if err != nil {
		h.log.FromContext(r.Context()).Error("api_v1_products_list_failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list products")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"products": products,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
	})
}

// /api/v1/products/{id} — GET (single), PATCH (update), DELETE (soft delete).
func (h *APIv1ProductsHandler) HandleResource(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/products/")
	id = strings.TrimSuffix(id, "/")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusBadRequest, "missing id")
		return
	}
	tenantID := TenantID(r.Context())
	switch r.Method {
	case http.MethodGet:
		p, err := h.products.Get(r.Context(), tenantID, id)
		if err != nil {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeJSON(w, http.StatusOK, p)
	case http.MethodPatch:
		var update domain.ProductUpdate
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if err := h.products.Update(r.Context(), tenantID, id, update); err != nil {
			h.log.FromContext(r.Context()).Error("api_v1_product_update_failed", "error", err)
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case http.MethodDelete:
		// Soft delete via UpdateProduct stub: API doesn't expose deleted_at
		// directly today. Mark stock=0 + return 501 with TODO. Once a port
		// method `SoftDelete` lands on AdminCatalogPort we'll wire it here.
		writeError(w, http.StatusNotImplemented, "soft delete not yet wired in admin port (M4 polish)")
	default:
		writeError(w, http.StatusMethodNotAllowed, "GET / PATCH / DELETE only")
	}
}
