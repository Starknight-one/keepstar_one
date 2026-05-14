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
	"keepstar-curator/internal/domain"
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

// --- Tenants ---

func (h *Handler) ListTenants(w http.ResponseWriter, r *http.Request) {
	tenants, err := h.Client.ListTenants(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list tenants")
		return
	}
	if tenants == nil {
		tenants = []domain.TenantSummary{} // ensure JSON [] not null
	}
	writeJSON(w, http.StatusOK, map[string]any{"tenants": tenants})
}

func (h *Handler) GetTenant(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/curator/tenants/")
	if i := strings.Index(id, "/"); i >= 0 {
		id = id[:i]
	}
	if id == "" {
		writeErr(w, http.StatusBadRequest, "tenant id required")
		return
	}
	t, err := h.Client.GetTenant(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (h *Handler) ListTenantProducts(w http.ResponseWriter, r *http.Request) {
	// /curator/tenants/{id}/products
	rest := strings.TrimPrefix(r.URL.Path, "/curator/tenants/")
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || parts[0] == "" {
		writeErr(w, http.StatusBadRequest, "tenant id required")
		return
	}
	id := parts[0]
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	products, total, err := h.Client.ListTenantProducts(r.Context(), id, q.Get("search"), limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list products")
		return
	}
	if products == nil {
		products = []domain.TenantProductRow{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"products": products, "total": total})
}

func (h *Handler) GetTenantSchema(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/curator/tenants/")
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || parts[0] == "" {
		writeErr(w, http.StatusBadRequest, "tenant id required")
		return
	}
	id := parts[0]
	s, err := h.Client.GetTenantSchema(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to read schema")
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// --- Master catalog ---

func (h *Handler) ListMasterProducts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	products, total, err := h.Client.ListMasterProducts(r.Context(),
		q.Get("vertical"), q.Get("search"), limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to list master products")
		return
	}
	if products == nil {
		products = []domain.MasterProductSummary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"products": products, "total": total})
}

func (h *Handler) GetMasterProduct(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/curator/master/products/")
	if i := strings.Index(id, "/"); i >= 0 {
		id = id[:i]
	}
	if id == "" {
		writeErr(w, http.StatusBadRequest, "master product id required")
		return
	}
	d, err := h.Client.GetMasterProduct(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, d)
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
