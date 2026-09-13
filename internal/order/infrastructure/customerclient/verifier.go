// Package customerclient is order's adapter onto the customer bounded
// context. It implements order/application.CustomerVerifier as a
// synchronous, request/response call against the same public gRPC contract
// (api/proto/customer/v1) that external clients use over the network.
//
// customerService is satisfied structurally — by *customer.Module, via its
// GRPC() accessor, in this monolith — without this package ever importing
// internal/customer (forbidden by tests/arch/layering_test.go). The call
// happens in-process, with no dial and no second network hop, the same trick
// customer/module.go's RegisterGateway uses for REST. The day this context is
// extracted into its own service, only the value wired into New changes (a
// real customerv1.CustomerServiceClient over a grpc.ClientConn) — the port in
// application/ports.go, and every caller above it, stays untouched.
package customerclient

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	customerv1 "github.com/example/myapp/api/proto/customer/v1"
	shareddomain "github.com/example/myapp/internal/shared/domain"
)

// customerService is the slice of customerv1.CustomerServiceServer this
// adapter needs.
type customerService interface {
	GetCustomer(ctx context.Context, req *customerv1.GetCustomerRequest) (*customerv1.Customer, error)
}

// Verifier implements order/application.CustomerVerifier.
type Verifier struct {
	customer customerService
}

// New wires the adapter to anything shaped like the customer gRPC service.
func New(customer customerService) *Verifier {
	return &Verifier{customer: customer}
}

// Verify performs the synchronous call. The customer context's NotFound
// status is translated back into the shared domain sentinel, so order's
// command handler can treat "unknown customer" exactly like any other
// failed lookup.
//
// Caveat: GetCustomer is served from customer's CQRS read model, which only
// catches up once the outbox relay has forwarded customer.created to it. A
// customer created a moment ago can therefore still read as not-found here
// until that catches up (bounded by the relay's poll interval) — a "sync
// call" only guarantees an immediate answer, not one consistent with a write
// that just happened on the other side of it.
func (v *Verifier) Verify(ctx context.Context, customerID string) error {
	_, err := v.customer.GetCustomer(ctx, &customerv1.GetCustomerRequest{Id: customerID})
	if err == nil {
		return nil
	}
	if s, ok := status.FromError(err); ok && s.Code() == codes.NotFound {
		return shareddomain.ErrNotFound
	}
	return err
}
