package handlers

import (
	"net/http"
	"strconv"
	"time"

	"wg-proxy-manager/backend/internal/daemon"
	"wg-proxy-manager/backend/internal/db"
)

type MetricsHandler struct {
	db     *db.DB
	daemon *daemon.Client
}

func NewMetricsHandler(database *db.DB, daemonClient *daemon.Client) *MetricsHandler {
	return &MetricsHandler{db: database, daemon: daemonClient}
}

func (h *MetricsHandler) Global(w http.ResponseWriter, r *http.Request) {
	metrics, err := h.daemon.GetMetrics()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get metrics")
		return
	}

	totalRx := int64(0)
	totalTx := int64(0)
	onlineCount := 0
	for _, m := range metrics {
		totalRx += m.RxBytes
		totalTx += m.TxBytes
		if m.Online {
			onlineCount++
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"total_devices":  len(metrics),
		"online_devices": onlineCount,
		"total_rx_bytes": totalRx,
		"total_tx_bytes": totalTx,
		"peers":          metrics,
	})
}

func (h *MetricsHandler) DeviceHistory(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	period := r.URL.Query().Get("period")
	since := time.Now().Add(-24 * time.Hour)
	switch period {
	case "7d":
		since = time.Now().Add(-7 * 24 * time.Hour)
	case "30d":
		since = time.Now().Add(-30 * 24 * time.Hour)
	}

	metrics, err := h.db.GetMetricsHistory(r.Context(), id, since)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get metrics")
		return
	}

	events, err := h.db.GetEvents(r.Context(), id, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get events")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"metrics": metrics,
		"events":  events,
	})
}
