package handlers

import (
	"context"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"wg-proxy-manager/backend/internal/db"
)

type contextKey string

const claimsKey contextKey = "claims"

// JWTMiddleware validates JWT tokens or Basic Auth on protected routes.
func JWTMiddleware(jwtSecret string, database *db.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "token required"})
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid authorization header"})
				return
			}

			switch parts[0] {
			case "Bearer":
				claims := &Claims{}
				token, err := jwt.ParseWithClaims(parts[1], claims, func(token *jwt.Token) (any, error) {
					if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
						return nil, jwt.ErrSignatureInvalid
					}
					return []byte(jwtSecret), nil
				})
				if err != nil || !token.Valid {
					writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired token"})
					return
				}
				ctx := context.WithValue(r.Context(), claimsKey, claims)
				next.ServeHTTP(w, r.WithContext(ctx))

			case "Basic":
				decoded, err := base64.StdEncoding.DecodeString(parts[1])
				if err != nil {
					writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid basic auth encoding"})
					return
				}
				creds := strings.SplitN(string(decoded), ":", 2)
				if len(creds) != 2 {
					writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid basic auth format"})
					return
				}
				user, err := database.GetUserByUsername(r.Context(), creds[0])
				if err != nil {
					writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
					return
				}
				if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(creds[1])); err != nil {
					writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
					return
				}
				claims := &Claims{UserID: user.ID, Username: user.Username}
				ctx := context.WithValue(r.Context(), claimsKey, claims)
				next.ServeHTTP(w, r.WithContext(ctx))

			default:
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid authorization header"})
			}
		})
	}
}

// ValidateWSToken validates a JWT token string for WebSocket connections.
func ValidateWSToken(jwtSecret, tokenStr string) *Claims {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(jwtSecret), nil
	})
	if err != nil || !token.Valid {
		return nil
	}
	return claims
}

// GetClaims extracts JWT claims from the request context.
func GetClaims(r *http.Request) *Claims {
	claims, _ := r.Context().Value(claimsKey).(*Claims)
	return claims
}
