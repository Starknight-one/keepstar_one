package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/usecases"
)

type AuthHandler struct {
	auth    *usecases.AuthUseCase
	log     *logger.Logger
	flags   AuthFeatureFlags
}

// AuthFeatureFlags surfaces which optional auth paths are wired at runtime.
// Frontend reads these from GET /auth/config to conditionally render OAuth
// buttons and email-only flows (reset, invite, email 2FA).
type AuthFeatureFlags struct {
	Google   bool   `json:"google"`
	Email    bool   `json:"email"`
	Telegram struct {
		Enabled     bool   `json:"enabled"`
		BotUsername string `json:"bot_username"`
	} `json:"telegram"`
}

func NewAuthHandler(auth *usecases.AuthUseCase, log *logger.Logger, flags AuthFeatureFlags) *AuthHandler {
	return &AuthHandler{auth: auth, log: log, flags: flags}
}

func (h *AuthHandler) HandleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	writeJSON(w, http.StatusOK, h.flags)
}

func (h *AuthHandler) HandleSignup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("handler.signup")
		defer endSpan()
	}

	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}

	var req usecases.SignupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	reqLog := h.log.FromContext(ctx)

	resp, err := h.auth.Signup(ctx, req)
	if err != nil {
		if errors.Is(err, domain.ErrEmailExists) {
			reqLog.Warn("signup_conflict", "email", req.Email)
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		reqLog.Error("signup_failed", "email", req.Email, "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	reqLog.Info("signup_success", "email", req.Email)
	writeJSON(w, http.StatusCreated, resp)
}

func (h *AuthHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if sc := domain.SpanFromContext(ctx); sc != nil {
		endSpan := sc.Start("handler.login")
		defer endSpan()
	}

	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}

	var req usecases.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	reqLog := h.log.FromContext(ctx)

	resp, err := h.auth.LoginWithMeta(ctx, req, r.Header.Get("User-Agent"), clientIP(r))
	if err != nil {
		if errors.Is(err, domain.ErrInvalidCredentials) {
			reqLog.Warn("login_failed", "email", req.Email)
			writeError(w, http.StatusUnauthorized, err.Error())
			return
		}
		reqLog.Error("login_error", "email", req.Email, "error", err)
		writeError(w, http.StatusInternalServerError, "login failed")
		return
	}

	reqLog.Info("login_success", "email", req.Email)
	writeJSON(w, http.StatusOK, resp)
}

func (h *AuthHandler) HandleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}

	uid := UserID(r.Context())
	user, err := h.auth.GetMe(r.Context(), uid)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	writeJSON(w, http.StatusOK, user)
}

func (h *AuthHandler) HandleGetTenant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}

	tid := TenantID(r.Context())
	tenant, err := h.auth.GetTenant(r.Context(), tid)
	if err != nil {
		writeError(w, http.StatusNotFound, "tenant not found")
		return
	}

	writeJSON(w, http.StatusOK, tenant)
}

// HandleSetPassword — POST /admin/api/auth/set-password
// Body: {"password": "..."}
// Auth-required. Lets a passwordless user (created via magic-link / OAuth)
// define a password the first time. Returns 409 if the user already has
// one (they should use the password-reset flow). Scenario 39.
func (h *AuthHandler) HandleSetPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	uid := UserID(r.Context())
	if uid == "" {
		writeError(w, http.StatusUnauthorized, "auth required")
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := h.auth.SetPasswordForPasswordless(r.Context(), uid, req.Password); err != nil {
		msg := err.Error()
		// The "already set" guard is a 409 so the frontend can branch into
		// the reset flow. Strength + length issues come back as 400.
		if strings.Contains(msg, "password already set") {
			writeError(w, http.StatusConflict, msg)
			return
		}
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
