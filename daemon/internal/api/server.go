package api

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"
)

type Server struct {
	socketPath string
	handler    *Handler
	httpServer *http.Server
	listener   net.Listener
}

func NewServer(socketPath string, handler *Handler) *Server {
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	return &Server{
		socketPath: socketPath,
		handler:    handler,
		httpServer: &http.Server{
			Handler:      mux,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
		},
	}
}

func (s *Server) Start() error {
	os.Remove(s.socketPath)

	ln, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return err
	}
	s.listener = ln

	if err := os.Chmod(s.socketPath, 0660); err != nil {
		slog.Warn("failed to chmod socket", "error", err)
	}

	slog.Info("daemon listening", "socket", s.socketPath)
	return s.httpServer.Serve(ln)
}

func (s *Server) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.httpServer.Shutdown(ctx)
}
