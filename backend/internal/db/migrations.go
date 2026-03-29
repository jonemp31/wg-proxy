package db

import (
	"context"
	"log/slog"

	"golang.org/x/crypto/bcrypt"
)

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

		`CREATE TABLE IF NOT EXISTS settings (
			key   VARCHAR(100) PRIMARY KEY,
			value TEXT NOT NULL
		)`,

		`INSERT INTO settings (key, value) VALUES ('webhook_url', 'https://webdurov.autopilots.trade/webhook/wire-proxys')
		 ON CONFLICT (key) DO NOTHING`,

		`CREATE TABLE IF NOT EXISTS users (
			id            SERIAL PRIMARY KEY,
			username      VARCHAR(50) NOT NULL UNIQUE,
			password_hash VARCHAR(255) NOT NULL,
			created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
	}

	for _, m := range migrations {
		if _, err := d.Pool.Exec(ctx, m); err != nil {
			return err
		}
	}

	return nil
}

func (d *DB) SeedDefaultUser(ctx context.Context, username, password string) error {
	var count int
	err := d.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return err
	}

	_, err = d.Pool.Exec(ctx,
		`INSERT INTO users (username, password_hash) VALUES ($1, $2) ON CONFLICT (username) DO NOTHING`,
		username, string(hash),
	)
	if err != nil {
		return err
	}

	slog.Info("default admin user created", "username", username)
	return nil
}
