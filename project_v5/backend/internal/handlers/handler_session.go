package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"keepstar_v5/internal/ports"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SessionHandler owns POST /api/v1/session/init and GET
// /api/v1/session/{id}.
//
// init: inserts a v5_chat_sessions row, calls statePort.CreateState,
// returns the new session id + tenant info. The chat widget uses this
// once at boot to obtain a session id it then passes to every pipeline
// call.
//
// get: reads back the session state for debugging / restoration.
type SessionHandler struct {
	state ports.StatePort
	pool  *pgxpool.Pool // for inserting v5_chat_sessions row directly
}

// NewSessionHandler constructs the handler. pool is needed for the
// session-row INSERT which sits outside StatePort (StatePort.CreateState
// fills the state shell after the session row exists).
func NewSessionHandler(state ports.StatePort, pool *pgxpool.Pool) *SessionHandler {
	return &SessionHandler{state: state, pool: pool}
}

// Init handles POST /api/v1/session/init.
//
// Headers: X-Tenant-Slug (resolved by WithTenant middleware → context).
// Body: empty. Response: {sessionId, tenant: {slug, name}}.
func (h *SessionHandler) Init(w http.ResponseWriter, r *http.Request) {
	tenant := TenantFromContext(r.Context())
	if tenant == nil {
		http.Error(w, "tenant unresolved", http.StatusInternalServerError)
		return
	}

	// Insert v5_chat_sessions row, get auto-generated UUID.
	var sessionID string
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO v5_chat_sessions (tenant_id) VALUES ($1::uuid) RETURNING id`,
		tenant.ID,
	).Scan(&sessionID)
	if err != nil {
		http.Error(w, "session create failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if _, err := h.state.CreateState(r.Context(), sessionID); err != nil {
		http.Error(w, "state init failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sessionId": sessionID,
		"tenant": map[string]string{
			"slug": tenant.Slug,
			"name": tenant.Name,
		},
	})
}

// Get handles GET /api/v1/session/{id}.
//
// Returns the current session state for debugging; production clients
// reconstruct state implicitly via /pipeline calls so this is mostly for
// /debug.
func (h *SessionHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/session/")
	if id == "" || strings.Contains(id, "/") {
		http.Error(w, "session id missing", http.StatusBadRequest)
		return
	}
	state, err := h.state.GetState(r.Context(), id)
	if err != nil {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sessionId": state.SessionID,
		"step":      state.Step,
		"current":   state.Current,
		"view":      state.View,
	})
}

// writeJSON serialises v as JSON with Content-Type and the given status
// code. Errors are silently swallowed — at this point the response is
// already partially written and we can't recover gracefully.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
