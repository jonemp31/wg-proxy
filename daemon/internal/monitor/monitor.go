package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"wg-proxy-manager/daemon/internal/state"
	"wg-proxy-manager/daemon/internal/wireguard"
)

// HealthStatus represents the 3-state health of a device
// "online"  = WG tunnel up + Every Proxy responding + internet works (green)
// "degraded" = WG tunnel up + Every Proxy responding, but internet check failed (yellow)
// "offline" = WG tunnel down or Every Proxy not responding (red/gray)
type PeerStatus struct {
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
	ISP             string  `json:"isp,omitempty"`
}

type Monitor struct {
	wg    *wireguard.Manager
	store *state.Store

	mu        sync.RWMutex
	statuses  map[string]*PeerStatus
	prevRx    map[string]int64
	prevTx    map[string]int64
	lastPoll  time.Time
	pollCount int

	ispCache map[string]string // IP -> ISP name
}

func New(wg *wireguard.Manager, store *state.Store) *Monitor {
	return &Monitor{
		wg:       wg,
		store:    store,
		statuses: make(map[string]*PeerStatus),
		prevRx:   make(map[string]int64),
		prevTx:   make(map[string]int64),
		ispCache: make(map[string]string),
	}
}

func (m *Monitor) Run(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	m.poll()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.poll()
		}
	}
}

