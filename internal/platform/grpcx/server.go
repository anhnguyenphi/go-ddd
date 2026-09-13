// Package grpcx provides a configured *grpc.Server with graceful shutdown and a
// small set of standard interceptors. It knows nothing about business
// services — those are registered by each bounded context.
package grpcx

import (
	"context"
	"log/slog"
	"net"

	"google.golang.org/grpc"

	"github.com/example/myapp/internal/platform/config"
)

// Server owns the listener lifecycle.
type Server struct {
	srv    *grpc.Server
	addr   string
	logger *slog.Logger
}

// New wraps grpcServer for the addr in cfg. grpcServer already has its
// services registered by the caller.
func New(cfg config.GRPCConfig, grpcServer *grpc.Server, logger *slog.Logger) *Server {
	return &Server{srv: grpcServer, addr: cfg.Addr, logger: logger}
}

// Run serves until ctx is cancelled, then stops gracefully.
func (s *Server) Run(ctx context.Context) error {
	var lc net.ListenConfig
	lis, err := lc.Listen(ctx, "tcp", s.addr)
	if err != nil {
		return err
	}

	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("grpc server listening", "addr", s.addr)
		errCh <- s.srv.Serve(lis)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		s.logger.Info("grpc server shutting down")
		s.srv.GracefulStop()
		return nil
	}
}
