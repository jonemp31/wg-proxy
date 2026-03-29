package api

import (
	"net/http"
	"strings"

	"wg-proxy-manager/backend/internal/db"
	"wg-proxy-manager/backend/internal/handlers"
	ws "wg-proxy-manager/backend/internal/websocket"
)

func NewRouter(
	deviceHandler *handlers.DeviceHandler,
	proxyHandler *handlers.ProxyHandler,
	metricsHandler *handlers.MetricsHandler,
	webhookHandler *handlers.WebhookHandler,
	authHandler *handlers.AuthHandler,
	hub *ws.Hub,
	jwtSecret string,
	database *db.DB,
) http.Handler {
	mux := http.NewServeMux()

	// Auth endpoints (public)
	mux.HandleFunc("POST /api/auth/login", authHandler.Login)

	// Health (public)
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Protected routes
	protected := http.NewServeMux()

	// Auth (protected)
	protected.HandleFunc("GET /api/auth/me", authHandler.Me)
	protected.HandleFunc("POST /api/auth/change-password", authHandler.ChangePassword)

	// Device endpoints
	protected.HandleFunc("GET /api/devices", deviceHandler.List)
	protected.HandleFunc("POST /api/devices", deviceHandler.Create)
	protected.HandleFunc("GET /api/devices/{id}", deviceHandler.Get)
	protected.HandleFunc("DELETE /api/devices/{id}", deviceHandler.Delete)
	protected.HandleFunc("GET /api/devices/{id}/qrcode", deviceHandler.GetQRCode)
	protected.HandleFunc("GET /api/devices/{id}/config", deviceHandler.GetConfig)

	// Proxy endpoints
	protected.HandleFunc("GET /api/proxies", proxyHandler.List)
	protected.HandleFunc("GET /api/proxies/available", proxyHandler.Available)

	// Metrics endpoints
	protected.HandleFunc("GET /api/metrics", metricsHandler.Global)
	protected.HandleFunc("GET /api/metrics/{id}", metricsHandler.DeviceHistory)

	// Webhook settings
	protected.HandleFunc("GET /api/settings/webhook", webhookHandler.Get)
	protected.HandleFunc("PUT /api/settings/webhook", webhookHandler.Set)
	protected.HandleFunc("DELETE /api/settings/webhook", webhookHandler.Delete)
	protected.HandleFunc("POST /api/settings/webhook/test", webhookHandler.Test)

	// Apply JWT middleware to protected routes
	jwtMiddleware := handlers.JWTMiddleware(jwtSecret, database)
	mux.Handle("/api/", jwtMiddleware(protected))

	// QR code endpoint — allows token via query param (for <img src=>)
	mux.HandleFunc("GET /api/devices/{id}/qrcode", func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		jwtMiddleware(http.HandlerFunc(deviceHandler.GetQRCode)).ServeHTTP(w, r)
	})

	// WebSocket with token validation via query param
	mux.HandleFunc("GET /ws", func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" {
			// Also check Authorization header
			auth := r.Header.Get("Authorization")
			if strings.HasPrefix(auth, "Bearer ") {
				token = auth[7:]
			}
		}
		if claims := handlers.ValidateWSToken(jwtSecret, token); claims == nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		hub.HandleWS(w, r)
	})

	// Apply middleware chain
	var handler http.Handler = mux
	handler = CORSMiddleware(handler)
	handler = LoggingMiddleware(handler)
	handler = RecoveryMiddleware(handler)

	return handler
}
