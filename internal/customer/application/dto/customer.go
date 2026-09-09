// Package dto holds the customer context's application-layer data contracts:
// the shapes returned by query handlers. They are decoupled from both the
// aggregate and the HTTP/gRPC representation.
package dto

import "time"

// Customer is the read model for a single customer.
type Customer struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}
