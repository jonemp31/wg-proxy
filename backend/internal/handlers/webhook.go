package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"wg-proxy-manager/backend/internal/config"
	"wg-proxy-manager/backend/internal/db"
)

type WebhookHandler struct {
	db  *db.DB
	cfg *config.Config
}

func NewWebhookHandler(database *db.DB, cfg *config.Config) *WebhookHandler {
	return &WebhookHandler{db: database, cfg: cfg}
}

func (h *WebhookHandler) Get(w http.ResponseWriter, r *http.Request) {
	url, err := h.db.GetSetting(r.Context(), "webhook_url")
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{"webhook_url": ""})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"webhook_url": url})
}

func (h *WebhookHandler) Set(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}

	if err := h.db.SetSetting(r.Context(), "webhook_url", req.URL); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save webhook")
		return
	}

	slog.Info("webhook configured", "url", req.URL)
	writeJSON(w, http.StatusOK, map[string]string{"status": "saved", "webhook_url": req.URL})
}

func (h *WebhookHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if err := h.db.DeleteSetting(r.Context(), "webhook_url"); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete webhook")
		return
	}
	slog.Info("webhook removed")
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (h *WebhookHandler) Test(w http.ResponseWriter, r *http.Request) {
	url, err := h.db.GetSetting(r.Context(), "webhook_url")
	if err != nil || url == "" {
		writeError(w, http.StatusBadRequest, "no webhook configured")
		return
	}

	payload := map[string]any{
		"event":     "test",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"message":   "Teste de webhook do WG Proxy Manager",
	}

	statusCode, sendErr := SendWebhook(url, payload)
	if sendErr != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "error",
			"error":  sendErr.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "sent",
		"status_code": statusCode,
	})
}

// SendWebhook sends a POST request with JSON payload to the given URL.
// Returns the HTTP status code and any error.
func SendWebhook(url string, payload any) (int, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("marshal payload: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("send webhook: %w", err)
	}
	defer resp.Body.Close()

	return resp.StatusCode, nil
}

// NotifyStatusChange sends a webhook notification when a device status changes.
func NotifyStatusChange(database *db.DB, hostIP string, deviceID int, deviceName string, proxyPort int, proxyUser, proxyPass, previousStatus, currentStatus string) {
	url, err := database.GetSetting(context.Background(), "webhook_url")
	if err != nil || url == "" {
		return
	}

	payload := map[string]any{
		"event":     "status_changed",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"device": map[string]any{
			"id":         deviceID,
			"name":       deviceName,
			"proxy_port": proxyPort,
			"proxy_url":  fmt.Sprintf("socks5://%s:%s@%s:%d", proxyUser, proxyPass, hostIP, proxyPort),
		},
		"status": map[string]string{
			"previous": previousStatus,
			"current":  currentStatus,
		},
	}

	go func() {
		statusCode, err := SendWebhook(url, payload)
		if err != nil {
			slog.Error("webhook failed", "device", deviceName, "error", err)
		} else {
			slog.Info("webhook sent", "device", deviceName, "from", previousStatus, "to", currentStatus, "http_status", statusCode)
		}
	}()
}
