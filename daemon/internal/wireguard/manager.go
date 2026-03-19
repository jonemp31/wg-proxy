package wireguard

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type Manager struct {
	iface string
}

func NewManager(iface string) *Manager {
	return &Manager{iface: iface}
}

type PeerInfo struct {
	PublicKey       string `json:"public_key"`
	PresharedKey    string `json:"preshared_key"`
	Endpoint        string `json:"endpoint"`
	AllowedIPs      string `json:"allowed_ips"`
	LatestHandshake int64  `json:"latest_handshake"`
	RxBytes         int64  `json:"rx_bytes"`
	TxBytes         int64  `json:"tx_bytes"`
	Keepalive       string `json:"keepalive"`
}

func (m *Manager) AddPeer(publicKey, presharedKey, allowedIP string) error {
	pskFile, err := os.CreateTemp("", "wg-psk-*")
	if err != nil {
		return fmt.Errorf("create psk temp file: %w", err)
	}
	defer os.Remove(pskFile.Name())

	if _, err := pskFile.WriteString(presharedKey); err != nil {
		pskFile.Close()
		return fmt.Errorf("write psk: %w", err)
	}
	pskFile.Close()

	cmd := exec.Command("wg", "set", m.iface,
		"peer", publicKey,
		"preshared-key", pskFile.Name(),
		"allowed-ips", allowedIP+"/32",
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wg set peer: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

func (m *Manager) RemovePeer(publicKey string) error {
	cmd := exec.Command("wg", "set", m.iface, "peer", publicKey, "remove")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wg remove peer: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

func (m *Manager) ShowDump() ([]PeerInfo, error) {
	output, err := exec.Command("wg", "show", m.iface, "dump").Output()
	if err != nil {
		return nil, fmt.Errorf("wg show dump: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) < 2 {
		return nil, nil
	}

	var peers []PeerInfo
	for _, line := range lines[1:] {
		fields := strings.Split(line, "\t")
		if len(fields) < 8 {
			continue
		}

		peer := PeerInfo{
			PublicKey:    fields[0],
			PresharedKey: fields[1],
			Endpoint:     fields[2],
			AllowedIPs:   fields[3],
			Keepalive:    fields[7],
		}

		if v, err := strconv.ParseInt(fields[4], 10, 64); err == nil {
			peer.LatestHandshake = v
		}
		if v, err := strconv.ParseInt(fields[5], 10, 64); err == nil {
			peer.RxBytes = v
		}
		if v, err := strconv.ParseInt(fields[6], 10, 64); err == nil {
			peer.TxBytes = v
		}

		peers = append(peers, peer)
	}

	return peers, nil
}

func (m *Manager) PeerExists(publicKey string) bool {
	peers, err := m.ShowDump()
	if err != nil {
		return false
	}
	for _, p := range peers {
		if p.PublicKey == publicKey {
			return true
		}
	}
	return false
}
