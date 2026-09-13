// Package order is a partially implemented bounded context: domain,
// application, persistence, and a delivery adapter (interfaces/grpc, also
// transcoded to REST by grpc-gateway — see api/proto/order/v1/order.proto)
// are real, on the same footing as customer.
//
// It ships one complete slice on purpose: PlaceOrder, which synchronously
// calls the customer context (see application.CustomerVerifier and
// infrastructure/customerclient) before an order can be built. That is the
// reference example for *synchronous* cross-context communication, the
// counterpart to the async customer -> notification integration-event flow.
//
// Boundary rules (enforced by tests/arch/layering_test.go):
//   - order/domain imports only the stdlib and shared/domain
//   - other contexts never import internal/order/... — they react to
//     order.v1.* integration events
package order
