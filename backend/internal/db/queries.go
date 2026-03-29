package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"wg-proxy-manager/backend/internal/models"
)

func (d *DB) InsertDevice(ctx context.Context, dev *models.Device) error {
	encPrivKey, err := d.enc.Encrypt(dev.WGPrivateKey)
	if err != nil {
		return err
	}
	encPSK, err := d.enc.Encrypt(dev.WGPresharedKey)
	if err != nil {
		return err
	}
	encPass, err := d.enc.Encrypt(dev.ProxyPass)
	if err != nil {
		return err
	}
	encConfig, err := d.enc.Encrypt(dev.ClientConfig)
	if err != nil {
		return err
	}

	return d.Pool.QueryRow(ctx,
		`INSERT INTO devices (id, name, wg_public_key, wg_private_key, wg_preshared_key,
			wg_ip, proxy_port, proxy_user, proxy_pass, client_config, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING created_at, updated_at`,
		dev.ID, dev.Name, dev.WGPublicKey, encPrivKey, encPSK,
		dev.WGIP, dev.ProxyPort, dev.ProxyUser, encPass, encConfig, dev.Status,
	).Scan(&dev.CreatedAt, &dev.UpdatedAt)
}

func (d *DB) GetDevice(ctx context.Context, id int) (*models.Device, error) {
	dev := &models.Device{}
	var encPrivKey, encPSK, encPass, encConfig string

	err := d.Pool.QueryRow(ctx,
		`SELECT id, name, wg_public_key, wg_private_key, wg_preshared_key,
			wg_ip, proxy_port, proxy_user, proxy_pass, client_config,
			status, real_ip, last_handshake, rx_bytes, tx_bytes,
			created_at, updated_at
		FROM devices WHERE id = $1`, id,
	).Scan(
		&dev.ID, &dev.Name, &dev.WGPublicKey, &encPrivKey, &encPSK,
		&dev.WGIP, &dev.ProxyPort, &dev.ProxyUser, &encPass, &encConfig,
		&dev.Status, &dev.RealIP, &dev.LastHandshake, &dev.RxBytes, &dev.TxBytes,
		&dev.CreatedAt, &dev.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	dev.WGPrivateKey, _ = d.enc.Decrypt(encPrivKey)
	dev.WGPresharedKey, _ = d.enc.Decrypt(encPSK)
	dev.ProxyPass, _ = d.enc.Decrypt(encPass)
	dev.ClientConfig, _ = d.enc.Decrypt(encConfig)

	return dev, nil
}

func (d *DB) GetAllDevices(ctx context.Context) ([]models.Device, error) {
	rows, err := d.Pool.Query(ctx,
		`SELECT id, name, wg_public_key, wg_ip, proxy_port, proxy_user, proxy_pass,
			status, real_ip, last_handshake, rx_bytes, tx_bytes,
			created_at, updated_at
		FROM devices ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []models.Device
	for rows.Next() {
		var dev models.Device
		var encPass string
		if err := rows.Scan(
			&dev.ID, &dev.Name, &dev.WGPublicKey, &dev.WGIP,
			&dev.ProxyPort, &dev.ProxyUser, &encPass,
			&dev.Status, &dev.RealIP, &dev.LastHandshake, &dev.RxBytes, &dev.TxBytes,
			&dev.CreatedAt, &dev.UpdatedAt,
		); err != nil {
			return nil, err
		}
		dev.ProxyPass, _ = d.enc.Decrypt(encPass)
		devices = append(devices, dev)
	}

	return devices, nil
}

func (d *DB) DeleteDevice(ctx context.Context, id int) error {
	tag, err := d.Pool.Exec(ctx, `DELETE FROM devices WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (d *DB) UpdateDeviceStatus(ctx context.Context, id int, status string, realIP *string, lastHandshake *time.Time, rxBytes, txBytes int64) error {
	_, err := d.Pool.Exec(ctx,
		`UPDATE devices SET status = $2, real_ip = $3, last_handshake = $4,
			rx_bytes = $5, tx_bytes = $6, updated_at = NOW()
		WHERE id = $1`,
		id, status, realIP, lastHandshake, rxBytes, txBytes,
	)
	return err
}

func (d *DB) InsertMetric(ctx context.Context, m *models.DeviceMetric) error {
	_, err := d.Pool.Exec(ctx,
		`INSERT INTO device_metrics (device_id, rx_bytes, tx_bytes, latency_ms)
		VALUES ($1, $2, $3, $4)`,
		m.DeviceID, m.RxBytes, m.TxBytes, m.LatencyMs,
	)
	return err
}

func (d *DB) GetMetricsHistory(ctx context.Context, deviceID int, since time.Time) ([]models.DeviceMetric, error) {
	rows, err := d.Pool.Query(ctx,
		`SELECT id, device_id, rx_bytes, tx_bytes, latency_ms, recorded_at
		FROM device_metrics
		WHERE device_id = $1 AND recorded_at >= $2
		ORDER BY recorded_at DESC
		LIMIT 1000`,
		deviceID, since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var metrics []models.DeviceMetric
	for rows.Next() {
		var m models.DeviceMetric
		if err := rows.Scan(&m.ID, &m.DeviceID, &m.RxBytes, &m.TxBytes, &m.LatencyMs, &m.RecordedAt); err != nil {
			return nil, err
		}
		metrics = append(metrics, m)
	}

	return metrics, nil
}

type Traffic24h struct {
	DeviceID int
	Rx24h    int64
	Tx24h    int64
}

func (d *DB) GetTraffic24h(ctx context.Context) (map[int]Traffic24h, error) {
	rows, err := d.Pool.Query(ctx,
		`SELECT device_id,
			COALESCE(MAX(rx_bytes) - MIN(rx_bytes), 0) as rx_24h,
			COALESCE(MAX(tx_bytes) - MIN(tx_bytes), 0) as tx_24h
		FROM device_metrics
		WHERE recorded_at >= NOW() - INTERVAL '24 hours'
		GROUP BY device_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int]Traffic24h)
	for rows.Next() {
		var t Traffic24h
		if err := rows.Scan(&t.DeviceID, &t.Rx24h, &t.Tx24h); err != nil {
			return nil, err
		}
		result[t.DeviceID] = t
	}
	return result, nil
}

func (d *DB) InsertEvent(ctx context.Context, e *models.DeviceEvent) error {
	_, err := d.Pool.Exec(ctx,
		`INSERT INTO device_events (device_id, event_type, details)
		VALUES ($1, $2, $3)`,
		e.DeviceID, e.EventType, e.Details,
	)
	return err
}

func (d *DB) GetEvents(ctx context.Context, deviceID int, limit int) ([]models.DeviceEvent, error) {
	rows, err := d.Pool.Query(ctx,
		`SELECT id, device_id, event_type, details, occurred_at
		FROM device_events
		WHERE device_id = $1
		ORDER BY occurred_at DESC
		LIMIT $2`,
		deviceID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []models.DeviceEvent
	for rows.Next() {
		var e models.DeviceEvent
		if err := rows.Scan(&e.ID, &e.DeviceID, &e.EventType, &e.Details, &e.OccurredAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}

	return events, nil
}

func (d *DB) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := d.Pool.QueryRow(ctx, `SELECT value FROM settings WHERE key = $1`, key).Scan(&value)
	if err != nil {
		return "", err
	}
	return value, nil
}

func (d *DB) SetSetting(ctx context.Context, key, value string) error {
	_, err := d.Pool.Exec(ctx,
		`INSERT INTO settings (key, value) VALUES ($1, $2)
		 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`,
		key, value,
	)
	return err
}

func (d *DB) DeleteSetting(ctx context.Context, key string) error {
	_, err := d.Pool.Exec(ctx, `DELETE FROM settings WHERE key = $1`, key)
	return err
}

type User struct {
	ID           int    `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
}

func (d *DB) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	u := &User{}
	err := d.Pool.QueryRow(ctx,
		`SELECT id, username, password_hash FROM users WHERE username = $1`, username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (d *DB) UpdateUserPassword(ctx context.Context, id int, passwordHash string) error {
	_, err := d.Pool.Exec(ctx,
		`UPDATE users SET password_hash = $2 WHERE id = $1`,
		id, passwordHash,
	)
	return err
}
