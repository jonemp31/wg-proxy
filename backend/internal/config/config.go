package config

import "os"

type Config struct {
	Port          string
	DBUrl         string
	EncryptionKey string
	DaemonSocket  string
	HostIP        string
}

func Load() *Config {
	return &Config{
		Port:          getEnv("PORT", "8080"),
		DBUrl:         getEnv("DATABASE_URL", "postgres://proxy_manager:proxy_manager@postgres:5432/proxy_manager?sslmode=disable"),
		EncryptionKey: getEnv("ENCRYPTION_KEY", ""),
		DaemonSocket:  getEnv("DAEMON_SOCKET", "/var/run/wg-manager.sock"),
		HostIP:        getEnv("HOST_IP", "192.168.100.152"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
