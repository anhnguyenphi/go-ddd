// Package order is the composition root for the order bounded context. It
// exposes a gRPC surface, also transcoded to REST by grpc-gateway (see
// RegisterGateway), built on top of one complete slice, PlaceOrder, which
// demonstrates synchronous, request/response communication between bounded
// contexts — the counterpart to the async customer -> notification flow (see
// internal/notification).
//
// Boundary rules (enforced by tests/arch/layering_test.go):
//   - order/domain imports only the stdlib and shared/domain
//   - order never imports internal/customer/... — its only line to that
//     context is the customerv1 proto contract, via infrastructure/customerclient
package order

import (
	"context"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"

	orderv1 "github.com/example/myapp/api/proto/order/v1"
	"github.com/example/myapp/internal/order/application"
	"github.com/example/myapp/internal/order/application/commands"
	"github.com/example/myapp/internal/order/domain"
	"github.com/example/myapp/internal/order/infrastructure/persistence/memory"
	ordergrpc "github.com/example/myapp/internal/order/interfaces/grpc"
	sharedapp "github.com/example/myapp/internal/shared/application"
)

// Deps are the infrastructure choices the bootstrap injects.
type Deps struct {
	Repository domain.Repository // optional; defaults to an in-memory repo
	Clock      sharedapp.Clock
	IDs        sharedapp.IDGenerator
	// Customer is the synchronous port onto the customer context — see
	// application/ports.go and infrastructure/customerclient.
	Customer application.CustomerVerifier
}

// Module is the wired context.
type Module struct {
	grpc *ordergrpc.Server
}

// New wires the context.
func New(d Deps) *Module {
	repo := d.Repository
	if repo == nil {
		repo = memory.New()
	}
	placeOrder := commands.NewPlaceOrderHandler(commands.Deps{
		Repo:     repo,
		Clock:    d.Clock,
		IDs:      d.IDs,
		Customer: d.Customer,
	})
	return &Module{grpc: ordergrpc.NewServer(placeOrder)}
}

// RegisterGRPC mounts this context's gRPC service.
func (m *Module) RegisterGRPC(s *grpc.Server) {
	orderv1.RegisterOrderServiceServer(s, m.grpc)
}

// RegisterGateway mounts this context's REST routes on the grpc-gateway mux,
// transcoding HTTP/JSON straight to the in-process gRPC server (no network
// hop) per the google.api.http annotations in order.proto — see
// customer/module.go's RegisterGateway for why this is the actual REST
// implementation rather than a hand-written adapter.
func (m *Module) RegisterGateway(ctx context.Context, mux *runtime.ServeMux) error {
	return orderv1.RegisterOrderServiceHandlerServer(ctx, mux, m.grpc)
}
