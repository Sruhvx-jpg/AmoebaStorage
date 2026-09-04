package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
}

func New(port int, logger *slog.Logger) *Server {
	mux := http.NewServeMux()

	srv := &Server{
		logger: logger,
	}

	mux.HandleFunc("GET /heartbeat", srv.handleHeartBeat)

	srv.httpServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	return srv
}

func (s *Server) Start() error {
	s.logger.Info("HTTP Server listening on port -> ", "addr", s.httpServer.Addr)

	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("HTTP Server failed to run - ERROR: %w", err)
	}

	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("shutting down http server......")

	return s.httpServer.Shutdown(ctx)
}

func (s *Server) handleHeartBeat(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("beat\n"))
}
