package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"

	"wg-proxy-manager/daemon/internal/config"
	"wg-proxy-manager/daemon/internal/monitor"
	"wg-proxy-manager/daemon/internal/proxy"
	"wg-proxy-manager/daemon/internal/state"
	"wg-proxy-manager/daemon/internal/wireguard"
)

type Handler struct {
	mu      sync.Mutex
	cfg     *config.Config
	store   *state.Store
	wg      *wireguard.Manager
	proxy   *proxy.Manager
	monitor *monitor.Monitor
}

func NewHandler(
	cfg *config.Config,
	store *state.Store,
	wg *wireguard.Manager,
	proxyMgr *proxy.Manager,
	mon *monitor.Monitor,
) *Handler {
	return &Handler{
		cfg:     cfg,
		store:   store,
		wg:      wg,
		proxy:   proxyMgr,
		monitor: mon,
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", h.healthHandler)
	mux.HandleFunc("GET /peers", h.listPeersHandler)
	mux.HandleFunc("POST /peers", h.addPeerHandler)
	mux.HandleFunc("DELETE /peers/{id}", h.removePeerHandler)
	mux.HandleFunc("GET /peers/{id}/status", h.peerStatusHandler)
	mux.HandleFunc("GET /proxies", h.listProxiesHandler)
	mux.HandleFunc("POST /proxies/{id}/restart", h.restartProxyHandler)
	mux.HandleFunc("GET /metrics", h.metricsHandler)
}

func (h *Handler) healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type addPeerRequest struct {
	Name string `json:"name"`
}

type addPeerResponse struct {
	DeviceID       int    `json:"device_id"`
	Name           string `json:"name"`
	WGIP           string `json:"wg_ip"`
	WGPublicKey    string `json:"wg_public_key"`
	WGPrivateKey   string `json:"wg_private_key"`
	WGPresharedKey string `json:"wg_preshared_key"`
	ProxyPort      int    `json:"proxy_port"`
	ProxyUser      string `json:"proxy_user"`
	ProxyPass      string `json:"proxy_pass"`
	ClientConfig   string `json:"client_config"`
	Status         string `json:"status"`
}

func (h *Handler) addPeerHandler(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	var req addPeerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	id := h.store.NextID(h.cfg.MaxDevices)
	if id < 0 {
		writeError(w, http.StatusConflict, fmt.Sprintf("maximum devices reached (%d)", h.cfg.MaxDevices))
		return
	}

	privKey, pubKey, err := wireguard.GenerateKeyPair()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "generate keypair: "+err.Error())
		return
	}

	psk, err := wireguard.GeneratePSK()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "generate psk: "+err.Error())
		return
	}

	wgIP := state.DeviceIP(h.cfg.WGSubnet, id)
	proxyPort := h.cfg.ProxyPortBase + id - 1
	proxyUser := req.Name
	proxyPass := state.GeneratePassword(32)

	clientConfig := wireguard.GenerateClientConfig(
		privKey, psk, wgIP, h.cfg.ServerPubKey, h.cfg.WGEndpoint, h.cfg.WGSubnetCIDR,
	)

	dev := state.Device{
		ID:             id,
		Name:           req.Name,
		WGPublicKey:    pubKey,
		WGPrivateKey:   privKey,
		WGPresharedKey: psk,
		WGIP:           wgIP,
		ProxyPort:      proxyPort,
		ProxyUser:      proxyUser,
		ProxyPass:      proxyPass,
		PhoneProxyPort: h.cfg.PhoneProxyPort,
		ClientConfig:   clientConfig,
	}

	if err := h.wg.AddPeer(pubKey, psk, wgIP); err != nil {
		writeError(w, http.StatusInternalServerError, "add wg peer: "+err.Error())
		return
	}

	if err := h.proxy.Start(dev); err != nil {
		h.wg.RemovePeer(pubKey)
		writeError(w, http.StatusInternalServerError, "start proxy: "+err.Error())
		return
	}

	if err := h.store.Add(dev); err != nil {
		h.proxy.Stop(dev.ID)
		h.wg.RemovePeer(pubKey)
		writeError(w, http.StatusInternalServerError, "save state: "+err.Error())
		return
	}

	slog.Info("peer added", "device_id", id, "name", req.Name, "wg_ip", wgIP, "port", proxyPort)

	writeJSON(w, http.StatusCreated, addPeerResponse{
		DeviceID:       id,
		Name:           req.Name,
		WGIP:           wgIP,
		WGPublicKey:    pubKey,
		WGPrivateKey:   privKey,
		WGPresharedKey: psk,
		ProxyPort:      proxyPort,
		ProxyUser:      proxyUser,
		ProxyPass:      proxyPass,
		ClientConfig:   clientConfig,
		Status:         "awaiting_connection",
	})
}

