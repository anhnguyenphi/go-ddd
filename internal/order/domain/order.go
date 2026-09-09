// Package domain is the order context's domain layer (skeleton).
package domain

import (
	"time"

	shareddomain "github.com/example/myapp/internal/shared/domain"
)

// OrderID identifies an Order aggregate.
type OrderID struct{ value string }

// NewOrderID validates a raw id.
func NewOrderID(v string) (OrderID, error) {
	if v == "" {
		return OrderID{}, shareddomain.Invalid("order id must not be empty")
	}
	return OrderID{value: v}, nil
}

// String returns the raw id.
func (id OrderID) String() string { return id.value }

// Order is the aggregate root (fields elided in the skeleton).
type Order struct {
	shareddomain.AggregateRoot
	id         OrderID
	customerID string
	placedAt   time.Time
}

// Place creates a new order and records OrderPlaced.
func Place(id OrderID, customerID string, now time.Time) (*Order, error) {
	if customerID == "" {
		return nil, shareddomain.Invalid("order requires a customer")
	}
	o := &Order{id: id, customerID: customerID, placedAt: now}
	o.RecordEvent(OrderPlaced{OrderID: id.String(), CustomerID: customerID, At: now})
	return o, nil
}

// ID returns the aggregate identity.
func (o *Order) ID() OrderID { return o.id }

// OrderPlaced is a domain event. Its integration form (order.v1.placed) is what
// other contexts, e.g. customer, would subscribe to.
type OrderPlaced struct {
	OrderID    string
	CustomerID string
	At         time.Time
}

func (e OrderPlaced) EventName() string     { return "order.placed" }
func (e OrderPlaced) OccurredAt() time.Time { return e.At }
func (e OrderPlaced) AggregateID() string   { return e.OrderID }
