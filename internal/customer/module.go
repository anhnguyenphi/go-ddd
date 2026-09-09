// Package customer is the composition root for the customer bounded context. It
// is the ONLY package other code (the bootstrap) imports from this context; the
// layers underneath stay private to the context by convention.
package customer

import (
	"log/slog"
	"net/http"

	"github.com/example/myapp/internal/customer/application/commands"
	"github.com/example/myapp/internal/customer/application/queries"
	"github.com/example/myapp/internal/customer/domain"
	"github.com/example/myapp/internal/customer/domain/services"
	"github.com/example/myapp/internal/customer/infrastructure/messaging"
	"github.com/example/myapp/internal/customer/infrastructure/projections"
	customerhttp "github.com/example/myapp/internal/customer/interfaces/http"
	"github.com/example/myapp/internal/eventbus"
	sharedapp "github.com/example/myapp/internal/shared/application"
)

// Deps are the infrastructure choices the bootstrap injects. Swapping any of
// them (memory <-> postgres, outbox <-> direct) never touches this file's body.
type Deps struct {
	Logger          *slog.Logger
	Repository      domain.Repository
	UnitOfWork      sharedapp.UnitOfWork
	OutboxPublisher eventbus.Publisher
	Clock           sharedapp.Clock
	IDs             sharedapp.IDGenerator
}

// Module is the wired context.
type Module struct {
	http      *customerhttp.Handler
	readModel *projections.MemoryReadModel
	inbound   *messaging.InboundHandlers
	logger    *slog.Logger
}

// New wires the context top to bottom.
func New(d Deps) *Module {
	uniqueness := services.NewEmailUniqueness(d.Repository)
	dispatcher := messaging.NewDispatcher(d.OutboxPublisher)

	cmdDeps := commands.Deps{
		Repo:       d.Repository,
		UoW:        d.UnitOfWork,
		Events:     dispatcher,
		Clock:      d.Clock,
		IDs:        d.IDs,
		Uniqueness: uniqueness,
	}

	readModel := projections.NewMemoryReadModel()
	qryDeps := queries.Deps{ReadModel: readModel}

	handler := customerhttp.NewHandler(
		commands.NewCreateCustomerHandler(cmdDeps),
		commands.NewChangeCustomerEmailHandler(cmdDeps),
		queries.NewGetCustomerHandler(qryDeps),
		queries.NewListCustomersHandler(qryDeps),
	)

	return &Module{
		http:      handler,
		readModel: readModel,
		inbound:   messaging.NewInboundHandlers(d.Logger),
		logger:    d.Logger,
	}
}

// RegisterHTTP mounts this context's routes.
func (m *Module) RegisterHTTP(mux *http.ServeMux) {
	m.http.Register(mux)
}

// RegisterSubscriptions binds this context's event handlers (read-model
// projection + inbound integration handlers) to the bus.
func (m *Module) RegisterSubscriptions(bus eventbus.Bus) error {
	if err := m.readModel.Register(bus); err != nil {
		return err
	}
	return m.inbound.Register(bus)
}
