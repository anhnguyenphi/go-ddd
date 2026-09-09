// Package logging carries a *slog.Logger through context so any layer can log
// with request-scoped fields (request id, correlation id, ...) already attached.
package logging

import (
	"context"
	"log/slog"
)

type ctxKey struct{}

// Into returns a child context carrying logger.
func Into(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, logger)
}

// From returns the context logger, or slog.Default() if none was attached.
func From(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return slog.Default()
}

// With attaches key/value attributes to the context logger.
func With(ctx context.Context, args ...any) context.Context {
	return Into(ctx, From(ctx).With(args...))
}
