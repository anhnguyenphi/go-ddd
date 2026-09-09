package eventbus

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Middleware decorates a Handler with a cross-cutting concern.
type Middleware func(Handler) Handler

// Chain wraps h with mw in order, so mw[0] is the outermost decorator.
func Chain(h Handler, mw ...Middleware) Handler {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}

// Recover converts a panicking handler into an error so one bad subscriber does
// not take the dispatch loop down.
func Recover(logger *slog.Logger) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, e Event) (err error) {
			defer func() {
				if p := recover(); p != nil {
					logger.ErrorContext(ctx, "event handler panicked",
						"event", e.Name, "panic", p)
					err = fmt.Errorf("handler panic: %v", p)
				}
			}()
			return next(ctx, e)
		}
	}
}

// Logging records the outcome and latency of every handled event.
func Logging(logger *slog.Logger) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, e Event) error {
			start := time.Now()
			err := next(ctx, e)
			attrs := []any{
				"event", e.Name,
				"event_id", e.ID,
				"aggregate_id", e.AggregateID,
				"duration_ms", time.Since(start).Milliseconds(),
			}
			if err != nil {
				logger.ErrorContext(ctx, "event handler failed", append(attrs, "error", err)...)
			} else {
				logger.DebugContext(ctx, "event handled", attrs...)
			}
			return err
		}
	}
}
