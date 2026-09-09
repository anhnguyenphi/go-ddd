// Package observability wires logging (and is the natural home for tracing and
// metrics wiring as the system grows).
package observability

import (
	"log/slog"
	"os"

	"github.com/example/myapp/internal/platform/config"
)

// NewLogger builds the application logger from config and installs it as the
// slog default so libraries that reach for slog.Default() stay consistent.
func NewLogger(cfg config.LogConfig) *slog.Logger {
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}
