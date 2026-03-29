package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"wg-proxy-manager/backend/internal/api"
	"wg-proxy-manager/backend/internal/config"
	"wg-proxy-manager/backend/internal/crypto"
	"wg-proxy-manager/backend/internal/daemon"
	"wg-proxy-manager/backend/internal/db"
	"wg-proxy-manager/backend/internal/handlers"
	ws "wg-proxy-manager/backend/internal/websocket"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg := config.Load()

	if cfg.EncryptionKey == "" {
		slog.Error("ENCRYPTION_KEY not set (must be 64 hex chars / 32 bytes)")
		os.Exit(1)
	}

	enc, err := crypto.NewEncryptor(cfg.EncryptionKey)
	if err != nil {
		slog.Error("invalid encryption key", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	database, err := db.Connect(ctx, cfg.DBUrl, enc)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	if err := database.RunMigrations(ctx); err != nil {
		slog.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}
	slog.Info("database migrations complete")

	if err := database.SeedDefaultUser(ctx, cfg.AdminUser, cfg.AdminPassword); err != nil {
		slog.Error("failed to seed default user", "error", err)
		os.Exit(1)
	}

	if cfg.JWTSecret == "" {
		slog.Error("JWT_SECRET not set")
		os.Exit(1)
	}

	daemonClient := daemon.NewClient(cfg.DaemonSocket)

	hub := ws.NewHub()
	hub.StartPingLoop()

	deviceHandler := handlers.NewDeviceHandler(database, daemonClient, hub, cfg)
	proxyHandler := handlers.NewProxyHandler(daemonClient, cfg)
	metricsHandler := handlers.NewMetricsHandler(database, daemonClient)
	webhookHandler := handlers.NewWebhookHandler(database, cfg)
	authHandler := handlers.NewAuthHandler(database, cfg)

	// Start metrics polling (every 15 seconds)
	deviceHandler.StartPolling(15 * time.Second)

	router := api.NewRouter(deviceHandler, proxyHandler, metricsHandler, webhookHandler, authHandler, hub, cfg.JWTSecret, database)

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		slog.Info("backend API starting", "port", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	<-sigCh

	slog.Info("shutting down backend...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	server.Shutdown(shutdownCtx)
	slog.Info("backend shutdown complete")
}
