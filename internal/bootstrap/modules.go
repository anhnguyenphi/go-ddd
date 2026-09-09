package bootstrap

import (
	"log/slog"
	"net/http"

	"github.com/example/myapp/internal/customer"
	"github.com/example/myapp/internal/eventbus"
	"github.com/example/myapp/internal/notification"
	"github.com/example/myapp/internal/shared/infrastructure/clock"
	"github.com/example/myapp/internal/shared/infrastructure/id"
)

// modules holds every wired bounded context.
type modules struct {
	customer     *customer.Module
	notification *notification.Module
}

// newModules wires each bounded context with its infrastructure dependencies.
// Adding a context is a one-line change here plus its own module.go.
func newModules(logger *slog.Logger, p persistence, outboxPub eventbus.Publisher) modules {
	return modules{
		customer: customer.New(customer.Deps{
			Logger:          logger,
			Repository:      p.customerRepo,
			UnitOfWork:      p.unitOfWork,
			OutboxPublisher: outboxPub,
			Clock:           clock.System{},
			IDs:             id.UUID{},
		}),
		notification: notification.New(notification.Deps{
			Logger: logger,
		}),
	}
}

// registerHTTP mounts every context's routes on mux.
func (m modules) registerHTTP(mux *http.ServeMux) {
	m.customer.RegisterHTTP(mux)
}

// registerSubscriptions binds every context's event handlers to the bus.
func (m modules) registerSubscriptions(bus eventbus.Bus) error {
	if err := m.customer.RegisterSubscriptions(bus); err != nil {
		return err
	}
	return m.notification.RegisterSubscriptions(bus)
}
