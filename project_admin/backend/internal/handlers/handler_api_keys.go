// Package handlers — admin-side CRUD for tenant api_keys (M10). JWT auth.
// Public consumers of the API talk to handler_api_v1_*.go with X-API-Key.
package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/ports"
)

type APIKeysHandler struct {
	keys ports.APIKeysPort
	log  *logger.Logger
}

func NewAPIKeysHandler(keys ports.APIKeysPort, log *logger.Logger) *APIKeysHandler {
	return &APIKeysHandler{keys: keys, log: log}
}

type createAPIKeyPayload struct {
	Label string `json:"label"`
}

type createAPIKeyResponse struct {
	ID        string `json:"id"`
	Label     string `json:"label,omitempty"`
	KeyPrefix string `json:"keyPrefix"`
	PlainKey  string `json:"plainKey"`
	CreatedAt string `json:"createdAt"`
}

// POST /admin/api/api-keys — create a new key. Returns plain text once.
func (h *APIKeysHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var p createAPIKeyPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil && err.Error() != "EOF" {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	key, plain, err := h.keys.Create(r.Context(), TenantID(r.Context()), p.Label)
	if err != nil {
		h.log.FromContext(r.Context()).Error("create_api_key_failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to create api key")
		return
	}
	writeJSON(w, http.StatusOK, createAPIKeyResponse{
		ID:        key.ID,
		Label:     key.Label,
		KeyPrefix: key.KeyPrefix,
		PlainKey:  plain,
		CreatedAt: key.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// GET /admin/api/api-keys — list all keys for the current tenant.
func (h *APIKeysHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	keys, err := h.keys.List(r.Context(), TenantID(r.Context()))
	if err != nil {
		h.log.FromContext(r.Context()).Error("list_api_keys_failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list api keys")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": keys})
}

// DELETE /admin/api/api-keys/{id} — revoke the key.
func (h *APIKeysHandler) HandleRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "DELETE only")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/admin/api/api-keys/")
	id = strings.TrimSuffix(id, "/")
	if id == "" || strings.Contains(id, "/") {
		writeError(w, http.StatusBadRequest, "missing id")
		return
	}
	if err := h.keys.Revoke(r.Context(), TenantID(r.Context()), id); err != nil {
		h.log.FromContext(r.Context()).Error("revoke_api_key_failed", "error", err)
		writeError(w, http.StatusNotFound, "key not found or already revoked")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
