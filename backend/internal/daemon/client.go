package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
)

type Client struct {
	http *http.Client
}

type AddPeerResponse struct {
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

type PeerEntry struct {
	ID              int     `json:"id"`
	Name            string  `json:"name"`
	WGPublicKey     string  `json:"wg_public_key"`
	WGIP            string  `json:"wg_ip"`
	ProxyPort       int     `json:"proxy_port"`
	ProxyUser       string  `json:"proxy_user"`
	ProxyPass       string  `json:"proxy_pass"`
	Online          bool    `json:"online"`
	HealthStatus    string  `json:"health_status"`
	Endpoint        string  `json:"endpoint"`
	LatestHandshake int64   `json:"latest_handshake"`
	RxRate          float64 `json:"rx_rate"`
	TxRate          float64 `json:"tx_rate"`
	LiveRxBytes     int64   `json:"live_rx_bytes"`
	LiveTxBytes     int64   `json:"live_tx_bytes"`
	ISP             string  `json:"isp,omitempty"`
}

type MetricEntry struct {
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
	ISP             string  `json:"isp,omitempty"`
}

type ProxyEntry struct {
	DeviceID int    `json:"device_id"`
	Name     string `json:"name"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Pass     string `json:"pass"`
	Online   bool   `json:"online"`
	ProxyURL string `json:"proxy_url"`
}

func NewClient(socketPath string) *Client {
	return &Client{
		http: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", socketPath)
				},
			},
		},
	}
}

func (c *Client) Health() error {
	resp, err := c.http.Get("http://daemon/health")
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon unhealthy: %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) AddPeer(name string) (*AddPeerResponse, error) {
	body, _ := json.Marshal(map[string]string{"name": name})
	resp, err := c.http.Post("http://daemon/peers", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("call daemon: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("daemon error %d: %s", resp.StatusCode, string(data))
	}

	var result AddPeerResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &result, nil
}

func (c *Client) RemovePeer(id int) error {
	req, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("http://daemon/peers/%d", id), nil)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("call daemon: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon error: %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) ListPeers() ([]PeerEntry, error) {
	resp, err := c.http.Get("http://daemon/peers")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result []PeerEntry
	json.NewDecoder(resp.Body).Decode(&result)
	return result, nil
}

func (c *Client) GetMetrics() ([]MetricEntry, error) {
	resp, err := c.http.Get("http://daemon/metrics")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result []MetricEntry
	json.NewDecoder(resp.Body).Decode(&result)
	return result, nil
}

func (c *Client) ListProxies() ([]ProxyEntry, error) {
	resp, err := c.http.Get("http://daemon/proxies")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result []ProxyEntry
	json.NewDecoder(resp.Body).Decode(&result)
	return result, nil
}

func (c *Client) RestartProxy(id int) error {
	resp, err := c.http.Post(fmt.Sprintf("http://daemon/proxies/%d/restart", id), "", nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("restart failed: %d", resp.StatusCode)
	}
	return nil
}
