// Command worker runs background processing: the transactional-outbox relay and
// (as the system grows) event-bus consumers and scheduled jobs. It shares the
// exact same wiring as the API via internal/bootstrap.
package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	// To enable Postgres, add a driver here, e.g.:
	//   _ "github.com/jackc/pgx/v5/stdlib"

	"github.com/example/myapp/internal/bootstrap"
	"github.com/example/myapp/internal/platform/config"
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

	app.Logger.Info("worker started")
	if err := app.RunBackground(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	app.Logger.Info("worker stopped")
	return nil
}
