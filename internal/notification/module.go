// Package notification is the composition root for the notification bounded
// context. It is a pure consumer: it has no HTTP surface, it only reacts to
// integration events.
package notification

import (
	"log/slog"

	"github.com/example/myapp/internal/eventbus"
	"github.com/example/myapp/internal/notification/application/handlers"
	"github.com/example/myapp/internal/notification/domain"
	"github.com/example/myapp/internal/notification/infrastructure/email"
)

// Deps are the infrastructure choices injected by the bootstrap.
type Deps struct {
	Logger *slog.Logger
	// Sender is optional; a logging sender is used when nil.
	Sender domain.NotificationService
}

// Module is the wired context.
type Module struct {
	customerCreated *handlers.CustomerCreated
}

// New wires the context.
func New(d Deps) *Module {
	sender := d.Sender
	if sender == nil {
		sender = email.NewLogSender(d.Logger)
	}
	return &Module{
		customerCreated: handlers.NewCustomerCreated(sender),
	}
}

// RegisterSubscriptions binds this context's handlers to the bus.
func (m *Module) RegisterSubscriptions(bus eventbus.Subscriber) error {
	return bus.Subscribe(handlers.EventCustomerCreated, m.customerCreated.Handle)
}
