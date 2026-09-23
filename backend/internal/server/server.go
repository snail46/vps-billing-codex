package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

const (
	readHeaderTimeout = 5 * time.Second
	shutdownTimeout   = 10 * time.Second
)

type Server struct {
	httpServer *http.Server
	listener   net.Listener
	logger     *slog.Logger
}

func New(address string, handler http.Handler, logger *slog.Logger) (*Server, error) {
	if handler == nil {
		handler = http.NewServeMux()
	}
	if logger == nil {
		logger = slog.Default()
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("listen on %q: %w", address, err)
	}

	return &Server{
		httpServer: &http.Server{
			Addr:              listener.Addr().String(),
			Handler:           handler,
			ReadHeaderTimeout: readHeaderTimeout,
		},
		listener: listener,
		logger:   logger,
	}, nil
}

func (s *Server) Address() string {
	return s.listener.Addr().String()
}

func (s *Server) Run(ctx context.Context) error {
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- s.httpServer.Serve(s.listener)
	}()

	s.logger.Info("server started", "address", s.Address())

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shut down HTTP server: %w", err)
	}

	err := <-serveErr
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve HTTP during shutdown: %w", err)
	}
	s.logger.Info("server stopped")
	return nil
}
