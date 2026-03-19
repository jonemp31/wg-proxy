package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"wg-proxy-manager/backend/internal/crypto"
)

type DB struct {
	Pool *pgxpool.Pool
	enc  *crypto.Encryptor
}

func Connect(ctx context.Context, dbURL string, enc *crypto.Encryptor) (*DB, error) {
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return &DB{Pool: pool, enc: enc}, nil
}

func (d *DB) Close() {
	d.Pool.Close()
}
