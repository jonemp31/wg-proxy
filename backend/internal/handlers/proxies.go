package handlers

import (
	"math/rand/v2"
	"net/http"

	"wg-proxy-manager/backend/internal/config"
	"wg-proxy-manager/backend/internal/daemon"
)

type ProxyHandler struct {
	daemon *daemon.Client
	cfg    *config.Config
}

func NewProxyHandler(daemonClient *daemon.Client, cfg *config.Config) *ProxyHandler {
	return &ProxyHandler{daemon: daemonClient, cfg: cfg}
}

func (h *ProxyHandler) List(w http.ResponseWriter, r *http.Request) {
	proxies, err := h.daemon.ListProxies()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list proxies")
		return
	}
	writeJSON(w, http.StatusOK, proxies)
}

func (h *ProxyHandler) Available(w http.ResponseWriter, r *http.Request) {
	proxies, err := h.daemon.ListProxies()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list proxies")
		return
	}

	var online []daemon.ProxyEntry
	for _, p := range proxies {
		if p.Online {
			online = append(online, p)
		}
	}

	if len(online) == 0 {
		writeError(w, http.StatusServiceUnavailable, "no proxies available")
		return
	}

	selected := online[rand.IntN(len(online))]
	writeJSON(w, http.StatusOK, selected)
}
