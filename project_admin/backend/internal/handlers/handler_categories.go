// Package handlers — categories CRUD + M:N listing↔category mapping for the
// admin tenant UI. Spec: docs/New features/admin_catalog_design_2026-04-23.md
// §3.2 / §6.4.
//
// All endpoints under /admin/api/categories/* require JWT auth + tenant scope
// (the AuthMiddleware populates ctx.tenant_id). Master categories are
// read-only here — promotion lives in the curator service (M11).
package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

type CategoriesHandler struct {
	categories ports.CategoriesPort
	audit      ports.AuditPort // optional
	log        *logger.Logger
}

func NewCategoriesHandler(categories ports.CategoriesPort, audit ports.AuditPort, log *logger.Logger) *CategoriesHandler {
	return &CategoriesHandler{categories: categories, audit: audit, log: log}
}

// GET /admin/api/categories/tenant — tenant tree with product counts.
func (h *CategoriesHandler) HandleListTenant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	cats, err := h.categories.ListTenantCategoriesWithCounts(r.Context(), TenantID(r.Context()))
	if err != nil {
		h.log.FromContext(r.Context()).Error("list_tenant_categories_failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list tenant categories")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": cats})
}

type tenantCategoryPayload struct {
	ParentID   string `json:"parentId,omitempty"`
	ExternalID string `json:"externalId,omitempty"`
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	Kind       string `json:"kind,omitempty"`
}

// POST /admin/api/categories/tenant — create a new tenant category.
func (h *CategoriesHandler) HandleCreateTenant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var p tenantCategoryPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if p.Name == "" || p.Slug == "" {
		writeError(w, http.StatusBadRequest, "name and slug required")
		return
	}
	tc := &domain.TenantCategory{
		TenantID:   TenantID(r.Context()),
		ParentID:   p.ParentID,
		ExternalID: p.ExternalID,
		Slug:       p.Slug,
		Name:       p.Name,
		Kind:       domain.TenantCategoryKind(p.Kind),
	}
	id, err := h.categories.UpsertTenantCategory(r.Context(), tc)
	if err != nil {
		h.log.FromContext(r.Context()).Error("create_tenant_category_failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create category")
		return
	}
	if h.audit != nil {
		_ = h.audit.LogHuman(r.Context(), TenantID(r.Context()), UserID(r.Context()),
			domain.EntityKindCategory, id, domain.AuditActionCreate,
			map[string]domain.FieldChange{"name": {New: p.Name}, "kind": {New: p.Kind}})
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id})
}

// PATCH /admin/api/categories/tenant/{id} — rename/move/relabel kind.
func (h *CategoriesHandler) HandleUpdateTenant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		writeError(w, http.StatusMethodNotAllowed, "PATCH only")
		return
	}
	id := extractID(r.URL.Path, "/admin/api/categories/tenant/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing id")
		return
	}
	var p tenantCategoryPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	tc := &domain.TenantCategory{
		ID:         id,
		TenantID:   TenantID(r.Context()),
		ParentID:   p.ParentID,
		ExternalID: p.ExternalID,
		Slug:       p.Slug,
		Name:       p.Name,
		Kind:       domain.TenantCategoryKind(p.Kind),
	}
	// UpsertTenantCategory upserts by external_id when present, otherwise inserts.
	// For an in-place edit we rely on the editor passing the original external_id.
	// If the caller doesn't have one (manual category), they should send a slug
	// change which is also fine. For MVP the simplest path is to write a fresh
	// row; harvester later reconciles. TODO: extend port with explicit Update by id.
	if _, err := h.categories.UpsertTenantCategory(r.Context(), tc); err != nil {
		h.log.FromContext(r.Context()).Error("update_tenant_category_failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to update category")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// DELETE /admin/api/categories/tenant/{id} — delete (children re-parented to NULL).
func (h *CategoriesHandler) HandleDeleteTenant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "DELETE only")
		return
	}
	id := extractID(r.URL.Path, "/admin/api/categories/tenant/")
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing id")
		return
	}
	if err := h.categories.DeleteTenantCategory(r.Context(), TenantID(r.Context()), id); err != nil {
		h.log.FromContext(r.Context()).Error("delete_tenant_category_failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to delete category")
		return
	}
	if h.audit != nil {
		_ = h.audit.LogHuman(r.Context(), TenantID(r.Context()), UserID(r.Context()),
			domain.EntityKindCategory, id, domain.AuditActionDelete, nil)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /admin/api/categories/master?vertical= — read-only master taxonomy.
func (h *CategoriesHandler) HandleListMaster(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	vertical := r.URL.Query().Get("vertical")
	cats, err := h.categories.ListMasterCategories(r.Context(), vertical)
	if err != nil {
		h.log.FromContext(r.Context()).Error("list_master_categories_failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list master categories")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": cats})
}

type categoryMappingPayload struct {
	TenantCategoryID string `json:"tenantCategoryId"`
	MasterCategoryID string `json:"masterCategoryId,omitempty"`
	Confidence       string `json:"confidence,omitempty"`
}

// POST /admin/api/categories/mapping — set tenant_category → master_category.
func (h *CategoriesHandler) HandleSetMapping(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var p categoryMappingPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if p.TenantCategoryID == "" {
		writeError(w, http.StatusBadRequest, "tenantCategoryId required")
		return
	}
	m := &domain.CategoryMapping{
		TenantCategoryID: p.TenantCategoryID,
		MasterCategoryID: p.MasterCategoryID,
		Confidence:       p.Confidence,
		MappedBy:         "tenant",
	}
	if err := h.categories.SetCategoryMapping(r.Context(), m); err != nil {
		h.log.FromContext(r.Context()).Error("set_category_mapping_failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to set mapping")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GET /admin/api/products/{id}/categories — list categories a listing belongs to.
// PUT /admin/api/products/{id}/categories — replace links: { categoryIds: [...] }.
func (h *CategoriesHandler) HandleListingCategories(w http.ResponseWriter, r *http.Request) {
	listingID := extractListingID(r.URL.Path)
	if listingID == "" {
		writeError(w, http.StatusBadRequest, "missing listing id")
		return
	}
	switch r.Method {
	case http.MethodGet:
		cats, err := h.categories.ListListingCategories(r.Context(), listingID)
		if err != nil {
			h.log.FromContext(r.Context()).Error("list_listing_categories_failed", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to list categories")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"categories": cats})
	case http.MethodPut:
		var body struct {
			CategoryIDs []string `json:"categoryIds"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if err := h.categories.LinkListingToCategories(r.Context(), listingID, body.CategoryIDs); err != nil {
			h.log.FromContext(r.Context()).Error("link_listing_categories_failed", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to link categories")
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	default:
		writeError(w, http.StatusMethodNotAllowed, "GET or PUT only")
	}
}

// extractListingID parses /admin/api/products/{id}/categories.
func extractListingID(path string) string {
	const prefix = "/admin/api/products/"
	const suffix = "/categories"
	rest := strings.TrimPrefix(path, prefix)
	rest = strings.TrimSuffix(rest, suffix)
	rest = strings.TrimSuffix(rest, "/")
	if strings.Contains(rest, "/") {
		return ""
	}
	return rest
}
