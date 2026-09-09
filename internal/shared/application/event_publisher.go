package application

import (
	"context"

	"github.com/example/myapp/internal/shared/domain"
)

// DomainEventPublisher is how the application layer hands buffered domain events
// off for delivery. It MUST be called inside the same UnitOfWork that persisted
// the aggregate, so that "state changed" and "event recorded" are atomic.
//
// The production implementation maps each domain event to a versioned
// integration event and writes it to the transactional outbox; a relay then
// forwards outbox rows to the real bus. See internal/eventbus/outbox.
type DomainEventPublisher interface {
	Publish(ctx context.Context, events ...domain.DomainEvent) error
}
