package outbox

import (
	"context"
	"log/slog"
	"time"

	"github.com/example/myapp/internal/eventbus"
)

// Relay polls the Store and forwards pending messages to the downstream
// Publisher (the real bus). Delivery is at-least-once, so downstream handlers
// must be idempotent (dedupe on Event.ID).
type Relay struct {
	store     Store
	publisher eventbus.Publisher
	logger    *slog.Logger

	interval  time.Duration
	batchSize int
}

// Options tunes the relay loop.
type Options struct {
	Interval  time.Duration // poll period; default 1s
	BatchSize int           // rows per tick; default 100
}

// NewRelay builds a relay.
func NewRelay(store Store, publisher eventbus.Publisher, logger *slog.Logger, opts Options) *Relay {
	if opts.Interval <= 0 {
		opts.Interval = time.Second
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = 100
	}
	return &Relay{
		store:     store,
		publisher: publisher,
		logger:    logger,
		interval:  opts.Interval,
		batchSize: opts.BatchSize,
	}
}

// Run blocks until ctx is cancelled, draining the outbox every interval.
func (r *Relay) Run(ctx context.Context) error {
	t := time.NewTicker(r.interval)
	defer t.Stop()

	r.logger.Info("outbox relay started", "interval", r.interval.String(), "batch", r.batchSize)
	for {
		select {
		case <-ctx.Done():
			r.logger.Info("outbox relay stopped")
			return ctx.Err()
		case <-t.C:
			if n, err := r.drain(ctx); err != nil {
				r.logger.ErrorContext(ctx, "outbox drain failed", "error", err)
			} else if n > 0 {
				r.logger.DebugContext(ctx, "outbox drained", "count", n)
			}
		}
	}
}

// Drain performs a single publish pass immediately and returns how many
// messages were delivered. Useful on shutdown (flush before exit) and in tests.
func (r *Relay) Drain(ctx context.Context) (int, error) {
	return r.drain(ctx)
}

// drain publishes one batch and returns how many messages were delivered.
func (r *Relay) drain(ctx context.Context) (int, error) {
	msgs, err := r.store.FetchPending(ctx, r.batchSize)
	if err != nil {
		return 0, err
	}
	published := 0
	for _, m := range msgs {
		if err := r.publisher.Publish(ctx, m.Event); err != nil {
			_ = r.store.MarkFailed(ctx, m.Event.ID, err.Error())
			r.logger.WarnContext(ctx, "outbox message publish failed",
				"event_id", m.Event.ID, "event", m.Event.Name, "attempts", m.Attempts+1, "error", err)
			continue
		}
		if err := r.store.MarkPublished(ctx, m.Event.ID); err != nil {
			return published, err
		}
		published++
	}
	return published, nil
}
