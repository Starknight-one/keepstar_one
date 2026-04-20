package handlers

import (
	"encoding/json"
	"net/http"

	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/usecases"
)

type TwoFactorHandler struct {
	tf   *usecases.TwoFactorUseCase
	auth *usecases.AuthUseCase
	log  *logger.Logger
}

func NewTwoFactorHandler(tf *usecases.TwoFactorUseCase, auth *usecases.AuthUseCase, log *logger.Logger) *TwoFactorHandler {
	return &TwoFactorHandler{tf: tf, auth: auth, log: log}
}

// HandleSetupTOTP — authenticated (Bearer). Generates + stores a fresh secret
// and returns it to the user's authenticator app.
func (h *TwoFactorHandler) HandleSetupTOTP(w http.ResponseWriter, r *http.Request) {
	if h.tf == nil {
		writeError(w, http.StatusNotImplemented, "2fa not configured")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	uid := UserID(r.Context())
	user, err := h.auth.GetMe(r.Context(), uid)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "user not found")
		return
	}
	res, err := h.tf.SetupTOTP(r.Context(), uid, user.Email)
	if err != nil {
		h.log.Error("totp_setup_failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// HandleConfirmTOTP — authenticated. Activates 2FA on successful first code.
func (h *TwoFactorHandler) HandleConfirmTOTP(w http.ResponseWriter, r *http.Request) {
	if h.tf == nil {
		writeError(w, http.StatusNotImplemented, "2fa not configured")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var req struct{ Code string `json:"code"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		writeError(w, http.StatusBadRequest, "code required")
		return
	}
	if err := h.tf.ConfirmTOTP(r.Context(), UserID(r.Context()), req.Code); err != nil {
		writeError(w, http.StatusBadRequest, "invalid code")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// HandleDisableTOTP — authenticated. Clears secret + enabled flag.
func (h *TwoFactorHandler) HandleDisableTOTP(w http.ResponseWriter, r *http.Request) {
	if h.tf == nil {
		writeError(w, http.StatusNotImplemented, "2fa not configured")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	if err := h.tf.DisableTOTP(r.Context(), UserID(r.Context())); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// HandleVerifyTOTP — pre-2FA scoped. Exchanges a TOTP code for a full
// session pair.
func (h *TwoFactorHandler) HandleVerifyTOTP(w http.ResponseWriter, r *http.Request) {
	if h.tf == nil {
		writeError(w, http.StatusNotImplemented, "2fa not configured")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var req struct{ Code string `json:"code"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		writeError(w, http.StatusBadRequest, "code required")
		return
	}
	resp, err := h.tf.VerifyTOTP(r.Context(), UserID(r.Context()), req.Code, r.Header.Get("User-Agent"), clientIP(r))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid code")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// HandleSendEmailCode — pre-2FA scoped. Emails a one-time code.
func (h *TwoFactorHandler) HandleSendEmailCode(w http.ResponseWriter, r *http.Request) {
	if h.tf == nil {
		writeError(w, http.StatusNotImplemented, "2fa not configured")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	uid := UserID(r.Context())
	user, err := h.auth.GetMe(r.Context(), uid)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "user not found")
		return
	}
	if err := h.tf.SendEmailCode(r.Context(), uid, user.Email); err != nil {
		h.log.Error("email_2fa_send_failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to send code")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// HandleVerifyEmailCode — pre-2FA scoped. Exchanges an email code for full
// tokens.
func (h *TwoFactorHandler) HandleVerifyEmailCode(w http.ResponseWriter, r *http.Request) {
	if h.tf == nil {
		writeError(w, http.StatusNotImplemented, "2fa not configured")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var req struct{ Code string `json:"code"` }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" {
		writeError(w, http.StatusBadRequest, "code required")
		return
	}
	resp, err := h.tf.VerifyEmailCode(r.Context(), UserID(r.Context()), req.Code, r.Header.Get("User-Agent"), clientIP(r))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid code")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
