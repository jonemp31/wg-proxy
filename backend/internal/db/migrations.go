package db

import "context"

func (d *DB) RunMigrations(ctx context.Context) error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS devices (
			id              INTEGER PRIMARY KEY,
			name            VARCHAR(255) NOT NULL,
			wg_public_key   VARCHAR(44) NOT NULL UNIQUE,
			wg_private_key  TEXT NOT NULL,
			wg_preshared_key TEXT NOT NULL,
			wg_ip           VARCHAR(15) NOT NULL UNIQUE,
			proxy_port      INTEGER NOT NULL UNIQUE,
			proxy_user      VARCHAR(50) NOT NULL,
			proxy_pass      TEXT NOT NULL,
			client_config   TEXT NOT NULL,
			status          VARCHAR(20) NOT NULL DEFAULT 'awaiting_connection',
			real_ip         VARCHAR(45),
			last_handshake  TIMESTAMPTZ,
			rx_bytes        BIGINT NOT NULL DEFAULT 0,
			tx_bytes        BIGINT NOT NULL DEFAULT 0,
			created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		`CREATE INDEX IF NOT EXISTS idx_devices_status ON devices(status)`,

		`CREATE TABLE IF NOT EXISTS device_metrics (
			id          BIGSERIAL PRIMARY KEY,
			device_id   INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
			rx_bytes    BIGINT NOT NULL,
			tx_bytes    BIGINT NOT NULL,
			latency_ms  INTEGER,
			recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		`CREATE INDEX IF NOT EXISTS idx_device_metrics_device_time
			ON device_metrics(device_id, recorded_at DESC)`,

		`CREATE TABLE IF NOT EXISTS device_events (
			id          BIGSERIAL PRIMARY KEY,
			device_id   INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
			event_type  VARCHAR(50) NOT NULL,
			details     TEXT,
			occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,

		`CREATE INDEX IF NOT EXISTS idx_device_events_device_time
			ON device_events(device_id, occurred_at DESC)`,
	}

	for _, m := range migrations {
		if _, err := d.Pool.Exec(ctx, m); err != nil {
			return err
		}
	}

	return nil
}