func (m *Monitor) poll() {
	peers, err := m.wg.ShowDump()
	if err != nil {
		slog.Error("monitor poll failed", "error", err)
		return
	}

	now := time.Now()
	m.mu.Lock()
	elapsed := now.Sub(m.lastPoll).Seconds()
	if elapsed <= 0 {
		elapsed = 15
	}
	m.pollCount++
	doFullCheck := m.pollCount%4 == 0 // every 4th poll (~60s)
	m.mu.Unlock()

	devices := m.store.GetAll()
	devByPubKey := make(map[string]state.Device)
	for _, d := range devices {
		devByPubKey[d.WGPublicKey] = d
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, peer := range peers {
		status := &PeerStatus{
			PublicKey:       peer.PublicKey,
			Endpoint:        peer.Endpoint,
			AllowedIPs:      peer.AllowedIPs,
			LatestHandshake: peer.LatestHandshake,
			RxBytes:         peer.RxBytes,
			TxBytes:         peer.TxBytes,
			HealthStatus:    "offline",
		}

		wgUp := false
		if peer.LatestHandshake > 0 {
			age := now.Unix() - peer.LatestHandshake
			wgUp = age < 180
		}

		if wgUp {
			dev, hasDev := devByPubKey[peer.PublicKey]
			if hasDev {
				// TCP connect to Every Proxy on the phone (port 1080 via WG tunnel)
				everyProxyUp := m.checkEveryProxy(dev.WGIP, dev.PhoneProxyPort)
				if everyProxyUp {
					if doFullCheck {
						// Full end-to-end check through the proxy chain
						if m.checkProxyChain(dev.ProxyPort, dev.ProxyUser, dev.ProxyPass) {
							status.HealthStatus = "online"
						} else {
							status.HealthStatus = "degraded"
						}
					} else {
						// Between full checks, keep previous health_status if it was online or degraded
						prev, existed := m.statuses[peer.PublicKey]
						if existed && (prev.HealthStatus == "online" || prev.HealthStatus == "degraded") {
							status.HealthStatus = prev.HealthStatus
						} else {
							status.HealthStatus = "online"
						}
					}
				} else {
					status.HealthStatus = "offline"
				}
			} else {
				status.HealthStatus = "online"
			}
		}

		status.Online = status.HealthStatus != "offline"

		if prevRx, ok := m.prevRx[peer.PublicKey]; ok && m.lastPoll.Unix() > 0 {
			rxDelta := peer.RxBytes - prevRx
			txDelta := peer.TxBytes - m.prevTx[peer.PublicKey]
			if rxDelta >= 0 {
				status.RxRate = float64(rxDelta) / elapsed
			}
			if txDelta >= 0 {
				status.TxRate = float64(txDelta) / elapsed
			}
		}

		prev, existed := m.statuses[peer.PublicKey]
		if existed && prev.HealthStatus != status.HealthStatus {
			if dev, ok := m.store.GetByPublicKey(peer.PublicKey); ok {
				slog.Info("peer health changed", "device_id", dev.ID, "name", dev.Name,
					"from", prev.HealthStatus, "to", status.HealthStatus)
			}
		}

		if existed && prev.Endpoint != "" && status.Endpoint != prev.Endpoint && status.Endpoint != "" {
			if dev, ok := m.store.GetByPublicKey(peer.PublicKey); ok {
				slog.Info("peer ip changed", "device_id", dev.ID,
					"old", prev.Endpoint, "new", status.Endpoint)
			}
		}

		// ISP lookup (cached by IP)
		if status.Endpoint != "" {
			ip := extractIP(status.Endpoint)
			if ip != "" {
				if isp, ok := m.ispCache[ip]; ok {
					status.ISP = isp
				} else {
					go m.lookupISP(ip, peer.PublicKey)
					if existed && prev.ISP != "" {
						status.ISP = prev.ISP
					}
				}
			}
		}

		m.statuses[peer.PublicKey] = status
		m.prevRx[peer.PublicKey] = peer.RxBytes
		m.prevTx[peer.PublicKey] = peer.TxBytes
	}

	m.lastPoll = now
}

// checkEveryProxy does a fast TCP connect to the phone's Every Proxy port via WG tunnel
func (m *Monitor) checkEveryProxy(wgIP string, port int) bool {
	addr := fmt.Sprintf("%s:%d", wgIP, port)
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// checkProxyChain does a full HTTP request through the SOCKS5 proxy chain to verify end-to-end connectivity
func (m *Monitor) checkProxyChain(proxyPort int, user, pass string) bool {
	proxyURL := fmt.Sprintf("socks5://%s:%s@127.0.0.1:%d", user, pass, proxyPort)

	// Use net.Dial directly through SOCKS5 to avoid importing x/net
	// Simple approach: just do a TCP connect through gost to a well-known service
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", proxyPort), 5*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(10 * time.Second))

	// SOCKS5 handshake with auth
	if _, err := conn.Write([]byte{0x05, 0x01, 0x02}); err != nil {
		return false
	}
	buf := make([]byte, 2)
	if _, err := readFull(conn, buf); err != nil {
		return false
	}
	if buf[0] != 0x05 || buf[1] != 0x02 {
		return false
	}

	// Auth
	auth := []byte{0x01, byte(len(user))}
	auth = append(auth, []byte(user)...)
	auth = append(auth, byte(len(pass)))
	auth = append(auth, []byte(pass)...)
	if _, err := conn.Write(auth); err != nil {
		return false
	}
	if _, err := readFull(conn, buf); err != nil {
		return false
	}
	if buf[1] != 0x00 {
		return false
	}

	// SOCKS5 CONNECT to httpbin.org:80 (domain-based)
	domain := "httpbin.org"
	req := []byte{0x05, 0x01, 0x00, 0x03, byte(len(domain))}
	req = append(req, []byte(domain)...)
	req = append(req, 0x00, 0x50) // port 80
	if _, err := conn.Write(req); err != nil {
		return false
	}

	// Read SOCKS5 reply (at least 10 bytes for IPv4)
	reply := make([]byte, 10)
	if _, err := readFull(conn, reply); err != nil {
		return false
	}
	if reply[1] != 0x00 {
		return false
	}

	// If address type is domain (0x03) or IPv6 (0x04), read remaining bytes
	if reply[3] == 0x03 {
		extra := make([]byte, int(reply[4])+2-5)
		readFull(conn, extra)
	} else if reply[3] == 0x04 {
		extra := make([]byte, 12)
		readFull(conn, extra)
	}

	// Send minimal HTTP request
	httpReq := "GET /ip HTTP/1.1\r\nHost: httpbin.org\r\nConnection: close\r\n\r\n"
	if _, err := conn.Write([]byte(httpReq)); err != nil {
		return false
	}

	// Read enough to check for HTTP 200
	respBuf := make([]byte, 32)
	n, err := conn.Read(respBuf)
	if err != nil || n < 12 {
		return false
	}

	_ = proxyURL // used for logging context only
	return string(respBuf[:12]) == "HTTP/1.1 200"
}

func readFull(conn net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func (m *Monitor) GetAllStatuses() map[string]*PeerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]*PeerStatus, len(m.statuses))
	for k, v := range m.statuses {
		copy := *v
		result[k] = &copy
	}
	return result
}

func extractIP(endpoint string) string {
	for i := len(endpoint) - 1; i >= 0; i-- {
		if endpoint[i] == ':' {
			return endpoint[:i]
		}
	}
	return endpoint
}

func (m *Monitor) lookupISP(ip, pubKey string) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://ip-api.com/json/%s?fields=isp", ip))
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return
	}
	var result struct {
		ISP string `json:"isp"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return
	}
	isp := strings.TrimSpace(result.ISP)
	if isp == "" {
		return
	}
	m.mu.Lock()
	m.ispCache[ip] = isp
	if s, ok := m.statuses[pubKey]; ok {
		s.ISP = isp
	}
	m.mu.Unlock()
	slog.Info("ISP resolved", "ip", ip, "isp", isp)
}

func (m *Monitor) GetStatus(publicKey string) (*PeerStatus, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.statuses[publicKey]
	if !ok {
		return nil, false
	}
	copy := *s
	return &copy, true
}
