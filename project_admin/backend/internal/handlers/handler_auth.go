package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

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
		BotID       int64  `json:"bot_id"`
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
