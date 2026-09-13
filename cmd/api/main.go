// Command api runs the HTTP API process: it serves the delivery layer and, for
// single-process development, also runs the outbox relay in the background.
package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	// Postgres driver: registers itself as "pgx" for database/sql.
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/example/myapp/internal/bootstrap"
	"github.com/example/myapp/internal/platform/config"
	"github.com/example/myapp/internal/platform/httpx"
)

func main() {
	if err := run(); err != nil {
		println("fatal:", err.Error())
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	app, err := bootstrap.Build(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = app.Close() }()

	server := httpx.New(cfg.HTTP, app.HTTPHandler(), app.Logger)
	grpcServer := app.GRPCServer()

	// Run the HTTP server, the gRPC server, and the background relay
	// concurrently; the first hard error (or a signal, via ctx) brings all down.
	errCh := make(chan error, 3)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() { errCh <- server.Run(runCtx) }()
	go func() { errCh <- grpcServer.Run(runCtx) }()
	go func() { errCh <- app.RunBackground(runCtx) }()

	err = <-errCh
	cancel()
	<-errCh // wait for the other two goroutines to unwind
	<-errCh

	if err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	app.Logger.Info("shutdown complete")
	return nil
}
