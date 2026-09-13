package bootstrap

import (
	"context"
	"log/slog"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"

	"github.com/example/myapp/internal/customer"
	"github.com/example/myapp/internal/eventbus"
	"github.com/example/myapp/internal/notification"
	"github.com/example/myapp/internal/order"
	"github.com/example/myapp/internal/order/infrastructure/customerclient"
	"github.com/example/myapp/internal/shared/infrastructure/clock"
	"github.com/example/myapp/internal/shared/infrastructure/id"
)

// modules holds every wired bounded context.
type modules struct {
	customer     *customer.Module
	notification *notification.Module
	order        *order.Module
}

// newModules wires each bounded context with its infrastructure dependencies.
// Adding a context is a one-line change here plus its own module.go.
func newModules(logger *slog.Logger, p persistence, outboxPub eventbus.Publisher) modules {
	customerMod := customer.New(customer.Deps{
		Logger:          logger,
		Repository:      p.customerRepo,
		ReadModel:       p.customerReadModel,
		UnitOfWork:      p.unitOfWork,
		OutboxPublisher: outboxPub,
		Clock:           clock.System{},
		IDs:             id.UUID{},
	})

	return modules{
		customer: customerMod,
		notification: notification.New(notification.Deps{
			Logger: logger,
		}),
		// order.PlaceOrder calls customerMod synchronously, in-process, via
		// the customerv1 gRPC contract — see infrastructure/customerclient
		// for why (a request/response cross-context call, not an event).
		order: order.New(order.Deps{
			Clock:    clock.System{},
			IDs:      id.UUID{},
			Customer: customerclient.New(customerMod.GRPC()),
		}),
	}
}

// registerGateway mounts every context's REST routes (transcoded from gRPC by
// grpc-gateway) on mux.
func (m modules) registerGateway(ctx context.Context, mux *runtime.ServeMux) error {
	if err := m.customer.RegisterGateway(ctx, mux); err != nil {
		return err
	}
	return m.order.RegisterGateway(ctx, mux)
}

// registerGRPC mounts every context's gRPC services on s.
func (m modules) registerGRPC(s *grpc.Server) {
	m.customer.RegisterGRPC(s)
	m.order.RegisterGRPC(s)
}

// registerSubscriptions binds every context's event handlers to the bus.
func (m modules) registerSubscriptions(bus eventbus.Bus) error {
	if err := m.customer.RegisterSubscriptions(bus); err != nil {
		return err
	}
	return m.notification.RegisterSubscriptions(bus)
}
