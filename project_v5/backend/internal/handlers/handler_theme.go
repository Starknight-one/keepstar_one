package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"keepstar_v5/internal/domain"
	"keepstar_v5/internal/ports"
)

// resolveThemeMap resolves the tenant's design-system theme and returns it as
// the map shape that rides on the rendered Document as the top-level `theme`
// field ({"tokens":{...},"isDefault":bool}). It is FAIL-OPEN: on any lookup or
// marshal error it returns the canonical defaults (IsDefault=true) so a theme
// hiccup never breaks the chat/preview render — worst case the widget injects
// the same vars the static widget.css already hardcodes (zero visual change).
//
// This is the SINGLE attach helper used by every document-assembly anchor in
// the Phase 1 contract (chat non-stream + chat SSE + internal preview) so the
// behaviour is identical across paths.
func resolveThemeMap(ctx context.Context, themes ports.ThemePort, tenantSlugOrID string, log *slog.Logger) map[string]interface{} {
	theme := domain.DefaultTheme()
	if themes != nil && tenantSlugOrID != "" {
		if t, err := themes.GetTheme(ctx, tenantSlugOrID); err != nil {
			if log != nil {
				log.Warn("theme resolve failed; using defaults", "tenant", tenantSlugOrID, "err", err)
			}
		} else {
			theme = t
		}
	}
	return themeToMap(theme)
}

// attachThemeToDocument sets doc["theme"] to the resolved theme map on the
// rendered Document the widget consumes. The single attach point shared by the
// chat non-stream + chat SSE paths (tenant carries tenant.ID). A nil document
// map (defensive — the chat paths always populate it) is a no-op; the helper is
// fail-open via resolveThemeMap.
func attachThemeToDocument(ctx context.Context, doc map[string]interface{}, themes ports.ThemePort, tenant *domain.Tenant, log *slog.Logger) {
	if doc == nil {
		return
	}
	id := ""
	if tenant != nil {
		id = tenant.ID
	}
	doc["theme"] = resolveThemeMap(ctx, themes, id, log)
}

// themeToMap round-trips domain.Theme → JSON → map so it can be dropped onto
// the Document's map[string]interface{} as the inert `theme` passenger.
func themeToMap(theme domain.Theme) map[string]interface{} {
	raw, err := json.Marshal(theme)
	if err != nil {
		// domain.Theme is a closed struct of basic maps — Marshal cannot fail.
		// Defensive only: an empty map still yields zero visual change.
		return map[string]interface{}{}
	}
	out := map[string]interface{}{}
	_ = json.Unmarshal(raw, &out)
	return out
}

// ThemeHandler owns the admin-facing theme endpoints for the CURRENT tenant:
//
//	GET /api/v1/internal/theme?tenant=<slug>  — read the resolved theme
//	PUT /api/v1/internal/theme?tenant=<slug>  — upsert the tenant override
//
// Both are gated by X-Internal-Key (the same guard as the kill switch + preset
// preview) and are exempt from WithTenant (every /api/v1/internal/ route is).
// Tenant is resolved explicitly from the ?tenant= query param — there is no
// cross-tenant surface: a caller can only read/write the tenant it names, and
// the key gate is the authorization boundary.
type ThemeHandler struct {
	themes ports.ThemePort
	log    *slog.Logger
}

// NewThemeHandler constructs the admin theme handler.
func NewThemeHandler(themes ports.ThemePort, log *slog.Logger) *ThemeHandler {
	return &ThemeHandler{themes: themes, log: log}
}

// Theme handles GET + PUT /api/v1/internal/theme?tenant=<slug>.
func (h *ThemeHandler) Theme(w http.ResponseWriter, r *http.Request) {
	if !checkInternalKey(w, r) {
		return
	}
	tenant := r.URL.Query().Get("tenant")
	if tenant == "" {
		http.Error(w, "tenant query param missing", http.StatusBadRequest)
		return
	}
	switch r.Method {
	case http.MethodGet:
		h.get(w, r, tenant)
	case http.MethodPut:
		h.put(w, r, tenant)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// get reads the resolved theme (override merged over defaults, or pure
// defaults with isDefault:true when the tenant has no row).
func (h *ThemeHandler) get(w http.ResponseWriter, r *http.Request, tenant string) {
	theme, err := h.themes.GetTheme(r.Context(), tenant)
	if err != nil {
		if errors.Is(err, domain.ErrTenantNotFound) {
			http.Error(w, "tenant not found", http.StatusNotFound)
			return
		}
		h.log.Error("theme get failed", "tenant", tenant, "err", err)
		http.Error(w, "theme get failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"theme": theme})
}

// themePutRequest is the PUT body: a partial or full override token set, same
// shape as domain.ThemeTokens. Missing keys are merged over defaults on read.
type themePutRequest struct {
	Tokens domain.ThemeTokens `json:"tokens"`
}

// put upserts the tenant's override token set, then returns the resolved
// (merged) theme so the caller sees exactly what render will attach.
func (h *ThemeHandler) put(w http.ResponseWriter, r *http.Request, tenant string) {
	var req themePutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.themes.UpsertTheme(r.Context(), tenant, req.Tokens); err != nil {
		if errors.Is(err, domain.ErrTenantNotFound) {
			http.Error(w, "tenant not found", http.StatusNotFound)
			return
		}
		h.log.Error("theme upsert failed", "tenant", tenant, "err", err)
		http.Error(w, "theme upsert failed", http.StatusInternalServerError)
		return
	}
	theme, err := h.themes.GetTheme(r.Context(), tenant)
	if err != nil {
		h.log.Error("theme reload after upsert failed", "tenant", tenant, "err", err)
		http.Error(w, "theme reload failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"theme": theme})
}
