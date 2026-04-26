// Package handlers — public REST API v1 for tenant categories (M10).
package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

type APIv1CategoriesHandler struct {
	categories ports.CategoriesPort
	log        *logger.Logger
}

func NewAPIv1CategoriesHandler(categories ports.CategoriesPort, log *logger.Logger) *APIv1CategoriesHandler {
	return &APIv1CategoriesHandler{categories: categories, log: log}
}

func (h *APIv1CategoriesHandler) HandleCollection(w http.ResponseWriter, r *http.Request) {
	tenantID := TenantID(r.Context())
	switch r.Method {
	case http.MethodGet:
		cats, err := h.categories.ListTenantCategoriesWithCounts(r.Context(), tenantID)
		if err != nil {
			h.log.FromContext(r.Context()).Error("api_v1_cats_list_failed", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to list categories")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"categories": cats})
	case http.MethodPost:
		var p struct {
			Name       string `json:"name"`
			Slug       string `json:"slug"`
			Kind       string `json:"kind"`
			ParentID   string `json:"parentId"`
			ExternalID string `json:"externalId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if p.Name == "" || p.Slug == "" {
			writeError(w, http.StatusBadRequest, "name and slug required")
			return
		}
		tc := &domain.TenantCategory{
			TenantID:   tenantID,
			ParentID:   p.ParentID,
			ExternalID: p.ExternalID,
			Slug:       p.Slug,
			Name:       p.Name,
			Kind:       domain.TenantCategoryKind(p.Kind),
		}
		id, err := h.categories.UpsertTenantCategory(r.Context(), tc)
		if err != nil {
			h.log.FromContext(r.Context()).Error("api_v1_cats_create_failed", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to create")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"id": id})
	default:
		writeError(w, http.StatusMethodNotAllowed, "GET or POST only")
	}
}

func (h *APIv1CategoriesHandler) HandleResource(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/categories/")
	id = strings.TrimSuffix(id, "/")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusBadRequest, "missing id")
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var p struct {
			Name     string `json:"name"`
			Slug     string `json:"slug"`
			Kind     string `json:"kind"`
			ParentID string `json:"parentId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		tc := &domain.TenantCategory{
			ID:       id,
			TenantID: TenantID(r.Context()),
			ParentID: p.ParentID,
			Slug:     p.Slug,
			Name:     p.Name,
			Kind:     domain.TenantCategoryKind(p.Kind),
		}
		if _, err := h.categories.UpsertTenantCategory(r.Context(), tc); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case http.MethodDelete:
		if err := h.categories.DeleteTenantCategory(r.Context(), TenantID(r.Context()), id); err != nil {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, "PATCH or DELETE only")
	}
}
