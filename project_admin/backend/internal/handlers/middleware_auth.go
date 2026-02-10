package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const (
	ctxUserID   contextKey = "uid"
	ctxTenantID contextKey = "tid"
	ctxRole     contextKey = "role"
)

func UserID(ctx context.Context) string   { v, _ := ctx.Value(ctxUserID).(string); return v }
func TenantID(ctx context.Context) string { v, _ := ctx.Value(ctxTenantID).(string); return v }

func AuthMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				writeError(w, http.StatusUnauthorized, "missing token")
				return
			}
			tokenStr := strings.TrimPrefix(auth, "Bearer ")

			token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
				return []byte(secret), nil
			})
			if err != nil || !token.Valid {
				writeError(w, http.StatusUnauthorized, "invalid token")
				return
			}

			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				writeError(w, http.StatusUnauthorized, "invalid claims")
				return
			}

			uid, _ := claims["uid"].(string)
			tid, _ := claims["tid"].(string)
			role, _ := claims["role"].(string)

			ctx := r.Context()
			ctx = context.WithValue(ctx, ctxUserID, uid)
			ctx = context.WithValue(ctx, ctxTenantID, tid)
			ctx = context.WithValue(ctx, ctxRole, role)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
