package handlers

import (
	"encoding/json"
	"net/http"

	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/usecases"
)

type PasswordResetHandler struct {
	reset  *usecases.PasswordResetUseCase
	verify *usecases.EmailVerifyUseCase
	log    *logger.Logger
}

func NewPasswordResetHandler(reset *usecases.PasswordResetUseCase, verify *usecases.EmailVerifyUseCase, log *logger.Logger) *PasswordResetHandler {
	return &PasswordResetHandler{reset: reset, verify: verify, log: log}
}

// HandleForgot — always 200 to avoid email enumeration.
func (h *PasswordResetHandler) HandleForgot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if h.reset == nil {
		writeError(w, http.StatusNotImplemented, "email is not configured")
		return
	}
	_ = h.reset.Forgot(r.Context(), req.Email)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *PasswordResetHandler) HandleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var req struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if h.reset == nil {
		writeError(w, http.StatusNotImplemented, "email is not configured")
		return
	}
	if err := h.reset.Reset(r.Context(), req.Token, req.NewPassword); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *PasswordResetHandler) HandleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if h.verify == nil {
		writeError(w, http.StatusNotImplemented, "email is not configured")
		return
	}
	if err := h.verify.Verify(r.Context(), req.Token); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *PasswordResetHandler) HandleResendVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if h.verify != nil {
		_ = h.verify.Resend(r.Context(), req.Email)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
