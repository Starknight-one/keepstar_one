// Package handlers — single-file HTTP layer for curator. Auth + candidates
// + junk + audit. Promote runs through adapters.PromoteAttribute (transactional
// ALTER TABLE).
package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"keepstar-curator/internal/adapters"
	"keepstar-curator/internal/session"
)

type Handler struct {
	Client *adapters.Client
	Secure bool // set Secure=true on cookies in prod
}

func New(client *adapters.Client, secure bool) *Handler {
	return &Handler{Client: client, Secure: secure}
}

// --- Auth ---

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var p struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	token, user, err := h.Client.Login(r.Context(), p.Email, p.Password)
	if err != nil {
		if errors.Is(err, adapters.ErrInvalidCredentials) {
			writeErr(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		writeErr(w, http.StatusInternalServerError, "login failed")
		return
	}
	session.SetCookie(w, token, h.Secure)
	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user":  user,
	})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("curator_session"); err == nil {
		_ = h.Client.Logout(r.Context(), c.Value)
	}
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		_ = h.Client.Logout(r.Context(), strings.TrimPrefix(auth, "Bearer "))
	}
	session.ClearCookie(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"id":    session.UserID(r.Context()),
		"email": session.Email(r.Context()),
		"role":  session.Role(r.Context()),
	})
}

// --- Candidates ---

func (h *Handler) ListAttributeCandidates(w http.ResponseWriter, r *http.Request) {
	cands, err := h.Client.ListAttributeCandidates(r.Context(), r.URL.Query().Get("status"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"candidates": cands})
}

func (h *Handler) ListCategoryCandidates(w http.ResponseWriter, r *http.Request) {
	cands, err := h.Client.ListCategoryCandidates(r.Context(), r.URL.Query().Get("status"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"candidates": cands})
}

func (h *Handler) PromoteAttribute(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/curator/candidates/attributes/"), "/promote")
	var p struct {
		Key        string `json:"key"`
		Vertical   string `json:"vertical"`
		ColumnType string `json:"columnType"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := h.Client.PromoteAttribute(r.Context(), id, p.Key, p.Vertical, p.ColumnType, session.UserID(r.Context())); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) DismissAttribute(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/curator/candidates/attributes/"), "/dismiss")
	if err := h.Client.DismissAttribute(r.Context(), id, session.UserID(r.Context())); err != nil {
		writeErr(w, http.StatusInternalServerError, "failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// --- Junk ---

func (h *Handler) ListJunk(w http.ResponseWriter, r *http.Request) {
	cands, err := h.Client.ListJunkCandidates(r.Context(), r.URL.Query().Get("status"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"candidates": cands})
}

func (h *Handler) ClassifyJunk(w http.ResponseWriter, r *http.Request) {
	// /curator/junk/{id}/classify
	rest := strings.TrimPrefix(r.URL.Path, "/curator/junk/")
	id := strings.TrimSuffix(rest, "/classify")
	var p struct {
		Classification string `json:"classification"`
	}
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if p.Classification != "confirmed_addon" && p.Classification != "false_positive" {
		writeErr(w, http.StatusBadRequest, "classification must be confirmed_addon or false_positive")
		return
	}
	if err := h.Client.ClassifyJunk(r.Context(), id, p.Classification, session.UserID(r.Context())); err != nil {
		writeErr(w, http.StatusInternalServerError, "failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// --- Audit ---

func (h *Handler) ListAudit(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	entries, err := h.Client.ListAudit(r.Context(), limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
