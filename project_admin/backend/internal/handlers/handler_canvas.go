package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"keepstar-admin/internal/adapters/postgres"
	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/usecases"
)

// CanvasHandler exposes the KeepstarCanvas editor API. All routes are
// tenant-scoped through the JWT auth middleware — the handler only ever
// reads/writes the caller's own tenant data.
type CanvasHandler struct {
	canvas *usecases.CanvasUseCase
	log    *logger.Logger
}

func NewCanvasHandler(canvas *usecases.CanvasUseCase, log *logger.Logger) *CanvasHandler {
	return &CanvasHandler{canvas: canvas, log: log}
}

// ---------------- route dispatchers ----------------

// HandlePresets dispatches GET (list) and POST (create) on /canvas/presets.
func (h *CanvasHandler) HandlePresets(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listPresets(w, r)
	case http.MethodPost:
		h.createPreset(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "GET or POST only")
	}
}

// HandlePresetByID dispatches on /canvas/presets/{id} and sub-resources
// /canvas/presets/{id}/publish and /canvas/presets/{id}/fork.
func (h *CanvasHandler) HandlePresetByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := TenantID(ctx)
	tail := strings.TrimPrefix(r.URL.Path, "/admin/api/canvas/presets/")
	tail = strings.TrimSuffix(tail, "/")

	presetID, action := splitIDAction(tail)
	if presetID == "" {
		writeError(w, http.StatusBadRequest, "missing preset id")
		return
	}

	switch action {
	case "":
		switch r.Method {
		case http.MethodGet:
			p, err := h.canvas.GetPreset(ctx, tenantID, presetID)
			if err != nil {
				writeCanvasError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, p)
		case http.MethodPut:
			h.updatePreset(w, r, tenantID, presetID)
		case http.MethodDelete:
			if err := h.canvas.DeletePreset(ctx, tenantID, presetID); err != nil {
				writeCanvasError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		default:
			writeError(w, http.StatusMethodNotAllowed, "GET/PUT/DELETE only")
		}
	case "publish":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "POST only")
			return
		}
		p, err := h.canvas.PublishPreset(ctx, tenantID, presetID)
		if err != nil {
			writeCanvasError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, p)
	case "fork":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "POST only")
			return
		}
		p, err := h.canvas.ForkPreset(ctx, tenantID, presetID, UserID(ctx))
		if err != nil {
			writeCanvasError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, p)
	default:
		writeError(w, http.StatusNotFound, "unknown canvas preset action")
	}
}

// HandleComponents dispatches GET/POST on /canvas/components.
func (h *CanvasHandler) HandleComponents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := TenantID(ctx)
	switch r.Method {
	case http.MethodGet:
		list, err := h.canvas.ListComponents(ctx, tenantID)
		if err != nil {
			h.log.FromContext(ctx).Error("canvas_components_list_failed", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to list components")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"components": list})
	case http.MethodPost:
		var in domain.ComponentCreateInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		c, err := h.canvas.CreateComponent(ctx, tenantID, UserID(ctx), in)
		if err != nil {
			writeCanvasError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, c)
	default:
		writeError(w, http.StatusMethodNotAllowed, "GET or POST only")
	}
}

// HandleComponentByID dispatches on /canvas/components/{id} and /publish sub-route.
func (h *CanvasHandler) HandleComponentByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := TenantID(ctx)
	tail := strings.TrimPrefix(r.URL.Path, "/admin/api/canvas/components/")
	tail = strings.TrimSuffix(tail, "/")

	componentID, action := splitIDAction(tail)
	if componentID == "" {
		writeError(w, http.StatusBadRequest, "missing component id")
		return
	}

	switch action {
	case "":
		switch r.Method {
		case http.MethodGet:
			c, err := h.canvas.GetComponent(ctx, tenantID, componentID)
			if err != nil {
				writeCanvasError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, c)
		case http.MethodPut:
			var in domain.ComponentUpdateInput
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				writeError(w, http.StatusBadRequest, "invalid JSON")
				return
			}
			c, err := h.canvas.UpdateComponent(ctx, tenantID, componentID, UserID(ctx), in)
			if err != nil {
				writeCanvasError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, c)
		case http.MethodDelete:
			if err := h.canvas.DeleteComponent(ctx, tenantID, componentID); err != nil {
				writeCanvasError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		default:
			writeError(w, http.StatusMethodNotAllowed, "GET/PUT/DELETE only")
		}
	case "publish":
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "POST only")
			return
		}
		c, err := h.canvas.PublishComponent(ctx, tenantID, componentID)
		if err != nil {
			writeCanvasError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, c)
	default:
		writeError(w, http.StatusNotFound, "unknown canvas component action")
	}
}

// HandleTokens dispatches GET (list) and POST (upsert) on /canvas/tokens.
func (h *CanvasHandler) HandleTokens(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := TenantID(ctx)
	switch r.Method {
	case http.MethodGet:
		list, err := h.canvas.ListTokens(ctx, tenantID)
		if err != nil {
			h.log.FromContext(ctx).Error("canvas_tokens_list_failed", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to list tokens")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"tokens": list})
	case http.MethodPost:
		var in domain.DesignTokenUpsertInput
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		tok, err := h.canvas.UpsertToken(ctx, tenantID, in)
		if err != nil {
			writeCanvasError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, tok)
	default:
		writeError(w, http.StatusMethodNotAllowed, "GET or POST only")
	}
}

// HandleTokenByID handles DELETE /canvas/tokens/{id}.
func (h *CanvasHandler) HandleTokenByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := TenantID(ctx)
	tokenID := extractID(r.URL.Path, "/admin/api/canvas/tokens/")
	if tokenID == "" {
		writeError(w, http.StatusBadRequest, "missing token id")
		return
	}
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "DELETE only")
		return
	}
	if err := h.canvas.DeleteToken(ctx, tenantID, tokenID); err != nil {
		writeCanvasError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------------- sub-handlers for /canvas/presets ----------------

func (h *CanvasHandler) listPresets(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := TenantID(ctx)
	list, err := h.canvas.ListPresets(ctx, tenantID)
	if err != nil {
		h.log.FromContext(ctx).Error("canvas_presets_list_failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list presets")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"presets": list})
}

func (h *CanvasHandler) createPreset(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := TenantID(ctx)
	var in domain.PresetCreateInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	p, err := h.canvas.CreatePreset(ctx, tenantID, UserID(ctx), in)
	if err != nil {
		writeCanvasError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *CanvasHandler) updatePreset(w http.ResponseWriter, r *http.Request, tenantID, presetID string) {
	ctx := r.Context()
	var in domain.PresetUpdateInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	p, err := h.canvas.UpdatePreset(ctx, tenantID, presetID, UserID(ctx), in)
	if err != nil {
		writeCanvasError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// ---------------- helpers ----------------

// splitIDAction splits a path tail into {id, action}. Examples:
//
//	"abc"           → "abc", ""
//	"abc/publish"   → "abc", "publish"
//	"abc/fork"      → "abc", "fork"
func splitIDAction(tail string) (string, string) {
	if tail == "" {
		return "", ""
	}
	parts := strings.SplitN(tail, "/", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

// writeCanvasError maps domain/adapter errors to HTTP status codes so handler
// bodies stay free of repetitive switch-case.
func writeCanvasError(w http.ResponseWriter, err error) {
	if errors.Is(err, postgres.ErrCanvasNotFound) {
		writeError(w, http.StatusNotFound, "canvas entity not found")
		return
	}
	writeError(w, http.StatusBadRequest, err.Error())
}
