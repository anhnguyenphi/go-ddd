package messaging

import (
	"context"

	"github.com/example/myapp/internal/eventbus"
	sharedapp "github.com/example/myapp/internal/shared/application"
	shareddomain "github.com/example/myapp/internal/shared/domain"
)

// Dispatcher adapts the application layer's DomainEventPublisher port to the
// event bus: it maps domain events to integration events and forwards them to
// the wrapped Publisher. Wire the outbox Publisher here so publication is
// transactional.
type Dispatcher struct {
	publisher eventbus.Publisher
}

// NewDispatcher wires the dispatcher to an eventbus.Publisher (normally
// outbox.Publisher).
func NewDispatcher(publisher eventbus.Publisher) *Dispatcher {
	return &Dispatcher{publisher: publisher}
}

var _ sharedapp.DomainEventPublisher = (*Dispatcher)(nil)

// Publish maps and forwards. Domain events with no integration mapping are
// silently dropped — that is a deliberate contract decision, not an error.
func (d *Dispatcher) Publish(ctx context.Context, domainEvents ...shareddomain.DomainEvent) error {
	if len(domainEvents) == 0 {
		return nil
	}
	out := make([]eventbus.Event, 0, len(domainEvents))
	for _, de := range domainEvents {
		if ie, ok := ToIntegrationEvent(de); ok {
			out = append(out, ie)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return d.publisher.Publish(ctx, out...)
}
