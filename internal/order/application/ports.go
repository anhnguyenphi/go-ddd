// Package application declares the order context's driven ports. Command
// handlers live in the commands/ sub-package.
package application

import "context"

// CustomerVerifier is order's synchronous, request/response port onto the
// customer bounded context — the sync counterpart to the async integration
// events used elsewhere (customer.created -> notification, see
// internal/notification). Placing an order needs a definite answer about the
// customer *before* the order can even be constructed, so there is nothing to
// gain by going through the event bus and reacting later.
//
// Defined here, by the consumer (order); implemented in
// infrastructure/customerclient by calling the customer context's public
// gRPC contract (api/proto/customer/v1). Order never imports
// internal/customer/... — enforced by tests/arch/layering_test.go.
type CustomerVerifier interface {
	// Verify returns nil if customerID exists, or shared/domain.ErrNotFound
	// if it does not.
	Verify(ctx context.Context, customerID string) error
}
