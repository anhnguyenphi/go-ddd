// Package local is an in-process implementation of eventbus.Bus. Delivery is
// synchronous and in-order on the caller's goroutine: good for a modular
// monolith and for tests, and a drop-in for Kafka later.
package local

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/example/myapp/internal/eventbus"
)

// Bus fans each published event out to every handler subscribed to its Name.
type Bus struct {
	mu       sync.RWMutex
	handlers map[string][]eventbus.Handler
	mw       []eventbus.Middleware
	logger   *slog.Logger
}

// New builds a Bus. The supplied middleware wraps every handler.
func New(logger *slog.Logger, mw ...eventbus.Middleware) *Bus {
	return &Bus{
		handlers: make(map[string][]eventbus.Handler),
		mw:       mw,
		logger:   logger,
	}
}

// Subscribe registers handler for eventName.
func (b *Bus) Subscribe(eventName string, handler eventbus.Handler) error {
	if eventName == "" || handler == nil {
		return errors.New("local: eventName and handler are required")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventName] = append(b.handlers[eventName], eventbus.Chain(handler, b.mw...))
	b.logger.Debug("event subscription registered", "event", eventName)
	return nil
}

// Publish delivers events to their handlers. Handler errors are collected and
// joined so one failure does not hide the others, but every handler still runs.
func (b *Bus) Publish(ctx context.Context, events ...eventbus.Event) error {
	var errs []error
	for _, e := range events {
		b.mu.RLock()
		hs := b.handlers[e.Name]
		b.mu.RUnlock()
		if len(hs) == 0 {
			b.logger.DebugContext(ctx, "no subscribers for event", "event", e.Name)
			continue
		}
		for _, h := range hs {
			if err := h(ctx, e); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}
