package models

import "time"

type Device struct {
	ID             int        `json:"id"`
	Name           string     `json:"name"`
	WGPublicKey    string     `json:"wg_public_key"`
	WGPrivateKey   string     `json:"-"`
	WGPresharedKey string     `json:"-"`
	WGIP           string     `json:"wg_ip"`
	ProxyPort      int        `json:"proxy_port"`
	ProxyUser      string     `json:"proxy_user"`
	ProxyPass      string     `json:"-"`
	ClientConfig   string     `json:"-"`
	Status         string     `json:"status"`
	RealIP         *string    `json:"real_ip,omitempty"`
	LastHandshake  *time.Time `json:"last_handshake,omitempty"`
	RxBytes        int64      `json:"rx_bytes"`
	TxBytes        int64      `json:"tx_bytes"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type DeviceEvent struct {
	ID        int64     `json:"id"`
	DeviceID  int       `json:"device_id"`
	EventType string    `json:"event_type"`
	Details   string    `json:"details,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
}

type DeviceMetric struct {
	ID         int64     `json:"id"`
	DeviceID   int       `json:"device_id"`
	RxBytes    int64     `json:"rx_bytes"`
	TxBytes    int64     `json:"tx_bytes"`
	LatencyMs  *int      `json:"latency_ms,omitempty"`
	RecordedAt time.Time `json:"recorded_at"`
}
