package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/usecases"
)

type InvitationsHandler struct {
	invites *usecases.InvitationsUseCase
	secret  string
	log     *logger.Logger
}

func NewInvitationsHandler(i *usecases.InvitationsUseCase, jwtSecret string, log *logger.Logger) *InvitationsHandler {
	return &InvitationsHandler{invites: i, secret: jwtSecret, log: log}
}

// HandleCreate lives behind authMW. Only owner/admin of the current tenant
// can invite; everyone else gets 403.
func (h *InvitationsHandler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	role := Role(r.Context())
	if role != "owner" && role != "admin" && role != "editor" {
		// editor is the legacy role name — treat as admin for backward-compat
		// until all users get migrated to the 4-role scheme.
		writeError(w, http.StatusForbidden, "only owners or admins can invite")
		return
	}
	tid := TenantID(r.Context())
	if tid == "" {
		writeError(w, http.StatusBadRequest, "no active workspace")
		return
	}

	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Role == "" {
		req.Role = "member"
	}

	if err := h.invites.Create(r.Context(), tid, UserID(r.Context()), req.Email, req.Role); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// HandleResend — authenticated. POST /admin/api/auth/invitations/{id}/resend.
// Re-emails an existing invitation with a freshly-rotated token. Used when
// the initial mailer.Send failed (sc 82) — invitee stranded.
func (h *InvitationsHandler) HandleResend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	// Path: /admin/api/auth/invitations/{id}/resend → extract {id}.
	path := strings.TrimPrefix(r.URL.Path, "/admin/api/auth/invitations/")
	id := strings.TrimSuffix(path, "/resend")
	if id == "" || id == path {
		writeError(w, http.StatusBadRequest, "invitation id required in path")
		return
	}
	if err := h.invites.Resend(r.Context(), id); err != nil {
		h.log.Warn("invitation_resend_failed", "id", id, "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// HandlePreview is public — anyone with the token sees what they've been
// invited to. No authentication required.
func (h *InvitationsHandler) HandlePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	token := extractInviteToken(r.URL.Path)
	if token == "" || strings.Contains(token, "/") {
		writeError(w, http.StatusBadRequest, "invalid token")
		return
	}
	preview, err := h.invites.Preview(r.Context(), token)
	if err != nil {
		writeError(w, http.StatusGone, "invitation is invalid or expired")
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

// HandleAccept is public but optionally reads a Bearer token. If present and
// valid, we attach the invite to the currently-signed-in user instead of
// creating a new account.
func (h *InvitationsHandler) HandleAccept(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	token := extractInviteToken(strings.TrimSuffix(r.URL.Path, "/accept"))
	if token == "" {
		writeError(w, http.StatusBadRequest, "invalid token")
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	currentUserID := h.tryExtractUserID(r)

	resp, err := h.invites.Accept(r.Context(), usecases.AcceptRequest{
		Token:         token,
		Password:      req.Password,
		CurrentUserID: currentUserID,
		UserAgent:     r.Header.Get("User-Agent"),
		IP:            clientIP(r),
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// tryExtractUserID returns the user ID from a valid Bearer token, or "" if
// absent or invalid. We don't fail on invalid tokens — the accept path works
// for both signed-in and signed-out invitees.
func (h *InvitationsHandler) tryExtractUserID(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	tok := strings.TrimPrefix(auth, "Bearer ")
	parsed, err := jwt.Parse(tok, func(t *jwt.Token) (any, error) { return []byte(h.secret), nil })
	if err != nil || !parsed.Valid {
		return ""
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return ""
	}
	scope, _ := claims["scope"].(string)
	// Only accept real access tokens — pre-2FA tokens shouldn't be able to
	// piggy-back an invite accept.
	if scope != "" && scope != "access" {
		return ""
	}
	uid, _ := claims["uid"].(string)
	return uid
}

func extractInviteToken(path string) string {
	const prefix = "/admin/api/auth/invitations/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(path, prefix)
	rest = strings.TrimSuffix(rest, "/")
	// Token shouldn't contain further slashes.
	if idx := strings.Index(rest, "/"); idx >= 0 {
		return rest[:idx]
	}
	return rest
}
