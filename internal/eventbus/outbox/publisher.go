package outbox

import (
	"context"

	"github.com/example/myapp/internal/eventbus"
)

// Publisher is an eventbus.Publisher that only appends to the Store. Because
// Store.Add enlists in the ambient transaction, publishing an integration event
// is atomic with the state change that produced it.
//
// It is deliberately NOT the thing domain events are handed to directly — each
// context maps its domain events to integration events first (see
// customer/infrastructure/messaging).
type Publisher struct {
	store Store
}

// NewPublisher wraps store.
func NewPublisher(store Store) *Publisher { return &Publisher{store: store} }

// Publish appends events to the outbox.
func (p *Publisher) Publish(ctx context.Context, events ...eventbus.Event) error {
	if len(events) == 0 {
		return nil
	}
	return p.store.Add(ctx, events...)
}
