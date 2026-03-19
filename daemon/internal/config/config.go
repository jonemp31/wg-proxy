package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	SocketPath     string
	WGInterface    string
	WGSubnet       string // first 3 octets base, e.g. "10.0" for 10.0.0.0/22
	WGSubnetCIDR   string // full CIDR for client configs, e.g. "10.0.0.0/22"
	WGServerIP     string
	WGListenPort   int
	WGEndpoint     string // DDNS hostname:port for client configs
	ProxyPortBase  int
	PhoneProxyPort int // port where phone's proxy app listens (default 1080)
	MaxDevices     int
	StatePath      string
	GostBinary     string
	ServerPubKey   string
}

func Load() *Config {
	cfg := &Config{
		SocketPath:     getEnv("SOCKET_PATH", "/var/run/wg-manager.sock"),
		WGInterface:    getEnv("WG_INTERFACE", "wg0"),
		WGSubnet:       getEnv("WG_SUBNET", "10.0"),
		WGSubnetCIDR:   getEnv("WG_SUBNET_CIDR", "10.0.0.0/22"),
		WGServerIP:     getEnv("WG_SERVER_IP", "10.0.0.1"),
		WGListenPort:   getEnvInt("WG_LISTEN_PORT", 51820),
		WGEndpoint:     getEnv("WG_ENDPOINT", ""),
		ProxyPortBase:  getEnvInt("PROXY_PORT_BASE", 1081),
		PhoneProxyPort: getEnvInt("PHONE_PROXY_PORT", 1080),
		MaxDevices:     getEnvInt("MAX_DEVICES", 1021),
		StatePath:      getEnv("STATE_PATH", "/var/lib/wg-manager/state.json"),
		GostBinary:     getEnv("GOST_BINARY", "/usr/local/bin/gost"),
	}

	pubKeyPath := getEnv("WG_SERVER_PUBKEY_PATH", "/etc/wireguard/server_public.key")
	if data, err := os.ReadFile(pubKeyPath); err == nil {
		cfg.ServerPubKey = strings.TrimSpace(string(data))
	}

	return cfg
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}
