package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"wg-proxy-manager/backend/internal/config"
	"wg-proxy-manager/backend/internal/daemon"
	"wg-proxy-manager/backend/internal/db"
	"wg-proxy-manager/backend/internal/models"
	ws "wg-proxy-manager/backend/internal/websocket"
)

type DeviceHandler struct {
	db     *db.DB
	daemon *daemon.Client
	hub    *ws.Hub
	cfg    *config.Config
}

func NewDeviceHandler(database *db.DB, daemonClient *daemon.Client, hub *ws.Hub, cfg *config.Config) *DeviceHandler {
	return &DeviceHandler{db: database, daemon: daemonClient, hub: hub, cfg: cfg}
}

func (h *DeviceHandler) List(w http.ResponseWriter, r *http.Request) {
	devices, err := h.db.GetAllDevices(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	peers, _ := h.daemon.ListPeers()

	type deviceResponse struct {
		models.Device
		Online       bool    `json:"online"`
		HealthStatus string  `json:"health_status"`
		Endpoint     string  `json:"endpoint,omitempty"`
		RxRate       float64 `json:"rx_rate"`
		TxRate       float64 `json:"tx_rate"`
		Rx24h        int64   `json:"rx_bytes_24h"`
		Tx24h        int64   `json:"tx_bytes_24h"`
		ProxyURL     string  `json:"proxy_url"`
		ISP          string  `json:"isp,omitempty"`
	}

	peerMap := make(map[int]daemon.PeerEntry)
	for _, p := range peers {
		peerMap[p.ID] = p
	}

	traffic24h, _ := h.db.GetTraffic24h(r.Context())

	result := make([]deviceResponse, 0, len(devices))
	for _, dev := range devices {
		resp := deviceResponse{
			Device:       dev,
			HealthStatus: "offline",
			ProxyURL:     fmt.Sprintf("socks5://%s:%s@%s:%d", dev.ProxyUser, dev.ProxyPass, h.cfg.HostIP, dev.ProxyPort),
		}
		if p, ok := peerMap[dev.ID]; ok {
			resp.Online = p.Online
			resp.HealthStatus = p.HealthStatus
			resp.Endpoint = p.Endpoint
			resp.RxRate = p.RxRate
			resp.TxRate = p.TxRate
			resp.ISP = p.ISP
		}
		if t, ok := traffic24h[dev.ID]; ok {
			resp.Rx24h = t.Rx24h
			resp.Tx24h = t.Tx24h
		}
		result = append(result, resp)
	}

	writeJSON(w, http.StatusOK, result)
}

type createDeviceRequest struct {
	Name string `json:"name"`
}

func (h *DeviceHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	peerResp, err := h.daemon.AddPeer(req.Name)
	if err != nil {
		slog.Error("daemon add peer failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to add device: "+err.Error())
		return
	}

	dev := &models.Device{
		ID:             peerResp.DeviceID,
		Name:           peerResp.Name,
		WGPublicKey:    peerResp.WGPublicKey,
		WGPrivateKey:   peerResp.WGPrivateKey,
		WGPresharedKey: peerResp.WGPresharedKey,
		WGIP:           peerResp.WGIP,
		ProxyPort:      peerResp.ProxyPort,
		ProxyUser:      peerResp.ProxyUser,
		ProxyPass:      peerResp.ProxyPass,
		ClientConfig:   peerResp.ClientConfig,
		Status:         peerResp.Status,
	}

	if err := h.db.InsertDevice(r.Context(), dev); err != nil {
		slog.Error("db insert device failed", "error", err)
		h.daemon.RemovePeer(dev.ID)
		writeError(w, http.StatusInternalServerError, "failed to save device")
		return
	}

	h.db.InsertEvent(r.Context(), &models.DeviceEvent{
		DeviceID:  dev.ID,
		EventType: "created",
		Details:   fmt.Sprintf("Device %s created", dev.Name),
	})

	h.hub.Broadcast(ws.Event{
		Type: "device_created",
		Data: map[string]any{"device_id": dev.ID, "name": dev.Name},
	})

	slog.Info("device created", "id", dev.ID, "name", dev.Name)

	writeJSON(w, http.StatusCreated, map[string]any{
		"device":        dev,
		"client_config": peerResp.ClientConfig,
		"proxy_url":     fmt.Sprintf("socks5://%s:%s@%s:%d", dev.ProxyUser, dev.ProxyPass, h.cfg.HostIP, dev.ProxyPort),
	})
}

func (h *DeviceHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	dev, err := h.db.GetDevice(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"device":    dev,
		"proxy_url": fmt.Sprintf("socks5://%s:%s@%s:%d", dev.ProxyUser, dev.ProxyPass, h.cfg.HostIP, dev.ProxyPort),
	})
}

