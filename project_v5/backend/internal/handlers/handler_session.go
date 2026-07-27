package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"

	"keepstar_v5/internal/domain"
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

// sessionInitRequest is the OPTIONAL body of POST /api/v1/session/init
// (R17). mode selects the session's form: "storefront" (default) or "crm".
// mode=crm requires a valid CRM surface token k (R13) — a hit stamps
// role=staff into the session row; every pipeline turn inherits it.
// mode=onboarding is rejected here: onboarding sessions are creatable ONLY
// via the cookie-gated POST /api/v1/onboard/session (handler_onboard.go).
type sessionInitRequest struct {
	Mode string `json:"mode,omitempty"`
	K    string `json:"k,omitempty"`
}

// Init handles POST /api/v1/session/init.
//
// Headers: X-Tenant-Slug (resolved by WithTenant middleware → context).
// Body: empty (legacy widgets) or {mode?, k?} — see sessionInitRequest.
// Response: {sessionId, mode, tenant: {slug, name}}.
func (h *SessionHandler) Init(w http.ResponseWriter, r *http.Request) {
	tenant := TenantFromContext(r.Context())
	if tenant == nil {
		http.Error(w, "tenant unresolved", http.StatusInternalServerError)
		return
	}

	// Body is optional: the deployed widget sends none (io.EOF), which
	// means the default storefront form. Malformed JSON is still a 400.
	var req sessionInitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	mode := req.Mode
	if mode == "" {
		mode = string(domain.ModeStorefront)
	}
	role := string(domain.RoleVisitor)
	switch mode {
	case string(domain.ModeStorefront):
		// Public form — visitor role, no token.
	case string(domain.ModeCRM):
		// R13: a valid, unrevoked surface token for THIS tenant stamps
		// role=staff. Absent/invalid token → 403, no session created.
		if req.K == "" {
			http.Error(w, "surface token required for crm", http.StatusForbidden)
			return
		}
		var tokenOK bool
		err := h.pool.QueryRow(r.Context(),
			`SELECT EXISTS(
			   SELECT 1 FROM v5_surface_tokens
			   WHERE tenant_id = $1::uuid AND surface = 'crm'
			     AND token = $2 AND revoked_at IS NULL)`,
			tenant.ID, req.K,
		).Scan(&tokenOK)
		if err != nil {
			http.Error(w, "surface token check failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if !tokenOK {
			http.Error(w, "invalid surface token", http.StatusForbidden)
			return
		}
		role = string(domain.RoleStaff)
	case string(domain.ModeOnboarding):
		http.Error(w, "onboarding sessions are created via /api/v1/onboard/session", http.StatusForbidden)
		return
	default:
		http.Error(w, "unknown mode", http.StatusBadRequest)
		return
	}

	// Insert v5_chat_sessions row, get auto-generated UUID. mode + role are
	// the per-turn truth (R17): the pipeline handler reads them back on
	// every call.
	var sessionID string
	err := h.pool.QueryRow(r.Context(),
		`INSERT INTO v5_chat_sessions (tenant_id, mode, role) VALUES ($1::uuid, $2, $3) RETURNING id`,
		tenant.ID, mode, role,
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
		"mode":      mode,
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

// --- Internal control: kill switch (curator cockpit) ---

// checkInternalKey gates the internal control endpoints. Empty env key ⇒ 503
// (route effectively disabled, never accidentally exposed). Mismatch ⇒ 403.
// Constant-time compare. These routes are exempt from WithTenant (see
// middleware_tenant.go) and live under /api/v1/internal/.
func checkInternalKey(w http.ResponseWriter, r *http.Request) bool {
	want := os.Getenv("V5_INTERNAL_KEY")
	if want == "" {
		http.Error(w, "internal control disabled", http.StatusServiceUnavailable)
		return false
	}
	got := r.Header.Get("X-Internal-Key")
	if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}

// Kill handles POST /api/v1/internal/session/{id}/kill. Soft-kills a session by
// stamping killed_at; GetState then refuses it (domain.ErrSessionKilled) so
// every continuation path stops. Idempotent (no-op if already killed). Never
// hard-deletes — that would cascade away state/deltas/traces.
func (h *SessionHandler) Kill(w http.ResponseWriter, r *http.Request) {
	if !checkInternalKey(w, r) {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "session id missing", http.StatusBadRequest)
		return
	}
	tag, err := h.pool.Exec(r.Context(),
		`UPDATE v5_chat_sessions SET killed_at = NOW(), killed_reason = $2, updated_at = NOW()
		 WHERE id = $1::uuid AND killed_at IS NULL`,
		id, r.URL.Query().Get("reason"),
	)
	if err != nil {
		http.Error(w, "kill failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"sessionId": id,
		"killed":    tag.RowsAffected() > 0, // false ⇒ already killed or unknown id
	})
}

// KillAll handles POST /api/v1/internal/sessions/kill-all. Incident kill switch:
// soft-kills every ACTIVE session (updated within 30 min, not already killed)
// across ALL tenants. Returns the count killed.
func (h *SessionHandler) KillAll(w http.ResponseWriter, r *http.Request) {
	if !checkInternalKey(w, r) {
		return
	}
	tag, err := h.pool.Exec(r.Context(),
		`UPDATE v5_chat_sessions SET killed_at = NOW(), killed_reason = $1, updated_at = NOW()
		 WHERE killed_at IS NULL AND updated_at > NOW() - INTERVAL '30 minutes'`,
		r.URL.Query().Get("reason"),
	)
	if err != nil {
		http.Error(w, "kill-all failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"killed": tag.RowsAffected(),
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