func (h *Handler) removePeerHandler(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	dev, ok := h.store.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}

	h.proxy.Stop(dev.ID)
	h.wg.RemovePeer(dev.WGPublicKey)

	if err := h.store.Remove(id); err != nil {
		slog.Error("failed to remove from state", "device_id", id, "error", err)
	}

	slog.Info("peer removed", "device_id", id, "name", dev.Name)
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func (h *Handler) listPeersHandler(w http.ResponseWriter, r *http.Request) {
	devices := h.store.GetAll()
	statuses := h.monitor.GetAllStatuses()

	type peerEntry struct {
		state.Device
		Online          bool    `json:"online"`
		HealthStatus    string  `json:"health_status"`
		Endpoint        string  `json:"endpoint"`
		LatestHandshake int64   `json:"latest_handshake"`
		RxRate          float64 `json:"rx_rate"`
		TxRate          float64 `json:"tx_rate"`
		LiveRxBytes     int64   `json:"live_rx_bytes"`
		LiveTxBytes     int64   `json:"live_tx_bytes"`
	}

	result := make([]peerEntry, 0, len(devices))
	for _, dev := range devices {
		entry := peerEntry{Device: dev, HealthStatus: "offline"}
		if s, ok := statuses[dev.WGPublicKey]; ok {
			entry.Online = s.Online
			entry.HealthStatus = s.HealthStatus
			entry.Endpoint = s.Endpoint
			entry.LatestHandshake = s.LatestHandshake
			entry.RxRate = s.RxRate
			entry.TxRate = s.TxRate
			entry.LiveRxBytes = s.RxBytes
			entry.LiveTxBytes = s.TxBytes
		}
		result = append(result, entry)
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) peerStatusHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	dev, ok := h.store.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}

	status, _ := h.monitor.GetStatus(dev.WGPublicKey)
	proxyHealthy := proxy.CheckHealth(dev.ProxyPort, dev.ProxyUser, dev.ProxyPass) == nil

	writeJSON(w, http.StatusOK, map[string]any{
		"device":        dev,
		"wg_status":     status,
		"proxy_running": h.proxy.IsRunning(dev.ID),
		"proxy_healthy": proxyHealthy,
	})
}

func (h *Handler) listProxiesHandler(w http.ResponseWriter, r *http.Request) {
	devices := h.store.GetAll()
	statuses := h.monitor.GetAllStatuses()

	type proxyEntry struct {
		DeviceID     int    `json:"device_id"`
		Name         string `json:"name"`
		Host         string `json:"host"`
		Port         int    `json:"port"`
		User         string `json:"user"`
		Pass         string `json:"pass"`
		Online       bool   `json:"online"`
		HealthStatus string `json:"health_status"`
		ProxyURL     string `json:"proxy_url"`
	}

	result := make([]proxyEntry, 0)
	for _, dev := range devices {
		online := false
		healthStatus := "offline"
		if s, ok := statuses[dev.WGPublicKey]; ok {
			online = s.Online
			healthStatus = s.HealthStatus
		}
		result = append(result, proxyEntry{
			DeviceID:     dev.ID,
			Name:         dev.Name,
			Host:         "192.168.100.152",
			Port:         dev.ProxyPort,
			User:         dev.ProxyUser,
			Pass:         dev.ProxyPass,
			Online:       online,
			HealthStatus: healthStatus,
			ProxyURL:     fmt.Sprintf("socks5://%s:%s@192.168.100.152:%d", dev.ProxyUser, dev.ProxyPass, dev.ProxyPort),
		})
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) restartProxyHandler(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	dev, ok := h.store.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "device not found")
		return
	}

	h.proxy.Stop(dev.ID)

	if err := h.proxy.Start(dev); err != nil {
		writeError(w, http.StatusInternalServerError, "restart proxy: "+err.Error())
		return
	}

	slog.Info("proxy restarted", "device_id", id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "restarted"})
}

func (h *Handler) metricsHandler(w http.ResponseWriter, r *http.Request) {
	statuses := h.monitor.GetAllStatuses()
	devices := h.store.GetAll()

	type metricEntry struct {
		DeviceID        int     `json:"device_id"`
		Name            string  `json:"name"`
		PublicKey       string  `json:"public_key"`
		Endpoint        string  `json:"endpoint"`
		AllowedIPs      string  `json:"allowed_ips"`
		LatestHandshake int64   `json:"latest_handshake"`
		Online          bool    `json:"online"`
		HealthStatus    string  `json:"health_status"`
		RxBytes         int64   `json:"rx_bytes"`
		TxBytes         int64   `json:"tx_bytes"`
		RxRate          float64 `json:"rx_rate"`
		TxRate          float64 `json:"tx_rate"`
	}

	result := make([]metricEntry, 0, len(devices))
	for _, dev := range devices {
		entry := metricEntry{
			DeviceID:     dev.ID,
			Name:         dev.Name,
			PublicKey:    dev.WGPublicKey,
			HealthStatus: "offline",
		}
		if s, ok := statuses[dev.WGPublicKey]; ok {
			entry.Endpoint = s.Endpoint
			entry.AllowedIPs = s.AllowedIPs
			entry.LatestHandshake = s.LatestHandshake
			entry.Online = s.Online
			entry.HealthStatus = s.HealthStatus
			entry.RxBytes = s.RxBytes
			entry.TxBytes = s.TxBytes
			entry.RxRate = s.RxRate
			entry.TxRate = s.TxRate
		}
		result = append(result, entry)
	}

	writeJSON(w, http.StatusOK, result)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
