package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// Pre2FAMiddleware only accepts JWTs signed with scope="pre_2fa" — short-
// lived (5 min default) tokens issued by Login when the user has 2FA
// enabled. It passes uid in context to the 2FA-verify handlers so they can
// mint the real session after code verification.
func Pre2FAMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				writeError(w, http.StatusUnauthorized, "missing pre-2fa token")
				return
			}
			tokenStr := strings.TrimPrefix(auth, "Bearer ")
			token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
				return []byte(secret), nil
			})
			if err != nil || !token.Valid {
				writeError(w, http.StatusUnauthorized, "invalid pre-2fa token")
				return
			}
			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				writeError(w, http.StatusUnauthorized, "invalid claims")
				return
			}
			scope, _ := claims["scope"].(string)
			if scope != "pre_2fa" {
				writeError(w, http.StatusUnauthorized, "wrong token scope")
				return
			}
			uid, _ := claims["uid"].(string)
			if uid == "" {
				writeError(w, http.StatusUnauthorized, "missing uid")
				return
			}
			ctx := context.WithValue(r.Context(), ctxUserID, uid)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
