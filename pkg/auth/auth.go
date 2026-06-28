package auth

import (
	"context"
	"net/http"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const userIDKey contextKey = "user_id"

// UserIDFromContext extracts the authenticated user ID from context.
func UserIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(userIDKey).(string)
	return v
}

// secret returns the JWT signing secret from env, falling back to dev default.
func secret() []byte {
	if s := os.Getenv("JWT_SECRET"); s != "" {
		return []byte(s)
	}
	return []byte("feeds-dev-secret")
}

// Middleware returns an auth middleware that validates JWT Bearer tokens.
// Paths in the publicPaths list bypass authentication; all others require a valid token.
//
// Usage:
//
//	public := []string{"/user/register", "/user/login"}
//	handler := auth.Middleware(mux.ServeHTTP, public)
//	http.ListenAndServe(":8080", handler)
func Middleware(next http.HandlerFunc, publicPaths []string) http.HandlerFunc {
	pub := make(map[string]bool, len(publicPaths))
	for _, p := range publicPaths {
		pub[p] = true
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if pub[r.URL.Path] {
			next(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, `{"error":"missing authorization header"}`, http.StatusUnauthorized)
			return
		}

		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return secret(), nil
		})
		if err != nil || !token.Valid {
			http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			http.Error(w, `{"error":"invalid token claims"}`, http.StatusUnauthorized)
			return
		}

		userID, _ := claims["user_id"].(string)
		if userID == "" {
			http.Error(w, `{"error":"missing user_id in token"}`, http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), userIDKey, userID)
		next(w, r.WithContext(ctx))
	}
}