func (h *DeviceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.daemon.RemovePeer(id); err != nil {
		slog.Error("daemon remove peer failed", "error", err, "device_id", id)
	}

	if err := h.db.DeleteDevice(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}

	h.hub.Broadcast(ws.Event{
		Type: "device_removed",
		Data: map[string]any{"device_id": id},
	})

	slog.Info("device deleted", "id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (h *DeviceHandler) GetQRCode(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	dev, err := h.db.GetDevice(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}

	png, err := generateQRCode(dev.ClientConfig)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "generate qr code: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(png)))
	w.Write(png)
}

func (h *DeviceHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	dev, err := h.db.GetDevice(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=wg-cel%d.conf", id))
	w.Write([]byte(dev.ClientConfig))
}

func (h *DeviceHandler) StartPolling(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			h.pollMetrics()
		}
	}()
}

func (h *DeviceHandler) pollMetrics() {
	metrics, err := h.daemon.GetMetrics()
	if err != nil {
		return
	}

	ctx := context.Background()
	devices, _ := h.db.GetAllDevices(ctx)
	devMap := make(map[int]models.Device)
	for _, d := range devices {
		devMap[d.ID] = d
	}

	updates := make([]map[string]any, 0)

	for _, m := range metrics {
		dev, ok := devMap[m.DeviceID]
		if !ok {
			continue
		}

		newStatus := "offline"
		if m.Online {
			newStatus = "online"
		}
		healthStatus := m.HealthStatus
		if healthStatus == "" {
			healthStatus = newStatus
		}

		var realIP *string
		if m.Endpoint != "" {
			host := extractHost(m.Endpoint)
			realIP = &host
		}

		var handshake *time.Time
		if m.LatestHandshake > 0 {
			t := time.Unix(m.LatestHandshake, 0)
			handshake = &t
		}

		h.db.UpdateDeviceStatus(ctx, dev.ID, newStatus, realIP, handshake, m.RxBytes, m.TxBytes)

		h.db.InsertMetric(ctx, &models.DeviceMetric{
			DeviceID: dev.ID,
			RxBytes:  m.RxBytes,
			TxBytes:  m.TxBytes,
		})

		if dev.Status != newStatus {
			h.db.InsertEvent(ctx, &models.DeviceEvent{
				DeviceID:  dev.ID,
				EventType: newStatus,
			})
		}

		updates = append(updates, map[string]any{
			"id":            dev.ID,
			"status":        newStatus,
			"health_status": healthStatus,
			"rx_rate":       m.RxRate,
			"tx_rate":       m.TxRate,
			"rx_bytes":      m.RxBytes,
			"tx_bytes":      m.TxBytes,
			"real_ip":       realIP,
			"isp":           m.ISP,
		})
	}

	if len(updates) > 0 {
		h.hub.Broadcast(ws.Event{
			Type: "metrics_update",
			Data: updates,
		})
	}
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func extractHost(endpoint string) string {
	for i := len(endpoint) - 1; i >= 0; i-- {
		if endpoint[i] == ':' {
			return endpoint[:i]
		}
	}
	return endpoint
}
