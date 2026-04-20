package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"keepstar-admin/internal/logger"
	"keepstar-admin/internal/usecases"
)

type OAuthHandler struct {
	google   *usecases.GoogleAuthUseCase
	telegram *usecases.TelegramAuthUseCase
	log      *logger.Logger
}

func NewOAuthHandler(g *usecases.GoogleAuthUseCase, t *usecases.TelegramAuthUseCase, log *logger.Logger) *OAuthHandler {
	return &OAuthHandler{google: g, telegram: t, log: log}
}

// HandleGoogleStart mints the consent URL. Frontend receives the URL and
// performs a client-side redirect (instead of a 302 here) so the SPA can show
// a loading state first and reuse the same URL under click-handlers later.
func (h *OAuthHandler) HandleGoogleStart(w http.ResponseWriter, r *http.Request) {
	if h.google == nil {
		writeError(w, http.StatusNotImplemented, "google oauth not configured")
		return
	}
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "POST or GET")
		return
	}
	res, err := h.google.Start(r.Context())
	if err != nil {
		h.log.Error("google_start_failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to start google oauth")
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// HandleGoogleCallback accepts {code, state}. Google redirects back to the
// frontend with these as query params; the SPA POSTs them here to avoid
// leaking the code into browser history / referrers.
func (h *OAuthHandler) HandleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	if h.google == nil {
		writeError(w, http.StatusNotImplemented, "google oauth not configured")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var req struct {
		Code  string `json:"code"`
		State string `json:"state"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	ua := r.Header.Get("User-Agent")
	ip := clientIP(r)
	resp, err := h.google.Complete(r.Context(), req.Code, req.State, ua, ip)
	if err != nil {
		h.log.Warn("google_callback_failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// HandleTelegramCallback accepts the raw widget payload (id, first_name,
// last_name, username, photo_url, auth_date, hash). The HMAC signature is the
// only authentication needed — no pre-registered state to verify.
func (h *OAuthHandler) HandleTelegramCallback(w http.ResponseWriter, r *http.Request) {
	if h.telegram == nil {
		writeError(w, http.StatusNotImplemented, "telegram login not configured")
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var raw map[string]any
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	fields := make(map[string]string, len(raw))
	for k, v := range raw {
		switch vv := v.(type) {
		case string:
			fields[k] = vv
		case float64:
			// JSON numbers arrive as float64; the widget emits them as strings on
			// the web but via the official JS SDK some fields (id, auth_date)
			// end up numeric. Format without trailing zeros.
			fields[k] = strconvFormatFloat(vv)
		}
	}
	ua := r.Header.Get("User-Agent")
	ip := clientIP(r)
	resp, err := h.telegram.Complete(r.Context(), fields, ua, ip)
	if err != nil {
		h.log.Warn("telegram_callback_failed", "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// strconvFormatFloat renders a JSON-sourced integer-ish float without the
// ".0" suffix so HMAC input matches Telegram's string concatenation.
func strconvFormatFloat(f float64) string {
	if f == float64(int64(f)) {
		return fmt.Sprintf("%d", int64(f))
	}
	return fmt.Sprintf("%v", f)
}
