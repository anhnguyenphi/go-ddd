// Package order is a placeholder bounded context. It ships only its domain
// skeleton to demonstrate the layout; application/infrastructure/interfaces
// layers and a module.go composition root are added the same way the customer
// context does it.
//
// Boundary rules (enforced by convention here, by architecture tests later):
//   - order/domain imports only the stdlib and shared/domain
//   - other contexts never import internal/order/... — they react to
//     order.v1.* integration events
package order
