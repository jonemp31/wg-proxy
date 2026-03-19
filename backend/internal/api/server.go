package api

import (
	"net/http"

	"wg-proxy-manager/backend/internal/handlers"
	ws "wg-proxy-manager/backend/internal/websocket"
)

func NewRouter(
	deviceHandler *handlers.DeviceHandler,
	proxyHandler *handlers.ProxyHandler,
	metricsHandler *handlers.MetricsHandler,
	hub *ws.Hub,
) http.Handler {
	mux := http.NewServeMux()

	// Device endpoints
	mux.HandleFunc("GET /api/devices", deviceHandler.List)
	mux.HandleFunc("POST /api/devices", deviceHandler.Create)
	mux.HandleFunc("GET /api/devices/{id}", deviceHandler.Get)
	mux.HandleFunc("DELETE /api/devices/{id}", deviceHandler.Delete)
	mux.HandleFunc("GET /api/devices/{id}/qrcode", deviceHandler.GetQRCode)
	mux.HandleFunc("GET /api/devices/{id}/config", deviceHandler.GetConfig)

	// Proxy endpoints
	mux.HandleFunc("GET /api/proxies", proxyHandler.List)
	mux.HandleFunc("GET /api/proxies/available", proxyHandler.Available)

	// Metrics endpoints
	mux.HandleFunc("GET /api/metrics", metricsHandler.Global)
	mux.HandleFunc("GET /api/metrics/{id}", metricsHandler.DeviceHistory)

	// WebSocket
	mux.HandleFunc("GET /ws", hub.HandleWS)

	// Health
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Apply middleware chain
	var handler http.Handler = mux
	handler = CORSMiddleware(handler)
	handler = LoggingMiddleware(handler)
	handler = RecoveryMiddleware(handler)

	return handler
}
