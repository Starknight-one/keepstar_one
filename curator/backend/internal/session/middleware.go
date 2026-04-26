// Package session — opaque session-token middleware for curator. Token is
// either a Bearer header or a same-site Cookie ("curator_session"). User id
// + email get stamped into context.
package session

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"keepstar-curator/internal/adapters"
	"keepstar-curator/internal/domain"
)

type ctxKey string

const (
	ctxUserID ctxKey = "uid"
	ctxEmail  ctxKey = "email"
	ctxRole   ctxKey = "role"
)

func UserID(ctx context.Context) string { v, _ := ctx.Value(ctxUserID).(string); return v }
func Email(ctx context.Context) string  { v, _ := ctx.Value(ctxEmail).(string); return v }
func Role(ctx context.Context) string   { v, _ := ctx.Value(ctxRole).(string); return v }

func Middleware(client *adapters.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := readToken(r)
			if token == "" {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
			user, err := client.ResolveSession(r.Context(), token)
			if err != nil {
				if errors.Is(err, adapters.ErrInvalidCredentials) {
					http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
					return
				}
				http.Error(w, `{"error":"server"}`, http.StatusInternalServerError)
				return
			}
			ctx := context.WithValue(r.Context(), ctxUserID, user.ID)
			ctx = context.WithValue(ctx, ctxEmail, user.Email)
			ctx = context.WithValue(ctx, ctxRole, user.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func readToken(r *http.Request) string {
	if c, err := r.Cookie("curator_session"); err == nil && c.Value != "" {
		return c.Value
	}
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}

// SetCookie writes the session cookie for browser clients.
func SetCookie(w http.ResponseWriter, token string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     "curator_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   7 * 24 * 3600,
	})
}

func ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:   "curator_session",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
}

// ResolveOnly is a helper for handlers that want to know who's calling
// without forcing the middleware (e.g. login handler).
func ResolveOnly(client *adapters.Client, r *http.Request) (domain.CuratorUser, error) {
	t := readToken(r)
	if t == "" {
		return domain.CuratorUser{}, adapters.ErrInvalidCredentials
	}
	return client.ResolveSession(r.Context(), t)
}
