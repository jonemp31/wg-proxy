package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"wg-proxy-manager/backend/internal/config"
	"wg-proxy-manager/backend/internal/db"
)

type AuthHandler struct {
	db  *db.DB
	cfg *config.Config
	rl  *rateLimiter
}

func NewAuthHandler(database *db.DB, cfg *config.Config) *AuthHandler {
	return &AuthHandler{db: database, cfg: cfg, rl: newRateLimiter()}
}

// --- Rate Limiter ---

type attempt struct {
	count        int
	blockedUntil time.Time
}

type rateLimiter struct {
	mu       sync.Mutex
	attempts map[string]*attempt
}

func newRateLimiter() *rateLimiter {
	rl := &rateLimiter{attempts: make(map[string]*attempt)}
	go rl.cleanup()
	return rl
}

func (rl *rateLimiter) isBlocked(ip string) (bool, time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	a, ok := rl.attempts[ip]
	if !ok {
		return false, 0
	}
	if time.Now().Before(a.blockedUntil) {
		return true, time.Until(a.blockedUntil)
	}
	if a.count >= 3 && time.Now().After(a.blockedUntil) {
		delete(rl.attempts, ip)
	}
	return false, 0
}

func (rl *rateLimiter) recordFail(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	a, ok := rl.attempts[ip]
	if !ok {
		a = &attempt{}
		rl.attempts[ip] = a
	}
	a.count++
	if a.count >= 3 {
		a.blockedUntil = time.Now().Add(10 * time.Minute)
		slog.Warn("IP blocked for 10 minutes due to failed login attempts", "ip", ip, "attempts", a.count)
	}
}

func (rl *rateLimiter) recordSuccess(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.attempts, ip)
}

func (rl *rateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, a := range rl.attempts {
			if a.count >= 3 && now.After(a.blockedUntil) {
				delete(rl.attempts, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func extractIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}

// --- Handlers ---

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	ip := extractIP(r)

	if blocked, remaining := h.rl.isBlocked(ip); blocked {
		slog.Warn("blocked login attempt", "ip", ip, "remaining", remaining.Round(time.Second))
		w.Header().Set("Retry-After", fmt.Sprintf("%d", int(remaining.Seconds())))
		writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"error":         "Muitas tentativas. Aguarde antes de tentar novamente.",
			"retry_after_s": int(remaining.Seconds()),
		})
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, err := h.db.GetUserByUsername(r.Context(), req.Username)
	if err != nil {
		h.rl.recordFail(ip)
		slog.Warn("login failed: user not found", "username", req.Username, "ip", ip)
		writeError(w, http.StatusUnauthorized, "Usuário ou senha incorretos")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		h.rl.recordFail(ip)
		slog.Warn("login failed: wrong password", "username", req.Username, "ip", ip)
		writeError(w, http.StatusUnauthorized, "Usuário ou senha incorretos")
		return
	}

	h.rl.recordSuccess(ip)

	token, err := h.generateToken(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	slog.Info("login successful", "username", user.Username, "ip", ip)
	writeJSON(w, http.StatusOK, map[string]any{
		"token":    token,
		"username": user.Username,
	})
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims := GetClaims(r)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":       claims.UserID,
		"username": claims.Username,
	})
}

func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	claims := GetClaims(r)
	if claims == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.NewPassword) < 6 {
		writeError(w, http.StatusBadRequest, "senha deve ter pelo menos 6 caracteres")
		return
	}

	user, err := h.db.GetUserByUsername(r.Context(), claims.Username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "user not found")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		writeError(w, http.StatusUnauthorized, "Senha atual incorreta")
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), 12)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	if err := h.db.UpdateUserPassword(r.Context(), user.ID, string(newHash)); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update password")
		return
	}

	slog.Info("password changed", "username", user.Username)
	writeJSON(w, http.StatusOK, map[string]string{"status": "password_changed"})
}

// --- JWT ---

type Claims struct {
	UserID   int    `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func (h *AuthHandler) generateToken(user *db.User) (string, error) {
	claims := Claims{
		UserID:   user.ID,
		Username: user.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(h.cfg.JWTSecret))
}
