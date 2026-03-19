package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"wg-proxy-manager/daemon/internal/api"
	"wg-proxy-manager/daemon/internal/config"
	"wg-proxy-manager/daemon/internal/monitor"
	"wg-proxy-manager/daemon/internal/proxy"
	"wg-proxy-manager/daemon/internal/state"
	"wg-proxy-manager/daemon/internal/wireguard"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg := config.Load()

	if cfg.ServerPubKey == "" {
		slog.Error("server public key not found, ensure WireGuard is set up")
		os.Exit(1)
	}
	if cfg.WGEndpoint == "" {
		slog.Error("WG_ENDPOINT not set (e.g., your-host.duckdns.org:51820)")
		os.Exit(1)
	}

	slog.Info("starting wg-manager daemon",
		"interface", cfg.WGInterface,
		"endpoint", cfg.WGEndpoint,
		"socket", cfg.SocketPath,
		"max_devices", cfg.MaxDevices,
		"phone_proxy_port", cfg.PhoneProxyPort,
	)

	store := state.NewStore(cfg.StatePath)
	if err := store.Load(); err != nil {
		slog.Error("failed to load state", "error", err)
		os.Exit(1)
	}

	wgMgr := wireguard.NewManager(cfg.WGInterface)
	proxyMgr := proxy.NewManager(cfg.GostBinary)
	mon := monitor.New(wgMgr, store)

	// Reconcile: re-provision all devices from saved state
	devices := store.GetAll()
	slog.Info("reconciling devices", "count", len(devices))

	for _, dev := range devices {
		slog.Info("reconciling", "device_id", dev.ID, "name", dev.Name)

		if !wgMgr.PeerExists(dev.WGPublicKey) {
			if err := wgMgr.AddPeer(dev.WGPublicKey, dev.WGPresharedKey, dev.WGIP); err != nil {
				slog.Error("reconcile: add wg peer failed", "device_id", dev.ID, "error", err)
				continue
			}
		}

		if err := proxyMgr.Start(dev); err != nil {
			slog.Error("reconcile: start proxy failed", "device_id", dev.ID, "error", err)
		}
	}

	// Start background monitor
	ctx, cancel := context.WithCancel(context.Background())
	go mon.Run(ctx)

	// Start API server
	handler := api.NewHandler(cfg, store, wgMgr, proxyMgr, mon)
	server := api.NewServer(cfg.SocketPath, handler)

	go func() {
		if err := server.Start(); err != nil {
			slog.Error("api server error", "error", err)
		}
	}()

	// Wait for termination signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	sig := <-sigCh

	slog.Info("received signal, shutting down", "signal", sig.String())

	cancel()
	server.Stop()
	proxyMgr.StopAll()

	slog.Info("shutdown complete")
}
