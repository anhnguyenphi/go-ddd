// Package customer is the composition root for the customer bounded context. It
// is the ONLY package other code (the bootstrap) imports from this context; the
// layers underneath stay private to the context by convention.
package customer

import (
	"context"
	"log/slog"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"

	customerv1 "github.com/example/myapp/api/proto/customer/v1"
	"github.com/example/myapp/internal/customer/application/commands"
	"github.com/example/myapp/internal/customer/application/queries"
	"github.com/example/myapp/internal/customer/domain"
	"github.com/example/myapp/internal/customer/domain/services"
	"github.com/example/myapp/internal/customer/infrastructure/messaging"
	"github.com/example/myapp/internal/customer/infrastructure/projections"
	customergrpc "github.com/example/myapp/internal/customer/interfaces/grpc"
	"github.com/example/myapp/internal/eventbus"
	sharedapp "github.com/example/myapp/internal/shared/application"
)

// Deps are the infrastructure choices the bootstrap injects. Swapping any of
// them (memory <-> postgres, outbox <-> direct) never touches this file's body.
type Deps struct {
	Logger          *slog.Logger
	Repository      domain.Repository
	ReadModel       projections.ReadModel
	UnitOfWork      sharedapp.UnitOfWork
	OutboxPublisher eventbus.Publisher
	Clock           sharedapp.Clock
	IDs             sharedapp.IDGenerator
}

// Module is the wired context.
type Module struct {
	grpc      *customergrpc.Server
	readModel projections.ReadModel
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

	qryDeps := queries.Deps{ReadModel: d.ReadModel}

	create := commands.NewCreateCustomerHandler(cmdDeps)
	changeEmail := commands.NewChangeCustomerEmailHandler(cmdDeps)
	get := queries.NewGetCustomerHandler(qryDeps)
	list := queries.NewListCustomersHandler(qryDeps)

	return &Module{
		grpc:      customergrpc.NewServer(create, changeEmail, get, list),
		readModel: d.ReadModel,
		inbound:   messaging.NewInboundHandlers(d.Logger),
		logger:    d.Logger,
	}
}

// RegisterGRPC mounts this context's gRPC service.
func (m *Module) RegisterGRPC(s *grpc.Server) {
	customerv1.RegisterCustomerServiceServer(s, m.grpc)
}

// GRPC exposes the raw gRPC service implementation for other contexts that
// need a synchronous, in-process call into customer — see
// order/infrastructure/customerclient, which is the only intended caller.
// External callers still go through RegisterGRPC as normal, over the network.
func (m *Module) GRPC() customerv1.CustomerServiceServer { return m.grpc }

// RegisterGateway mounts this context's REST routes on the grpc-gateway mux,
// transcoding HTTP/JSON straight to the in-process gRPC server (no network
// hop) per the google.api.http annotations in customer.proto. REST is no
// longer a hand-written adapter — the proto file is the single source of
// truth for both the gRPC contract and its REST/OpenAPI projection.
func (m *Module) RegisterGateway(ctx context.Context, mux *runtime.ServeMux) error {
	return customerv1.RegisterCustomerServiceHandlerServer(ctx, mux, m.grpc)
}

// RegisterSubscriptions binds this context's event handlers (read-model
// projection + inbound integration handlers) to the bus.
func (m *Module) RegisterSubscriptions(bus eventbus.Bus) error {
	if err := m.readModel.Register(bus); err != nil {
		return err
	}
	return m.inbound.Register(bus)
}
